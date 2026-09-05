// Package groupsvc is the group key, token, index and membership service.
//
// Creating a group draws a group key, a member trades that key for a short relay access token, and
// the index answers which streams a group key can see.
// All three are derivations rather than lookups, so no database sits behind any of them:
// possession of the group key is what lets somebody join,
// the prefix is that key's own digest (internal/group),
// and which streams exist is the relay's answer rather than this service's.
//
// The presence leases are the one thing held, and they are held because nobody else knows them:
// a member's own app states that it is here and refreshes it, and this is where it says so
// (internal/membership).
// Enforcing them closes connections at the relay, a token being unenforceable after the handshake.
//
// It lives in this repository rather than one of its own
// because the path-prefix derivation has to be identical on both sides,
// and two repositories are two copies of it (docs/plan.md, "Groups, auth and encryption").
//
// None of it is end to end.
// The relay terminates every protocol and re-muxes per listener, so it sees plaintext,
// and this service holds the key that signs every group's tokens.
// Both can watch a private stream, and the interface says so.
//
// Every refusal answers a caller rather than crashing.
// A malformed group key, a missing body and a caller over the creation bound are Umgebungsfehler:
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
	"sync/atomic"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/membership"
	"bjoernblessin.de/screenshare/internal/token"
)

// TokenWindow is how long a relay access token stays valid.
//
// Short, because a member who leaves holds whatever they were last issued:
// this window is how long they can still open a connection after their lease lapses.
// Long enough that a client reconnecting across a network blip
// is not asking for a token between every attempt.
//
// It bounds opening connections and nothing else.
// The relay checks at the handshake and not again, so a connection outlives its own token,
// and what a member already holds is closed rather than expired out (internal/membership).
const TokenWindow = 5 * time.Minute

// CreationsPerHour is how many groups one address may create in an hour.
//
// Creation is open, there being no accounts and so nobody to charge it to,
// which makes it something to bound rather than to gate.
// High enough that a person creating groups all afternoon never meets it,
// low enough that a script filling the relay with prefixes does.
const CreationsPerHour = 60

// Stream is one path the relay carries, as far as a member is told about it.
//
// Enough to open it and to say how it is going: the name, whether it carries anything,
// the video track deciding the protocols a viewer may receive it over,
// and the two figures a row of the grid draws beside it.
//
// The reader count crosses and the roster stays behind.
// A member is told how many are watching a stream of their own group,
// where naming them would answer a question about other members
// that presence already answers under its own request (membership.md).
type Stream struct {
	Path   string `json:"-"`
	Name   string `json:"name"`
	Ready  bool   `json:"ready"`
	Tracks string `json:"tracks,omitempty"`
	Format string `json:"format,omitempty"`
	// InMbps is what the publisher is pushing, Mbit/s, off the byte delta between two fetches.
	// Zero on the first fetch after a stream appears, there being no earlier sample to measure from.
	InMbps float64 `json:"inMbps,omitempty"`
	// Readers is how many are watching, the length of the roster the relay answered with.
	Readers int `json:"readers,omitempty"`
}

// Streams is what the relay is carrying, as the index reads it.
//
// An interface rather than the relay client: the index needs the rows above,
// where the client answers with a reader roster beside them.
// It is also what lets the index be tested without a relay.
type Streams interface {
	Paths() []Stream
}

// SrtKeys writes a group's SRT passphrase into the relay's per-prefix path configuration.
//
// The app derives the same value from the same key (internal/group, SrtPassphrase),
// so both ends of the leg agree without either being told.
// Idempotent: the state named is "this prefix is keyed with this passphrase",
// and a call that finds it already true writes nothing.
// The relay's configuration is the one copy,
// so a relay restarted empty is re-seeded by the next call rather than from anything held here.
type SrtKeys interface {
	Ensure(prefix, passphrase string) error
}

