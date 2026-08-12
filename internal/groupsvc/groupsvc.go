// Package groupsvc is the key, token and index service.
//
// It does three things and holds almost nothing. A group is created by drawing a key; a
// member trades that key for a short relay token; and the index answers which streams a key
// can see. All three are derivations rather than lookups, which is why there is no database
// behind any of them: possession of the key is membership, the prefix is the key's own digest
// (internal/group), and which streams exist is the relay's answer rather than this service's.
//
// It lives in this repository rather than in one of its own because the path-prefix
// derivation has to be identical on both sides, and two repositories means two copies of it
// (docs/plan.md, "Groups, auth and encryption").
//
// None of it is end to end. The relay terminates every protocol and re-muxes for every
// listener, so it sees plaintext by construction, and this service holds the key that signs
// tokens for every group. Both can watch a private stream, and the interface says so rather
// than implying otherwise.
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

// TokenWindow is how long a relay token is valid for.
//
// Short, because possession as membership means a member who leaves still holds the group
// key: what stops them is a rotation, and the window is how long the old key keeps working
// after one. Long enough that a client reconnecting across a network blip is not asking for a
// token between every attempt.
//
// A live connection survives its token expiring. The relay checks at the handshake and not
// again, so this is a bound on opening connections rather than on holding them, which is the
// assumption the whole lifetime rests on (docs/plan.md, "Assumptions to verify").
const TokenWindow = 5 * time.Minute

// CreationsPerHour is how many groups one address may create in an hour.
//
// Creation is open - there are no accounts, so there is nobody to charge it to - which makes
// it something to bound rather than something to gate. The figure is high enough that a
// person creating groups all afternoon never meets it and low enough that a script filling
// the relay with prefixes does.
const CreationsPerHour = 60

// PublicPrefix is where a stream anybody may watch lives.
//
// Public means watchable and discoverable, and both follow from the prefix: the relay grants
// reading on it to everyone, and the index answers it to a caller with no key. It is a group
// id's shape without being one - no key derives it - so a group's own listing cannot include
// it and a public stream cannot be published by holding a key.
const PublicPrefix = "public/"

// Streams is what the index reads: the paths the relay is carrying.
//
// An interface rather than the relay client itself, because what the index needs is a list of
// names and the client answers with bitrates, rosters and readiness. It is also what lets the
// index be tested without a relay, which is the only way it can be tested at all.
type Streams interface {
	Paths() []string
}

// Service answers the three questions. It is safe for concurrent use, which is what an HTTP
// server needs of a handler.
type Service struct {
	signer  *token.Signer
	streams Streams
	// now is read rather than called directly, so a test can issue a token at a moment it
	// chooses and read the window back off it.
	now func() time.Time

	mu sync.Mutex
	// created is when each address last created groups, oldest first, for the bound above.
	// A map of slices rather than a counter per address, because what has to age out is each
	// creation on its own hour.
	created map[string][]time.Time
}

// New is a service signing with this key and reading streams from there.
func New(signer *token.Signer, streams Streams) *Service {
	assert.IsNotNil(signer, "a service signs its tokens with a key")
	return &Service{signer: signer, streams: streams, now: time.Now, created: map[string][]time.Time{}}
}

// Handler is the service's routes.
//
// Every route is a POST or a GET and nothing else, and the two that take a key take it in the
// body rather than in the path: a key in a URL is a key in every proxy log between here and
// the client.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /groups", s.createGroup)
	mux.HandleFunc("POST /tokens", s.issueToken)
	mux.HandleFunc("GET /streams", s.listStreams)
	mux.HandleFunc("GET /jwks.json", s.jwks)
	return mux
}

// createGroup draws a key and hands it back. Nothing is stored, because there is nothing to
// store: the group exists by virtue of somebody holding the key, and the prefix is that key's
// own digest.
func (s *Service) createGroup(w http.ResponseWriter, r *http.Request) {
	if !s.allowCreation(caller(r)) {
		refuse(w, http.StatusTooManyRequests, "too many groups created from here in the last hour")
		return
	}
	key, err := group.NewKey()
	if err != nil {
		// The randomness failed, which is the one failure here that is this machine's
		// rather than the caller's. A key drawn from something weaker would look exactly
		// like a real one, so nothing is handed back at all.
		refuse(w, http.StatusInternalServerError, "no group key could be drawn")
		return
	}
	answer(w, map[string]string{"key": key.String(), "id": key.ID()})
}

// issueToken trades a group key for a relay token.
//
// The key is verified by deriving from it and nothing else. There is no membership list to
// check it against, which is the model: a caller holding a well-formed key is a member, and a
// caller holding a key nobody else has is a group of one.
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

// listStreams answers which streams the caller may see: their group's where they present a
// key, and the public ones where they do not.
//
// The split is enforced here rather than left to a shell to filter, which is the point of the
// index: a listing a client narrowed would be a listing that arrived carrying every group's
// streams. A group's listing hides the public ones for the same reason it hides another
// group's - what a member asked for is their group.
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

	names := []string{}
	if s.streams != nil {
		for _, path := range s.streams.Paths() {
			if name, ok := strings.CutPrefix(path, prefix); ok && name != "" && !strings.Contains(name, "/") {
				names = append(names, name)
			}
		}
	}
	answer(w, map[string]any{"prefix": prefix, "streams": names})
}

// jwks publishes the key every token is verified against. It is what the relay fetches once
// and checks every connection with locally, which is why nothing here is called per stream.
func (s *Service) jwks(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(s.signer.JWKS())
}

// bodyLimit is how much of a request body is read. A key is 44 characters of base64, so
// anything past this is not a request this service has a reading for.
const bodyLimit = 4096

// allowCreation reports whether this caller may create another group, and records it where it
// may.
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
	return true
}

// caller is who a request is from, for the creation bound alone.
//
// The remote address and never a forwarded header: a header is the client's own claim, so
// bounding by one is bounding by a number the caller chooses. A deployment behind the reverse
// proxy sees the proxy's address, which makes the bound the proxy's to enforce there and this
// one a backstop.
func caller(r *http.Request) string {
	host, _, ok := strings.Cut(r.RemoteAddr, ":")
	if !ok {
		return r.RemoteAddr
	}
	return host
}

// answer writes one JSON body.
func answer(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(body)
}

// refuse states why a request was not answered, in the shape every answer has.
//
// The sentence is the service's rather than a code, because this is not the app's own
// contract: the caller is a client of an HTTP API, which has no vocabulary to look a code up
// in (docs/ipc-api.md draws that line for the control plane, and this is the other side of it).
func refuse(w http.ResponseWriter, status int, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": reason})
}
