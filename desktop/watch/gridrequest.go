package watch

import (
	"encoding/json"
	"fmt"
)

// GridRequest is the watch leg as the native grid window wants it, written on
// the window's stdout as a single JSON line. It is the full state of that leg
// rather than a delta: the transport tiles are received over and every knob that
// transport declares, so the app replaces what it held instead of merging into
// it.
//
// Stream names the row the request was made on. The leg itself is one setting
// for the window, so the name is what the app checks the transport's carriage
// against and what it reports a refusal under, not a key it stores the leg per.
type GridRequest struct {
	Stream    string            `json:"stream"`
	Transport string            `json:"transport"`
	Options   map[string]string `json:"options"`
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