// Service answers the questions.
// Safe for concurrent use, which is what an HTTP server needs of a handler.
type Service struct {
	signer  *token.Signer
	streams Streams
	members *membership.Registry
	srtKeys SrtKeys
	// now is read rather than time.Now called directly,
	// so a test can issue a token at a moment it chooses and read the window back off it.
	now func() time.Time

	// Report bundles members send in, nil where this deployment keeps none (reports.go).
	reports Reports

	mu sync.Mutex
	// created is when each address created groups, oldest first, guarded by mu.
	// A map of slices rather than a counter per address: each creation ages out on its own hour.
	created map[string][]time.Time
	// sent is created's counterpart for reports, guarded by mu.
	sent map[string][]time.Time

	// What a scrape counts of this service.
	// Atomic rather than under mu, a count having no business waiting on the rate limiter's map.
	issued  atomic.Int64
	refused atomic.Int64
}

// Tallies counts the token route and nothing else.
//
// Every other route answers a question, where this one hands out the credential the relay checks,
// so a refusal here is the reading an operator came for.
type Tallies struct {
	TokensIssued  int64
	TokensRefused int64
}

// Tallies is what this service has counted since it was made.
func (s *Service) Tallies() Tallies {
	return Tallies{
		TokensIssued:  s.issued.Load(),
		TokensRefused: s.refused.Load(),
	}
}

// refuseToken refuses a token request and counts it.
// Beside refuse rather than inside it, the other routes' refusals being a different question.
func (s *Service) refuseToken(w http.ResponseWriter, status int, why string) {
	s.refused.Add(1)
	refuse(w, status, why)
}

// New is a service signing with this key, reading streams from there,
// holding membership in that, writing SRT keys through there
// and storing report bundles in reports, or none where it is nil.
func New(signer *token.Signer, streams Streams, members *membership.Registry, srtKeys SrtKeys, reports Reports) *Service {
	assert.IsNotNil(signer, "a service signs its tokens with a key")
	assert.IsNotNil(members, "a service holds its groups' membership in a registry")
	assert.IsNotNil(srtKeys, "a service keys its groups' prefixes at the relay")

	s := &Service{
		signer:  signer,
		streams: streams,
		members: members,
		srtKeys: srtKeys,
		reports: reports,
		now:     time.Now,
		created: map[string][]time.Time{},
		sent:    map[string][]time.Time{},
	}
	assert.IsNotNil(s.now, "a service reads the clock its windows are measured on")
	return s
}

// Handler is the service's routes.
//
// A group key travels in the request body, never in the path:
// a key in a URL is a key in every proxy log between here and the client.
// The index and the members view are the exceptions and take theirs in the query,
// a GET having no body a cache or proxy will honour.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /groups", s.createGroup)
	mux.HandleFunc("POST /tokens", s.issueToken)
	mux.HandleFunc("GET /streams", s.listStreams)
	mux.HandleFunc("GET /jwks.json", s.jwks)
	mux.HandleFunc("PUT /members", s.stateMember)
	mux.HandleFunc("DELETE /members", s.releaseMember)
	mux.HandleFunc("GET /members", s.viewMembers)
	// Reached by the relay's own read hook, which knows a path and holds no group key.
	// It grants nothing a key would have bought:
	// the answer is a run against the leases the members themselves stated,
	// and a group with no live member is left alone.
	mux.HandleFunc("POST /reconcile", s.reconcile)
	// Takes a body and no key, bounded per address (reports.go).
	mux.HandleFunc("POST /reports", s.takeReport)
	return mux
}

// createGroup draws a group key and hands it back.
// Nothing is stored because there is nothing to store:
// the group exists by somebody holding the key, and the prefix is that key's own digest.
func (s *Service) createGroup(w http.ResponseWriter, r *http.Request) {
	if !s.allowCreation(caller(r)) {
		refuse(w, http.StatusTooManyRequests, "too many groups created from here in the last hour")
		return
	}
	groupKey, err := group.NewKey()
	if err != nil {
		// The one failure here that is this machine's rather than the caller's.
		// Nothing is handed back, a key drawn from something weaker looking exactly like a real one.
		refuse(w, http.StatusInternalServerError, "no group key could be drawn")
		return
	}
	answer(w, map[string]string{"groupKey": groupKey.String(), "groupId": groupKey.ID()})
}

