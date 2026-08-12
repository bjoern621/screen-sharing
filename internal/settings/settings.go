// Package settings persists every user-controllable aspect of the product.
//
// Settings are stored as JSON in the user's config directory
// (os.UserConfigDir: %APPDATA% on Windows, XDG_CONFIG_HOME/~/.config on Linux).
//
// They divide into three groups, and the division is the wire's
// (api/proto/screenshare/v1/settings.proto): where the relay is, what this machine
// publishes, and how this machine watches. The three answer to different things and
// change at different times - a deployment, a publisher, a viewer - and a viewer that
// publishes nothing still has the whole of its own group.
package settings

import (
	"os"
	"runtime"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/cursor"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/receive"
)

// audioSourceNone is the Audio value of a stream with no second track.
//
// It is read from the platform table rather than spelled here, because which sources
// exist is that table's question and the absent one is a row of it: a constant typed in
// this package would be a second spelling of a value the table produces, and the two
// would agree until one of them was edited (docs/domain-model.md, "The second-track
// capture sources").
const audioSourceNone = platform.AudioSourceNone

// defaultAudioCodec is the codec a fresh stream and a file written before the
// option encode their track in. Opus is the one codec every transport here
// carries, WebRTC included, so it is the value that keeps a stored publish leg
// working whatever protocol it names.
const defaultAudioCodec = "opus"

// Settings is the three groups together: what a shell holds, what a resolve is
// computed over, and what the store round-trips.
type Settings struct {
	Relay   Relay   `json:"relay"`
	Publish Publish `json:"publish"`
	Viewer  Viewer  `json:"viewer"`
}

// Relay is where the relay is and which of its listeners are on which port.
//
// Every port here is a port on the relay, so none of them names the relay twice. One
// host and one port per protocol, because the relay serves each on its own.
type Relay struct {
	Host       string `json:"host"`
	SrtPort    int    `json:"srtPort"`    // UDP port of the relay's SRT listener
	ApiPort    int    `json:"apiPort"`    // TCP port of the relay's HTTP API
	RtspPort   int    `json:"rtspPort"`   // TCP port of the relay's RTSP listener
	WebrtcPort int    `json:"webrtcPort"` // TCP port of the relay's WebRTC/WHIP+WHEP HTTP listener
	RtmpPort   int    `json:"rtmpPort"`   // TCP port of the relay's RTMP listener
	HlsPort    int    `json:"hlsPort"`    // TCP port of the relay's HLS HTTP listener
	// GroupKey is the secret whose possession is membership of a group, as the key service
	// handed it over (internal/group). Empty is a machine that has joined none.
	//
	// It sits with the relay rather than with the publish, because it decides where every
	// stream lives on that relay and not how any one of them is encoded: a preset is a
	// publish group and nothing else, so a saved preset carries no group and applying one
	// cannot move a machine between them.
	GroupKey string `json:"groupKey,omitempty"`
	// SrtPassphrase keys the relay-wide SRT listener, and is empty for a relay that takes
	// none.
	//
	// SRT is the one leg no reverse proxy can wrap - it is UDP with no TLS - so what
	// protects the packets on the wire is a passphrase both ends hold. The relay takes one
	// value for every path through pathDefaults, so this is one setting rather than one per
	// stream, and it protects a different thing from the group key above: that one decides
	// which streams a member reaches, and this whether the packets are readable at all.
	SrtPassphrase string `json:"srtPassphrase,omitempty"`
}

// Path is where a stream of this name lives on the relay, which every transport builds its
// URL from.
//
// A group is a path prefix, so the whole of joining one is that every path gains it: the
// relay's own per-path permissions then do the enforcing, and "which streams may I see" is a
// string match rather than a query its API cannot answer (docs/plan.md).
//
// A machine in no group publishes under the bare name, which is what every stream did before
// groups existed and what a relay with no auth configured still serves. What makes a group
// required is the relay refusing an unauthenticated publish, not this function inventing a
// prefix nobody can obtain a key for.
func (r Relay) Path(name string) string {
	key, err := group.ParseKey(r.GroupKey)
	if err != nil {
		return name
	}
	path, err := key.Path(name)
	if err != nil {
		return name
	}
	return path
}

