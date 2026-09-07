package discordapi

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// get is one GET with its transport error checked.
func get(t *testing.T, address string) *http.Response {
	t.Helper()
	resp, err := http.Get(address)
	if err != nil {
		t.Fatalf("calling %s: %v", address, err)
	}
	return resp
}

func TestAWatchPageSendsTheBrowserToTheApp(t *testing.T) {
	server, _, _ := serve(t, nil)

	resp := get(t, server.URL+"/watch/G1/bob/monitor-0")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("a watch page is served, got %s", resp.Status)
	}
	// Both halves: the browser goes by itself, and the reader presses where it does not.
	if !strings.Contains(string(body), `location.replace("mirrorme://watch/G1/bob/monitor-0")`) {
		t.Errorf("the page sends the browser to the link, carried %s", body)
	}
	if !strings.Contains(string(body), `href="mirrorme://watch/G1/bob/monitor-0"`) {
		t.Errorf("the page states the link to press, carried %s", body)
	}
}

// A member goes by the name they claimed, and a stream is listed under it,
// so a link carries what the alphabet of an address cannot: "Björn/monitor-0" (internal/group).
func TestAWatchPageCarriesANameThroughItsEscapes(t *testing.T) {
	server, _, _ := serve(t, nil)

	resp := get(t, server.URL+"/watch/G1/Bj%C3%B6rn/monitor-0")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if !strings.Contains(string(body), `href="mirrorme://watch/G1/Bj%C3%B6rn/monitor-0"`) {
		t.Errorf("the link spells the name the way the app reads it back, carried %s", body)
	}
}

func TestAWatchPageNamingNoStreamIsRefused(t *testing.T) {
	server, _, _ := serve(t, nil)

	resp := get(t, server.URL+"/watch/G1")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("a path naming no stream opens nothing, got %s", resp.Status)
	}
}

func TestAWatchPageTakesNoSecret(t *testing.T) {
	server, _, _ := serve(t, nil)

	resp := get(t, server.URL+"/watch/G1/bob/monitor-0")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// The group id is the public digest every path on the relay carries,
	// and following the link is refused at the app that holds no seat in that group.
	if strings.Contains(string(body), "linkSecret") {
		t.Errorf("the page carries no credential, carried %s", body)
	}
}
