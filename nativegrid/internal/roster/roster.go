// Package roster is the process contract between the app and this window: the
// set of live streams, the app state that travels with it, how both are encoded,
// and how updates to them arrive.
//
// The app passes what it knows at spawn as one JSON argument and pushes the full
// state again as one JSON line per change (Pushes). The producing half is
// watch.BuildGridConfig in desktop/internal/watch/grid.go; the two name each other
// because no Go type crosses the module boundary.
//
// The window writes back on stdout in three kinds, each line tagged with its type
// (messages.go). A Request asks for the watch leg one stream should be on: the
// app decides what that means and pushes the roster it produces, so a request
// changes nothing here until the push arrives. A Command asks the app to act on
// its own state and is answered the same way. A Status reports which streams
// have a tile open and expects no answer.
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
// knobs of each of them, and the window renders a control per entry without
// knowing what any of them mean. Changing one is a Request; the answer is the
// next push, which carries the fragment and the values that resulted.
//
// Options is keyed by transport and holds every offered leg, not only the one in
// force. Picking another leg in the popover swaps its controls at once, which it
// could not do if the knobs of that leg only arrived with the app's answer.
type Stream struct {
	Name       string              `json:"name"`
	Transport  string              `json:"transport"`
	Source     string              `json:"source"`
	Transports []string            `json:"transports"`
	Options    map[string][]Option `json:"options"`
}

// App is the app's own state, the part of it this window draws and acts on:
// whether the app is publishing this machine's capture to the relay, and why the
// last command the window sent failed. The publish control is drawn from it and
// never from what the window last asked for, so a command the app refused or a
// state someone else changed corrects the control on the next push.
type App struct {
	Publishing   bool   `json:"publishing"`
	PublishError string `json:"publishError"`
}

// Config is the full roster of live streams as it crosses the process boundary,
// with the app state that travels beside it.
type Config struct {
	Streams []Stream `json:"streams"`
	// App is what the sidebar's app controls read and change. Its presence is
	// also what says there is an app behind this window: the demo run builds its
	// config here rather than receiving one, and carries none, so the controls
	// that would ask an app for something are not drawn.
	App *App `json:"app,omitempty"`
}

// Parse decodes a Config, either the -config argument or one push read from
// stdin. An empty stream list is valid: it is what an idle relay looks like at
// launch.
//
// What arrives here is written by another process, so a stream the window could
// not act on is an error and not an assertion: the pipe is where a malformed
// roster is caught, one layer before a name keys a lookup or a fragment reaches
// a player. One bad stream fails the whole config rather than being dropped from
// it, because a push is the full set of live streams and a set with an entry
// quietly missing is a tile that disappears with the reason nowhere.
func Parse(raw string) (Config, error) {
	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return Config{}, fmt.Errorf("bad config: %w", err)
	}
	if err := validate(cfg.Streams); err != nil {
		return Config{}, fmt.Errorf("bad config: %w", err)
	}
	return cfg, nil
}

// validate is what a stream has to carry for the window to take it on: a name,
// which every lookup on this side keys on and which therefore names one stream,
// and the source fragment a player is built from.
//
// Transport is not checked against Transports. The app names the legs the
// stream's format can be re-served on and opens the window on the leg it was
// configured for, which need not be one of them (desktop/internal/watch.WatchLeg), and
// the sidebar offers the current leg beside the declared ones for that reason.
func validate(streams []Stream) error {
	seen := make(map[string]bool, len(streams))
	for i, st := range streams {
		if st.Name == "" {
			return fmt.Errorf("stream %d carries no name", i)
		}
		if seen[st.Name] {
			return fmt.Errorf("stream %q is listed twice", st.Name)
		}
		seen[st.Name] = true
		if st.Source == "" {
			return fmt.Errorf("stream %q carries no source fragment", st.Name)
		}
	}
	return nil
}