// Publish is what this machine sends to the relay and how it is encoded. A preset is
// one of these and nothing else.
type Publish struct {
	Name      string `json:"name"`
	Transport string `json:"transport"` // publish leg (publisher to relay): registry key, e.g. "srt"
	Codec     string `json:"codec"`     // ffmpeg encoder name, a row of capabilities.Codecs
	Mode      string `json:"mode"`      // rate control: cbr vbr abr crf lossless
	Chroma    string `json:"chroma"`    // gbrp yuv444p yuv422p yuv420p p010le
	// ColorRange is pc or tv, and is ignored for gbrp, which is inherently full range.
	ColorRange string `json:"colorRange"`
	Fps        int    `json:"fps"`
	Cq         int    `json:"cq"`       // crf mode: constant-quality value, lower = better
	BitrateM   int    `json:"bitrateM"` // Mbps: target for cbr/vbr/abr
	MaxrateM   int    `json:"maxrateM"` // Mbps: vbr burst ceiling above the target
	VbvMs      int    `json:"vbvMs"`    // VBV/rate buffer in ms for cbr/vbr, 0 = encoder default
	Gop        int    `json:"gop"`      // keyframe interval in frames, 0 = auto (2*fps)
	Bframes    int    `json:"bframes"`  // lossy modes only; adds reorder latency
	// Effort is the step the encoder works at on its own ladder, and Tune what it
	// optimizes for. Both hold the encoder's own identifier - "slow", "9", "p7" - taken
	// from the ladder its capability row declares, so a value here is one that encoder
	// takes rather than a number normalized across codecs that would mean something else
	// on each.
	Effort  string `json:"effort"`
	Tune    string `json:"tune"`
	Capture string `json:"capture"` // a row of publish.Captures, applicable per OS and session
	// AudioSources are what the second track is mixed from, in the order a form draws
	// them. An empty list is a stream with no second track.
	AudioSources []AudioSource `json:"audioSources"`
	// LegacyAudio is the one source name a file written before the list carried, read so
	// the migration can turn it into the one entry (migrate.go).
	//
	// A field rather than a second pass over the bytes, because a stored preset is a
	// publish group with no bytes of its own to re-read. It is cleared by the migration
	// and omitted when empty, so a file that has been through one loses the key and a
	// file that has not keeps its value until it does.
	LegacyAudio string `json:"audio,omitempty"`
	// AudioCodec is the codec the mixed track is encoded in, a row of
	// capabilities.AudioCodecs. It is a field of its own rather than a property of
	// a source because the two answer to different tables: which sources exist is
	// the platform's, which codecs reach the relay is the engine's and the publish
	// leg's. It is read only where the list names at least one source.
	AudioCodec string `json:"audioCodec"`
	DrmMap     string `json:"drmMap"`  // kmsgrab DRM download strategy: auto vaapi vulkan none
	Monitor    int    `json:"monitor"` // ddagrab output_idx
	// CaptureMemory is where the frames reach the encoder: auto gpu system, the
	// values gpupath.Memories names. It decides whether the capture chain downloads
	// every frame and converts it on the CPU, or hands the encoder the device memory
	// the capture already produced.
	CaptureMemory string `json:"captureMemory"`
	// Cursor is what the pointer does in the captured frames, one of cursor.Modes.
	// Which of them a capture backend serves is that backend's own fact, so a stored
	// value the selected backend does not serve is repaired rather than passed on.
	Cursor string `json:"cursor"`
	// SrtPublishLatencyMs is this hop's SRT retransmit window. Glass-to-glass delay is
	// the sum of it and the watch hop's (Viewer.SrtWatchLatencyMs) plus encode and
	// decode: the two are independent SRT links, each holding packets for its own
	// window.
	SrtPublishLatencyMs int `json:"srtPublishLatencyMs"`
	// RtspPublishProtocol is this leg's RTP lower transport: "tcp" interleaves every
	// track over the RTSP connection the session already holds, "udp" negotiates a
	// port pair per track. The watch leg names its own, because the two cross
	// different networks and it is the network that decides whether that pair
	// survives.
	RtspPublishProtocol string `json:"rtspPublishProtocol"`
	UplinkMbps          int    `json:"uplinkMbps"` // user's known upload capacity, used for warnings only
	// OutputResolution is the picture the encoder is fed, as "WIDTHxHEIGHT", and the
	// empty string where the capture's own size reaches it unscaled.
	//
	// One compound field rather than a width and a height, because the user picks one
	// thing: two fields would be two controls that are only ever legal in pairs, and no
	// form can say that. A string rather than a struct for the reason Chroma is one:
	// the legal values are a list the backend generates from the selected monitor, so
	// the only strings that arrive are ones this side wrote (api/proto/screenshare/v1).
	OutputResolution string `json:"outputResolution"`
}

