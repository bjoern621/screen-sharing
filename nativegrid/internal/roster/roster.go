// Package roster is the process contract between the app and this window: the
// set of live streams, how it is encoded, and how updates to it arrive.
//
// The app passes the roster known at spawn as one JSON argument and pushes the
// full set again as one JSON line per change (Pushes). The producing half is
// watch.BuildGridConfig in desktop/watch/grid.go; the two name each other
// because no Go type crosses the module boundary.
package roster

import (
	"encoding/json"
	"fmt"
)

// Stream is one stream the app offers this window: the display name, the watch
// leg it arrives over, and the gst-launch fragment of its source elements. The
// fragment ends at the encoded stream; a player appends the decode and sink
// elements. Transport knowledge stays in the app, decode knowledge here: the
// transport name is a label for the stats overlay, not something this side acts
// on. It is the relay-to-viewer leg the app chose at launch, which says nothing
// about how the stream was published.
type Stream struct {
	Name      string `json:"name"`
	Transport string `json:"transport"`
	Source    string `json:"source"`
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
