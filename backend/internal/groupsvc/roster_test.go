package groupsvc

import (
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/roster"
	"bjoernblessin.de/screenshare/internal/token"
)

// A roster is stated over these routes and enforced at the relay.
// These hold the wiring: which request reaches which derivation, what a refusal says, and that a
// member's token names that member rather than the group.

// carrying is a relay holding these connections, and a record of what was closed.
type carrying struct {
	live   []relay.Session
	kicked []string
}

func (c *carrying) Sessions() ([]relay.Session, []relay.Unread) {
	return slices.Clone(c.live), nil
}

func (c *carrying) Kick(_, id string) error {
	c.kicked = append(c.kicked, id)
	c.live = slices.DeleteFunc(c.live, func(s relay.Session) bool { return s.ID == id })
	return nil
}

// enforcing is a service whose relay carries these connections.
func enforcing(t *testing.T, live ...relay.Session) (*Service, *carrying) {
	t.Helper()
	signer, err := token.NewSigner()
	if err != nil {
		t.Fatalf("drawing a signing key: %v", err)
	}
	relayed := &carrying{live: live}
	return New(signer, paths(nil), roster.New(relayed)), relayed
}

func mustKey(t *testing.T) group.Key {
	t.Helper()
	key, err := group.NewKey()
	if err != nil {
		t.Fatalf("drawing a group key: %v", err)
	}
	return key
}

func memberID(t *testing.T, key group.Key, name string) string {
	t.Helper()
	id, err := key.MemberID(name)
	if err != nil {
		t.Fatalf("deriving a member id: %v", err)
	}
	return id
}

// The subject is what the relay lists a connection under, so a token naming the group leaves every
// member's connection looking like every other one's.
func TestATokenForAMemberNamesThatMember(t *testing.T) {
	key := mustKey(t)
	service, _ := enforcing(t)

	status, body := call(t, service, "POST", "/tokens",
		`{"key":"`+key.String()+`","member":"alice"}`)
	if status != 200 {
		t.Fatalf("a member's token was refused with %d: %v", status, body)
	}

	claimed := claims(t, body["token"].(string))
	if want := `"sub":"` + memberID(t, key, "alice") + `"`; !strings.Contains(claimed, want) {
		t.Errorf("the token claims %s, and names no member", claimed)
	}
	// The grant is the group's, membership deciding who connects and never what they may reach.
	if !strings.Contains(claimed, key.Prefix()) {
		t.Errorf("a member's token grants %s, which is not their group's prefix", claimed)
	}
}

// A member is a member of a group, so naming one without the key that derives the group is a request
// this service cannot answer rather than one it answers for the public prefix.
func TestAMemberNamedWithoutAKeyIsRefused(t *testing.T) {
	service, _ := enforcing(t)

	status, body := call(t, service, "POST", "/tokens", `{"member":"alice"}`)
	if status != 400 {
		t.Fatalf("a member with no group was answered %d: %v", status, body)
	}
}

// Stating a roster is what removes somebody: the answer says what it closed, so a caller knows the
// removal happened rather than assuming it.
func TestStatingARosterClosesWhatANonMemberHolds(t *testing.T) {
	key := mustKey(t)
	service, relayed := enforcing(t,
		relay.Session{Segment: "srtconns", ID: "alice-1", Path: key.Prefix() + "desk", User: memberID(t, key, "alice"), State: "read"},
		relay.Session{Segment: "srtconns", ID: "bob-1", Path: key.Prefix() + "desk", User: memberID(t, key, "bob"), State: "read"},
	)

	status, body := call(t, service, "PUT", "/roster",
		`{"key":"`+key.String()+`","members":["alice"]}`)
	if status != 200 {
		t.Fatalf("stating a roster answered %d: %v", status, body)
	}
	if !slices.Equal(relayed.kicked, []string{"bob-1"}) {
		t.Fatalf("stating a roster closed %v", relayed.kicked)
	}

	kicked, _ := body["kicked"].([]any)
	if len(kicked) != 1 {
		t.Fatalf("the answer names %v as closed", body["kicked"])
	}
	if body["enforced"] != true {
		t.Errorf("a group with a roster answered enforced=%v", body["enforced"])
	}
}

