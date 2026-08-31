package relay

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// Enforcement reads the relay's own connection lists and closes what it finds there, so these hold
// what it may believe: every list that has a kick is swept, a listener that is off is not
// a failure, and a list that refuses is named rather than passed over.

const srtConnsWithTwo = `{"itemCount":2,"pageCount":1,"items":[
	{"id":"srt-1","path":"abc/desk","user":"alice","state":"read","remoteAddr":"10.0.0.1:5000"},
	{"id":"srt-2","path":"abc/desk","user":"bob","state":"publish","remoteAddr":"10.0.0.2:5000"}
]}`

const hlsSessionsWithOne = `{"itemCount":1,"pageCount":1,"items":[
	{"id":"hls-1","path":"abc/desk","user":"carol","remoteAddr":"10.0.0.3:5000"}
]}`

// One sweep names every connection the relay is carrying, whichever protocol carries it.
// A member watching over one leg and publishing over another is one member on two lists.
func TestSessionsGathersEveryListThatTakesAKick(t *testing.T) {
	host, port := relayServing(t, map[string]string{
		"/v3/srtconns/list":    srtConnsWithTwo,
		"/v3/hlssessions/list": hlsSessionsWithOne,
	})

	live, unread := New().Sessions(host, port)
	if len(unread) != 0 {
		t.Fatalf("a relay that answered every list it serves reported unread lists: %+v", unread)
	}
	if len(live) != 3 {
		t.Fatalf("three connections were served, and the sweep found %d: %+v", len(live), live)
	}

	bySession := map[string]Session{}
	for _, session := range live {
		bySession[session.ID] = session
	}

	alice := bySession["srt-1"]
	if alice.User != "alice" || alice.Path != "abc/desk" || alice.State != "read" {
		t.Errorf("the srt reader came back as %+v", alice)
	}
	if alice.Segment != "srtconns" {
		t.Errorf("a connection names the list it came from, and this one names %q", alice.Segment)
	}
	if bySession["hls-1"].Segment != "hlssessions" {
		t.Errorf("the hls session came back as %+v", bySession["hls-1"])
	}
}

// A relay serving no MoQ answers 404 for moqsessions, a fact about that deployment.
// Reporting it as a list that could not be read would leave every sweep carrying failures nobody
// can act on.
func TestAListenerThatIsOffIsNotAnUnreadList(t *testing.T) {
	host, port := relayServing(t, map[string]string{"/v3/srtconns/list": srtConnsWithTwo})

	live, unread := New().Sessions(host, port)
	if len(unread) != 0 {
		t.Fatalf("switched-off listeners were reported as unread: %+v", unread)
	}
	if len(live) != 2 {
		t.Fatalf("the one served list carries two connections, and the sweep found %d", len(live))
	}
}

// A list that answers something other than a listing leaves enforcement with a partial view, and
// kicking on a partial view leaves a member connected who should not be.
// The sweep says so rather than answering a short list that looks whole.
func TestAListThatRefusesIsNamed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v3/srtconns/list":
			_, _ = w.Write([]byte(srtConnsWithTwo))
		case "/v3/hlssessions/list":
			http.Error(w, "no", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	host, port := hostPort(t, server.URL)

	live, unread := New().Sessions(host, port)
	if len(live) != 2 {
		t.Errorf("the list that answered carries two connections, and the sweep found %d", len(live))
	}
	if len(unread) != 1 || unread[0].Segment != "hlssessions" {
		t.Fatalf("the refusing list was not named: %+v", unread)
	}
	if unread[0].Reason == "" {
		t.Error("a list that could not be read is named without saying why")
	}
}

