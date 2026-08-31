package groupsvc

import (
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/membership"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/token"
)

// Presence is stated over these routes and enforced at the relay.
// These hold the wiring: which request reaches which derivation, what a refusal says,
// and that a member's token names that member rather than the group.

// carrying is a relay holding these connections, and a record of what was closed.
type carrying struct {
	live   []relay.Session
	unread []relay.Unread
	kicked []string
}

func (c *carrying) Sessions() ([]relay.Session, []relay.Unread) {
	return slices.Clone(c.live), c.unread
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
	return New(signer, paths(nil), membership.New(relayed), &keyed{}), relayed
}

func mustKey(t *testing.T) group.Key {
	t.Helper()
	key, err := group.NewKey()
	if err != nil {
		t.Fatalf("drawing a group key: %v", err)
	}
	return key
}

func mustSecret(t *testing.T) group.MemberSecret {
	t.Helper()
	secret, err := group.NewMemberSecret()
	if err != nil {
		t.Fatalf("drawing a member secret: %v", err)
	}
	return secret
}

// presence is the body one member states its presence with.
func presence(key group.Key, secret group.MemberSecret, name string) string {
	return `{"groupKey":"` + key.String() + `","memberSecret":"` + secret.String() + `","displayName":"` + name + `"}`
}

// The subject is what the relay lists a connection under,
// so a token naming the group leaves every member's connection looking like every other one's.
func TestATokenForAMemberNamesThatMember(t *testing.T) {
	key, secret := mustKey(t), mustSecret(t)
	service, _ := enforcing(t)

	status, body := call(t, service, "POST", "/tokens",
		`{"groupKey":"`+key.String()+`","memberSecret":"`+secret.String()+`"}`)
	if status != 200 {
		t.Fatalf("a member's token was refused with %d: %v", status, body)
	}

	claimed := claims(t, body["relayAccessToken"].(string))
	if want := `"sub":"` + key.MemberID(secret) + `"`; !strings.Contains(claimed, want) {
		t.Errorf("the token claims %s, and names no member", claimed)
	}
	// The grant is the group's, membership deciding who connects and never what they may reach.
	if !strings.Contains(claimed, key.Prefix()) {
		t.Errorf("a member's token grants %s, which is not their group's prefix", claimed)
	}
}

// A member belongs to a group,
// so naming one without the group key that derives it is a request this service cannot answer
// rather than one it answers for the public prefix.
func TestAMemberSecretNamedWithoutAGroupKeyIsRefused(t *testing.T) {
	service, _ := enforcing(t)

	status, body := call(t, service, "POST", "/tokens", `{"memberSecret":"`+mustSecret(t).String()+`"}`)
	if status != 400 {
		t.Fatalf("a member with no group was answered %d: %v", status, body)
	}
}

// A group whose members state their presence is one
// where a token naming no member is a subject membership matches nobody to,
// so it is refused rather than issued and closed a moment later.
func TestAGroupThatStatesItsMembersRefusesATokenNamingNone(t *testing.T) {
	key, secret := mustKey(t), mustSecret(t)
	service, _ := enforcing(t)

	if status, body := call(t, service, "PUT", "/members", presence(key, secret, "Björn")); status != 200 {
		t.Fatalf("stating presence answered %d: %v", status, body)
	}

	status, body := call(t, service, "POST", "/tokens", `{"groupKey":"`+key.String()+`"}`)
	if status != 400 {
		t.Fatalf("a token naming no member was answered %d: %v", status, body)
	}
	if body["error"] != "this group states its members, and this request names none" {
		t.Errorf("the refusal reads %q", body["error"])
	}
}

// The first app in an empty group has nobody to be a member beside,
// so it buys a token under the group's own id and states its presence with it.
func TestAGroupWithNoLiveMembersBuysATokenForItself(t *testing.T) {
	key := mustKey(t)
	service, _ := enforcing(t)

	status, body := call(t, service, "POST", "/tokens", `{"groupKey":"`+key.String()+`"}`)
	if status != 200 {
		t.Fatalf("a token in an empty group was answered %d: %v", status, body)
	}
	if want := `"sub":"` + key.ID() + `"`; !strings.Contains(claims(t, body["relayAccessToken"].(string)), want) {
		t.Errorf("the token claims %s, and names neither a member nor the group", claims(t, body["relayAccessToken"].(string)))
	}
}