// The relay reports a read as it starts, which is what makes a reconnect on a token that has not
// expired close again instead of being served.
func TestReconcileTakesTheRelaysOwnPath(t *testing.T) {
	key := mustKey(t)
	service, relayed := enforcing(t)

	if status, body := call(t, service, "PUT", "/roster",
		`{"key":"`+key.String()+`","members":["alice"]}`); status != 200 {
		t.Fatalf("stating a roster answered %d: %v", status, body)
	}

	relayed.live = []relay.Session{
		{Segment: "hlssessions", ID: "back-again", Path: key.Prefix() + "desk", User: memberID(t, key, "bob"), State: "read"},
	}
	status, body := call(t, service, "POST", "/reconcile", `{"path":"`+key.Prefix()+`desk"}`)
	if status != 200 {
		t.Fatalf("a reconcile answered %d: %v", status, body)
	}
	if !slices.Equal(relayed.kicked, []string{"back-again"}) {
		t.Errorf("a reconnect after a roster change was left alone: closed %v", relayed.kicked)
	}
}

// A path belonging to no group names no roster, and answering one would enforce against a stream
// name.
func TestReconcileOnAPathOutsideAnyGroupIsRefused(t *testing.T) {
	service, _ := enforcing(t)

	if status, _ := call(t, service, "POST", "/reconcile", `{"path":"desk"}`); status != 400 {
		t.Errorf("a path outside any group was answered %d", status)
	}
}

// A group nobody stated a roster for is not enforced, so the hook firing for one is a no-op rather
// than a removal of everybody on it.
func TestReconcileOnAGroupWithNoRosterClosesNothing(t *testing.T) {
	key := mustKey(t)
	service, relayed := enforcing(t,
		relay.Session{Segment: "srtconns", ID: "stranger", Path: key.Prefix() + "desk", User: "whoever", State: "read"},
	)

	status, body := call(t, service, "POST", "/reconcile", `{"path":"`+key.Prefix()+`desk"}`)
	if status != 200 {
		t.Fatalf("a reconcile answered %d: %v", status, body)
	}
	if body["enforced"] != false {
		t.Errorf("a group with no roster answered enforced=%v", body["enforced"])
	}
	if len(relayed.kicked) != 0 {
		t.Errorf("a group with no roster closed %v", relayed.kicked)
	}
}

// The view is how a reader sees who is on and who is connected, without holding a key that reaches
// the relay's own API.
func TestTheViewNamesTheGroupsConnections(t *testing.T) {
	key := mustKey(t)
	service, _ := enforcing(t,
		relay.Session{Segment: "srtconns", ID: "alice-1", Path: key.Prefix() + "desk", User: memberID(t, key, "alice"), State: "read"},
	)
	if status, body := call(t, service, "PUT", "/roster",
		`{"key":"`+key.String()+`","members":["alice"]}`); status != 200 {
		t.Fatalf("stating a roster answered %d: %v", status, body)
	}

	status, body := call(t, service, "GET", "/roster?group="+url.QueryEscape(key.String()), "")
	if status != 200 {
		t.Fatalf("the view answered %d: %v", status, body)
	}

	sessions, _ := body["sessions"].([]any)
	if len(sessions) != 1 {
		t.Fatalf("the view carries %v", body["sessions"])
	}
	first, _ := sessions[0].(map[string]any)
	if first["member"] != "alice" {
		t.Errorf("a member's connection came back as %v", first)
	}
}

// Clearing is what an emptied voice channel means, and it stops the group being enforced rather than
// closing everything on it.
func TestClearingARosterStopsEnforcement(t *testing.T) {
	key := mustKey(t)
	service, relayed := enforcing(t)

	if status, _ := call(t, service, "PUT", "/roster",
		`{"key":"`+key.String()+`","members":["alice"]}`); status != 200 {
		t.Fatal("stating a roster was refused")
	}
	if status, body := call(t, service, "DELETE", "/roster", `{"key":"`+key.String()+`"}`); status != 200 {
		t.Fatalf("clearing a roster answered %d: %v", status, body)
	}

	relayed.live = []relay.Session{
		{Segment: "srtconns", ID: "after", Path: key.Prefix() + "desk", User: "whoever", State: "read"},
	}
	if status, body := call(t, service, "POST", "/reconcile", `{"path":"`+key.Prefix()+`desk"}`); status != 200 {
		t.Fatalf("a reconcile answered %d: %v", status, body)
	}
	if len(relayed.kicked) != 0 {
		t.Errorf("a cleared group closed %v", relayed.kicked)
	}
}

// A roster names a group, and a request carrying no key names none.
func TestARosterWithoutAKeyIsRefused(t *testing.T) {
	service, _ := enforcing(t)

	if status, _ := call(t, service, "PUT", "/roster", `{"members":["alice"]}`); status != 400 {
		t.Error("a roster with no group key was taken")
	}
	if status, _ := call(t, service, "GET", "/roster", ""); status != 400 {
		t.Error("a view with no group key was answered")
	}
}

