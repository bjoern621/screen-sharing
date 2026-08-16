package groupsvc

import (
	"net/http"
	"testing"

	"bjoernblessin.de/screenshare/internal/group"
)

// What a scrape counts of this service is the one thing it hands out.

func TestATokenIssuedIsCounted(t *testing.T) {
	s := service(t)

	if status, _ := call(t, s, "POST", "/tokens", "{}"); status != http.StatusOK {
		t.Fatalf("issuing a token for the public prefix answered %d", status)
	}

	tallies := s.Tallies()
	if tallies.TokensIssued != 1 {
		t.Errorf("one token was issued, and the tally counts %d", tallies.TokensIssued)
	}
	if tallies.TokensRefused != 0 {
		t.Errorf("nothing was refused, and the tally counts %d", tallies.TokensRefused)
	}
}

// A refusal is the reading an operator came for: a run of them is somebody holding a key this
// deployment does not sign for.
func TestATokenRefusedIsCounted(t *testing.T) {
	s := service(t)

	if status, _ := call(t, s, "POST", "/tokens", `{"groupKey":"not a key"}`); status != http.StatusBadRequest {
		t.Fatalf("a malformed group key answered %d", status)
	}

	tallies := s.Tallies()
	if tallies.TokensRefused != 1 {
		t.Errorf("one request was refused, and the tally counts %d", tallies.TokensRefused)
	}
	if tallies.TokensIssued != 0 {
		t.Errorf("nothing was issued, and the tally counts %d", tallies.TokensIssued)
	}
}

// Only the token route is counted here. A group that could not be created is its own refusal and is
// not a token anybody was refused.
func TestCreatingAGroupIsNotCountedAsAToken(t *testing.T) {
	s := service(t)

	if status, _ := call(t, s, "POST", "/groups", ""); status != http.StatusOK {
		t.Fatalf("creating a group answered %d", status)
	}

	if got := s.Tallies().TokensIssued; got != 0 {
		t.Errorf("no token was issued, and the tally counts %d", got)
	}
}

func TestATokenOnAGroupKeyIsCounted(t *testing.T) {
	s := service(t)
	groupKey, err := group.NewKey()
	if err != nil {
		t.Fatalf("drawing a group key: %v", err)
	}

	if status, _ := call(t, s, "POST", "/tokens", `{"groupKey":"`+groupKey.String()+`"}`); status != http.StatusOK {
		t.Fatalf("issuing a token on a group key answered %d", status)
	}

	if got := s.Tallies().TokensIssued; got != 1 {
		t.Errorf("one token was issued, and the tally counts %d", got)
	}
}
