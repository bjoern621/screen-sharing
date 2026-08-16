// Package groupsvc is the key, token, index and roster service.
//
// Creating a group draws a key, a member trades that key for a short relay token, and the index
// answers which streams a key can see.
// All three are derivations rather than lookups, so no database sits behind any of them: possession
// of the key is membership, the prefix is the key's own digest (internal/group),
// and which streams exist is the relay's answer rather than this service's.
//
// The roster is the one thing held, and it is held because nobody else knows it: which members a
// group has is whatever serves the voice channel talking, and it is pushed here (internal/roster).
// Enforcing it closes connections at the relay, a token being unenforceable after the handshake.
//
// It lives in this repository rather than one of its own because the path-prefix derivation has to
// be identical on both sides, and two repositories are two copies of it (docs/plan.md,
// "Groups, auth and encryption").
//
// None of it is end to end.
// The relay terminates every protocol and re-muxes per listener, so it sees plaintext by
// construction, and this service holds the key that signs every group's tokens.
// Both can watch a private stream, and the interface says so.
//
// Every refusal answers a caller rather than crashing.
// A malformed key, a missing body and a caller over the creation bound are Umgebungsfehler:
// the input arrives over HTTP from somebody this process does not control.
package groupsvc

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/roster"
	"bjoernblessin.de/screenshare/internal/token"
)

// TokenWindow is how long a relay token stays valid.
//
// Short, because a member who leaves holds whatever they were last issued: this window is how long
// they can still open a connection after whatever issues tokens stops issuing to them.
// Long enough that a client reconnecting across a network blip is not asking for a token between
// every attempt.
//
// It bounds opening connections and nothing else.
// The relay checks at the handshake and not again, so a connection outlives its own token, which is
// why what they already hold is closed rather than expired out (internal/roster).
const TokenWindow = 5 * time.Minute

// CreationsPerHour is how many groups one address may create in an hour.
//
// Creation is open, there being no accounts and so nobody to charge it to,
// which makes it something to bound rather than to gate.
// High enough that a person creating groups all afternoon never meets it, low enough that a script
// filling the relay with prefixes does.
const CreationsPerHour = 60

// PublicPrefix is where a stream anybody may watch lives, as internal/group derives every path
// from.
//
// Named through rather than restated: the publisher builds its path from that constant and this
// service grants a token on it, and two spellings of one prefix issue a token for a path nobody
// publishes to.
const PublicPrefix = group.PublicPrefix

// PublicSubject is what a public token is issued to.
//
// A subject is what the relay logs and lists a connection under: a member's id where one was named,
// and the group's own id otherwise.
// There is neither here, no key having derived this prefix, so the prefix's own name stands in and a
// log line still says which audience a connection belonged to.
const PublicSubject = "public"

// Stream is one path the relay carries, as far as a member is told about it.
//
// Enough to open it and no more: the name, whether it carries anything yet, and the video track,
// which decides the protocols a viewer may receive it over.
// Not who else is reading, nor at what rate, which is the relay's operational state rather than the
// group's (docs/plan.md).
type Stream struct {
	Path   string `json:"-"`
	Name   string `json:"name"`
	Ready  bool   `json:"ready"`
	Tracks string `json:"tracks,omitempty"`
	Format string `json:"format,omitempty"`
}

// Streams is what the relay is carrying, as the index reads it.
//
// An interface rather than the relay client: the index needs the rows above, where the client
// answers with bitrates and rosters beside them.
// It is also what lets the index be tested without a relay.
type Streams interface {
	Paths() []Stream
}

// Service answers the questions.
// Safe for concurrent use, which is what an HTTP server needs of a handler.
type Service struct {
	signer  *token.Signer
	streams Streams
	rosters *roster.Registry
	// now is read rather than time.Now called directly, so a test can issue a token at a moment it
	// chooses and read the window back off it.
	now func() time.Time

	mu sync.Mutex
	// created is when each address created groups, oldest first, guarded by mu.
	// A map of slices rather than a counter per address: each creation ages out on its own hour.
	created map[string][]time.Time
}

// New is a service signing with this key, reading streams from there and enforcing rosters through
// that.
func New(signer *token.Signer, streams Streams, rosters *roster.Registry) *Service {
	assert.IsNotNil(signer, "a service signs its tokens with a key")
	assert.IsNotNil(rosters, "a service enforces its rosters through a registry")

	s := &Service{
		signer:  signer,
		streams: streams,
		rosters: rosters,
		now:     time.Now,
		created: map[string][]time.Time{},
	}
	assert.IsNotNil(s.now, "a service reads the clock its windows are measured on")
	return s
}