// An empty roster is a voice channel nobody is in, which closes everything.
// Distinct from a group with no roster, which is one this service was never told about.
func TestAnEmptyRosterClosesEverything(t *testing.T) {
	key := mustKey(t)
	service, relayed := enforcing(t,
		relay.Session{Segment: "srtconns", ID: "last-one", Path: key.Prefix() + "desk", User: memberID(t, key, "alice"), State: "read"},
	)

	status, body := call(t, service, "PUT", "/roster", `{"key":"`+key.String()+`","members":[]}`)
	if status != 200 {
		t.Fatalf("an empty roster answered %d: %v", status, body)
	}
	if !slices.Equal(relayed.kicked, []string{"last-one"}) {
		t.Errorf("an empty roster closed %v", relayed.kicked)
	}
}

// Every route refuses a body it cannot read rather than acting on half of one.
func TestARequestBodyTheServiceCannotReadIsRefused(t *testing.T) {
	service, relayed := enforcing(t)

	for _, request := range []struct {
		method, target string
	}{
		{"PUT", "/roster"},
		{"DELETE", "/roster"},
		{"POST", "/reconcile"},
	} {
		status, body := call(t, service, request.method, request.target, `{"key":`)
		if status != 400 {
			t.Errorf("%s %s took a body that does not read as JSON: %d %v", request.method, request.target, status, body)
		}
	}
	if len(relayed.kicked) != 0 {
		t.Errorf("a request nobody could read closed %v", relayed.kicked)
	}
}

// A key of the wrong length is one this service did not make, and a prefix derived from it would
// enforce against a group nobody is in.
func TestARosterKeyTheServiceCannotReadIsRefused(t *testing.T) {
	service, _ := enforcing(t)

	for _, request := range []struct{ method, target, body string }{
		{"PUT", "/roster", `{"key":"not-a-key","members":["alice"]}`},
		{"DELETE", "/roster", `{"key":"not-a-key"}`},
	} {
		if status, _ := call(t, service, request.method, request.target, request.body); status != 400 {
			t.Errorf("%s %s took a key it cannot read: %d", request.method, request.target, status)
		}
	}
	if status, _ := call(t, service, "GET", "/roster?group=not-a-key", ""); status != 400 {
		t.Error("the view took a key it cannot read")
	}
}

// Streams under the public prefix are watchable by anybody, so a run there is refused with the
// reason rather than answered as a group whose roster nobody stated.
func TestAReconcileOnThePublicPrefixIsRefused(t *testing.T) {
	service, relayed := enforcing(t,
		relay.Session{Segment: "srtconns", ID: "public-read", Path: group.PublicPrefix + "desk", User: "public", State: "read"},
	)

	status, body := call(t, service, "POST", "/reconcile", `{"path":"`+group.PublicPrefix+`desk"}`)
	if status != 400 {
		t.Fatalf("a run on the public prefix was answered %d: %v", status, body)
	}
	if len(relayed.kicked) != 0 {
		t.Errorf("a run on the public prefix closed %v", relayed.kicked)
	}
}

// Clearing says whether there was anything to clear, so a caller stopping a group twice learns the
// second one changed nothing.
func TestClearingAGroupWithNoRosterSaysSo(t *testing.T) {
	key := mustKey(t)
	service, _ := enforcing(t)

	status, body := call(t, service, "DELETE", "/roster", `{"key":"`+key.String()+`"}`)
	if status != 200 {
		t.Fatalf("clearing answered %d: %v", status, body)
	}
	if body["cleared"] != false {
		t.Errorf("clearing a group that had no roster answered cleared=%v", body["cleared"])
	}
}

// A group with no roster still has connections, and a key holder asking about their own group is
// answered them. What they are not told is that anybody is being held to anything.
func TestTheViewOfAGroupWithNoRosterCarriesItsConnections(t *testing.T) {
	key := mustKey(t)
	service, _ := enforcing(t,
		relay.Session{Segment: "srtconns", Transport: "srt", ID: "one", Path: key.Prefix() + "desk", User: "whoever", State: "read"},
	)

	status, body := call(t, service, "GET", "/roster?group="+url.QueryEscape(key.String()), "")
	if status != 200 {
		t.Fatalf("the view answered %d: %v", status, body)
	}
	if body["enforced"] != false {
		t.Errorf("a group with no roster answered enforced=%v", body["enforced"])
	}
	if sessions, _ := body["sessions"].([]any); len(sessions) != 1 {
		t.Errorf("the view carries %v", body["sessions"])
	}
}