// issueToken trades a group key for a relay access token,
// and a request carrying no key for a public one.
//
// Deriving from the group key is the whole verification:
// holding one is what lets somebody join, so a caller holding a well-formed one is in the group,
// and one holding a key nobody else has is a group of one.
// Minted per call and never stored: the signature derives from the key like every other answer here,
// so a held token would be a second copy of a fact the key already carries.
//
// A stream lives in a group, so a request naming no key is refused:
// there is no prefix to grant and nothing outside a group for the relay to carry.
// A malformed key is refused on its own ground, being a group the caller cannot reach.
func (s *Service) issueToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		GroupKey     string `json:"groupKey"`
		MemberSecret string `json:"memberSecret"`
	}
	// An empty body reads as a request naming no group key, which the refusal below covers.
	// Anything else that will not decode is malformed rather than keyless,
	// and a caller who meant to send a key is better told.
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, bodyLimit)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		s.refuseToken(w, http.StatusBadRequest, "the request carries no group key this service can read")
		return
	}
	if strings.TrimSpace(body.GroupKey) == "" {
		s.refuseToken(w, http.StatusBadRequest, "a stream lives in a group, and this request names none")
		return
	}

	groupKey, err := group.ParseKey(body.GroupKey)
	if err != nil {
		s.refuseToken(w, http.StatusBadRequest, err.Error())
		return
	}
	subject, prefix := groupKey.ID(), groupKey.Prefix()

	// Naming a member secret moves the subject from the group to the member it derives,
	// which lets enforcement tell one member's connections from another's at the relay.
	// The grant does not move with it: membership decides who may connect, never what they reach.
	named := strings.TrimSpace(body.MemberSecret) != ""
	if named {
		secret, err := group.ParseMemberSecret(body.MemberSecret)
		if err != nil {
			s.refuseToken(w, http.StatusBadRequest, err.Error())
			return
		}
		subject = groupKey.MemberID(secret)
	}

	// The relay checks a token at the handshake and not again,
	// so a subject the leases do not hold buys a connection the next run closes.
	// Refused here instead, on the test that run makes (internal/membership).
	//
	// Two grounds and a message each, both naming the call that clears them:
	// a member states presence, which takes no token,
	// and a request naming none carries the group's own id, a subject no member ever matches.
	if s.members.Swept(groupKey, subject) {
		refusal := "this group states its members, and this request names none"
		if named {
			refusal = "this group holds no presence for the member this request names, so state presence before asking for a token"
		}
		s.refuseToken(w, http.StatusBadRequest, refusal)
		return
	}

	// Ahead of every connection the token buys,
	// so the relay knows the prefix's SRT keys before the first handshake carries them.
	s.keySrt(groupKey)

	now := s.now()
	signed, err := s.signer.Sign(subject, token.GroupPermissions(prefix), now, TokenWindow)
	if err != nil {
		s.refuseToken(w, http.StatusInternalServerError, "no token could be signed")
		return
	}
	s.issued.Add(1)
	answer(w, map[string]any{
		"relayAccessToken": signed,
		"prefix":           prefix,
		"expires":          now.Add(TokenWindow).UTC().Format(time.RFC3339),
	})
}

