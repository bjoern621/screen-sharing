package discordapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/channelgroup"
	"bjoernblessin.de/screenshare/internal/discordoauth"
)

// fakeBroker answers one linked install standing in one channel.
type fakeBroker struct{}

func (fakeBroker) Presence(linkSecret string) (channelgroup.Answer, error) {
	if linkSecret != "known" {
		return channelgroup.Answer{}, channelgroup.ErrUnlinked
	}
	return channelgroup.Answer{
		Channel: &channelgroup.Channel{Guild: "Guild", Name: "General"},
		Group:   &channelgroup.Group{Prefix: "PFX/", SrtPassphrase: "pass", MemberID: "m1", DisplayName: "Bob", LeaseSeconds: 20},
	}, nil
}

func (fakeBroker) Token(linkSecret string) (string, string, error) {
	switch linkSecret {
	case "known":
		return "tok", "PFX/", nil
	case "adrift":
		return "", "", channelgroup.ErrNoChannel
	}
	return "", "", channelgroup.ErrUnlinked
}

type fakeLinks struct {
	drawnFor string
	// secret is what a draw answers, "drawn-secret" for the zero value.
	secret string
}

func (f *fakeLinks) Draw(userID string) (string, error) {
	f.drawnFor = userID
	if f.secret != "" {
		return f.secret, nil
	}
	return "drawn-secret", nil
}

type fakeOAuth struct{ identified string }

func (fakeOAuth) AuthorizeURL(state string) string {
	return "https://discord.example/authorize?state=" + url.QueryEscape(state)
}

func (fakeOAuth) Application() string { return "fixture-application" }

func (f *fakeOAuth) Identify(code string) (discordoauth.Identity, error) {
	f.identified = code
	if code != "good-code" {
		return discordoauth.Identity{}, errors.New("Discord refused the code trade")
	}
	return discordoauth.Identity{UserID: "u1", Username: "bob"}, nil
}

func serve(t *testing.T, links *fakeLinks) (*httptest.Server, *fakeLinks, *fakeOAuth) {
	t.Helper()
	if links == nil {
		links = &fakeLinks{}
	}
	oauth := &fakeOAuth{}
	service := New(fakeBroker{}, links, oauth)
	server := httptest.NewServer(service.Handler("test"))
	t.Cleanup(server.Close)
	return server, links, oauth
}

func put(t *testing.T, address, body string) *http.Response {
	t.Helper()
	request, _ := http.NewRequest(http.MethodPut, address, strings.NewReader(body))
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("calling %s: %v", address, err)
	}
	return resp
}

func TestPresenceAnswersTheGroup(t *testing.T) {
	server, _, _ := serve(t, nil)

	resp := put(t, server.URL+"/presence", `{"linkSecret":"known"}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("a known secret is answered, got %s", resp.Status)
	}
	var answer struct {
		Channel *struct{ Guild, Name string } `json:"channel"`
		Group   *struct {
			Prefix        string `json:"prefix"`
			SrtPassphrase string `json:"srtPassphrase"`
			MemberID      string `json:"memberId"`
		} `json:"group"`
	}
	json.NewDecoder(resp.Body).Decode(&answer)
	if answer.Channel == nil || answer.Channel.Name != "General" {
		t.Fatalf("the answer names the channel, got %+v", answer.Channel)
	}
	if answer.Group == nil || answer.Group.Prefix != "PFX/" || answer.Group.SrtPassphrase != "pass" {
		t.Fatalf("the answer carries the derived facts, got %+v", answer.Group)
	}
}

func TestPresenceForAnUnknownSecretIs401(t *testing.T) {
	server, _, _ := serve(t, nil)

	resp := put(t, server.URL+"/presence", `{"linkSecret":"stranger"}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("an unlinked secret is 401, got %s", resp.Status)
	}
}

// post is one POST with its transport error checked.
func post(t *testing.T, address, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(address, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("calling %s: %v", address, err)
	}
	return resp
}