// Stating presence is claim and refresh at once, and the answer is the whole group:
// who this member is, how long it holds, and everybody else beside it.
func TestStatingPresenceAnswersTheGroup(t *testing.T) {
	key, secret := mustKey(t), mustSecret(t)
	service, _ := enforcing(t)

	status, body := call(t, service, "PUT", "/members", presence(key, secret, "Björn"))
	if status != 200 {
		t.Fatalf("stating presence answered %d: %v", status, body)
	}
	if body["memberId"] != key.MemberID(secret) {
		t.Errorf("the answer names %v, where the secret derives %s", body["memberId"], key.MemberID(secret))
	}
	if body["displayName"] != "Björn" {
		t.Errorf("the answer holds the name %v", body["displayName"])
	}
	if body["leaseSeconds"] != float64(membership.Lease.Seconds()) {
		t.Errorf("the answer states a lease of %v seconds", body["leaseSeconds"])
	}

	members, _ := body["members"].([]any)
	if len(members) != 1 {
		t.Fatalf("the answer carries %v", body["members"])
	}
	first, _ := members[0].(map[string]any)
	if first["memberId"] != body["memberId"] || first["displayName"] != "Björn" || first["publishing"] != false {
		t.Errorf("a member came back as %v", first)
	}
}

// A name another member holds is refused with the status a caller can tell from a malformed request,
// so an app can ask its user for another name rather than reporting a broken service.
func TestANameAnotherMemberHoldsIsRefused(t *testing.T) {
	key := mustKey(t)
	service, _ := enforcing(t)

	if status, body := call(t, service, "PUT", "/members", presence(key, mustSecret(t), "Björn")); status != 200 {
		t.Fatalf("stating presence answered %d: %v", status, body)
	}

	status, body := call(t, service, "PUT", "/members", presence(key, mustSecret(t), "Björn"))
	if status != 409 {
		t.Fatalf("a name another member holds was answered %d: %v", status, body)
	}
	if body["error"] != "that name is taken in this group" {
		t.Errorf("the refusal reads %q", body["error"])
	}
}

// Stating presence reconciles before it answers,
// so a lease that lapsed has its connections closed by the call that noticed.
func TestStatingPresenceClosesWhatNoMemberHolds(t *testing.T) {
	key, secret := mustKey(t), mustSecret(t)
	service, relayed := enforcing(t)
	relayed.live = []relay.Session{
		{Segment: "srtconns", ID: "stranger", Path: key.Prefix() + "desk", User: "whoever", State: "read"},
		{Segment: "srtconns", ID: "mine", Path: key.Prefix() + "desk", User: key.MemberID(secret), State: "read"},
	}

	if status, body := call(t, service, "PUT", "/members", presence(key, secret, "Björn")); status != 200 {
		t.Fatalf("stating presence answered %d: %v", status, body)
	}
	if !slices.Equal(relayed.kicked, []string{"stranger"}) {
		t.Errorf("stating presence closed %v", relayed.kicked)
	}
}

// Releasing is idempotent,
// so it answers whether there was a lease rather than refusing the second call.
func TestReleasingAMemberAnswersWhetherItHeldALease(t *testing.T) {
	key, secret := mustKey(t), mustSecret(t)
	service, _ := enforcing(t)

	released := `{"groupKey":"` + key.String() + `","memberSecret":"` + secret.String() + `"}`
	status, body := call(t, service, "DELETE", "/members", released)
	if status != 200 {
		t.Fatalf("releasing a member who holds no lease answered %d: %v", status, body)
	}
	if body["released"] != false || body["memberId"] != key.MemberID(secret) {
		t.Errorf("releasing a member who holds no lease answered %v", body)
	}

	if status, body := call(t, service, "PUT", "/members", presence(key, secret, "Björn")); status != 200 {
		t.Fatalf("stating presence answered %d: %v", status, body)
	}
	if _, body := call(t, service, "DELETE", "/members", released); body["released"] != true {
		t.Errorf("releasing a member who held a lease answered %v", body)
	}
}

// The view is how an app reads the group without stating anything,
// which is what a member joining asks before it picks a name.
func TestTheMembersViewStatesNothing(t *testing.T) {
	key, secret := mustKey(t), mustSecret(t)
	service, relayed := enforcing(t,
		relay.Session{Segment: "rtspsessions", ID: "mine", Path: key.Prefix() + "desk",
			User: key.MemberID(secret), State: "publish"},
	)

	status, body := call(t, service, "GET", "/members?groupKey="+url.QueryEscape(key.String()), "")
	if status != 200 {
		t.Fatalf("the view answered %d: %v", status, body)
	}
	if members, _ := body["members"].([]any); len(members) != 0 {
		t.Fatalf("a group nobody stated presence in carries %v", body["members"])
	}

	if status, body := call(t, service, "PUT", "/members", presence(key, secret, "Björn")); status != 200 {
		t.Fatalf("stating presence answered %d: %v", status, body)
	}

	_, body = call(t, service, "GET", "/members?groupKey="+url.QueryEscape(key.String()), "")
	members, _ := body["members"].([]any)
	if len(members) != 1 {
		t.Fatalf("the view carries %v", body["members"])
	}
	first, _ := members[0].(map[string]any)
	if first["displayName"] != "Björn" || first["publishing"] != true {
		t.Errorf("a publishing member came back as %v", first)
	}
	if len(relayed.kicked) != 0 {
		t.Errorf("a view closed %v", relayed.kicked)
	}
}

