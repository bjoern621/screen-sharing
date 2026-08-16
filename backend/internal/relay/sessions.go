package relay

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"
)

// Membership is enforced by closing connections, never by withholding a token.
//
// The relay reads a token at the handshake and not again, measured against v1.20.0: a session
// outlives its token on every leg, and a client that is kicked opens another with the same one.
// So a member who leaves is removed by kicking what they hold, and kept out by whatever issues
// tokens declining to issue another (internal/roster).

// Session is one connection the relay is carrying, as enforcement reads it.
//
// Enough to decide and to act: who opened it, what it is on, and which list it came from, that being
// where its kick lives.
// The figures a viewer row shows are Reader's instead (readers.go), which is a different question
// asked of the same connection.
type Session struct {
	// Segment is the per-protocol list this was found on, and the one its kick goes to.
	Segment string `json:"segment"`
	ID      string `json:"id"`
	Path    string `json:"path"`
	// User is the subject of the token the connection was opened with: a member id where a member was
	// named, and the group's own id where none was (internal/group).
	User string `json:"user"`
	// State is "publish" or "read" in the relay's own words, and empty on a list whose connections are
	// only ever readers.
	State      string `json:"state,omitempty"`
	RemoteAddr string `json:"remoteAddr,omitempty"`
	// Transport is the leg in this app's vocabulary, "srt" where the relay says "srtconns".
	// Filled from readerKinds rather than from the answer, the relay naming a list and not a
	// transport.
	Transport string `json:"transport,omitempty"`
}

// Unread is a per-protocol list that exists and would not answer, and why.
//
// A listener that is off is not one of these. It answers 404, which is a fact about the deployment
// and not a failure (readers.go).
// This is a sweep that came back partial, and a caller kicking on a partial sweep leaves somebody
// connected who should not be, so it travels beside the connections rather than being logged here.
type Unread struct {
	Segment string `json:"segment"`
	Reason  string `json:"reason"`
}

// sessionsPerPage is how much of a list one request asks for.
// The relay pages every list, so a sweep reading one page kicks nobody past it.
const sessionsPerPage = 100

// listLimit bounds one list's body.
// A relay carrying more connections than this in one page is answering something this is not
// reading.
const listLimit = 4 << 20

// errNoListener is a list this relay does not serve, which is its answer for a protocol whose
// listener is switched off.
var errNoListener = errors.New("no listener")

// Sessions is every connection the relay reports across the lists enforcement can act on, and the
// lists that would not answer.
//
// Both are returned because a caller has to see the second: a short sweep looks exactly like a quiet
// relay, and acting on one leaves a member watching.
func (c *Client) Sessions(host string, apiPort int) ([]Session, []Unread) {
	assert.Assert(apiPort > 0, "apiPort comes from validated settings", apiPort)

	client := c.httpClient()
	live := []Session{}
	unread := []Unread{}

	for _, segment := range kickableLists() {
		found, err := fetchSessions(client, host, apiPort, segment)
		switch {
		case errors.Is(err, errNoListener):
			continue
		case err != nil:
			unread = append(unread, Unread{Segment: segment, Reason: err.Error()})
			continue
		}
		live = append(live, found...)
	}

	for _, session := range live {
		assert.Assert(session.Segment != "", "a swept connection names the list it came from", session.ID)
	}
	return live, unread
}

// Kick closes one connection.
//
// A refusal is carried out rather than swallowed: the connection is a member still watching, and a
// caller told it succeeded would report a removal that did not happen.
func (c *Client) Kick(host string, apiPort int, segment, id string) error {
	assert.Assert(apiPort > 0, "apiPort comes from validated settings", apiPort)
	assert.Assert(segment != "", "a kick names the list the connection is on")
	assert.Assert(id != "", "a kick names the connection to close")

	// Escaped, the id being the relay's own word travelling back to it: an unescaped one reaches a
	// path of its own making.
	address := fmt.Sprintf("http://%s:%d/v3/%s/kick/%s", host, apiPort, segment, url.PathEscape(id))
	resp, err := c.httpClient().Post(address, "application/json", nil)
	if err != nil {
		return fmt.Errorf("the relay did not answer a kick on %s: %w", segment, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, listLimit))
		return fmt.Errorf("the relay refused to close %s %s: %s %s",
			segment, id, resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

// kickableLists names the per-protocol lists enforcement sweeps, in one order so two sweeps read the
// same relay the same way.
//
// Derived from readerKinds rather than written out a second time, that table already stating which
// list describes which kind of connection.
func kickableLists() []string {
	lists := []string{}
	for _, kind := range readerKinds {
		if kind.kick && !slices.Contains(lists, kind.list) {
			lists = append(lists, kind.list)
		}
	}
	slices.Sort(lists)
	return lists
}

// transportOf is this app's name for the leg a list describes, and empty for a list no kind names.
func transportOf(segment string) string {
	for _, kind := range readerKinds {
		if kind.list == segment {
			return kind.transport
		}
	}
	return ""
}

// fetchSessions reads one list, following its pages.
func fetchSessions(client *http.Client, host string, apiPort int, segment string) ([]Session, error) {
	found := []Session{}

	for page := 0; ; page++ {
		address := fmt.Sprintf("http://%s:%d/v3/%s/list?page=%d&itemsPerPage=%d",
			host, apiPort, segment, page, sessionsPerPage)
		resp, err := client.Get(address)
		if err != nil {
			return nil, fmt.Errorf("the relay did not answer %s: %w", segment, err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, listLimit))
		resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return nil, errNoListener
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("the relay answered %s with %s", segment, resp.Status)
		}
		if readErr != nil {
			return nil, fmt.Errorf("the relay's answer for %s broke off: %w", segment, readErr)
		}

		var answer struct {
			PageCount int       `json:"pageCount"`
			Items     []Session `json:"items"`
		}
		if err := json.Unmarshal(body, &answer); err != nil {
			return nil, fmt.Errorf("the relay's %s listing does not read as one: %w", segment, err)
		}

		transport := transportOf(segment)
		for _, item := range answer.Items {
			item.Segment, item.Transport = segment, transport
			found = append(found, item)
		}
		if page+1 >= answer.PageCount {
			return found, nil
		}
	}
}