// listStreams answers which streams the caller may see: their own group's, and no other's.
//
// The split is enforced here rather than left to a shell, which is the point of the index:
// a listing a client narrowed arrived carrying every group's streams.
// A stream lives in a group, so a request naming no key is asking about streams nobody holds,
// and it is refused rather than answered with a listing of everything outside every group.
func (s *Service) listStreams(w http.ResponseWriter, r *http.Request) {
	encoded := strings.TrimSpace(r.URL.Query().Get("groupKey"))
	if encoded == "" {
		refuse(w, http.StatusBadRequest, "a stream lives in a group, and this request names none")
		return
	}
	groupKey, err := group.ParseKey(encoded)
	if err != nil {
		refuse(w, http.StatusBadRequest, err.Error())
		return
	}
	prefix := groupKey.Prefix()

	streams := []Stream{}
	if s.streams != nil {
		for _, stream := range s.streams.Paths() {
			name, ok := strings.CutPrefix(stream.Path, prefix)
			if !ok || !group.NameHolds(name) {
				continue
			}
			stream.Name = name
			streams = append(streams, stream)
		}
	}

	assert.Assert(prefix != "", "a listing names the prefix it answered for")
	answer(w, map[string]any{"prefix": prefix, "streams": streams})
}

// stateMember states one member's presence, which is a claim and a refresh at once.
//
// Idempotent by construction: the request names the state it wants true,
// that this member is here under this name,
// so an app can send it on every pass of the poll it already runs.
// The answer is the whole group,
// so the same call that refreshes is the one that reads who else is here and what they are sharing.
//
// A name another member holds is 409 rather than 400:
// the caller's request is well formed and the answer is an app asking its user for another name.
func (s *Service) stateMember(w http.ResponseWriter, r *http.Request) {
	var body struct {
		GroupKey     string `json:"groupKey"`
		MemberSecret string `json:"memberSecret"`
		DisplayName  string `json:"displayName"`
	}
	if !readBody(w, r, &body) {
		return
	}
	groupKey, ok := groupOf(w, body.GroupKey)
	if !ok {
		return
	}
	secret, ok := memberOf(w, body.MemberSecret)
	if !ok {
		return
	}

	// On the poll a member's app already runs,
	// so a relay whose configuration restarted empty is keyed again within one pass
	// rather than when the next token is asked for.
	s.keySrt(groupKey)

	stated, err := s.members.State(groupKey, secret, body.DisplayName)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, membership.ErrNameTaken) {
			status = http.StatusConflict
		}
		refuse(w, status, err.Error())
		return
	}
	answer(w, map[string]any{
		"memberId":     stated.MemberID,
		"displayName":  stated.DisplayName,
		"leaseSeconds": int(stated.Lease.Seconds()),
		"members":      stated.Members,
		// Publishing false under a list the relay would not answer
		// is what a member sending nothing looks like,
		// so the answer says which of the two it read.
		"publishingUnread": len(stated.Unread) > 0,
	})
}

// releaseMember drops one member's presence and closes what it held.
//
// Idempotent: a member holding no lease is already in the state this names,
// so it answers that there was none to release and succeeds.
func (s *Service) releaseMember(w http.ResponseWriter, r *http.Request) {
	var body struct {
		GroupKey     string `json:"groupKey"`
		MemberSecret string `json:"memberSecret"`
	}
	if !readBody(w, r, &body) {
		return
	}
	groupKey, ok := groupOf(w, body.GroupKey)
	if !ok {
		return
	}
	secret, ok := memberOf(w, body.MemberSecret)
	if !ok {
		return
	}

	released, err := s.members.Release(groupKey, secret)
	if err != nil {
		refuse(w, http.StatusBadRequest, err.Error())
		return
	}
	answer(w, map[string]any{"memberId": released.MemberID, "released": released.Released})
}

// viewMembers answers who is in a group without stating anything.
//
// The group key in the query, as the index takes it and for the same reason:
// a GET has no body a cache or a proxy will honour.
func (s *Service) viewMembers(w http.ResponseWriter, r *http.Request) {
	groupKey, ok := groupOf(w, r.URL.Query().Get("groupKey"))
	if !ok {
		return
	}
	view := s.members.View(groupKey)
	answer(w, map[string]any{"members": view.Members, "publishingUnread": len(view.Unread) > 0})
}