// Publishing is read off the relay's connection lists,
// and a list that would not answer leaves every member on it publishing nothing,
// which is what a member sending nothing looks like.
// Both answers say which of the two they read.
func TestTheAnswersNameAConnectionListThatWouldNotAnswer(t *testing.T) {
	key, secret := mustKey(t), mustSecret(t)
	service, relayed := enforcing(t)
	relayed.unread = []relay.Unread{{Segment: "srtconns", Reason: "the relay answered 500"}}

	status, body := call(t, service, "PUT", "/members", presence(key, secret, "Björn"))
	if status != 200 {
		t.Fatalf("stating presence answered %d: %v", status, body)
	}
	if body["publishingUnread"] != true {
		t.Errorf("a statement answered %v, naming no list that would not answer", body)
	}

	_, body = call(t, service, "GET", "/members?groupKey="+url.QueryEscape(key.String()), "")
	if body["publishingUnread"] != true {
		t.Errorf("a view answered %v, naming no list that would not answer", body)
	}
}

// Where every list answered, publishing false is a member sending nothing, and the answer says so.
func TestAnAnswerOffAWholeReadNamesNoUnreadList(t *testing.T) {
	key, secret := mustKey(t), mustSecret(t)
	service, _ := enforcing(t)

	_, body := call(t, service, "PUT", "/members", presence(key, secret, "Björn"))
	if body["publishingUnread"] != false {
		t.Errorf("a statement off a whole read answered %v", body)
	}
}

// The relay reports a read as it starts,
// which makes a reconnect on an unexpired token close again instead of being served.
func TestReconcileTakesTheRelaysOwnPath(t *testing.T) {
	key, secret := mustKey(t), mustSecret(t)
	service, relayed := enforcing(t)

	if status, body := call(t, service, "PUT", "/members", presence(key, secret, "Björn")); status != 200 {
		t.Fatalf("stating presence answered %d: %v", status, body)
	}

	relayed.live = []relay.Session{
		{Segment: "hlssessions", ID: "back-again", Path: key.Prefix() + "desk", User: "whoever", State: "read"},
	}
	// One look at the relay per group per membership.SweepWindow,
	// and the statement above took this group's.
	// The reconnect is read off the next one.
	time.Sleep(membership.SweepWindow)

	status, body := call(t, service, "POST", "/reconcile", `{"path":"`+key.Prefix()+`desk"}`)
	if status != 200 {
		t.Fatalf("a reconcile answered %d: %v", status, body)
	}
	if !slices.Equal(relayed.kicked, []string{"back-again"}) {
		t.Errorf("a reconnect by a non-member was left alone: closed %v", relayed.kicked)
	}
}

// A path belonging to no group names no group's members,
// and answering one would enforce against a stream name.
func TestReconcileOnAPathOutsideAnyGroupIsRefused(t *testing.T) {
	service, _ := enforcing(t)

	if status, _ := call(t, service, "POST", "/reconcile", `{"path":"desk"}`); status != 400 {
		t.Errorf("a path outside any group was answered %d", status)
	}
}

// A group nobody stated presence in is not enforced,
// so the hook firing for one is a no-op rather than a removal of everybody on it.
func TestReconcileOnAGroupWithNoLiveMembersClosesNothing(t *testing.T) {
	key := mustKey(t)
	service, relayed := enforcing(t,
		relay.Session{Segment: "srtconns", ID: "stranger", Path: key.Prefix() + "desk", User: "whoever", State: "read"},
	)

	status, body := call(t, service, "POST", "/reconcile", `{"path":"`+key.Prefix()+`desk"}`)
	if status != 200 {
		t.Fatalf("a reconcile answered %d: %v", status, body)
	}
	if body["enforced"] != false {
		t.Errorf("a group with no live members answered enforced=%v", body["enforced"])
	}
	if len(relayed.kicked) != 0 {
		t.Errorf("a group with no live members closed %v", relayed.kicked)
	}
}

// Every route refuses a body it cannot read rather than acting on half of one.
func TestARequestBodyTheServiceCannotReadIsRefused(t *testing.T) {
	service, relayed := enforcing(t)

	for _, request := range []struct {
		method, target string
	}{
		{"PUT", "/members"},
		{"DELETE", "/members"},
		{"POST", "/reconcile"},
	} {
		status, body := call(t, service, request.method, request.target, `{"groupKey":`)
		if status != 400 {
			t.Errorf("%s %s took a body that does not read as JSON: %d %v", request.method, request.target, status, body)
		}
	}
	if len(relayed.kicked) != 0 {
		t.Errorf("a request nobody could read closed %v", relayed.kicked)
	}
}

