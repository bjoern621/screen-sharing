package watch

import (
	"encoding/json"
	"fmt"
	"strings"

	"bjoernblessin.de/screenshare/settings"
	"bjoernblessin.de/screenshare/transport"
)

// GridStream is one stream the native grid window offers in its sidebar: the
// display name, the transport it arrives over, and the gst-launch fragment of
// its source elements. The fragment ends at the encoded stream; the grid binary
// appends its own decode and sink elements, so transport knowledge stays on this
// side of the process boundary and decode knowledge on the other. The transport
// travels as a name because the grid only labels it, in the tile's stats
// overlay.
type GridStream struct {
	Name      string `json:"name"`
	Transport string `json:"transport"`
	Source    string `json:"source"`
}

// GridConfig is the process contract between the app and the native grid
// binary, passed as a single JSON argument. The consuming half is the
// nativegrid module's internal/roster package; the two name each other because
// no Go type can cross the module boundary.
type GridConfig struct {
	Streams []GridStream `json:"streams"`
}

// BuildGridConfig serializes the named streams, received over the named
// transport, into the native grid's JSON config. An empty stream list is
// valid: the grid opens on an idle relay and fills from roster pushes. The
// transport must have a GStreamer watch form (transport.GstWatcher), checked
// up front so a bad transport fails at open, not at the first push.
func BuildGridConfig(s settings.Stream, streamNames []string, transportName string) (string, error) {
	if !transport.CanGstWatch(transportName) {
		return "", fmt.Errorf("transport %q has no GStreamer watch form", transportName)
	}

	// Streams starts non-nil so an empty roster marshals as [], not null.
	cfg := GridConfig{Streams: []GridStream{}}
	for _, name := range streamNames {
		src, _ := transport.GstSource(transportName, s, name)
		cfg.Streams = append(cfg.Streams, GridStream{
			Name:      name,
			Transport: transportName,
			Source:    strings.Join(src, " "),
		})
	}

	out, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