func TestTokensTradeAndRefusals(t *testing.T) {
	server, _, _ := serve(t, nil)

	resp := post(t, server.URL+"/tokens", `{"linkSecret":"known"}`)
	defer resp.Body.Close()
	var answer struct {
		Token  string `json:"relayAccessToken"`
		Prefix string `json:"prefix"`
	}
	json.NewDecoder(resp.Body).Decode(&answer)
	if answer.Token != "tok" || answer.Prefix != "PFX/" {
		t.Fatalf("the trade answers token and prefix, got %+v", answer)
	}

	adrift := post(t, server.URL+"/tokens", `{"linkSecret":"adrift"}`)
	defer adrift.Body.Close()
	if adrift.StatusCode != http.StatusBadRequest {
		t.Fatalf("no channel is a refusal in words, got %s", adrift.Status)
	}
}

// startLink begins a link and answers the state the authorize redirect carries.
func startLink(t *testing.T, server *httptest.Server) string {
	t.Helper()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err := client.Get(server.URL + "/link?port=8123")
	if err != nil {
		t.Fatalf("starting a link: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("a link start redirects to Discord, got %s", resp.Status)
	}
	location, _ := url.Parse(resp.Header.Get("Location"))
	state := location.Query().Get("state")
	if state == "" {
		t.Fatal("the authorize redirect carries a state")
	}
	return state
}

func TestLinkFlowLandsTheSecretOnLoopback(t *testing.T) {
	server, links, oauth := serve(t, nil)
	state := startLink(t, server)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(server.URL + "/link/callback?code=good-code&state=" + url.QueryEscape(state))
	if err != nil {
		t.Fatalf("finishing a link: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("a finished link redirects to the app, got %s", resp.Status)
	}
	if oauth.identified != "good-code" {
		t.Fatalf("the callback spends its code, spent %q", oauth.identified)
	}
	if links.drawnFor != "u1" {
		t.Fatalf("the link is drawn for the identified user, drawn for %q", links.drawnFor)
	}
	// The account rides along because the app names it beside the link,
	// and this trade is the one read of it (internal/app, storeDiscordLink).
	location := resp.Header.Get("Location")
	if location != "http://127.0.0.1:8123/?linkSecret=drawn-secret&account=bob" {
		t.Fatalf("the secret and the account land on the port the start named, got %s", location)
	}
}

func TestACallbackStateIsSpentOnUse(t *testing.T) {
	server, _, _ := serve(t, nil)
	state := startLink(t, server)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	first, err := client.Get(server.URL + "/link/callback?code=good-code&state=" + url.QueryEscape(state))
	if err != nil {
		t.Fatalf("spending the state: %v", err)
	}
	first.Body.Close()

	second, err := client.Get(server.URL + "/link/callback?code=good-code&state=" + url.QueryEscape(state))
	if err != nil {
		t.Fatalf("replaying the state: %v", err)
	}
	defer second.Body.Close()
	if second.StatusCode != http.StatusBadRequest {
		t.Fatalf("a spent state is refused, got %s", second.Status)
	}
}

func TestALinkStartNeedsAUsablePort(t *testing.T) {
	server, _, _ := serve(t, nil)

	resp, err := http.Get(server.URL + "/link?port=notanumber")
	if err != nil {
		t.Fatalf("starting a link without a port: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a port that is no port is refused, got %s", resp.Status)
	}
}

// The manager answers a check on a route of its own, taking no credential and touching no link:
// a relay check dials it to say whether Discord mode has a manager behind it (internal/reach).
func TestTheHealthRouteAnswersWithoutACredential(t *testing.T) {
	service := New(fakeBroker{}, &fakeLinks{}, &fakeOAuth{})

	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	service.Handler("0.9.0").ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("the health route answers %d, want %d", w.Code, http.StatusNoContent)
	}
	if got, want := w.Header().Get("Server"), "discordd/0.9.0"; got != want {
		t.Errorf("the answer names %q, want %q", got, want)
	}
}

// The secret crosses to the app in a query string, where an unescaped "+" is read as a space.
// The store draws a URL-safe alphabet (internal/linkstore), and the escape holds for whatever it draws.
func TestALinkSecretSurvivesTheRedirect(t *testing.T) {
	secret := "a+b/c="
	server, _, _ := serve(t, &fakeLinks{secret: secret})
	state := startLink(t, server)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(server.URL + "/link/callback?code=good-code&state=" + url.QueryEscape(state))
	if err != nil {
		t.Fatalf("finishing a link: %v", err)
	}
	defer resp.Body.Close()

	landed, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("the redirect is no address: %v", err)
	}
	if got := landed.Query().Get("linkSecret"); got != secret {
		t.Fatalf("the app reads the secret %q, want %q", got, secret)
	}
}
