package watch

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"bjoernblessin.de/screenshare/settings"
	"bjoernblessin.de/screenshare/transport"
)

// LiveStream is one stream the relay reports live: the name the grid lists it
// under and the bitstream format its watch-leg choice is narrowed by.
type LiveStream struct {
	Name   string
	Format string
}

// WatchChoice is one stream's deviation from the window's watch leg: the
// transport it is received over and the values of that transport's knobs, keyed
// as the transport declares them. An empty Transport takes the window's.
//
// A choice lives for the window it was made in and is not persisted: it is a
// per-stream deviation from the app's settings, not a new setting, and keeping
// one per stream would multiply what a restart has to restore.
type WatchChoice struct {
	Transport string
	Options   map[string]string
}

// GridStream is one stream the native grid window offers in its sidebar: the
// display name, the transport it arrives over, the gst-launch fragment of its
// source elements, and the watch legs the sidebar can move it to. The fragment
// ends at the encoded stream; the grid binary appends its own decode and sink
// elements, so transport knowledge stays on this side of the process boundary
// and decode knowledge on the other.
//
// Transports and Options carry that same split into the sidebar's per-stream
// watch-leg popover: the transports this stream can be received over and the
// selected one's knobs with their current values, both declared here, so the
// grid renders a control per entry and names no transport itself.
type GridStream struct {
	Name       string                  `json:"name"`
	Transport  string                  `json:"transport"`
	Source     string                  `json:"source"`
	Transports []string                `json:"transports"`
	Options    []transport.WatchOption `json:"options"`
}

// GridApp is the app's own state as the window draws it: whether the app is
// publishing this machine's capture, and why the last command the window sent
// failed. The window has no second copy of either, so a publish button it drew
// from one push is corrected by the next.
//
// It is always sent, and its presence is what tells the window there is an app
// behind it: the window's demo run builds a config of its own and carries none.
type GridApp struct {
	Publishing   bool   `json:"publishing"`
	PublishError string `json:"publishError"`
}

// GridConfig is the process contract between the app and the native grid
// binary, passed as a single JSON argument and pushed again per change. The
// consuming half is the nativegrid module's internal/roster package; the two
// packages are the two halves of one contract and name each other because no Go
// type can cross the module boundary. What the window writes back on its stdout
// is decoded by ParseGridMessage.
type GridConfig struct {
	Streams []GridStream `json:"streams"`
	App     GridApp      `json:"app"`
}