// The relay pages its lists, and a sweep that reads the first page alone kicks nobody
// on the second.
func TestSessionsReadsEveryPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/srtconns/list" {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Query().Get("page") {
		case "", "0":
			_, _ = fmt.Fprint(w, `{"itemCount":2,"pageCount":2,"items":[{"id":"srt-1","path":"abc/desk","user":"alice","state":"read"}]}`)
		case "1":
			_, _ = fmt.Fprint(w, `{"itemCount":2,"pageCount":2,"items":[{"id":"srt-2","path":"abc/desk","user":"bob","state":"read"}]}`)
		default:
			_, _ = fmt.Fprint(w, `{"itemCount":2,"pageCount":2,"items":[]}`)
		}
	}))
	t.Cleanup(server.Close)
	host, port := hostPort(t, server.URL)

	live, unread := New().Sessions(host, port)
	if len(unread) != 0 {
		t.Fatalf("a paged list was reported unread: %+v", unread)
	}
	if len(live) != 2 {
		t.Fatalf("two pages carry two connections, and the sweep found %d: %+v", len(live), live)
	}
}

// A kick goes to the list the connection was found on, that being where the relay keeps it.
func TestKickPostsToTheListTheConnectionCameFrom(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(server.Close)
	host, port := hostPort(t, server.URL)

	if err := New().Kick(host, port, "webrtcsessions", "session-7"); err != nil {
		t.Fatalf("kicking a connection the relay closed: %v", err)
	}
	if method != http.MethodPost {
		t.Errorf("a kick was sent as %s", method)
	}
	if path != "/v3/webrtcsessions/kick/session-7" {
		t.Errorf("a kick reached %s", path)
	}
}

// A connection the relay would not close is a member still watching, so the refusal carries
// the relay's own words rather than becoming a silent success.
func TestAKickTheRelayRefusesIsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status":"error","error":"session not found"}`))
	}))
	t.Cleanup(server.Close)
	host, port := hostPort(t, server.URL)

	err := New().Kick(host, port, "srtconns", "gone")
	if err == nil {
		t.Fatal("a refused kick answered success")
	}
	if !strings.Contains(err.Error(), "session not found") {
		t.Errorf("a refused kick lost the relay's own words: %v", err)
	}
}

// An id carrying a separator would address a kick of its own making, so it is escaped into one
// segment rather than pasted into the address.
func TestAKickEscapesTheIDItWasGiven(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(server.Close)
	host, port := hostPort(t, server.URL)

	if err := New().Kick(host, port, "srtconns", "a/../b"); err != nil {
		t.Fatalf("kicking: %v", err)
	}
	if want := "/v3/srtconns/kick/" + url.PathEscape("a/../b"); path != want {
		t.Errorf("a kick reached %s, where the id belongs in one segment: %s", path, want)
	}
}

// The sweep reads the relay's answer, so a listing this app cannot parse is the relay's word and
// not a contract this side may assert on.
func TestAListingThatDoesNotParseIsNamed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/srtconns/list" {
			_, _ = w.Write([]byte(`{"items":`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	host, port := hostPort(t, server.URL)

	live, unread := New().Sessions(host, port)
	if len(live) != 0 {
		t.Errorf("a listing that does not parse yielded connections: %+v", live)
	}
	if len(unread) != 1 || unread[0].Segment != "srtconns" {
		t.Fatalf("a listing that does not parse was not named: %+v", unread)
	}
}

// Every list enforcement sweeps has a kick beside it, so a found connection is one it can act on.
func TestEverySweptListTakesAKick(t *testing.T) {
	swept := map[string]bool{}
	for _, segment := range kickableLists() {
		swept[segment] = true
	}
	if len(swept) == 0 {
		t.Fatal("enforcement sweeps no list at all")
	}

	for kind, reader := range readerKinds {
		if !reader.kick {
			continue
		}
		if !swept[reader.list] {
			t.Errorf("%s names the list %s, which takes a kick and is not swept", kind, reader.list)
		}
	}
	for _, segment := range kickableLists() {
		if segment == "" {
			t.Error("enforcement sweeps a list with no name")
		}
	}
}

func hostPort(t *testing.T, serverURL string) (string, int) {
	t.Helper()

	address, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("the test server named an address that does not parse: %v", err)
	}
	port, err := strconv.Atoi(address.Port())
	if err != nil {
		t.Fatalf("the test server named a port that is not a number: %v", err)
	}
	return address.Hostname(), port
}