// reconcile runs a group's leases against the relay again,
// named by a path rather than by a group key.
//
// The relay's read hook is the caller: it reports a path as a connection opens,
// which closes a member whose lease lapsed and who came back on an unexpired token.
// A path belonging to no group is refused rather than treated as one,
// a bare stream name naming no group.
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
	answer(w, s.members.Reconcile(prefix))
}

// keySrt writes this group's SRT keys through to the relay.
//
// Best effort per request:
// a relay that will not take them costs the SRT leg until a later call reaches it,
// where a refusal here would cost every leg the answer buys.
// Warnf and never Errorf, an unreachable relay API being an Umgebungsfehler this service outlives.
func (s *Service) keySrt(groupKey group.Key) {
	if err := s.srtKeys.Ensure(groupKey.Prefix(), groupKey.SrtPassphrase()); err != nil {
		logger.Warnf("the relay is not taking SRT keys for %s: %v", groupKey.ID(), err)
	}
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
		refuse(w, http.StatusBadRequest, "a group is named by its group key, and this request names none")
		return nil, false
	}
	groupKey, err := group.ParseKey(encoded)
	if err != nil {
		refuse(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	return groupKey, true
}

// memberOf reads the member a request names, and refuses the caller where it names none.
//
// The secret and never the display name:
// a name is a label anybody may claim, where the secret is what nobody but that member holds.
func memberOf(w http.ResponseWriter, encoded string) (group.MemberSecret, bool) {
	if strings.TrimSpace(encoded) == "" {
		refuse(w, http.StatusBadRequest, "a member is named by its member secret, and this request names none")
		return nil, false
	}
	secret, err := group.ParseMemberSecret(encoded)
	if err != nil {
		refuse(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	return secret, true
}

// jwks publishes the key every token is verified against.
// The relay fetches it once and checks every connection locally, so nothing here runs per stream.
func (s *Service) jwks(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(s.signer.JWKS())
}

// bodyLimit is how much of a request body is read.
// A group key and a member secret are 44 characters of base64 each,
// so anything past this is not a request this service reads.
const bodyLimit = 4096

// allowCreation reports whether this caller may create another group,
// and records the creation where it may.
func (s *Service) allowCreation(caller string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return recordWithin(s.created, caller, CreationsPerHour, s.now())
}

// recordWithin records one event for caller where fewer than bound happened in the last hour,
// and reports whether it did.
// Each event ages out on its own hour, which is what the slices are for.
// Callers hold mu.
func recordWithin(record map[string][]time.Time, caller string, bound int, now time.Time) bool {
	assert.IsNotNil(record, "a bound is kept against a record")
	assert.Assert(bound > 0, "a bound admits at least one event an hour", bound)

	kept := record[caller][:0]
	for _, at := range record[caller] {
		if now.Sub(at) < time.Hour {
			kept = append(kept, at)
		}
	}
	if len(kept) >= bound {
		record[caller] = kept
		return false
	}
	record[caller] = append(kept, now)

	assert.Assert(len(record[caller]) <= bound,
		"a caller's recorded events stay inside the hour's bound", len(record[caller]))
	return true
}

// caller is who a request is from, for the creation bound alone.
//
// The remote address and never a forwarded header: a header is the client's own claim,
// so bounding by one bounds by a number the caller picks.
// Behind a reverse proxy this is the proxy's address,
// which leaves the real bound to the proxy and makes this one a backstop.
func caller(r *http.Request) string {
	// SplitHostPort rather than a cut at the first colon:
	// an IPv6 address carries colons of its own and is bracketed,
	// so "[2001:db8::1]:53321" cut that way yields "[2001",
	// bucketing every IPv6 caller under one key.
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
// A sentence rather than a code: the caller is a client of an HTTP API
// rather than of this app's own contract, and it has no vocabulary to look a code up in.
// docs/ipc-api.md draws that line for the control plane, and this is its other side.
func refuse(w http.ResponseWriter, status int, reason string) {
	assert.Assert(status >= 400, "a refusal carries a failing status", status)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": reason})
}