// Viewer is how this machine watches, which is independent of what it publishes: the
// relay re-serves every ingested stream on all its listeners, so a viewer receives over
// a leg chosen here rather than over the one the stream arrived on.
type Viewer struct {
	// PlayerWatchTransport is the leg an external player opens, narrowed to the
	// protocols a player reaches by URL.
	PlayerWatchTransport string `json:"playerWatchTransport"`
	// TileWatchTransport is the leg a receive pipeline decodes from, which also reaches
	// WHEP. It is a field of its own rather than the player's because the two receivers
	// reach different protocol sets: a receive pipeline reaches WHEP, which no player
	// URL expresses, while a player opens the relay's HLS, which nothing here decodes.
	// One field would leave each viewer able to store a leg the other cannot run.
	TileWatchTransport string `json:"tileWatchTransport"`
	// RtspWatchProtocol is the watch leg's RTP lower transport, "tcp" or "udp". Both
	// receivers read it: a player passes it to libavformat, a receive pipeline to
	// rtspsrc.
	RtspWatchProtocol string `json:"rtspWatchProtocol"`
	// SrtWatchLatencyMs is the watch hop's SRT retransmit window, the second half of the
	// pair Publish.SrtPublishLatencyMs holds the first of.
	SrtWatchLatencyMs int `json:"srtWatchLatencyMs"`
	// RtspWatchLatencyMs sizes a receive pipeline's jitter buffer in milliseconds and
	// reaches the tile alone: an external player buffers by reorder queue rather than by
	// time, which is not the same knob under another name.
	RtspWatchLatencyMs int `json:"rtspWatchLatencyMs"`
	// RenderChain names the elements a receive pipeline converts decoded frames with,
	// one of the chains receive.Chains offers. It is one value for every tile rather
	// than one per stream: a chain falls back because a driver cannot run it, and that
	// is a property of the machine.
	RenderChain string `json:"renderChain"`
}

// AudioTrack is the audio codec the publish leg has to carry: the configured one where
// the list names at least one source, and capabilities.AudioNone where it names none.
// Both publish engines validate with it, so "no track" is one value both tables read
// rather than a branch each engine takes on its own.
//
// A list of nothing but muted sources still carries a track. Mute is a level and not a
// removal: the mixer keeps the branch, the stream keeps its track, and unmuting is a
// value written to a pipeline that is already running rather than a relaunch.
func (p Publish) AudioTrack() string {
	if len(p.Recorded()) == 0 {
		return capabilities.AudioNone
	}
	return p.AudioCodec
}

// Recorded is the sources that produce a branch, which is every entry naming a kind.
//
// An entry naming none is the row a form draws at the end of the list for a reader to
// grow it by, and it is what an entry set back to no source becomes. Neither is a source,
// so neither reaches a pipeline; the repair is what takes them off a stored draft
// (form/repair.go).
func (p Publish) Recorded() []AudioSource {
	out := make([]AudioSource, 0, len(p.AudioSources))
	for _, a := range p.AudioSources {
		if a.Records() {
			out = append(out, a)
		}
	}
	return out
}

