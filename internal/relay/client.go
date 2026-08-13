// Package relay talks to the MediaMTX HTTP API for stream discovery.
//
// The relay is the single source of truth for "who is live".
// Live per-path bitrates are derived from bytesReceived deltas between two polls,
// since the API itself only exposes counters.
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
// Safe for concurrent use.
type Client struct {
	mu   sync.Mutex
	prev map[string]byteSample
}

type byteSample struct {
	bytes int64
	at    time.Time
}

// Path is one live stream as shown to the UI.
type Path struct {
	Name   string `json:"name"`
	Ready  bool   `json:"ready"`
	Tracks string `json:"tracks"`
	// Format is the video track's bitstream format in the vocabulary the codec table keys on,
	// and empty for a path whose tracks name none.
	// It decides which protocols can carry the stream, so both the viewer's refusal and the watch
	// dropdown read it rather than each parsing Tracks their own way.
	Format string `json:"format"`
	// Readers is how many the relay is serving this path to, and Roster is who they are.
	// Both are read off the one array the relay answered with, so the count is the roster's length by
	// construction rather than by agreement.
	Readers int      `json:"readers"`
	Roster  []Reader `json:"roster"`
	InMbps  float64  `json:"inMbps"` // live ingest bitrate from byte deltas
}

// trackFormats maps the codec names a relay reports on a path to the bitstream formats the codec
// table keys on.
// The relay names a track after the coding format, never after the encoder that produced it,
// which is why one entry serves every encoder of a format.
//
// Both spellings of the two H.26x formats appear, since a relay may report either the ITU name or
// the MPEG one depending on how the stream was ingested.
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
// reports, and the empty string when none of them names a format this app knows.
// A path carries at most one video track here, so the first match is the answer.
func formatOfTracks(tracks []string) string {
	for _, track := range tracks {
		if format, ok := trackFormats[strings.ToUpper(track)]; ok {
			return format
		}
	}
	return ""
}

// Status is one full relay snapshot.
type Status struct {
	Reachable bool   `json:"reachable"`
	Error     string `json:"error,omitempty"`
	Paths     []Path `json:"paths"`
}

// apiPathList mirrors the subset of GET /v3/paths/list this consumes.
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
// Everything a row shows about that reader lives in the per-protocol list the type names
// (readers.go).
type apiReader struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

func New() *Client {
	return &Client{prev: map[string]byteSample{}}
}

// Fetch queries the relay once and returns the snapshot.
//
// An unreachable relay is an Umgebungsfehler rather than an error return: it is reported inside the
// Status so the UI can display it, which is what lets one poll answer both halves.
func (c *Client) Fetch(host string, apiPort int) Status {
	assert.Assert(apiPort > 0, "apiPort comes from validated settings", apiPort)

	url := fmt.Sprintf("http://%s:%d/v3/paths/list", host, apiPort)
	httpClient := http.Client{Timeout: 3 * time.Second}

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

	// The per-protocol lists are read before the lock and not under it.
	// They are HTTP calls, the lock guards nothing but the byte samples, and holding it across a
	// network round trip would make one slow relay stall every other caller of this client.
	// Their failures are already absences rather than errors (readers.go), so there is nothing here
	// for the snapshot to fail on.
	named := make([]apiReader, 0, len(list.Items))
	for _, item := range list.Items {
		named = append(named, item.Readers...)
	}
	conns := fetchConnLists(&httpClient, host, apiPort, named)

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

	// Forget paths that vanished so a re-appearing stream starts a fresh delta.
	for name := range c.prev {
		if !seen[name] {
			delete(c.prev, name)
		}
	}

	assert.Assert(len(status.Paths) == len(list.Items),
		"a reachable snapshot carries a path per item the relay listed", len(status.Paths), len(list.Items))
	return status
}
