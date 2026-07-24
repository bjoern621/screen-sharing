package watch

import (
	"encoding/json"
	"fmt"
	"strings"

	"bjoernblessin.de/screenshare/settings"
	"bjoernblessin.de/screenshare/transport"
)

// GridViewerExe names the GTK grid viewer binary (cmd/gridviewer). It is a
// separate executable because the Wails process links GTK3 through WebKitGTK,
// and GTK3 and GTK4 cannot share a process.
const GridViewerExe = "screenshare-gridviewer"

// EnvGridViewer overrides where the app finds the grid viewer binary, for
// running against a fresh build without installing it next to the app.
const EnvGridViewer = "SCREENSHARE_GRIDVIEWER"

// GridStream is one stream the grid viewer renders: its display name and the
// gst-launch fragment of its source elements. The fragment ends at the encoded
// stream; the viewer appends its own decode and sink elements.
type GridStream struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

// GridConfig is the process contract between the app and the grid viewer
// binary, passed as a single JSON argument.
type GridConfig struct {
	Streams []GridStream `json:"streams"`
}

// BuildGridConfig serializes the named streams, received over the named
// transport, into the grid viewer's JSON config. Like the wall, it needs the
// transport's GStreamer watch form.
func BuildGridConfig(s settings.Stream, streamNames []string, transportName string) (string, error) {
	if len(streamNames) == 0 {
		return "", fmt.Errorf("the grid viewer needs at least one stream")
	}

	cfg := GridConfig{}
	for _, name := range streamNames {
		src, ok := transport.GstSource(transportName, s, name)
		if !ok {
			return "", fmt.Errorf("transport %q has no GStreamer watch form", transportName)
		}
		cfg.Streams = append(cfg.Streams, GridStream{Name: name, Source: strings.Join(src, " ")})
	}

	out, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
