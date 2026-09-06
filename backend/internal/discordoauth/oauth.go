// Package discordoauth is the manager's side of Discord's OAuth2 code flow,
// asking the identify scope alone: which Discord account is linking.
//
// Every call reaches Discord, so every failure is an Umgebungsfehler and leaves as an error.
// No token outlives Identify: the access token is spent on one read of the account
// and dropped, the link secret being what the install holds instead (docs/discord-mode.md).
package discordoauth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
)

// authBase is where a person's browser is sent to consent.
const authBase = "https://discord.com/oauth2/authorize"

// apiBase is where the code is traded and the account read.
const apiBase = "https://discord.com/api/v10"

// timeout bounds both calls of Identify.
// A linking user is watching a browser tab, so a Discord that is not answering says so quickly.
const timeout = 10 * time.Second

// Config names the Discord application this manager links through.
type Config struct {
	ClientID     string
	ClientSecret string
	// RedirectURL is the public callback, stated identically at authorize and at the trade,
	// Discord refusing a trade whose redirect differs from the one consented to.
	RedirectURL string
	// APIBase overrides apiBase, for a test standing in for Discord.
	APIBase string
}

// Identity is the account a code resolved to.
type Identity struct {
	UserID   string
	Username string
}

// Application is the Discord application this manager speaks for.
//
// Public, being what every authorize URL carries,
// and what an app's own Discord client is handed to draw an activity under
// (internal/discordrpc).
func (c Config) Application() string {
	assert.Assert(c.ClientID != "", "a manager names the application it links through")
	return c.ClientID
}

// AuthorizeURL is where a linking browser is sent,
// state riding along and coming back on the callback untouched.
func (c Config) AuthorizeURL(state string) string {
	assert.Assert(c.ClientID != "", "an authorize URL names the app consent is asked for")
	assert.Assert(c.RedirectURL != "", "an authorize URL names where consent lands")
	assert.Assert(state != "", "an authorize URL carries the state that ties the callback to its start")

	q := url.Values{
		"client_id":     {c.ClientID},
		"response_type": {"code"},
		"scope":         {"identify"},
		"state":         {state},
		"redirect_uri":  {c.RedirectURL},
	}
	return authBase + "?" + q.Encode()
}

// Identify trades the callback's code for the account that consented.
// The access token stays inside this call.
func (c Config) Identify(code string) (Identity, error) {
	assert.Assert(c.ClientID != "" && c.ClientSecret != "", "a trade authenticates the app it is for")
	assert.Assert(code != "", "a trade spends the code the callback carried")

	base := c.APIBase
	if base == "" {
		base = apiBase
	}
	client := &http.Client{Timeout: timeout}

	form := url.Values{
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {c.RedirectURL},
	}
	resp, err := client.Post(base+"/oauth2/token", "application/x-www-form-urlencoded",
		strings.NewReader(form.Encode()))
	if err != nil {
		return Identity{}, fmt.Errorf("Discord's token endpoint cannot be reached: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return Identity{}, fmt.Errorf("Discord refused the code trade with %s", resp.Status)
	}
	var trade struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&trade); err != nil || trade.AccessToken == "" {
		return Identity{}, fmt.Errorf("Discord's token answer carries no access token")
	}

	request, err := http.NewRequest(http.MethodGet, base+"/users/@me", nil)
	if err != nil {
		return Identity{}, fmt.Errorf("addressing Discord's account read: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+trade.AccessToken)
	account, err := client.Do(request)
	if err != nil {
		return Identity{}, fmt.Errorf("Discord's account endpoint cannot be reached: %w", err)
	}
	defer account.Body.Close()
	if account.StatusCode >= 400 {
		return Identity{}, fmt.Errorf("Discord refused the account read with %s", account.Status)
	}
	var user struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(account.Body).Decode(&user); err != nil || user.ID == "" {
		return Identity{}, fmt.Errorf("Discord's account answer names no user")
	}

	return Identity{UserID: user.ID, Username: user.Username}, nil
}