// Handler is the service's routes.
//
// A key travels in the request body, never in the path: a key in a URL is a key in every proxy log
// between here and the client.
// The index is the exception and takes its key in the query, a GET having no body a cache or proxy
// will honour.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /groups", s.createGroup)
	mux.HandleFunc("POST /tokens", s.issueToken)
	mux.HandleFunc("GET /streams", s.listStreams)
	mux.HandleFunc("GET /jwks.json", s.jwks)
	mux.HandleFunc("PUT /roster", s.setRoster)
	mux.HandleFunc("GET /roster", s.viewRoster)
	mux.HandleFunc("DELETE /roster", s.clearRoster)
	// Reached by the relay's own read hook, which knows a path and holds no key.
	// It grants nothing a key would have bought: the answer is a run against a roster somebody else
	// stated, and a group nobody stated one for is left alone.
	mux.HandleFunc("POST /reconcile", s.reconcile)
	return mux
}

// createGroup draws a key and hands it back.
// Nothing is stored because there is nothing to store: the group exists by somebody holding the
// key, and the prefix is that key's own digest.
func (s *Service) createGroup(w http.ResponseWriter, r *http.Request) {
	if !s.allowCreation(caller(r)) {
		refuse(w, http.StatusTooManyRequests, "too many groups created from here in the last hour")
		return
	}
	key, err := group.NewKey()
	if err != nil {
		// The one failure here that is this machine's rather than the caller's.
		// Nothing is handed back, a key drawn from something weaker looking exactly like a real one.
		refuse(w, http.StatusInternalServerError, "no group key could be drawn")
		return
	}
	answer(w, map[string]string{"key": key.String(), "id": key.ID()})
}

// issueToken trades a group key for a relay token, and a request carrying no key for a public one.
//
// Deriving from the key is the whole verification, there being no membership list to check it
// against: a caller holding a well-formed key is a member, and one holding a key nobody else has is
// a group of one.
// Minted per call and never stored: the signature derives from the key like every other answer
// here, so a held token would be a second copy of a fact the key already carries.
//
// A keyless request is answered rather than refused, and answering it grants nothing a refusal
// would have withheld: the public prefix is one anybody may ask for a token on, so the token says
// who the audience is and never who the caller is.
// The relay still authenticates the connection and still encrypts it.
// Only a malformed key is a refusal, an empty one being a request for the public prefix and a
// truncated one being a group the caller cannot reach.
func (s *Service) issueToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key    string `json:"key"`
		Member string `json:"member"`
	}
	// An empty body is a request for the public prefix, the same as a body naming an empty key.
	// Anything else that will not decode is malformed rather than keyless, and a caller who meant to
	// send a key is better told than quietly given somebody else's audience.
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, bodyLimit)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		refuse(w, http.StatusBadRequest, "the request carries no group key this service can read")
		return
	}

	subject, prefix := PublicSubject, PublicPrefix
	if strings.TrimSpace(body.Key) != "" {
		key, err := group.ParseKey(body.Key)
		if err != nil {
			refuse(w, http.StatusBadRequest, err.Error())
			return
		}
		subject, prefix = key.ID(), key.Prefix()

		// Naming a member moves the subject from the group to that member, which is what lets a roster
		// tell one member's connections from another's at the relay.
		// The grant does not move with it: membership decides who may connect, never what they reach.
		//
		// A field with something in it is a caller naming a member, so a name that is whitespace alone
		// is refused by the derivation rather than quietly becoming the group's own token.
		if body.Member != "" {
			member, err := key.MemberID(body.Member)
			if err != nil {
				refuse(w, http.StatusBadRequest, err.Error())
				return
			}
			subject = member
		}
	} else if body.Member != "" {
		refuse(w, http.StatusBadRequest, "a member belongs to a group, and this request names no group key")
		return
	}

	now := s.now()
	signed, err := s.signer.Sign(subject, token.GroupPermissions(prefix), now, TokenWindow)
	if err != nil {
		refuse(w, http.StatusInternalServerError, "no token could be signed")
		return
	}
	answer(w, map[string]any{
		"token":   signed,
		"prefix":  prefix,
		"expires": now.Add(TokenWindow).UTC().Format(time.RFC3339),
	})
}

// listStreams answers which streams the caller may see: their group's on a key, the public ones
// without.
//
// The split is enforced here rather than left to a shell, which is the point of the index:
// a listing a client narrowed arrived carrying every group's streams.
// A group's listing hides the public ones for the same reason it hides another group's,
// that a member asked about their group.
func (s *Service) listStreams(w http.ResponseWriter, r *http.Request) {
	prefix := PublicPrefix
	if encoded := strings.TrimSpace(r.URL.Query().Get("group")); encoded != "" {
		key, err := group.ParseKey(encoded)
		if err != nil {
			refuse(w, http.StatusBadRequest, err.Error())
			return
		}
		prefix = key.Prefix()
	}

	streams := []Stream{}
	if s.streams != nil {
		for _, stream := range s.streams.Paths() {
			name, ok := strings.CutPrefix(stream.Path, prefix)
			if !ok || name == "" || strings.Contains(name, "/") {
				continue
			}
			stream.Name = name
			streams = append(streams, stream)
		}
	}

	assert.Assert(prefix != "", "a listing names the prefix it answered for")
	answer(w, map[string]any{"prefix": prefix, "streams": streams})
}

