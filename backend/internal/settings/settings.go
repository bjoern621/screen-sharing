// Package settings persists every user-controllable aspect of the product.
//
// JSON in the user's config directory (os.UserConfigDir: %APPDATA% on Windows, XDG_CONFIG_HOME or
// ~/.config on Linux).
//
// Three groups, split as the wire splits them (api/proto/screenshare/v1/settings.proto): where the
// relay is, what this machine publishes, how it watches.
// A deployment, a publisher and a viewer change at different times, and a machine that publishes
// nothing still holds the whole of its own group.
//
// Everything here round-trips through a file the user owns, so a value that comes back wrong is an
// Umgebungsfehler, repaired or refused and never asserted.
package settings

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/cursor"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/receive"
)

// audioSourceNone is the Audio value of a stream with no second track.
//
// Read off the platform table rather than spelled here: which sources exist is that table's
// question and the absent one is a row of it (docs/domain-model.md, "The second-track capture
// sources").
// A constant typed here would be a second spelling, agreeing with the table until one was edited.
const audioSourceNone = platform.AudioSourceNone

// defaultAudioCodec encodes the track of a fresh stream and of a file written before the option.
// Opus is the one codec every transport here carries, WebRTC included, so a stored publish leg
// keeps working whatever protocol it names.
const defaultAudioCodec = "opus"

type Settings struct {
	Relay   Relay   `json:"relay"`
	Publish Publish `json:"publish"`
	Viewer  Viewer  `json:"viewer"`
}

// Relay is where the relay is and which of its listeners answers on which port.
// One port per protocol, all of them the relay's, so no field names the relay twice.
type Relay struct {
	Host       string `json:"host"`
	SrtPort    int    `json:"srtPort"`    // relay's SRT listener, UDP
	ApiPort    int    `json:"apiPort"`    // relay's HTTP API, TCP
	RtspPort   int    `json:"rtspPort"`   // relay's RTSP listener, TCP
	WebrtcPort int    `json:"webrtcPort"` // relay's WHIP+WHEP HTTP listener, TCP
	RtmpPort   int    `json:"rtmpPort"`   // relay's RTMP listener, TCP
	HlsPort    int    `json:"hlsPort"`    // relay's HLS HTTP listener, TCP
	// MoqPort is the relay's Media-over-QUIC listener, TCP and UDP on the one number: the player page
	// over HTTP/2, the WebTransport session over HTTP/3.
	//
	// Addressed on this port under Tls too, where the others are not.
	// No reverse proxy carries WebTransport, so the relay terminates that leg itself wherever it runs
	// (transport.MoQ).
	MoqPort int `json:"moqPort"`
	// GroupKey is the secret whose possession is membership of a group, as the key service handed it
	// over (internal/group).
	// Empty is a machine in no group.
	//
	// With the relay rather than with the publish: it decides where every stream lives on that relay
	// and not how one of them is encoded.
	// A preset is a publish group and nothing else, so it carries no group and applying one cannot
	// move a machine between them.
	GroupKey string `json:"groupKey,omitempty"`
	// SrtPassphrase keys the relay-wide SRT listener, empty for a relay that takes none.
	//
	// SRT is the one leg no reverse proxy wraps, being UDP with no TLS, so what protects the packets
	// is a passphrase both ends hold.
	// The relay takes one value for every path through pathDefaults, hence one setting and not one
	// per stream.
	// GroupKey decides which streams a member reaches, this whether the packets are readable at all.
	SrtPassphrase string `json:"srtPassphrase,omitempty"`
	// Token is the relay credential the leg being built carries, and not a setting.
	//
	// A short-lived JWT the group service signed in exchange for GroupKey, so it belongs to that
	// service: the json tag keeps it out of the store, the control contract has no field for it, and
	// one place writes it (internal/app, settingsForCommand).
	// It rides in the snapshot because every serialization already reads the whole snapshot.
	//
	// Empty is a relay that authenticates nothing, which is what a LAN relay does.
	Token string `json:"-"`
}

