package discordoauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAuthorizeURLCarriesTheContract(t *testing.T) {
	c := Config{ClientID: "app123", RedirectURL: "https://relay/discord/link/callback"}

	raw := c.AuthorizeURL("state-1")

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("an authorize URL parses: %v", err)
	}
	q := parsed.Query()
	if q.Get("client_id") != "app123" || q.Get("response_type") != "code" {
		t.Fatalf("the URL names the app and asks for a code, got %s", raw)
	}
	if q.Get("scope") != "identify" || q.Get("state") != "state-1" {
		t.Fatalf("the URL asks identify under the caller's state, got %s", raw)
	}
	if q.Get("redirect_uri") != "https://relay/discord/link/callback" {
		t.Fatalf("the URL names the callback, got %s", raw)
	}
}

func TestIdentifyTradesTheCodeAndReadsTheUser(t *testing.T) {
	var tokenForm url.Values
	discord := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/token":
			r.ParseForm()
			tokenForm = r.PostForm
			json.NewEncoder(w).Encode(map[string]string{"access_token": "at-1", "token_type": "Bearer"})
		case "/users/@me":
			if r.Header.Get("Authorization") != "Bearer at-1" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"id": "u1", "username": "bob"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer discord.Close()

	c := Config{
		ClientID: "app123", ClientSecret: "shh",
		RedirectURL: "https://relay/cb", APIBase: discord.URL,
	}

	identity, err := c.Identify("code-1")
	if err != nil {
		t.Fatalf("identifying: %v", err)
	}
	if identity.UserID != "u1" || identity.Username != "bob" {
		t.Fatalf("the identity is the user Discord answered, got %+v", identity)
	}
	if tokenForm.Get("code") != "code-1" || tokenForm.Get("grant_type") != "authorization_code" {
		t.Fatalf("the trade names the code, got %v", tokenForm)
	}
	if tokenForm.Get("client_secret") != "shh" || tokenForm.Get("redirect_uri") != "https://relay/cb" {
		t.Fatalf("the trade authenticates the app and repeats the callback, got %v", tokenForm)
	}
}

func TestIdentifyRefusalLeavesAsAnError(t *testing.T) {
	discord := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer discord.Close()

	c := Config{ClientID: "app123", ClientSecret: "shh", RedirectURL: "https://relay/cb", APIBase: discord.URL}

	if _, err := c.Identify("spent-code"); err == nil || !strings.Contains(err.Error(), "Discord") {
		t.Fatalf("a refused trade says Discord refused it, got %v", err)
	}
}
