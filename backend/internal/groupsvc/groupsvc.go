// Package groupsvc is the key, token and index service.
//
// Three things, and almost nothing held.
// Creating a group draws a key, a member trades that key for a short relay token,
// and the index answers which streams a key can see.
// All three are derivations rather than lookups, so no database sits behind any of them: possession
// of the key is membership, the prefix is the key's own digest (internal/group),
// and which streams exist is the relay's answer rather than this service's.
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
	"net/http"
	"strings"
	"sync"
	"time"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/token"
)

// TokenWindow is how long a relay token stays valid.
//
// Short, because a member who leaves still holds the group key: only a rotation stops them,
// and this window is how long the old key keeps working after one.
// Long enough that a client reconnecting across a network blip is not asking for a token between
// every attempt.
//
// A live connection survives its token expiring.
// The relay checks at the handshake and not again, so this bounds opening connections rather than
// holding them, the assumption the whole lifetime rests on (docs/plan.md, "Assumptions to verify").
const TokenWindow = 5 * time.Minute

// CreationsPerHour is how many groups one address may create in an hour.
//
// Creation is open, there being no accounts and so nobody to charge it to,
// which makes it something to bound rather than to gate.
// High enough that a person creating groups all afternoon never meets it, low enough that a script
// filling the relay with prefixes does.
const CreationsPerHour = 60

// PublicPrefix is where a stream anybody may watch lives.
//
// Watchable and discoverable both follow from the prefix: the relay grants reading on it to
// everyone, and the index answers it to a caller with no key.
// It has a group id's shape without being one, since no key derives it, so a group's own listing
// cannot include it and holding a key cannot publish into it.
const PublicPrefix = "public/"

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

// Service answers the three questions.
// Safe for concurrent use, which is what an HTTP server needs of a handler.
type Service struct {
	signer  *token.Signer
	streams Streams
	// now is read rather than time.Now called directly, so a test can issue a token at a moment it
	// chooses and read the window back off it.
	now func() time.Time

	mu sync.Mutex
	// created is when each address created groups, oldest first, guarded by mu.
	// A map of slices rather than a counter per address: each creation ages out on its own hour.
	created map[string][]time.Time
}

// New is a service signing with this key and reading streams from there.
func New(signer *token.Signer, streams Streams) *Service {
	assert.IsNotNil(signer, "a service signs its tokens with a key")

	s := &Service{signer: signer, streams: streams, now: time.Now, created: map[string][]time.Time{}}
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

// issueToken trades a group key for a relay token.
//
// Deriving from the key is the whole verification, there being no membership list to check it
// against: a caller holding a well-formed key is a member, and one holding a key nobody else has is
// a group of one.
// Minted per call and never stored: the signature derives from the key like every other answer
// here, so a held token would be a second copy of a fact the key already carries.
func (s *Service) issueToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, bodyLimit)).Decode(&body); err != nil {
		refuse(w, http.StatusBadRequest, "the request carries no group key")
		return
	}
	key, err := group.ParseKey(body.Key)
	if err != nil {
		refuse(w, http.StatusBadRequest, err.Error())
		return
	}

	now := s.now()
	signed, err := s.signer.Sign(key.ID(), token.GroupPermissions(key.Prefix()), now, TokenWindow)
	if err != nil {
		refuse(w, http.StatusInternalServerError, "no token could be signed")
		return
	}
	answer(w, map[string]any{
		"token":   signed,
		"prefix":  key.Prefix(),
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
	host, _, ok := strings.Cut(r.RemoteAddr, ":")
	if !ok {
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
