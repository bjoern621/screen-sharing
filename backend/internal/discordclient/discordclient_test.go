package discordclient

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"bjoernblessin.de/screenshare/internal/groupclient"
)

// manager answers as discordd would for one linked install in one channel.
func manager(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			LinkSecret string `json:"linkSecret"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.LinkSecret != "known" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "this link secret names no linked Discord account"})
			return
		}
		switch r.URL.Path {
		case "/presence":
			json.NewEncoder(w).Encode(map[string]any{
				"channel": map[string]string{"guild": "Guild", "name": "General"},
				"group": map[string]any{
					"prefix": "PFX/", "srtPassphrase": "pass", "memberId": "m1",
					"displayName": "Bob", "leaseSeconds": 20,
					"members": []map[string]any{{"memberId": "m1", "displayName": "Bob", "publishing": true}},
					"streams": []map[string]any{{"name": "bob/monitor-0", "ready": true}},
				},
			})
		case "/tokens":
			json.NewEncoder(w).Encode(map[string]string{"relayAccessToken": "tok", "prefix": "PFX/"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestPresenceReadsTheWholeAnswer(t *testing.T) {
	server := manager(t)
	c := New()

	answer, err := c.Presence(server.URL, "known")
	if err != nil {
		t.Fatalf("stating presence: %v", err)
	}
	if answer.Channel == nil || answer.Channel.Name != "General" {
		t.Fatalf("the answer names the channel, got %+v", answer.Channel)
	}
	g := answer.Group
	if g == nil || g.Prefix != "PFX/" || g.SrtPassphrase != "pass" || g.DisplayName != "Bob" {
		t.Fatalf("the answer carries the brokered facts, got %+v", g)
	}
	if len(g.Members) != 1 || !g.Members[0].Publishing || len(g.Streams) != 1 {
		t.Fatalf("members and streams cross whole, got %+v", g)
	}
}

func TestAnUnlinkedSecretComesBackAsARefusal(t *testing.T) {
	server := manager(t)
	c := New()

	_, err := c.Presence(server.URL, "stranger")
	var refusal *groupclient.Refusal
	if !errors.As(err, &refusal) || refusal.Status != http.StatusUnauthorized {
		t.Fatalf("the manager's own sentence travels with its status, got %v", err)
	}
}

func TestTokenTrades(t *testing.T) {
	server := manager(t)
	c := New()

	token, prefix, err := c.Token(server.URL, "known")
	if err != nil {
		t.Fatalf("trading: %v", err)
	}
	if token != "tok" || prefix != "PFX/" {
		t.Fatalf("the trade answers token and prefix, got %q %q", token, prefix)
	}
}

func TestNoManagerNamedIsAnError(t *testing.T) {
	c := New()

	if _, err := c.Presence("", "known"); err == nil {
		t.Fatal("no address is no manager to ask")
	}
	if _, _, err := c.Token("", "known"); err == nil {
		t.Fatal("no address is no manager to trade at")
	}
}
