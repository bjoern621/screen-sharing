// Package roster is the process contract between the app and this window: the
// set of live streams, how it is encoded, and how updates to it arrive.
//
// The app passes the roster known at spawn as one JSON argument and pushes the
// full set again as one JSON line per change (Pushes). The producing half is
// watch.BuildGridConfig in desktop/watch/grid.go; the two name each other
// because no Go type crosses the module boundary.
//
// The window answers in one direction only, with a Request: the watch leg it
// wants a single stream on. The app decides what that means and pushes the
// roster it produces, so a request changes nothing here until the push arrives.
package roster

import (
	"encoding/json"
	"fmt"
)

// The kinds an Option takes, which is what the sidebar switches on to pick a
// widget for it.
const (
	OptionInt    = "int"
	OptionChoice = "choice"
)

// Option is one knob of the watch leg a stream arrives over: the key a Request
// names it by, how to present it, and the value in force. Values are text
// whatever the kind, because the app is the only side that parses them.
type Option struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Tip     string   `json:"tip"`
	Kind    string   `json:"kind"`
	Value   string   `json:"value"`
	Min     int      `json:"min,omitempty"`
	Choices []string `json:"choices,omitempty"`
}

// Stream is one stream the app offers this window: the display name, the watch
// leg it arrives over, the gst-launch fragment of its source elements, and the
// legs it could be moved to. The fragment ends at the encoded stream; a player
// appends the decode and sink elements. Transport knowledge stays in the app,
// decode knowledge here: the transport name is a label for the stats overlay,
// not something this side acts on. It is the relay-to-viewer leg, which says
// nothing about how the stream was published.
//
// Transports and Options are the same split applied to the sidebar's watch-leg
// popover: the app names the legs this stream can be received over and the
// knobs of the one it is on, and the window renders a control per entry without
// knowing what any of them mean. Changing one is a Request; the answer is the
// next push, which carries the fragment and the values that resulted.
type Stream struct {
	Name       string   `json:"name"`
	Transport  string   `json:"transport"`
	Source     string   `json:"source"`
	Transports []string `json:"transports"`
	Options    []Option `json:"options"`
}

// Config is the full roster of live streams as it crosses the process boundary.
type Config struct {
	Streams []Stream `json:"streams"`
}

// Parse decodes a Config, either the -config argument or one push read from
// stdin. An empty stream list is valid: it is what an idle relay looks like at
// launch.
func Parse(raw string) (Config, error) {
	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return Config{}, fmt.Errorf("bad config: %w", err)
	}
	return cfg, nil
}
