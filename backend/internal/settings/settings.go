// Package settings persists every user-controllable aspect of the product.
//
// JSON in the user's config directory (os.UserConfigDir: %APPDATA% on Windows, XDG_CONFIG_HOME or
// ~/.config on Linux).
//
// Three groups, split as the wire splits them (api/proto/screenshare/v1/settings.proto):
// where the relay is, what this machine publishes, how it watches.
// A deployment, a publisher and a viewer change at different times,
// and a machine that publishes nothing still holds the whole of its own group.
//
// Everything here round-trips through a file the user owns,
// so a value that comes back wrong is an Umgebungsfehler,
// repaired or refused and never asserted.
package settings

import (
	"fmt"
	"net"
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
// Read off the platform table rather than spelled here:
// which sources exist is that table's question and the absent one is a row of it
// (docs/domain-model.md, "The second-track capture sources").
// A constant typed here would be a second spelling, agreeing with the table until one was edited.
const audioSourceNone = platform.AudioSourceNone

// defaultAudioCodec encodes the track of a fresh stream and of a file written before the option.
// Opus is the one codec every transport here carries, WebRTC included,
// so a stored publish leg keeps working whatever protocol it names.
const defaultAudioCodec = "opus"

type Settings struct {
	Relay   Relay   `json:"relay"`
	Publish Publish `json:"publish"`
	Viewer  Viewer  `json:"viewer"`

	// streamName overrides what StreamName derives, for a stream this machine did not capture:
	// a synthetic test pattern (internal/publish/teststream.go), which no fact about Publish could
	// ever name.
	// Never on the wire and never stored: WithStreamName is the only way in, and no struct tag here
	// gives it a json key.
	streamName string
}

// WithStreamName is s publishing under name rather than under what StreamName would derive.
func (s Settings) WithStreamName(name string) Settings {
	assert.Assert(name != "", "an override still names a stream")

	s.streamName = name
	return s
}

// Relay is where the relay is and which of its listeners answers on which port.
// One port per protocol, all of them the relay's, so no field names the relay twice.
type Relay struct {
	Host       string `json:"host"`
	SrtPort    int    `json:"srtPort"`    // relay's SRT listener, UDP
	RtspPort   int    `json:"rtspPort"`   // relay's RTSP listener, TCP
	WebrtcPort int    `json:"webrtcPort"` // relay's WHIP+WHEP HTTP listener, TCP
	RtmpPort   int    `json:"rtmpPort"`   // relay's RTMP listener, TCP
	HlsPort    int    `json:"hlsPort"`    // relay's HLS HTTP listener, TCP
	// MoqPort is the relay's Media-over-QUIC listener, TCP and UDP on the one number:
	// the player page over HTTP/2, the WebTransport session over HTTP/3.
	//
	// Addressed on this port under Tls too, where the others are not.
	// No reverse proxy carries WebTransport,
	// so the relay terminates that leg itself wherever it runs (transport.MoQ).
	MoqPort int `json:"moqPort"`
	// GroupKey is the secret whose possession is membership of a group,
	// as the key service handed it over (internal/group).
	// Empty is a machine in no group.
	//
	// With the relay rather than with the publish:
	// it decides where every stream lives on that relay and not how one of them is encoded.
	// A preset is a publish group and nothing else,
	// so it carries no group and applying one cannot move a machine between them.
	GroupKey string `json:"groupKey,omitempty"`
	// DisplayName is what this machine calls itself in a group:
	// claimed on the first join, shown beside every stream it publishes,
	// and never identity, which is the member secret's job (internal/member).
	//
	// Empty is a machine with no name, and joining a group asks for one.
	// With the relay because a group is, and a preset carries neither.
	DisplayName string `json:"displayName,omitempty"`
	// DiscordMode has the group follow the voice channel this machine's linked account sits in.
	// While set, GroupKey is left unread:
	// membership, paths and passphrases come brokered from the Discord manager instead,
	// and the stored key waits for the toggle to go off (docs/discord-mode.md).
	DiscordMode bool `json:"discordMode,omitempty"`
	// DiscordLink names this install as a Discord account at the manager.
	// Drawn by the link flow, held like GroupKey and carrying the same trust:
	// whoever reads it watches this account's channels.
	// Empty is an install that has not linked.
	DiscordLink string `json:"discordLink,omitempty"`
	// brokered is the manager's answer standing in for the group key's derivations,
	// runtime like Token: written per pass or command (WithBrokeredGroup), never stored.
	// nil is Discord mode outside any voice channel, and every mode before injection.
	brokered *BrokeredGroup
	// Token is the relay credential the leg being built carries, and not a setting.
	//
	// A short-lived JWT the group service signed in exchange for GroupKey,
	// so it belongs to that service:
	// the json tag keeps it out of the store,
	// the control contract has no field for it,
	// and one place writes it (internal/app, settingsForCommand).
	// It rides in the snapshot, every serialization already reading the whole snapshot.
	//
	// Empty until the group service beside the relay issues one (GroupService).
	Token string `json:"-"`
}

// HTTPOrigin is where one of the relay's HTTP listeners answers:
// "https://relay.example.com", or "http://192.168.1.9:8888".
//
// The caller names the port, the relay serving each protocol on one of its own.
// Behind the proxy there is no such choice, one name on the standard port,
// so the direct port is dropped rather than carried into a URL nothing listens on.
// The host is not asserted:
// a stored value the migration repairs, not a contract between two functions here.
func (r Relay) HTTPOrigin(directPort int) string {
	if r.Tls() {
		return "https://" + r.Host
	}
	return fmt.Sprintf("http://%s:%d", r.Host, directPort)
}

// OnTrustedNetwork reports whether this relay sits on a network nobody else operates:
// this machine, or an address reserved for a private network.
//
// A name rather than an address answers false,
// resolving it being a question this cannot ask,
// and a guess in the wrong direction being a stream in the clear.
// "localhost" is the one name that is its own answer.
func (r Relay) OnTrustedNetwork() bool {
	if r.OnThisMachine() {
		return true
	}
	ip := net.ParseIP(r.Host)
	if ip == nil {
		return false
	}
	return ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// OnThisMachine reports whether the relay runs where this app does,
// the whole of where a listener the relay binds to loopback answers
// (deploy/mediamtx-groups.yml).
//
// A name rather than an address answers false, for the reason OnTrustedNetwork does,
// and "localhost" is the one name that is its own answer.
func (r Relay) OnThisMachine() bool {
	if r.Host == "localhost" {
		return true
	}
	ip := net.ParseIP(r.Host)
	return ip != nil && ip.IsLoopback()
}

// Tls says this relay's HTTP legs are reached through a TLS reverse proxy,
// under one name on the standard port, rather than on listeners of the relay's own
// (deploy/Caddyfile).
//
// Not whether the connection is encrypted.
// RTSP, RTMP and MoQ terminate TLS at the relay itself wherever it runs,
// so those legs are encrypted whichever way this answers
// (deploy/mediamtx-groups.yml, internal/transport).
// What it decides is an address: one name on 443, or a port per listener.
//
// Derived from the address and never stored, being no decision anybody makes:
// what puts a proxy in front of a relay is the network between it and here,
// so an address across somebody else's network is reached through that proxy,
// and an address this machine or this network reaches directly is reached on the relay's own ports.
// Held as a field it would be a second copy of a fact the host already carries,
// and the two would disagree the moment a host was edited:
// a stored "yes" beside an address on this network addresses listeners that are not there,
// and a stored "no" beside a public name asks the proxy's ports of a relay that has none open.
func (r Relay) Tls() bool {
	if r.Host == "" {
		// No relay is named, so there is no address to build either way.
		return false
	}
	return !r.OnTrustedNetwork()
}

// GroupServicePort is where groupd answers on a relay this network reaches directly,
// its own default (cmd/groupd, -listen).
// Behind the proxy there is no such port, the service's routes being paths under the one name.
const GroupServicePort = 9443

// GroupService is where group keys, relay tokens, membership and the stream index are answered,
// ok=false where no relay is named.
//
// Every relay this repository runs has one beside it,
// so both deployments are addresses rather than a presence and an absence:
// the proxy's own name, one certificate covering relay and service (deploy/Caddyfile),
// or the port groupd binds where the relay is reached directly (deploy/relay.sh).
//
// The relay refuses a publisher carrying no token,
// so a service this answered false for is a relay nothing can publish to.
func (r Relay) GroupService() (base string, ok bool) {
	if r.Host == "" {
		return "", false
	}
	if r.OnTrustedNetwork() {
		return fmt.Sprintf("http://%s:%d", r.Host, GroupServicePort), true
	}
	return "https://" + r.Host, true
}

// InGroup reports whether these settings state membership of a group.
//
// A key and a name together, membership being both:
// the key derives the prefix every path lives under,
// and the name is what this machine claims inside the group (internal/membership).
// A key with no name claimed states no presence,
// so the relay closes what it publishes on the next sweep,
// and a publish is refused ahead of that (internal/form, diagnosticsAboutTheAudience).
//
// A key that will not parse reads as membership here and reaches a path every relay refuses (Path),
// this being the settings a user typed rather than a fact this app can repair.
func (r Relay) InGroup() bool {
	if r.DiscordMode {
		return r.brokered != nil && r.DisplayName != ""
	}
	return r.GroupKey != "" && r.DisplayName != ""
}

// Path is where a stream of this name lives on the relay,
// which every transport builds its URL from.
//
// A group is a path prefix, so joining one is every path gaining it:
// the relay's own per-path permissions do the enforcing,
// and "which streams may I see" is a string match rather than a query its API cannot answer
// (docs/plan.md).
//
// Two answers, and which one applies is the deployment's rather than a preference:
//   - a group key, so the group's own prefix
//   - no key, or one that will not parse, so the bare name
//
// A stream lives in a group, so the bare name is under no prefix and every relay refuses it.
// A stored key that will not parse is an Umgebungsfehler and reaches it like an empty field:
// somebody who set a key meant to restrict who watches,
// and a damaged one is answered by a path nobody can open rather than by a wider audience.
// What keeps a machine outside a group from reaching here at all
// is the publish refusing before a path is built (internal/form, diagnosticsAboutTheAudience).
func (r Relay) Path(name string) string {
	// In Discord mode the brokered prefix is the group's, held to the same name rule,
	// and the stored key stays unread: paths must follow the voice channel alone.
	if r.DiscordMode {
		if r.brokered == nil || !group.NameHolds(name) {
			return name
		}
		return r.brokered.Prefix + name
	}
	if r.GroupKey == "" {
		return name
	}

	groupKey, err := group.ParseKey(r.GroupKey)
	if err != nil {
		return name
	}
	path, err := groupKey.Path(name)
	if err != nil {
		return name
	}
	return path
}

// StreamName is what a viewer's list shows this machine's own stream beside:
// this machine's own claim in its group and the stream's own name together, or the stream's own name
// alone outside a group, there being no claim to show it beside.
//
// On Settings and not on Publish or Relay alone, needing a fact from each:
// what is captured (Publish.Name) and what this machine is known by where it published it
// (Relay.DisplayName), which a preset carries the first of and never the second.
// Two machines choosing the same monitor still land on two names, DisplayName being claimed
// first-come and unique inside a group (internal/membership, ErrNameTaken).
//
// WithStreamName stands in front of Publish.Name for a stream no capture names,
// and the claim leads that too:
// a synthetic set carries one name per slot, so two machines in a group publish over each other
// under an unclaimed one.
func (s Settings) StreamName() string {
	name := s.Publish.Name()
	if s.streamName != "" {
		name = s.streamName
	}
	if s.Relay.DisplayName == "" {
		return name
	}
	return s.Relay.DisplayName + "/" + name
}

// PublishPath is where this machine's own stream lives on the relay,
// which every transport builds its publish URL from.
func (s Settings) PublishPath() string {
	return s.Relay.Path(s.StreamName())
}

// WatchPath is where a stream somebody asked to watch lives on the relay,
// which every transport builds its watch, receive and browser URL from.
//
// PublishPath's counterpart for a stream this machine did not capture.
// A shell names a stream the way a viewer's list carries it,
// inside the prefix this machine reaches under (wire.RelayStatus),
// and the relay serves it under that prefix,
// so the one derivation that puts the prefix back on is here rather than at each builder.
func (s Settings) WatchPath(streamName string) string {
	assert.Assert(streamName != "", "a watch path names the stream it opens")

	path := s.Relay.Path(streamName)
	assert.Assert(path != "", "a named stream reaches a path", streamName)
	return path
}

// SrtPassphrase keys this machine's SRT legs, derived and never stored or typed.
//
// SRT is the one leg no reverse proxy wraps, being UDP with no TLS,
// so a passphrase is the whole of what encrypts it.
// It follows the group key the way Path does,
// so the audience of the packets is the audience of the stream:
// the group's own derivation under its key,
// and none where Path answers the bare name every relay refuses.
// The relay's side of the same value is written per prefix by the group service
// (internal/groupsvc).
func (r Relay) SrtPassphrase() string {
	// Brokered beside the prefix it belongs to, the manager deriving both from the one key.
	if r.DiscordMode {
		if r.brokered == nil {
			return ""
		}
		return r.brokered.SrtPassphrase
	}
	if r.GroupKey == "" {
		return ""
	}

	groupKey, err := group.ParseKey(r.GroupKey)
	if err != nil {
		return ""
	}
	return groupKey.SrtPassphrase()
}

// Prefix leads every path this machine reaches, and is empty where Path answers a bare name.
//
// Read back off Path rather than restating the choice Path makes,
// between a group's prefix and none.
// Two statements of that rule drift, and the wrong one would be the one a viewer's list prints.
func (r Relay) Prefix() string {
	// One path segment, the shape Path puts a prefix in front of:
	// what comes back ahead of it is that prefix.
	const segment = "s"

	path := r.Path(segment)
	assert.Assert(strings.HasSuffix(path, segment),
		"a path is its prefix and the stream's own name, so it ends in the name it was built for", path)
	return strings.TrimSuffix(path, segment)
}

// Publish is what this machine sends to the relay and how it is encoded.
// A preset is one of these and nothing else.
type Publish struct {
	Transport string `json:"transport"` // publish leg, publisher to relay: a registry key, "srt"
	// Format is the bitstream every viewer decodes and a transport carries, "h264".
	// Encoder is what produces it on this machine, at the grain a picker offers one:
	// a family wherever that family is one encoder,
	// and the library where several share a family, "nvenc", "x264", "svt-av1".
	//
	// Two fields, the two questions having different answers on different machines:
	// a format outlives the encoder it was picked beside,
	// and an encoder outlives a change of format.
	// The row they address together is Codec, which is stored nowhere.
	Format  string `json:"format"`
	Encoder string `json:"encoder"`
	Mode    string `json:"mode"`   // rate control: cbr vbr abr crf lossless
	Chroma  string `json:"chroma"` // gbrp yuv444p yuv422p yuv420p p010le
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
	// FlatCodec is the one encoder name a file written before the pair carried,
	// read so the migration can split it into Format and Encoder (migrate.go).
	// Cleared there and omitted when empty, so a file that has been through one loses the key.
	FlatCodec string `json:"codec,omitempty"`
	// FlatAudio is the one source name a file written before the list carried,
	// read so the migration can turn it into the one entry (migrate.go).
	//
	// A field rather than a second pass over the bytes,
	// a stored preset being a publish group with no bytes of its own to re-read.
	// The migration clears it and it is omitted when empty,
	// so a file that has been through one loses the key.
	FlatAudio string `json:"audio,omitempty"`
	// AudioCodec encodes the mixed track, a row of capabilities.AudioCodecs.
	// Its own field rather than a property of a source, the two answering to different tables:
	// which sources exist is the platform's,
	// which codecs reach the relay the engine's and the publish leg's.
	// Read only where the list names at least one source.
	AudioCodec string `json:"audioCodec"`
	DrmMap     string `json:"drmMap"`  // kmsgrab DRM download strategy: auto vaapi vulkan none
	Monitor    int    `json:"monitor"` // ddagrab output_idx
	// CaptureMemory is where the frames reach the encoder:
	// auto, gpu or system, the values gpupath.Memories names.
	// Whether the capture chain downloads every frame and converts it on the CPU,
	// or hands the encoder the device memory the capture already produced.
	CaptureMemory string `json:"captureMemory"`
	// Cursor is what the pointer does in the captured frames, one of cursor.Modes.
	// Which of them a capture backend serves is that backend's own fact,
	// so a stored value the selected backend does not serve is repaired rather than passed on.
	Cursor string `json:"cursor"`
	// SrtPublishLatencyMs is this hop's SRT retransmit window, in ms.
	// Glass to glass is it plus the watch hop's (Viewer.SrtWatchLatencyMs) plus encode and decode:
	// two independent SRT links, each holding packets for its own window.
	SrtPublishLatencyMs int `json:"srtPublishLatencyMs"`
	// RtspPublishProtocol is this leg's RTP lower transport:
	// "tcp" interleaves every track over the RTSP connection the session already holds,
	// "udp" negotiates a port pair per track.
	// The watch leg names its own,
	// the two crossing different networks and the network deciding whether that pair survives.
	RtspPublishProtocol string `json:"rtspPublishProtocol"`
	UplinkMbps          int    `json:"uplinkMbps"` // Mbps of upload the user states, read for warnings
	// OutputResolution is the picture the encoder is fed, "1920x1080",
	// and empty where the capture's own size reaches it unscaled.
	//
	// One compound field rather than a width and a height, the user picking one thing:
	// two fields would be two controls only ever legal in pairs, which no form can say.
	// A string rather than a struct for the reason Chroma is one:
	// the legal values are a list the backend generates from the selected monitor,
	// so the only strings that arrive are ones this side wrote (api/proto/screenshare/v1).
	OutputResolution string `json:"outputResolution"`
}

// Viewer is how this machine watches, independent of what it publishes.
// The relay re-serves every ingested stream on all of its listeners,
// so a viewer receives over a leg chosen here rather than over the one the stream arrived on.
type Viewer struct {
	// TileWatchTransport is the leg a receive pipeline decodes from, WHEP included.
	// The only stored leg: an external player and a browser page are opened per press,
	// on a leg the call names, so neither has a value to keep.
	TileWatchTransport string `json:"tileWatchTransport"`
	// RtspWatchProtocol is the watch leg's RTP lower transport, "tcp" or "udp".
	// Both receivers read it: a player passes it to libavformat, a receive pipeline to rtspsrc.
	RtspWatchProtocol string `json:"rtspWatchProtocol"`
	// SrtWatchLatencyMs is the watch hop's SRT retransmit window in ms,
	// the second half of the pair Publish.SrtPublishLatencyMs holds the first of.
	SrtWatchLatencyMs int `json:"srtWatchLatencyMs"`
	// RtspWatchLatencyMs sizes a receive pipeline's jitter buffer in ms and reaches the tile alone.
	// An external player buffers by reorder queue rather than by time,
	// which is not the same knob under another name.
	RtspWatchLatencyMs int `json:"rtspWatchLatencyMs"`
	// RenderChain names the elements a receive pipeline converts decoded frames with,
	// one of the chains receive.Chains offers.
	// One value for every tile rather than one per stream:
	// a chain falls back where a driver cannot run it, which is a property of the machine.
	RenderChain string `json:"renderChain"`
	// PreviewRoute is the picture the broadcast preview draws, one of PreviewRoutes.
	//
	// With the viewer because the end-to-end route is a relay client:
	// it decodes this machine's own stream off the relay over TileWatchTransport
	// and takes a reader slot for it, where the local route reads a copy that stays here.
	// A preset is a publish group, so applying one leaves the picture where the reader put it.
	PreviewRoute string `json:"previewRoute"`
}

// Name is the stream's own name, past whatever this machine is known by in its group (Settings.StreamName):
// what a viewer's list shows this stream under, once the machine's own claim is peeled off.
//
// Derived from what is captured rather than typed, so a viewer's list needs no free text kept in step
// with what the picture actually is, and a second simultaneous stream a future capture adds gets its
// own name from its own capture rather than one two streams could type alike.
// Monitor is the whole of what is captured today, every backend reading captured surfaces off it
// (publish.gstcapture.go), so it is the whole of what the name is derived from.
func (p Publish) Name() string {
	return fmt.Sprintf("monitor-%d", p.Monitor)
}

// Codec is the encoder the format and the encoder fields name between them,
// as every engine, probe and log line spells it: "hevc_nvenc".
// Empty for a pair no row carries,
// which a draft nothing repaired can hold and every consumer handles as a codec outside the table
// (capabilities.Get).
func (p Publish) Codec() string {
	c, ok := capabilities.Row(p.Format, p.Encoder)
	if !ok {
		return ""
	}
	return c.Name
}

// UseCodec points the encode at one row of the capability table:
// the two fields written from the row that name addresses, which is Codec read backwards.
//
// The name is the caller's own rather than a stored one,
// so a name no row carries is an Entwicklungsfehler and asserts.
// What a settings file holds is the migration's question,
// and it answers it against the table before writing either field (migrate.go).
func (p *Publish) UseCodec(name string) {
	c, ok := capabilities.Get(name)
	assert.Assert(ok, "an encode is pointed at a row of the capability table", name)

	p.Format, p.Encoder = c.Format, c.Encoder()
}

// AudioTrack is the audio codec the publish leg has to carry:
// the configured one where the list names at least one source,
// capabilities.AudioNone where it names none.
// Both publish engines validate with it,
// so "no track" is one value both tables read rather than a branch each engine takes on its own.
//
// A list of nothing but muted sources still carries a track.
// Mute is a level and not a removal:
// the mixer keeps the branch, the stream keeps its track,
// and unmuting is a value written to a running pipeline rather than a relaunch.
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
// Neither is a source, so neither reaches a pipeline,
// and the repair is what takes them off a stored draft (form/repair.go).
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

// CapabilityOptions are the option values a codec's gaps are read against,
// keyed as capabilities.Options names them.
// Both publish engines hand it to capabilities.Validate,
// so one place decides which value each option was asked with, and the two cannot disagree.
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
	capture := "ddagrab"
	if runtime.GOOS != "windows" {
		capture = "x11grab"
	}

	d := Settings{
		Relay: Relay{
			// Ports: the listeners deploy/mediamtx-groups.yml binds,
			// that file being the configuration every relay runs.
			Host: "streamrelay.bjoernblessin.de", SrtPort: 8890,
			RtspPort: 8322, WebrtcPort: 8889, RtmpPort: 1936, HlsPort: 8888, MoqPort: 8892,
		},
		Publish: Publish{
			Transport: "srt", Format: "hevc", Encoder: capabilities.FamilyNvenc,
			Mode: "lossless", Chroma: "gbrp",
			ColorRange: "pc", Fps: 60, Cq: 19, BitrateM: 150, MaxrateM: 200, VbvMs: 0,
			Gop: 0, Bframes: 0,
			Capture: capture, DrmMap: "auto", Monitor: 0,
			// No source: a fresh installation publishes the picture alone,
			// so a first stream cannot put a room on the internet nobody meant to.
			AudioSources: nil, AudioCodec: defaultAudioCodec,
			CaptureMemory: gpupath.MemoryAuto,
			// Embedded is what a viewer expects:
			// a screen share whose pointer is missing reads as a broken capture rather than as a choice.
			Cursor:              cursor.Embedded,
			SrtPublishLatencyMs: 300, // ms, about the glass-to-glass budget with the watch hop below
			// Both legs start on TCP,
			// which asks nothing of the path beyond the connection the session already made.
			// UDP depends on its port pair crossing the same NAT and firewall, and never retransmits,
			// so what it fails as is a connected stream and no picture.
			RtspPublishProtocol: "tcp",
			UplinkMbps:          50,
		},
		Viewer: Viewer{
			// The one watch leg carrying every format this app publishes,
			// so a stream in any of them comes back on it,
			// and the measured way through the relay on it is an order of magnitude
			// shorter than SRT's or HLS's (docs/delay-measurement.md).
			TileWatchTransport: "rtsp",
			RtspWatchProtocol:  "tcp",
			// The publish hop's own window, the glass-to-glass budget being the two added.
			// The relay negotiates the larger of its 120 ms and this, so it is what the hop runs at.
			SrtWatchLatencyMs: 300,
			// rtspsrc's own default is 2000 ms, seconds of display delay above what a LAN needs.
			RtspWatchLatencyMs: 200,
			RenderChain:        receive.DefaultChain,
			// The picture that costs nothing beyond one decode here:
			// no uplink, no reader slot, and viewer figures that describe viewers.
			PreviewRoute: PreviewLocal,
		},
	}

	// Both ladder steps are read off the codec's own row rather than written here.
	// Where a mode starts is a fact about the encoder,
	// and a constant beside the codec name would be a second answer to it,
	// left on the old step the day the row moved that mode.
	d.Publish.Effort, d.Publish.Tune = LadderSteps(d.Publish.Codec(), d.Publish.Mode)

	assert.Assert(d.Publish.Name() != "" && d.Publish.Codec() != "" && d.Publish.Capture != "",
		"a fresh installation names what it publishes, what encodes it and what captures it",
		d.Publish.Name(), d.Publish.Format, d.Publish.Encoder, d.Publish.Capture)
	assert.Assert(d.Relay.Host != "", "a fresh installation names a relay to reach")
	return d
}

// LadderSteps is where a codec's mode starts on its effort and tune ladders,
// and the empty string for a ladder the codec does not declare.
//
// The one place a step is chosen for the user:
// a fresh installation takes them, and so does a draft whose codec changed,
// the ladders not corresponding, and a step carried across names a value the other encoder lacks.
//
// A codec no table knows is a stored value rather than a broken contract,
// so it yields two empty steps instead of asserting.
func LadderSteps(codec, mode string) (effort, tune string) {
	c, ok := capabilities.Get(codec)
	if !ok {
		return "", ""
	}
	effort, _ = c.Effort.StepFor(mode)
	tune, _ = c.Tune.StepFor(mode)
	return effort, tune
}
