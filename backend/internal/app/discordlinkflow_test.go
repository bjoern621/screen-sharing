package app

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"bjoernblessin.de/screenshare/internal/channelgroup"
	"bjoernblessin.de/screenshare/internal/discordapi"
	"bjoernblessin.de/screenshare/internal/discordoauth"
)

// The two halves of the link flow are written in two packages,
// so the nonce holds only where both spell it the same way.
// The manager below is the real one, and the browser is an HTTP client following its redirects.

type flowBroker struct{}

func (flowBroker) Presence(string) (channelgroup.Answer, error) { return channelgroup.Answer{}, nil }
func (flowBroker) Token(string) (string, string, error)         { return "", "", nil }

type flowLinks struct{}

func (flowLinks) Draw(string) (string, error) { return "drawn-secret", nil }

// flowOAuth stands in for Discord: consent is a redirect straight back to the callback.
type flowOAuth struct{ callback string }

func (flowOAuth) Application() string { return "an-application" }

func (o flowOAuth) AuthorizeURL(state string) string {
	return o.callback + "?code=good-code&state=" + url.QueryEscape(state)
}

func (flowOAuth) Identify(string) (discordoauth.Identity, error) {
	return discordoauth.Identity{UserID: "u1", Username: "bob"}, nil
}

func TestALinkLandsThroughTheManagerUnderItsNonce(t *testing.T) {
	oauth := &flowOAuth{}
	manager := httptest.NewServer(discordapi.New(flowBroker{}, flowLinks{}, oauth).Handler("test"))
	defer manager.Close()
	oauth.callback = manager.URL + "/link/callback"

	nonce, err := drawLinkNonce()
	if err != nil {
		t.Fatalf("drawing a nonce: %v", err)
	}

	landed := make(chan landedLink, 1)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("binding the loopback listener: %v", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	server := &http.Server{Handler: linkHandler(nonce, landed)}
	go server.Serve(listener)
	defer server.Close()

	start := manager.URL + "/link?port=" + strconv.Itoa(port) + "&nonce=" + url.QueryEscape(nonce)
	resp, err := http.Get(start)
	if err != nil {
		t.Fatalf("walking the link flow: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the flow lands on the app, got %s", resp.Status)
	}
	select {
	case link := <-landed:
		if link.secret != "drawn-secret" || link.account != "bob" {
			t.Fatalf("the flow landed %+v, want the drawn secret and the account it was drawn for", link)
		}
	default:
		t.Fatal("the flow passes its link on")
	}

	// The same port, reached by a page that guessed it.
	sprayed, err := http.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/?nonce=guessed&linkSecret=chosen-elsewhere")
	if err != nil {
		t.Fatalf("landing a link under a guessed nonce: %v", err)
	}
	defer sprayed.Body.Close()

	if sprayed.StatusCode != http.StatusForbidden {
		t.Fatalf("a guessed nonce is refused, got %s", sprayed.Status)
	}
	select {
	case link := <-landed:
		t.Fatalf("a guessed nonce landed %q", link.secret)
	default:
	}
}