// BuildGridConfig serializes the live streams into the native grid's JSON
// config, each on the watch leg its choice names and the rest on
// defaultTransport, with the app state the window's own controls read. An empty
// stream list is valid: the grid opens on an idle relay and fills from roster
// pushes. The default transport must have a GStreamer watch form
// (transport.GstWatcher), checked up front so a bad transport fails at open, not
// at the first push.
func BuildGridConfig(s settings.Stream, live []LiveStream, defaultTransport string, choices map[string]WatchChoice, app GridApp) (string, error) {
	if !transport.CanGstWatch(defaultTransport) {
		return "", fmt.Errorf("transport %q has no GStreamer watch form", defaultTransport)
	}

	// Streams starts non-nil so an empty roster marshals as [], not null.
	cfg := GridConfig{Streams: []GridStream{}, App: app}
	for _, l := range live {
		leg, name, err := WatchLeg(s, l, defaultTransport, choices[l.Name])
		if err != nil {
			return "", err
		}
		src, _ := transport.GstSource(name, leg, l.Name)
		cfg.Streams = append(cfg.Streams, GridStream{
			Name:       l.Name,
			Transport:  name,
			Source:     strings.Join(src, " "),
			Transports: GstWatchTransports(l.Format),
			Options:    transport.WatchOptions(name, leg),
		})
	}

	out, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// WatchLeg resolves one stream's watch leg: the transport it is received over
// and the settings its knobs were written into, both after the choice is
// applied. The base settings are copied, so a per-stream choice reaches that
// stream and nothing else.
//
// A chosen transport the stream cannot be watched over and a knob the transport
// does not declare are refused, which is how a choice is rejected where it
// arrives instead of turning into a source fragment nothing plays. A stream
// with no choice of its own takes defaultTransport unexamined: the window was
// opened on it, and dropping it here would move the stream to a leg nobody
// picked.
func WatchLeg(base settings.Stream, l LiveStream, defaultTransport string, c WatchChoice) (settings.Stream, string, error) {
	name := defaultTransport
	if c.Transport != "" {
		offered := GstWatchTransports(l.Format)
		if !slices.Contains(offered, c.Transport) {
			return base, "", fmt.Errorf("stream %q cannot be watched over %s: %s carries %s",
				l.Name, c.Transport, strings.Join(offered, " or "), l.Format)
		}
		name = c.Transport
	}

	// Sorted, so a rejected choice names the same key on every build.
	leg := base
	for _, key := range slices.Sorted(maps.Keys(c.Options)) {
		if err := transport.SetWatchOption(name, &leg, key, c.Options[key]); err != nil {
			return base, "", fmt.Errorf("stream %q: %w", l.Name, err)
		}
	}
	return leg, name, nil
}

// PruneWatchChoices drops the choices the live streams no longer support and
// returns what each cost, for the caller to report. A stream that came back in
// another format can leave a choice behind whose transport no longer carries
// it, and BuildGridConfig refuses the whole roster over one such stream, which
// would freeze the window on the last roster it managed to build.
//
// Choices of streams the relay does not report live are kept: a stream that
// comes back finds the leg it was on, the way it finds its slot in the order.
func PruneWatchChoices(base settings.Stream, live []LiveStream, defaultTransport string, choices map[string]WatchChoice) []error {
	var dropped []error
	for _, l := range live {
		c, ok := choices[l.Name]
		if !ok {
			continue
		}
		if _, _, err := WatchLeg(base, l, defaultTransport, c); err != nil {
			delete(choices, l.Name)
			dropped = append(dropped, err)
		}
	}
	return dropped
}

// GstWatchTransports lists the transports the native grid can receive a stream
// of this format over: those with a GStreamer watch form, narrowed to the ones
// the relay re-serves the format on. MPEG-TS over SRT carries H.264 and H.265,
// so a VP9 or AV1 stream is left with the transports that have a payload
// mapping for it. A format the codec table does not name narrows nothing, which
// is the transport package's rule and not this function's.
//
// The list is the grid's alone and is wider than a player's: WHEP has no URL a
// viewer program opens, and a receiving pipeline reaches it all the same.
func GstWatchTransports(format string) []string {
	return transport.GstWatchNamesFor(format)
}

// The kinds of message the grid window writes on its stdout, carried in every line as "type".
// One pipe holds all three kinds, so the discriminator is what keeps this side from reading a report as a request.
// The producing half is the nativegrid module's internal/roster package, which declares the same three names.
// The command kind carries the Kind suffix because GridCommand is its payload.
const (
	GridWatchLeg    = "watch-leg"
	GridWatchSet    = "watch-set"
	GridCommandKind = "command"
)

// The commands the grid window can send, each an action on the app's own state
// rather than on a stream. The window names one and reads what it did off the
// next push; the names are the roster package's, which declares the same three.
const (
	GridShowSettings = "show-settings"
	GridStartPublish = "start-publish"
	GridStopPublish  = "stop-publish"
)

// GridCommand is one action the window asked the app to take.
// The two publish commands name the state they want rather than a toggle,
// so a button the window drew from a push the app has since left cannot flip the state the other way.
type GridCommand struct {
	Name string `json:"command"`
}

// GridStatus is what the grid window reports it has open: the names of the streams with a tile, sorted.
// It is the whole set rather than what changed, so the app replaces what it held instead of merging into it.
type GridStatus struct {
	Watching []string `json:"watching"`
}

// GridMessage is one line the grid window wrote, decoded: the kind it declared and the payload of that kind.
// Only the field the kind names is filled.
type GridMessage struct {
	Kind    string
	Request GridRequest
	Status  GridStatus
	Command GridCommand
}

// ParseGridMessage decodes one line the grid window wrote.
// A line that declares no kind is an error rather than an ignored line:
// the window logs to stderr and writes nothing but messages here,
// so a line without a discriminator came from a library printing over the channel.
//
// A kind this build does not know is returned with an empty payload and no error,
// for the caller to report and skip.
// The two halves ship together, so an unknown kind means a grid binary from another build,
// which is a mismatch to notice rather than a line to refuse.
func ParseGridMessage(line string) (GridMessage, error) {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(line), &head); err != nil {
		return GridMessage{}, fmt.Errorf("bad grid message %q: %w", line, err)
	}

	switch head.Type {
	case GridWatchLeg:
		r, err := ParseGridRequest(line)
		if err != nil {
			return GridMessage{}, err
		}
		return GridMessage{Kind: head.Type, Request: r}, nil
	case GridWatchSet:
		var s GridStatus
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			return GridMessage{}, fmt.Errorf("bad grid watch set %q: %w", line, err)
		}
		return GridMessage{Kind: head.Type, Status: s}, nil
	case GridCommandKind:
		var c GridCommand
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			return GridMessage{}, fmt.Errorf("bad grid command %q: %w", line, err)
		}
		if c.Name == "" {
			return GridMessage{}, fmt.Errorf("grid command %q names no command", line)
		}
		return GridMessage{Kind: head.Type, Command: c}, nil
	case "":
		return GridMessage{}, fmt.Errorf("grid message %q names no type", line)
	default:
		return GridMessage{Kind: head.Type}, nil
	}
}
