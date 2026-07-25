package roster

import "fmt"

// DemoTransport labels the built-in streams in the stats overlay, where a real
// roster carries the transport its source fragment speaks.
const DemoTransport = "demo"

// demoStream is one entry of the standalone roster: either a videotestsrc
// pattern encoded like a real stream, or a raw source fragment for the cases a
// pattern cannot express.
type demoStream struct {
	name    string
	pattern string
	// audio muxes a quiet sine beside the test pattern, so decodebin exposes an
	// audio pad and the tile's volume control has something to drive.
	audio bool
	// source replaces the generated fragment, for an entry that is not a
	// pattern.
	source string
}

// demoStreams is the standalone stand-in for the app's -config: patterns behind
// the same Stream contract, one of them with audio, plus a broken source to
// exercise the failure path.
var demoStreams = []demoStream{
	{name: "bjoern-desk", pattern: "smpte"},
	{name: "lab-cam", pattern: "ball", audio: true},
	{name: "build-wall", pattern: "gradient"},
	{name: "kitchen-pi", pattern: "pinwheel"},
	{name: "studio-b", pattern: "spokes"},
	{name: "talkback", source: "brokensrc"},
}

// DemoConfig is the roster a run without -config opens on.
func DemoConfig() Config {
	cfg := Config{Streams: make([]Stream, 0, len(demoStreams))}
	for _, d := range demoStreams {
		cfg.Streams = append(cfg.Streams, Stream{
			Name:      d.name,
			Transport: DemoTransport,
			Source:    d.fragment(),
		})
	}
	return cfg
}

// fragment renders one demo entry's source elements. The pattern is encoded
// rather than played raw, because a raw stream exercises none of the figures
// the stats overlay reads off the encoded side: no decoder, no bitrate, no
// keyframe spacing. ultrafast keeps concurrent encodes cheap. An audio entry
// muxes both raw tracks through Matroska, which decodebin demuxes back into a
// video and an audio pad.
func (d demoStream) fragment() string {
	if d.source != "" {
		return d.source
	}
	video := fmt.Sprintf(
		"videotestsrc is-live=true pattern=%s"+
			" ! video/x-raw,width=1280,height=720,framerate=30/1"+
			" ! x264enc speed-preset=ultrafast tune=zerolatency key-int-max=60"+
			" ! h264parse",
		d.pattern)
	if !d.audio {
		return video
	}
	return video + " ! mux." +
		" audiotestsrc is-live=true freq=440 volume=0.1" +
		" ! audio/x-raw,format=S16LE,rate=48000,channels=2 ! opusenc ! mux." +
		" matroskamux name=mux streamable=true"
}
