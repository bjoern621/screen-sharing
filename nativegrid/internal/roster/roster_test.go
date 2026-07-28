package roster

import (
	"strings"
	"testing"
)

// A stream the window could not act on is refused at the pipe, where it is the
// other process's mistake, rather than reaching a player that asserts on it.
// The whole push goes with it: a set of live streams missing an entry is a tile
// gone with no reason anywhere.
func TestParseRefusesAStreamTheWindowCannotShow(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "no name to key the stream by",
			raw:  `{"streams":[{"name":"","source":"srtsrc"}]}`,
			want: "no name",
		},
		{
			name: "two streams under one name",
			raw:  `{"streams":[{"name":"a","source":"srtsrc"},{"name":"a","source":"rtspsrc"}]}`,
			want: "listed twice",
		},
		{
			name: "no source fragment to build a player from",
			raw:  `{"streams":[{"name":"a","source":""}]}`,
			want: "no source fragment",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(c.raw)
			if err == nil {
				t.Fatalf("Parse(%s) took the config, want it refused", c.raw)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %q, want it to say %q", err, c.want)
			}
		})
	}
}

// The leg a stream is on need not be one of the legs it offers: the app opens
// the window on a transport the stream's format may not be re-served on, and the
// sidebar offers that one beside the rest.
func TestParseTakesALegOutsideTheOfferedOnes(t *testing.T) {
	cfg, err := Parse(`{"streams":[{"name":"a","transport":"srt","source":"srtsrc","transports":["rtsp"]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Streams[0].Transport; got != "srt" {
		t.Errorf("transport = %q, want the leg the app put the stream on", got)
	}
}

// The literal is the wire format the app's watch package writes.
func TestParseReadsTheAppState(t *testing.T) {
	cfg, err := Parse(`{"streams":[],"app":{"publishing":true,"publishError":"no encoder"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.App == nil {
		t.Fatal("the push carried app state and the config has none")
	}
	if !cfg.App.Publishing || cfg.App.PublishError != "no encoder" {
		t.Errorf("app = %+v, want a publishing app with the encoder message", *cfg.App)
	}
}

// A config with no app state is what a window with no app behind it opens on,
// and the sidebar draws no app controls for it. Reading a missing section as an
// idle app instead would put a publish button in front of nobody.
func TestParseReadsAMissingAppStateAsNone(t *testing.T) {
	cfg, err := Parse(`{"streams":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.App != nil {
		t.Errorf("app = %+v, want none", *cfg.App)
	}
}