// CapabilityOptions are the option values a codec's gaps are read against, keyed as
// capabilities.Options names them. Both publish engines hand it to
// capabilities.Validate, so one place decides which value each option was asked
// with and the two engines cannot answer differently.
func (p Publish) CapabilityOptions() map[string]string {
	return map[string]string{
		capabilities.OptionChroma:     p.Chroma,
		capabilities.OptionMode:       p.Mode,
		capabilities.OptionColorRange: p.ColorRange,
	}
}

// Defaults returns the settings a fresh installation starts with.
// The capture backend is chosen per OS.
func Defaults() Settings {
	host, err := os.Hostname()
	if err != nil {
		host = "me"
	}

	capture := "ddagrab"
	if runtime.GOOS != "windows" {
		capture = "x11grab"
	}

	d := Settings{
		Relay: Relay{
			Host: "streamrelay.bjoernblessin.de", SrtPort: 8890, ApiPort: 9997,
			RtspPort: 8554, WebrtcPort: 8889, RtmpPort: 1935, HlsPort: 8888,
		},
		Publish: Publish{
			Name: host, Transport: "srt", Codec: "hevc_nvenc", Mode: "lossless", Chroma: "gbrp",
			ColorRange: "pc", Fps: 60, Cq: 19, BitrateM: 150, MaxrateM: 200, VbvMs: 0,
			Gop: 0, Bframes: 0,
			Capture: capture, DrmMap: "auto", Monitor: 0,
			// No source: a fresh installation publishes the picture and nothing else, so a
			// first stream cannot put a room on the internet nobody meant to.
			AudioSources: nil, AudioCodec: defaultAudioCodec,
			CaptureMemory: gpupath.MemoryAuto,
			// Embedded is what every backend but kmsgrab did before the setting existed,
			// and it is what a viewer expects: a screen share whose pointer is missing
			// reads as a broken capture rather than as a choice somebody made.
			Cursor:              cursor.Embedded,
			SrtPublishLatencyMs: 300, // with the watch hop below, ≈ the glass-to-glass budget
			// Both legs start on TCP because it asks nothing of the path beyond the
			// connection the session already made, where the UDP alternative depends on
			// its port pair crossing the same NAT and firewall and never retransmits:
			// the failure it produces is a connected stream and no picture.
			RtspPublishProtocol: "tcp",
			UplinkMbps:          50,
		},
		Viewer: Viewer{
			PlayerWatchTransport: "srt",
			TileWatchTransport:   "srt",
			RtspWatchProtocol:    "tcp",
			SrtWatchLatencyMs:    1200,
			// rtspsrc defaults to 2000 ms of jitter buffer, two seconds of display delay
			// above what a LAN needs.
			RtspWatchLatencyMs: 200,
			RenderChain:        receive.DefaultChain,
		},
	}

	// The two ladder steps are read off the codec's own row rather than written here.
	// Where each mode starts is a fact about the encoder, and a constant beside the codec
	// name would be a second answer to it: the day the row moved a mode to another step,
	// a fresh installation would keep starting on the old one with nothing saying so.
	d.Publish.Effort, d.Publish.Tune = LadderSteps(d.Publish.Codec, d.Publish.Mode)
	return d
}

// LadderSteps is where a codec's mode starts on its effort and tune ladders, and the empty
// string for a ladder the codec does not declare.
//
// It is the one place a step is chosen for the user: a fresh installation takes them, and
// so does a draft whose codec changed, since the ladders do not correspond and a step
// carried across would name a value the new encoder never heard of.
func LadderSteps(codec, mode string) (effort, tune string) {
	c, ok := capabilities.Get(codec)
	if !ok {
		return "", ""
	}
	effort, _ = c.Effort.StepFor(mode)
	tune, _ = c.Tune.StepFor(mode)
	return effort, tune
}