// A group key of the wrong length is one this service did not make,
// and a prefix derived from it would enforce against a group nobody is in.
func TestAMembersGroupKeyTheServiceCannotReadIsRefused(t *testing.T) {
	service, _ := enforcing(t)
	secret := mustSecret(t).String()

	for _, request := range []struct{ method, target, body string }{
		{"PUT", "/members", `{"groupKey":"not-a-key","memberSecret":"` + secret + `","displayName":"Björn"}`},
		{"DELETE", "/members", `{"groupKey":"not-a-key","memberSecret":"` + secret + `"}`},
	} {
		if status, _ := call(t, service, request.method, request.target, request.body); status != 400 {
			t.Errorf("%s %s took a group key it cannot read: %d", request.method, request.target, status)
		}
	}
	if status, _ := call(t, service, "GET", "/members?groupKey=not-a-key", ""); status != 400 {
		t.Error("the view took a group key it cannot read")
	}
}

// A member is named by the secret only that member holds,
// so a request carrying none or one of the wrong length names nobody.
func TestAMemberSecretTheServiceCannotReadIsRefused(t *testing.T) {
	key := mustKey(t)
	service, _ := enforcing(t)

	for _, body := range []string{
		`{"groupKey":"` + key.String() + `","displayName":"Björn"}`,
		`{"groupKey":"` + key.String() + `","memberSecret":"c2hvcnQ=","displayName":"Björn"}`,
	} {
		if status, _ := call(t, service, "PUT", "/members", body); status != 400 {
			t.Errorf("%s stated a presence", body)
		}
	}
	if status, _ := call(t, service, "DELETE", "/members", `{"groupKey":"`+key.String()+`"}`); status != 400 {
		t.Error("a release naming no member secret was answered")
	}
}

// A member states a name to be known by in the group, so a claim carrying none is refused.
func TestAPresenceWithNoDisplayNameIsRefused(t *testing.T) {
	key := mustKey(t)
	service, _ := enforcing(t)

	if status, _ := call(t, service, "PUT", "/members", presence(key, mustSecret(t), "   ")); status != 400 {
		t.Error("a member with no display name stated a presence")
	}
}

// Membership names a group, and a request carrying no group key names none.
func TestAMembersRequestWithoutAGroupKeyIsRefused(t *testing.T) {
	service, _ := enforcing(t)

	if status, _ := call(t, service, "PUT", "/members",
		`{"memberSecret":"`+mustSecret(t).String()+`","displayName":"Björn"}`); status != 400 {
		t.Error("a presence with no group key was taken")
	}
	if status, _ := call(t, service, "GET", "/members", ""); status != 400 {
		t.Error("a view with no group key was answered")
	}
}

// Streams under the public prefix are watchable by anybody,
// so a run there is refused with the reason
// rather than answered as a group nobody stated presence in.
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

// The answers name a stream inside the group and a leg in this app's vocabulary,
// which is what the index answers under and what every other surface of this app says.
// The relay's session id and the address a connection came from are an operator's view of the relay,
// and a group key is a way into the group rather than an operator's credential.
func TestTheAnswersCarryNoRelayOperationalState(t *testing.T) {
	key, secret := mustKey(t), mustSecret(t)
	service, _ := enforcing(t,
		relay.Session{Segment: "srtconns", Transport: "srt", ID: "session-id-here",
			Path: key.Prefix() + "desk", User: "whoever", State: "read",
			RemoteAddr: "203.0.113.7:5000"},
	)

	for _, answered := range []string{
		bodyOf(t, service, "PUT", "/members", presence(key, secret, "Björn")),
		bodyOf(t, service, "GET", "/members?groupKey="+url.QueryEscape(key.String()), ""),
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

// A route answers the methods it names and nothing else,
// so a caller reaching for one it does not serve is told rather than falling through to another.
func TestARouteAnswersTheMethodsItNames(t *testing.T) {
	service, _ := enforcing(t)

	for _, request := range []struct{ method, target string }{
		{"POST", "/members"},
		{"PUT", "/reconcile"},
		{"GET", "/reconcile"},
		{"DELETE", "/tokens"},
	} {
		if status, _ := raw(t, service, request.method, request.target, `{}`); status != 405 {
			t.Errorf("%s %s answered %d, where the route names no such method", request.method, request.target, status)
		}
	}
}

// raw makes one request and returns its status and the body as it was written,
// for the answers that are not JSON and for the ones read for what they do not carry.
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

// The index derives a prefix from the group key it is given,
// so one it cannot read names no group to answer for.
func TestAnIndexGroupKeyTheServiceCannotReadIsRefused(t *testing.T) {
	service, _ := enforcing(t)

	if status, _ := call(t, service, "GET", "/streams?groupKey=not-a-key", ""); status != 400 {
		t.Error("the index took a group key it cannot read")
	}
}