// HTTPOrigin is where one of the relay's HTTP listeners answers: "https://relay.example.com",
// or "http://192.168.1.9:8888".
//
// The caller names the port, the relay serving each protocol on one of its own.
// Behind the proxy there is no such choice, one name on the standard port, so the direct port is
// dropped rather than carried into a URL nothing listens on.
// The host is not asserted: a stored value the migration repairs, not a contract between two
// functions here.
func (r Relay) HTTPOrigin(directPort int) string {
	if r.Tls() {
		return "https://" + r.Host
	}
	return fmt.Sprintf("http://%s:%d", r.Host, directPort)
}

// OnTrustedNetwork reports whether this relay is one the packets reach without crossing a network
// somebody else operates: this machine, or an address reserved for a private network.
//
// A name rather than an address answers false, resolving it being a question this cannot ask and a
// guess in the wrong direction being a stream in the clear.
// "localhost" is the one name that is its own answer.
func (r Relay) OnTrustedNetwork() bool {
	if r.Host == "localhost" {
		return true
	}
	ip := net.ParseIP(r.Host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// Tls says every leg to this relay is encrypted, and the HTTP ones reached through a TLS reverse
// proxy under one name on the standard port (deploy/Caddyfile).
//
// Derived from the address and never stored, because it is not a decision anybody makes: a relay
// across a network somebody else operates is encrypted, and one on this machine or this network is
// the LAN deployment in mediamtx.yml, which terminates nothing.
// Held as a field it would be a second copy of a fact the host already carries, and the two would
// disagree the moment a host was edited: a stored "yes" beside a LAN address addresses listeners
// that are not there, and a stored "no" beside a public name sends the picture in the clear.
//
// It also says whether a group service can be reached, that service answering on the proxy's name.
// A relay with no proxy has nowhere to ask for a token, and no port is invented to look on.
func (r Relay) Tls() bool {
	if r.Host == "" {
		// No relay is named, so there is no connection to encrypt.
		// Answering yes here would address TLS listeners of a host that does not exist.
		return false
	}
	return !r.OnTrustedNetwork()
}

// GroupService is where keys, tokens and the stream index are answered, ok=false where the
// deployment has none.
//
// The proxy's own name: one certificate covers relay and service, and the service's routes are
// paths under it (deploy/Caddyfile).
// A relay reached directly has no proxy and so no service.
// False rather than a guessed port, which would hang a token request on every publish.
func (r Relay) GroupService() (base string, ok bool) {
	if !r.Tls() || r.Host == "" {
		return "", false
	}
	return "https://" + r.Host, true
}

// Path is where a stream of this name lives on the relay, which every transport builds its URL
// from.
//
// A group is a path prefix, so joining one is every path gaining it: the relay's own per-path
// permissions do the enforcing, and "which streams may I see" is a string match rather than a query
// its API cannot answer (docs/plan.md).
//
// Three answers, and which one applies is the deployment's rather than a preference:
//   - a group key, so the group's own prefix
//   - no key at all on a relay that has a group service, so the public prefix, a stream anybody may
//     watch
//   - no key on a relay that has none, so the bare name, which is the LAN shape where the relay
//     authenticates nothing and there is no prefix to derive
//
// A stored key that will not parse is an Umgebungsfehler and yields the bare name, never the public
// prefix.
// The two keyless cases are not one: a field nobody filled in is a stream nobody restricted, and a
// key that came back damaged is a stream somebody meant to restrict, so widening its audience on
// the strength of a broken key would publish to everyone on the evidence that something is wrong.
// The bare name is refused by a relay that authenticates, which is the outcome that keeps the
// stream off the public prefix.
func (r Relay) Path(name string) string {
	if r.GroupKey != "" {
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

	if _, hasService := r.GroupService(); !hasService {
		return name
	}
	path, err := group.PublicPath(name)
	if err != nil {
		return name
	}
	return path
}

// Prefix leads every path this machine reaches, and is empty where Path answers a bare name.
//
// Read back off Path rather than restating the choice Path makes between a group's prefix, the
// public one and none.
// Two statements of that rule drift, and the wrong one would be the one a viewer's list prints.
func (r Relay) Prefix() string {
	// One path segment, the shape Path puts a prefix in front of: what comes back ahead of it is
	// that prefix.
	const segment = "s"

	path := r.Path(segment)
	assert.Assert(strings.HasSuffix(path, segment),
		"a path is its prefix and the stream's own name, so it ends in the name it was built for", path)
	return strings.TrimSuffix(path, segment)
}

// Publish is what this machine sends to the relay and how it is encoded.
// A preset is one of these and nothing else.
type Publish struct {
	Name      string `json:"name"`
	Transport string `json:"transport"` // publish leg, publisher to relay: a registry key, "srt"
	Codec     string `json:"codec"`     // ffmpeg encoder name, a row of capabilities.Codecs
	Mode      string `json:"mode"`      // rate control: cbr vbr abr crf lossless
	Chroma    string `json:"chroma"`    // gbrp yuv444p yuv422p yuv420p p010le
	// ColorRange is pc or tv, and is ignored for gbrp, which is full range by construction.
	ColorRange string `json:"colorRange"`
	Fps        int    `json:"fps"`
	Cq         int    `json:"cq"`       // crf mode: quantizer target, lower is better
	BitrateM   int    `json:"bitrateM"` // Mbps target, cbr/vbr/abr
	MaxrateM   int    `json:"maxrateM"` // Mbps burst ceiling above the target, vbr
	VbvMs      int    `json:"vbvMs"`    // VBV/rate buffer in ms, cbr/vbr, 0 = encoder default
	Gop        int    `json:"gop"`      // keyframe interval in frames, 0 = auto (2*fps)
	Bframes    int    `json:"bframes"`  // lossy modes only, adds reorder latency
	// Effort is the step the encoder works at on its own ladder, Tune what it works towards.
	// Both hold the encoder's own identifier, "slow", "9", "p7", off the ladder its capability row
	// declares, rather than a number normalized across codecs that would land somewhere else on each.
	Effort  string `json:"effort"`
	Tune    string `json:"tune"`
	Capture string `json:"capture"` // a row of publish.Captures, applicable per OS and session
	// AudioSources are what the second track is mixed from, in the order a form draws them.
	// An empty list is a stream with no second track.
	AudioSources []AudioSource `json:"audioSources"`
	// LegacyAudio is the one source name a file written before the list carried, read so the
	// migration can turn it into the one entry (migrate.go).
	//
	// A field rather than a second pass over the bytes, a stored preset being a publish group with no
	// bytes of its own to re-read.
	// The migration clears it and it is omitted when empty, so a file that has been through one loses
	// the key.
	LegacyAudio string `json:"audio,omitempty"`
	// AudioCodec encodes the mixed track, a row of capabilities.AudioCodecs.
	// Its own field rather than a property of a source, the two answering to different tables:
	// which sources exist is the platform's, which codecs reach the relay the engine's and the
	// publish leg's.
	// Read only where the list names at least one source.
	AudioCodec string `json:"audioCodec"`
	DrmMap     string `json:"drmMap"`  // kmsgrab DRM download strategy: auto vaapi vulkan none
	Monitor    int    `json:"monitor"` // ddagrab output_idx
	// CaptureMemory is where the frames reach the encoder: auto, gpu or system, the values
	// gpupath.Memories names.
	// Whether the capture chain downloads every frame and converts it on the CPU, or hands the
	// encoder the device memory the capture already produced.
	CaptureMemory string `json:"captureMemory"`
	// Cursor is what the pointer does in the captured frames, one of cursor.Modes.
	// Which of them a capture backend serves is that backend's own fact, so a stored value the
	// selected backend does not serve is repaired rather than passed on.
	Cursor string `json:"cursor"`
	// SrtPublishLatencyMs is this hop's SRT retransmit window, in ms.
	// Glass to glass is it plus the watch hop's (Viewer.SrtWatchLatencyMs) plus encode and decode:
	// two independent SRT links, each holding packets for its own window.
	SrtPublishLatencyMs int `json:"srtPublishLatencyMs"`
	// RtspPublishProtocol is this leg's RTP lower transport: "tcp" interleaves every track over the
	// RTSP connection the session already holds, "udp" negotiates a port pair per track.
	// The watch leg names its own, the two crossing different networks and the network deciding
	// whether that pair survives.
	RtspPublishProtocol string `json:"rtspPublishProtocol"`
	UplinkMbps          int    `json:"uplinkMbps"` // Mbps of upload the user states, read for warnings
	// OutputResolution is the picture the encoder is fed, "1920x1080", and empty where the capture's
	// own size reaches it unscaled.
	//
	// One compound field rather than a width and a height, the user picking one thing: two fields
	// would be two controls only ever legal in pairs, which no form can say.
	// A string rather than a struct for the reason Chroma is one: the legal values are a list the
	// backend generates from the selected monitor, so the only strings that arrive are ones this side
	// wrote (api/proto/screenshare/v1).
	OutputResolution string `json:"outputResolution"`
}

// Viewer is how this machine watches, independent of what it publishes.
// The relay re-serves every ingested stream on all of its listeners, so a viewer receives over a
// leg chosen here rather than over the one the stream arrived on.
type Viewer struct {
	// TileWatchTransport is the leg a receive pipeline decodes from, WHEP included.
	// The only stored leg: an external player and a browser page are opened per press on a leg the
	// call names, so neither has a value to keep.
	TileWatchTransport string `json:"tileWatchTransport"`
	// RtspWatchProtocol is the watch leg's RTP lower transport, "tcp" or "udp".
	// Both receivers read it: a player passes it to libavformat, a receive pipeline to rtspsrc.
	RtspWatchProtocol string `json:"rtspWatchProtocol"`
	// SrtWatchLatencyMs is the watch hop's SRT retransmit window in ms, the second half of the pair
	// Publish.SrtPublishLatencyMs holds the first of.
	SrtWatchLatencyMs int `json:"srtWatchLatencyMs"`
	// RtspWatchLatencyMs sizes a receive pipeline's jitter buffer in ms and reaches the tile alone.
	// An external player buffers by reorder queue rather than by time, which is not the same knob
	// under another name.
	RtspWatchLatencyMs int `json:"rtspWatchLatencyMs"`
	// RenderChain names the elements a receive pipeline converts decoded frames with, one of the
	// chains receive.Chains offers.
	// One value for every tile rather than one per stream: a chain falls back because a driver cannot
	// run it, which is a property of the machine.
	RenderChain string `json:"renderChain"`
}

// AudioTrack is the audio codec the publish leg has to carry: the configured one where the list
// names at least one source, capabilities.AudioNone where it names none.
// Both publish engines validate with it, so "no track" is one value both tables read rather than a
// branch each engine takes on its own.
//
// A list of nothing but muted sources still carries a track.
// Mute is a level and not a removal: the mixer keeps the branch, the stream keeps its track, and
// unmuting is a value written to a running pipeline rather than a relaunch.
func (p Publish) AudioTrack() string {
	if len(p.Recorded()) == 0 {
		return capabilities.AudioNone
	}
	return p.AudioCodec
}

// Recorded is the sources that produce a branch, which is every entry naming a kind.
//
// An entry naming none is the row a form draws past the end of the list for a reader to grow it by,
// and is what an entry set back to no source becomes.
// Neither is a source, so neither reaches a pipeline, and the repair is what takes them off a
// stored draft (form/repair.go).
func (p Publish) Recorded() []AudioSource {
	out := make([]AudioSource, 0, len(p.AudioSources))
	for _, a := range p.AudioSources {
		if a.Records() {
			out = append(out, a)
		}
	}

	assert.Assert(len(out) <= len(p.AudioSources),
		"the recorded sources are a subset of the configured ones", len(out), len(p.AudioSources))
	return out
}

// CapabilityOptions are the option values a codec's gaps are read against, keyed as
// capabilities.Options names them.
// Both publish engines hand it to capabilities.Validate, so one place decides which value each
// option was asked with and the two cannot answer differently.
func (p Publish) CapabilityOptions() map[string]string {
	return map[string]string{
		capabilities.OptionChroma:     p.Chroma,
		capabilities.OptionMode:       p.Mode,
		capabilities.OptionColorRange: p.ColorRange,
		capabilities.OptionTune:       p.Tune,
	}
}

// Defaults is what a fresh installation starts with.
// The capture backend is the one this OS runs.
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
			RtspPort: 8554, WebrtcPort: 8889, RtmpPort: 1935, HlsPort: 8888, MoqPort: 8892,
		},
		Publish: Publish{
			Name: host, Transport: "srt", Codec: "hevc_nvenc", Mode: "lossless", Chroma: "gbrp",
			ColorRange: "pc", Fps: 60, Cq: 19, BitrateM: 150, MaxrateM: 200, VbvMs: 0,
			Gop: 0, Bframes: 0,
			Capture: capture, DrmMap: "auto", Monitor: 0,
			// No source: a fresh installation publishes the picture alone, so a first stream cannot put a
			// room on the internet nobody meant to.
			AudioSources: nil, AudioCodec: defaultAudioCodec,
			CaptureMemory: gpupath.MemoryAuto,
			// Embedded is what a viewer expects: a screen share whose pointer is missing reads as a broken
			// capture rather than as a choice somebody made.
			Cursor:              cursor.Embedded,
			SrtPublishLatencyMs: 300, // ms, about the glass-to-glass budget with the watch hop below
			// Both legs start on TCP, which asks nothing of the path beyond the connection the session
			// already made.
			// UDP depends on its port pair crossing the same NAT and firewall and never retransmits, so
			// what it fails as is a connected stream and no picture.
			RtspPublishProtocol: "tcp",
			UplinkMbps:          50,
		},
		Viewer: Viewer{
			TileWatchTransport: "srt",
			RtspWatchProtocol:  "tcp",
			SrtWatchLatencyMs:  1200,
			// rtspsrc's own default is 2000 ms, seconds of display delay above what a LAN needs.
			RtspWatchLatencyMs: 200,
			RenderChain:        receive.DefaultChain,
		},
	}

	// Both ladder steps are read off the codec's own row rather than written here.
	// Where a mode starts is a fact about the encoder, and a constant beside the codec name would be
	// a second answer to it, left on the old step the day the row moved that mode.
	d.Publish.Effort, d.Publish.Tune = LadderSteps(d.Publish.Codec, d.Publish.Mode)

	assert.Assert(d.Publish.Name != "" && d.Publish.Codec != "" && d.Publish.Capture != "",
		"a fresh installation names what it publishes, what encodes it and what captures it",
		d.Publish.Name, d.Publish.Codec, d.Publish.Capture)
	assert.Assert(d.Relay.Host != "", "a fresh installation names a relay to reach")
	return d
}

// LadderSteps is where a codec's mode starts on its effort and tune ladders, and the empty string
// for a ladder the codec does not declare.
//
// The one place a step is chosen for the user: a fresh installation takes them, and so does a draft
// whose codec changed, the ladders not corresponding and a step carried across naming a value the
// new encoder never heard of.
//
// A codec no table knows is a stored value rather than a broken contract, so it yields two empty
// steps instead of asserting.
func LadderSteps(codec, mode string) (effort, tune string) {
	c, ok := capabilities.Get(codec)
	if !ok {
		return "", ""
	}
	effort, _ = c.Effort.StepFor(mode)
	tune, _ = c.Tune.StepFor(mode)
	return effort, tune
}
