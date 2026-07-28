package watch

import (
	"encoding/json"
	"fmt"
)

// GridRequest is one stream's watch leg as the native grid window wants it,
// written on the window's stdout as a single JSON line. It is the full state of
// that leg rather than a delta: the transport the stream is received over and
// every knob that transport declares, so the app replaces the choice it held
// instead of merging into it.
type GridRequest struct {
	Stream    string            `json:"stream"`
	Transport string            `json:"transport"`
	Options   map[string]string `json:"options"`
}

// Choice is the request in the form the config builder takes.
func (r GridRequest) Choice() WatchChoice {
	return WatchChoice{Transport: r.Transport, Options: r.Options}
}

// ParseGridRequest decodes one line the grid window wrote. A line that is not a
// request is an error rather than an empty one: the window logs to stderr and
// writes nothing else here, so an unparseable line came from a library printing
// over the channel, and reading it as a request would move a stream nobody
// touched.
func ParseGridRequest(line string) (GridRequest, error) {
	var r GridRequest
	if err := json.Unmarshal([]byte(line), &r); err != nil {
		return GridRequest{}, fmt.Errorf("bad grid request %q: %w", line, err)
	}
	if r.Stream == "" {
		return GridRequest{}, fmt.Errorf("grid request %q names no stream", line)
	}
	return r, nil
}
