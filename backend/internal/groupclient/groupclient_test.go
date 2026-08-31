package groupclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The service's own answers, as internal/groupsvc spells them.
// A client reading another spelling asks a real service for fields it never answers,
// which is a token that never arrives and a membership nobody states.

// service records what each request carried and answers what the route it names answers.
type service struct {
	body   map[string]map[string]any
	query  map[string]string
	answer map[string]any
	status int
}

func started(t *testing.T, s *service) (*Client, string) {
	t.Helper()
	s.body, s.query = map[string]map[string]any{}, map[string]string{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := r.Method + " " + r.URL.Path
		s.query[route] = r.URL.RawQuery
		if r.Body != nil {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			s.body[route] = body
		}
		if s.status >= 400 {
			w.WriteHeader(s.status)
		}
		json.NewEncoder(w).Encode(s.answer)
	}))
	t.Cleanup(srv.Close)
	return New(), srv.URL
}

// The names on the wire are the service's, and every one names what it carries:
// the group's key, the member's secret, the relay's access token.
func TestTheWireCarriesTheNamesTheServiceReads(t *testing.T) {
	s := &service{answer: map[string]any{
		"relayAccessToken": "a.b.c",
		"prefix":           "abc/",
		"expires":          "2999-01-01T00:00:00Z",
	}}
	c, base := started(t, s)

	token, err := c.Token(base, "a-group-key", "a-member-secret")
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if token != "a.b.c" {
		t.Errorf("Token = %q, want the relay access token the service signed", token)
	}
	if got := s.body["POST /tokens"]; got["groupKey"] != "a-group-key" || got["memberSecret"] != "a-member-secret" {
		t.Errorf("the token request carried %v, want groupKey and memberSecret", got)
	}
}

// The index takes its group in the query, a GET having no body a cache or a proxy honours.
func TestTheIndexNamesTheGroupInTheQuery(t *testing.T) {
	s := &service{answer: map[string]any{
		"prefix":  "abc/",
		"streams": []map[string]any{{"name": "standup", "ready": true, "format": "h264"}},
	}}
	c, base := started(t, s)

	streams, err := c.Streams(base, "a-group-key")
	if err != nil {
		t.Fatalf("Streams: %v", err)
	}
	if len(streams) != 1 || streams[0].Name != "standup" || !streams[0].Ready {
		t.Errorf("Streams = %+v, want the one stream the index answered", streams)
	}
	if got := s.query["GET /streams"]; got != "groupKey=a-group-key" {
		t.Errorf("the index was asked %q, want the group key under groupKey", got)
	}
}

func TestCreatingAGroupReadsBothHalvesOfTheAnswer(t *testing.T) {
	s := &service{answer: map[string]any{"groupKey": "a-group-key", "groupId": "an-id"}}
	c, base := started(t, s)

	key, id, err := c.CreateGroup(base)
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if key != "a-group-key" || id != "an-id" {
		t.Errorf("CreateGroup = %q, %q, want the drawn key and the id it derives", key, id)
	}
}

// Stating presence is a claim and a refresh at once, and the answer is the whole group.
func TestStatingPresenceAnswersTheGroup(t *testing.T) {
	s := &service{answer: map[string]any{
		"memberId":         "me",
		"displayName":      "Björn",
		"leaseSeconds":     20,
		"publishingUnread": true,
		"members": []map[string]any{
			{"memberId": "me", "displayName": "Björn", "publishing": true},
			{"memberId": "other", "displayName": "Ada"},
		},
	}}
	c, base := started(t, s)

	held, err := c.State(base, "a-group-key", "a-member-secret", "Björn")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if held.MemberID != "me" || held.DisplayName != "Björn" || held.LeaseSeconds != 20 {
		t.Errorf("State = %+v, want the member the service stated and its lease", held)
	}
	if len(held.Members) != 2 || !held.Members[0].Publishing || held.Members[1].DisplayName != "Ada" {
		t.Errorf("the group is %+v, want both members and what each is doing", held.Members)
	}
	// Publishing false under a list that would not answer is not a member sending nothing,
	// and the client carries the difference rather than flattening it.
	if !held.PublishingUnread {
		t.Errorf("State = %+v, and drops the list the service could not read", held)
	}
	if got := s.body["PUT /members"]; got["displayName"] != "Björn" || got["memberSecret"] != "a-member-secret" {
		t.Errorf("the presence request carried %v, want the secret and the display name", got)
	}
}

