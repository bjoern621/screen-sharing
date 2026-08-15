// Package relay talks to the MediaMTX HTTP API for stream discovery.
//
// The relay is the single source of truth for "who is live".
// The API exposes counters and no rates, so a per-path bitrate is a bytesReceived delta between two
// fetches: the figure is a rate only while a caller asks at a steady cadence, which is the poll
// loop in internal/app (watch.go) and never a shell.
//
// Everything the relay answers with is another process's word, so nothing here asserts on it.
// An unreachable relay, a malformed response and a path naming a format this app does not know are
// all Umgebungsfehler, and each is carried in the snapshot rather than raised.
package relay

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
)

// Client polls the MediaMTX HTTP API.
// Safe for concurrent use: mu guards prev, the byte sample each rate is measured against.
type Client struct {
	mu   sync.Mutex
	prev map[string]byteSample
	// Renders the credential every request carries. nil for a relay that answers its API to anyone.
	//
	// A function and not a value, because the relay checks the token's window too: what a caller
	// holds is the means to sign one, and a stored string stops working partway through a process's
	// life.
	authorize func() string
}

type byteSample struct {
	bytes int64
	at    time.Time
}

// Path is one stream the relay is carrying, as a viewer's list shows it.
type Path struct {
	Name string `json:"name"`
	// OwnName is Name with the prefix this machine reaches under taken off, and Name where none
	// was.
	// Nothing here derives it: which prefix that is comes off the settings, so the poll fills it in
	// (internal/app, groups.go).
	OwnName string `json:"ownName"`
	Ready   bool   `json:"ready"`
	Tracks  string `json:"tracks"`
	// Format is the video track's bitstream format in the vocabulary the codec table keys on, and
	// empty for a path whose tracks name none.
	// It decides which protocols can carry the stream, so the viewer's refusal and the watch dropdown
	// both read it rather than each parsing Tracks their own way.
	Format string `json:"format"`
	// Readers is how many the relay is serving this path to, and Roster is who they are.
	// Both are read off the one array the relay answered with, so the count is the roster's length by
	// construction rather than by agreement.
	Readers int      `json:"readers"`
	Roster  []Reader `json:"roster"`
	InMbps  float64  `json:"inMbps"` // Mbit/s ingest, from the byte delta since the previous fetch
}

// trackFormats maps the codec names a relay reports on a path to the bitstream formats the codec
// table keys on.
// The relay names a track after the coding format and never after the encoder that produced it,
// which is why one entry serves every encoder of a format.
//
// Both spellings of the two H.26x formats appear, since a relay reports either the ITU name or the
// MPEG one depending on how the stream was ingested.
var trackFormats = map[string]string{
	"H264": "h264",
	"AVC":  "h264",
	"H265": "hevc",
	"HEVC": "hevc",
	"VP8":  "vp8",
	"VP9":  "vp9",
	"AV1":  "av1",
}

// formatOfTracks returns the bitstream format of the video track among the ones a relay path
// reports, and "" where none of them names a format this app knows.
// A path carries at most one video track here, so the first match is the answer.
func formatOfTracks(tracks []string) string {
	for _, track := range tracks {
		if format, ok := trackFormats[strings.ToUpper(track)]; ok {
			return format
		}
	}
	return ""
}

// Status is one whole snapshot of the relay.
type Status struct {
	Reachable bool   `json:"reachable"`
	Error     string `json:"error,omitempty"`
	Paths     []Path `json:"paths"`
	// FromIndex is a snapshot off the group service's index rather than the relay's own API, which is
	// what a member gets on a relay that authenticates it (internal/app, groups.go).
	//
	// The two sources answer different amounts, not different truths: an index row names a stream and
	// what it carries, and knows nothing of readers or rate.
	// Those fields are zero here, and a reader that did not know would show "no viewers" for "not
	// answered here".
	FromIndex bool `json:"fromIndex,omitempty"`
}

// apiPathList is the subset of GET /v3/paths/list this package decodes.
type apiPathList struct {
	Items []struct {
		Name          string      `json:"name"`
		Ready         bool        `json:"ready"`
		Tracks        []string    `json:"tracks"`
		BytesReceived int64       `json:"bytesReceived"`
		Readers       []apiReader `json:"readers"`
	} `json:"items"`
}

// apiReader is how the path list names one reader: a type and an id, and nothing else.
// Everything else a row shows about that reader lives in the per-protocol list the type names
// (readers.go).
type apiReader struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