// setRoster states who a group's members are now, and closes what anybody else holds.
//
// The whole roster and never a departure: two callers racing on "who left" can leave a member the
// last one did not name, where two callers racing on "who is here" cannot.
// The answer names what it closed, so a caller learns the removal happened rather than assuming it.
func (s *Service) setRoster(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key     string   `json:"key"`
		Members []string `json:"members"`
	}
	if !readBody(w, r, &body) {
		return
	}
	key, ok := groupOf(w, body.Key)
	if !ok {
		return
	}

	result, err := s.rosters.Set(key, body.Members)
	if err != nil {
		refuse(w, http.StatusBadRequest, err.Error())
		return
	}
	answer(w, result)
}

// viewRoster answers a group's roster beside the connections the relay is carrying for it.
//
// The key in the query, as the index takes it and for the same reason: a GET has no body a cache or
// a proxy will honour.
func (s *Service) viewRoster(w http.ResponseWriter, r *http.Request) {
	key, ok := groupOf(w, r.URL.Query().Get("group"))
	if !ok {
		return
	}
	answer(w, s.rosters.View(key))
}

// clearRoster stops enforcing a group, which is what an emptied voice channel means.
// Distinct from an empty roster, which is a channel nobody is in and closes everything on it.
func (s *Service) clearRoster(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key string `json:"key"`
	}
	if !readBody(w, r, &body) {
		return
	}
	key, ok := groupOf(w, body.Key)
	if !ok {
		return
	}
	answer(w, map[string]any{"prefix": key.Prefix(), "cleared": s.rosters.Clear(key)})
}

// reconcile runs a group's roster against the relay again, named by a path rather than by a key.
//
// The relay's read hook is the caller: it reports a path as a connection opens, which is what closes
// a member who left and came back on a token that has not expired yet.
// A path belonging to no group is refused rather than treated as one, a bare stream name naming no
// roster.
func (s *Service) reconcile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if !readBody(w, r, &body) {
		return
	}

	prefix, ok := group.PrefixOf(strings.TrimSpace(body.Path))
	if !ok {
		refuse(w, http.StatusBadRequest,
			"a run is named by the relay path a connection is on, and this one belongs to no group")
		return
	}
	// Refused rather than answered as a group with no roster, which is what it would otherwise look
	// like: streams under the public prefix are watchable by anybody, so a run there has nobody to
	// hold them against and a caller expecting one is better told.
	if prefix == PublicPrefix {
		refuse(w, http.StatusBadRequest,
			"streams under "+PublicPrefix+" are watchable by anybody, and a roster names who may watch")
		return
	}
	answer(w, s.rosters.Reconcile(prefix))
}

// readBody decodes one request body, and reports whether the caller was already refused.
func readBody(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, bodyLimit)).Decode(into); err != nil {
		refuse(w, http.StatusBadRequest, "this request carries nothing this service can read")
		return false
	}
	return true
}

// groupOf reads the group a request names, and refuses the caller where it names none.
func groupOf(w http.ResponseWriter, encoded string) (group.Key, bool) {
	if strings.TrimSpace(encoded) == "" {
		refuse(w, http.StatusBadRequest, "a group is named by its key, and this request names none")
		return nil, false
	}
	key, err := group.ParseKey(encoded)
	if err != nil {
		refuse(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	return key, true
}

// jwks publishes the key every token is verified against.
// The relay fetches it once and checks every connection locally, so nothing here runs per stream.
func (s *Service) jwks(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(s.signer.JWKS())
}

// bodyLimit is how much of a request body is read.
// A key is 44 characters of base64, so anything past this is not a request this service reads.
const bodyLimit = 4096

// allowCreation reports whether this caller may create another group, and records the creation
// where it may.
func (s *Service) allowCreation(caller string) bool {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()

	kept := s.created[caller][:0]
	for _, at := range s.created[caller] {
		if now.Sub(at) < time.Hour {
			kept = append(kept, at)
		}
	}
	if len(kept) >= CreationsPerHour {
		s.created[caller] = kept
		return false
	}
	s.created[caller] = append(kept, now)

	assert.Assert(len(s.created[caller]) <= CreationsPerHour,
		"a caller's recorded creations stay inside the hour's bound", len(s.created[caller]))
	return true
}

// caller is who a request is from, for the creation bound alone.
//
// The remote address and never a forwarded header: a header is the client's own claim,
// so bounding by one bounds by a number the caller picks.
// Behind a reverse proxy this is the proxy's address, which leaves the real bound to the proxy and
// makes this one a backstop.
func caller(r *http.Request) string {
	// SplitHostPort rather than a cut at the first colon: an IPv6 address carries colons of its own and
	// is bracketed, so "[2001:db8::1]:53321" cut that way yields "[2001" and buckets every IPv6 caller
	// under the same key.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func answer(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(body)
}

// refuse states why a request was not answered, in the shape every answer has.
//
// A sentence and not a code: the caller is a client of an HTTP API rather than of this app's own
// contract, and it has no vocabulary to look a code up in.
// docs/ipc-api.md draws that line for the control plane, and this is its other side.
func refuse(w http.ResponseWriter, status int, reason string) {
	assert.Assert(status >= 400, "a refusal carries a failing status", status)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": reason})
}