// The answers name a stream inside the group and a leg in this app's vocabulary, which is what the
// index answers under and what every other surface of this app says.
// The relay's session id and the address a connection came from are an operator's view of the relay,
// and a group key is membership rather than an operator's credential.
func TestTheAnswersCarryNoRelayOperationalState(t *testing.T) {
	key := mustKey(t)
	service, _ := enforcing(t,
		relay.Session{Segment: "srtconns", Transport: "srt", ID: "session-id-here",
			Path: key.Prefix() + "desk", User: memberID(t, key, "bob"), State: "read",
			RemoteAddr: "203.0.113.7:5000"},
	)

	for _, answered := range []string{
		bodyOf(t, service, "PUT", "/roster", `{"key":"`+key.String()+`","members":["alice"]}`),
		bodyOf(t, service, "GET", "/roster?group="+url.QueryEscape(key.String()), ""),
	} {
		if strings.Contains(answered, "203.0.113.7") {
			t.Errorf("an answer carried another member's address: %s", answered)
		}
		if strings.Contains(answered, "session-id-here") {
			t.Errorf("an answer carried the relay's own session id: %s", answered)
		}
		if strings.Contains(answered, "srtconns") {
			t.Errorf("an answer named a leg in the relay's vocabulary: %s", answered)
		}
	}
}

// A route answers the methods it names and nothing else, so a caller reaching for one it does not
// serve is told rather than falling through to another.
func TestARouteAnswersTheMethodsItNames(t *testing.T) {
	service, _ := enforcing(t)

	for _, request := range []struct{ method, target string }{
		{"POST", "/roster"},
		{"PUT", "/reconcile"},
		{"GET", "/reconcile"},
		{"DELETE", "/tokens"},
	} {
		if status, _ := raw(t, service, request.method, request.target, `{}`); status != 405 {
			t.Errorf("%s %s answered %d, where the route names no such method", request.method, request.target, status)
		}
	}
}

// A member named with nothing but spaces is a caller naming a member, so it is refused rather than
// quietly answered with the group's own token.
func TestAMemberNamedWithNothingIsRefused(t *testing.T) {
	key := mustKey(t)
	service, _ := enforcing(t)

	if status, _ := call(t, service, "POST", "/tokens", `{"key":"`+key.String()+`","member":"   "}`); status != 400 {
		t.Error("a member named with spaces alone bought a token")
	}
}

// raw makes one request and returns its status and the body as it was written, for the answers that
// are not JSON and for the ones being read for what they do not carry.
func raw(t *testing.T, s *Service, method, target, body string) (int, string) {
	t.Helper()
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.RemoteAddr = "192.0.2.1:1234"
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w.Code, w.Body.String()
}

// bodyOf is one request's answer as it was written.
func bodyOf(t *testing.T, s *Service, method, target, body string) string {
	t.Helper()
	_, answered := raw(t, s, method, target, body)
	return answered
}

// The index and the view answer about the same streams, so they name them the same way: the name
// inside the group, with the prefix off. A caller joining the two derives nothing of its own.
func TestTheIndexAndTheViewNameOneStreamAlike(t *testing.T) {
	key := mustKey(t)
	signer, err := token.NewSigner()
	if err != nil {
		t.Fatalf("drawing a signing key: %v", err)
	}
	relayed := &carrying{live: []relay.Session{
		{Segment: "srtconns", Transport: "srt", ID: "one", Path: key.Prefix() + "desk",
			User: memberID(t, key, "alice"), State: "read"},
	}}
	service := New(signer, paths{key.Prefix() + "desk"}, roster.New(relayed))

	if status, body := call(t, service, "PUT", "/roster",
		`{"key":"`+key.String()+`","members":["alice"]}`); status != 200 {
		t.Fatalf("stating a roster answered %d: %v", status, body)
	}

	_, indexed := call(t, service, "GET", "/streams?group="+url.QueryEscape(key.String()), "")
	_, viewed := call(t, service, "GET", "/roster?group="+url.QueryEscape(key.String()), "")

	if indexed["prefix"] != viewed["prefix"] {
		t.Errorf("the index answered for %v and the view for %v", indexed["prefix"], viewed["prefix"])
	}

	streams, _ := indexed["streams"].([]any)
	sessions, _ := viewed["sessions"].([]any)
	if len(streams) != 1 || len(sessions) != 1 {
		t.Fatalf("the index carries %v and the view %v", indexed["streams"], viewed["sessions"])
	}

	named, _ := streams[0].(map[string]any)
	held, _ := sessions[0].(map[string]any)
	if named["name"] != held["stream"] {
		t.Errorf("the index names the stream %v and the view names it %v", named["name"], held["stream"])
	}
}

// The index derives a prefix from the key it is given, so a key it cannot read names no group to
// answer for.
func TestAnIndexKeyTheServiceCannotReadIsRefused(t *testing.T) {
	service, _ := enforcing(t)

	if status, _ := call(t, service, "GET", "/streams?group=not-a-key", ""); status != 400 {
		t.Error("the index took a key it cannot read")
	}
}
