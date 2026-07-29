package watch

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/settings"
	"bjoernblessin.de/screenshare/transport"
)

// LiveStream is one stream the relay reports live: the name the grid lists it
// under and the bitstream format its watch-leg choice is narrowed by.
type LiveStream struct {
	Name   string
	Format string
}

// GridStream is one stream the native grid window offers in its sidebar: the
// display name, the transport it arrives over, the gst-launch fragment of its
// source elements, and the watch legs the sidebar can move it to. The fragment
// ends at the encoded stream; the grid binary appends its own decode and sink
// elements, so transport knowledge stays on this side of the process boundary
// and decode knowledge on the other.
//
// Transports and Options carry that same split into the sidebar's watch-leg
// popover: the transports this stream can be received over and the knobs of each
// of them with their current values, both declared here, so the grid renders a
// control per entry and names no transport itself.
//
// Options holds every offered leg rather than the one in force, because the
// popover swaps its controls the moment another leg is picked and would
// otherwise have to wait for the app to answer before it could show what that
// leg offers.
type GridStream struct {
	Name       string                             `json:"name"`
	Transport  string                             `json:"transport"`
	Source     string                             `json:"source"`
	Transports []string                           `json:"transports"`
	Options    map[string][]transport.WatchOption `json:"options"`
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
// config, every one of them on s.GridTransport, with the app state the window's
// own controls read. An empty stream list is valid: the grid opens on an idle
// relay and fills from roster pushes. The grid leg must have a GStreamer watch
// form (transport.GstWatcher), checked up front so a bad transport fails at
// open, not at the first push.
//
// The leg is one setting for the whole window rather than a choice per stream.
// A knob turned in the sidebar is written back into the settings and saved
// (ApplyWatchLeg), so what a viewer picks survives the window that picked it,
// and the next push puts every tile on it.
//
// A stream the grid could not key a row on is dropped rather than refused: the
// names come off the relay's path list, and one bad path would otherwise cost
// the window every other stream in the same roster.
func BuildGridConfig(s settings.Stream, live []LiveStream, app GridApp) (string, error) {
	if !transport.CanWatch(s.GridTransport, capabilities.EngineGst) {
		return "", fmt.Errorf("transport %q has no GStreamer watch form", s.GridTransport)
	}

	// Streams starts non-nil so an empty roster marshals as [], not null.
	cfg := GridConfig{Streams: []GridStream{}, App: app}
	// The name keys the window's rows, its watch-leg requests and its watch
	// reports, and the relay is another process, free to answer with an unnamed or
	// repeated path. Neither survives that keying, so both are dropped with the
	// reason instead of reaching a window that refuses the whole roster.
	taken := map[string]bool{}
	for _, l := range live {
		if l.Name == "" {
			logger.Warnf("native grid roster: dropped a live stream the relay reports under no name")
			continue
		}
		if taken[l.Name] {
			logger.Warnf("native grid roster: dropped a second live stream named %q, which the window cannot tell from the first", l.Name)
			continue
		}
		taken[l.Name] = true

		// The leg is s.GridTransport, checked above, so every stream's fragment is
		// built by a transport with a GStreamer watch form. A leg without one used to
		// reach the grid as an empty fragment and fail in that process instead of this
		// one.
		src, ok := transport.GstSource(s.GridTransport, s, l.Name)
		assert.Assert(ok, "the grid leg has a GStreamer watch form", l.Name, s.GridTransport)
		source := strings.Join(src, " ")
		assert.Assert(source != "", "a grid stream carries the source fragment to decode it", l.Name, s.GridTransport)

		offered := GstWatchTransports(l.Format)
		cfg.Streams = append(cfg.Streams, GridStream{
			Name:       l.Name,
			Transport:  s.GridTransport,
			Source:     source,
			Transports: offered,
			Options:    watchLegOptions(s, offeredLegs(s.GridTransport, offered)),
		})
	}

	assert.Assert(len(cfg.Streams) <= len(live), "a grid stream per live stream at most", len(cfg.Streams), len(live))

	// The other half of this contract is the nativegrid module's roster.Parse,
	// which refuses a config with an empty or repeated stream name. Emitting one
	// fails here rather than in the window this config is spawned on.
	emitted := map[string]bool{}
	for _, stream := range cfg.Streams {
		assert.Assert(stream.Name != "" && !emitted[stream.Name], "a grid stream carries a name unique in the roster", stream.Name)
		emitted[stream.Name] = true
	}

	out, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// offeredLegs is what the sidebar's transport dropdown holds: the legs the
// stream's format can be re-served on, plus the one the window runs on when that
// is not among them. A window opened on a leg a stream's format does not reach
// still shows that stream, and a dropdown that could not name the leg in force
// would read as a leg nobody set.
func offeredLegs(current string, offered []string) []string {
	if slices.Contains(offered, current) {
		return offered
	}
	return append([]string{current}, offered...)
}

// watchLegOptions is the knob set of every leg in legs, keyed by transport and
// carrying the values the settings hold. The sidebar draws the controls of
// whichever leg its dropdown shows, so it is handed all of them at once.
//
// A leg with no knobs gets an empty list rather than no entry: the window tells
// a leg it was told about from one it has no declaration for, and a transport
// that declares nothing is the first, not the second.
func watchLegOptions(s settings.Stream, legs []string) map[string][]transport.WatchOption {
	out := make(map[string][]transport.WatchOption, len(legs))
	for _, name := range legs {
		options := transport.WatchOptions(name, s)
		if options == nil {
			options = []transport.WatchOption{}
		}
		out[name] = options
	}
	assert.Assert(len(out) == len(legs), "a knob set per offered leg", len(out), len(legs))
	return out
}

// ApplyWatchLeg writes one watch-leg request from the grid window into the
// settings: the transport every tile is received over and the knobs that
// transport declares. The result is the settings the caller persists, and base
// is returned untouched where the request is refused.
//
// A transport the stream cannot be watched over and a knob the transport does
// not declare are both refused, which is how a request is rejected where it
// arrives instead of turning into a source fragment nothing plays.
//
// The leg the window already runs on is accepted whatever the stream's format
// carries. The window opens on one leg and shows a stream whose format that leg
// does not carry all the same, so the sidebar offers it beside the legs it
// lists; refusing the name would leave that stream unable to state the leg it
// is already on, and its knobs, which travel with the name, unreachable with it.
func ApplyWatchLeg(base settings.Stream, l LiveStream, r GridRequest) (settings.Stream, error) {
	assert.Assert(transport.CanWatch(base.GridTransport, capabilities.EngineGst), "a grid window runs on a GStreamer watch leg", base.GridTransport)

	name := base.GridTransport
	if r.Transport != "" && r.Transport != name {
		offered := GstWatchTransports(l.Format)
		if !slices.Contains(offered, r.Transport) {
			return base, fmt.Errorf("stream %q cannot be watched over %s: %s carries %s",
				l.Name, r.Transport, strings.Join(offered, " or "), l.Format)
		}
		name = r.Transport
	}

	// Sorted, so a rejected request names the same key on every call.
	next := base
	next.GridTransport = name
	for _, key := range slices.Sorted(maps.Keys(r.Options)) {
		if err := transport.SetWatchOption(name, &next, key, r.Options[key]); err != nil {
			return base, fmt.Errorf("stream %q: %w", l.Name, err)
		}
	}
	return next, nil
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
	out := transport.WatchNamesFor(capabilities.EngineGst, format)
	// A choice off this list reaches GstSource unexamined, so the narrowing must
	// not leave a transport without a watch form in it.
	for _, name := range out {
		assert.Assert(transport.CanWatch(name, capabilities.EngineGst), "an offered watch leg has a GStreamer watch form", name)
	}
	return out
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