func New() *Client {
	return &Client{prev: map[string]byteSample{}}
}

// NewAuthorized is the client for a relay that answers its API to nobody anonymous.
// Its one caller is the group service, whose credential is one it signs itself (cmd/groupd).
func NewAuthorized(authorize func() string) *Client {
	assert.IsNotNil(authorize, "an authorized client can render its credential")

	return &Client{prev: map[string]byteSample{}, authorize: authorize}
}

// httpClient is the client one fetch runs on, carrying the credential where there is one.
//
// The round tripper adds the header rather than each call site: a fetch is the path list plus a
// reader list per protocol (readers.go), and a per-site credential is one a later endpoint forgets.
func (c *Client) httpClient() *http.Client {
	client := &http.Client{Timeout: 3 * time.Second}
	if c.authorize != nil {
		client.Transport = authorizing{authorize: c.authorize}
	}
	return client
}

// authorizing adds the credential to every request through it.
type authorizing struct {
	authorize func() string
}

func (a authorizing) RoundTrip(r *http.Request) (*http.Response, error) {
	assert.IsNotNil(a.authorize, "an authorizing round tripper renders a credential")

	// Cloned rather than written to: net/http documents that a RoundTripper must not modify the
	// request it is handed.
	authorized := r.Clone(r.Context())
	if credential := a.authorize(); credential != "" {
		authorized.Header.Set("Authorization", "Bearer "+credential)
	}
	return http.DefaultTransport.RoundTrip(authorized)
}

// Fetch queries the relay once and returns the snapshot.
//
// An unreachable relay is an Umgebungsfehler and rides in the Status rather than an error return,
// which is what lets one poll answer both halves.
func (c *Client) Fetch(host string, apiPort int) Status {
	assert.Assert(apiPort > 0, "apiPort comes from validated settings", apiPort)

	url := fmt.Sprintf("http://%s:%d/v3/paths/list", host, apiPort)
	httpClient := c.httpClient()

	resp, err := httpClient.Get(url)
	if err != nil {
		return Status{Reachable: false, Error: err.Error()}
	}
	defer resp.Body.Close()

	var list apiPathList
	err = json.NewDecoder(resp.Body).Decode(&list)
	if err != nil {
		return Status{Reachable: false, Error: "invalid API response: " + err.Error()}
	}

	// The per-protocol lists are read before the lock and never under it.
	// They are HTTP calls, the lock guards nothing but the byte samples, and holding it across a
	// network round trip would let one slow relay stall every other caller of this client.
	// Their failures are absences rather than errors (readers.go), so there is nothing here for the
	// snapshot to fail on.
	named := make([]apiReader, 0, len(list.Items))
	for _, item := range list.Items {
		named = append(named, item.Readers...)
	}
	conns := fetchConnLists(httpClient, host, apiPort, named)

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	status := Status{Reachable: true, Paths: make([]Path, 0, len(list.Items))}
	seen := map[string]bool{}

	for _, item := range list.Items {
		seen[item.Name] = true

		path := Path{
			Name:    item.Name,
			Ready:   item.Ready,
			Readers: len(item.Readers),
			Roster:  joinReaders(conns, item.Readers),
			Format:  formatOfTracks(item.Tracks),
		}
		assert.Assert(len(path.Roster) == path.Readers,
			"a path's roster names every reader it counts", len(path.Roster), path.Readers)
		for i, track := range item.Tracks {
			if i > 0 {
				path.Tracks += ","
			}
			path.Tracks += track
		}

		prev, present := c.prev[item.Name]
		if present {
			deltaSeconds := now.Sub(prev.at).Seconds()
			if deltaSeconds > 0 && item.BytesReceived >= prev.bytes {
				path.InMbps = float64(item.BytesReceived-prev.bytes) * 8 / deltaSeconds / 1e6
			}
		}
		c.prev[item.Name] = byteSample{bytes: item.BytesReceived, at: now}

		status.Paths = append(status.Paths, path)
	}

	// A vanished path is forgotten, so a stream that comes back starts a fresh delta rather than one
	// spanning its absence.
	for name := range c.prev {
		if !seen[name] {
			delete(c.prev, name)
		}
	}

	assert.Assert(len(status.Paths) == len(list.Items),
		"a reachable snapshot carries a path per item the relay listed", len(status.Paths), len(list.Items))
	return status
}