// Releasing a member who holds no lease is the state the call names, so it succeeds.
func TestReleasingAMemberWithNoLeaseSucceeds(t *testing.T) {
	s := &service{answer: map[string]any{"memberId": "me", "released": false}}
	c, base := started(t, s)

	if err := c.Release(base, "a-group-key", "a-member-secret"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if got := s.body["DELETE /members"]; got["groupKey"] != "a-group-key" {
		t.Errorf("the release request carried %v, want the group key and the member secret", got)
	}
}

// A refusal carries the service's own sentence and its status,
// so a caller can tell a name another member holds from a service that is not answering,
// and show the reason either way.
func TestARefusalCarriesTheServicesOwnAnswer(t *testing.T) {
	s := &service{status: http.StatusConflict, answer: map[string]any{"error": "that name is taken in this group"}}
	c, base := started(t, s)

	_, err := c.State(base, "a-group-key", "a-member-secret", "Björn")
	if err == nil {
		t.Fatal("a refused claim came back as a membership")
	}
	if !NameTaken(err) {
		t.Errorf("the refusal %v is not read as a name another member holds", err)
	}
	if !strings.Contains(err.Error(), "that name is taken in this group") {
		t.Errorf("the refusal reads %q, want the service's own sentence", err)
	}

	// Every other refusal is a refusal and not a name that is taken.
	s.status, s.answer = http.StatusBadRequest, map[string]any{"error": "a group is named by its group key"}
	if _, err := c.State(base, "", "a-member-secret", "Björn"); err == nil || NameTaken(err) {
		t.Errorf("a malformed request reads as a taken name: %v", err)
	}

	// So is a service that is not there, which is not a refusal at all.
	if NameTaken(c.Release("http://127.0.0.1:1/nothing", "a-group-key", "a-member-secret")) {
		t.Error("an unreachable service reads as a taken name")
	}
}

// The held token is minted from a base, a group key and a member secret together,
// so a change to any of them mints again
// rather than reusing a credential for a member or a group this app has left.
func TestAHeldTokenBelongsToWhatMintedIt(t *testing.T) {
	s := &service{answer: map[string]any{
		"relayAccessToken": "a.b.c",
		"expires":          "2999-01-01T00:00:00Z",
	}}
	c, base := started(t, s)

	if _, err := c.Token(base, "a-group-key", "a-member-secret"); err != nil {
		t.Fatalf("Token: %v", err)
	}
	s.answer["relayAccessToken"] = "second"

	if token, _ := c.Token(base, "a-group-key", "a-member-secret"); token != "a.b.c" {
		t.Errorf("a second call for the same group minted %q, want the held token", token)
	}
	if token, _ := c.Token(base, "a-group-key", "another-member-secret"); token != "second" {
		t.Errorf("another member reused %q, want a token minted for it", token)
	}
	if token, _ := c.Token(base, "another-group-key", "a-member-secret"); token != "second" {
		t.Errorf("another group reused %q, want a token minted for it", token)
	}

	// Forget is what a caller whose connection the relay refused calls, and the next call mints.
	s.answer["relayAccessToken"] = "third"
	c.Forget()
	if token, _ := c.Token(base, "another-group-key", "a-member-secret"); token != "third" {
		t.Errorf("a forgotten token came back as %q, want one minted again", token)
	}
}

// A group key is what lets anybody join the group, and every error text is selectable,
// so the key never reaches a sentence a reader can copy out of the app.
func TestNoSecretReachesAnErrorText(t *testing.T) {
	c := New()
	const key = "a-group-key"

	_, err := c.Streams("http://127.0.0.1:1/nothing", key)
	if err == nil {
		t.Fatal("an unreachable service answered an index")
	}
	if strings.Contains(err.Error(), key) {
		t.Errorf("the failure reads %q, which carries the group key out of the app", err)
	}
}
