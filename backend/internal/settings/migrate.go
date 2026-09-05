package settings

import (
	"encoding/json"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
)

// flat is the settings shape from before the three groups:
// one object with every key at the top level, the leg spelled into the field names that needed it.
//
// Kept so a stored file survives the split rather than reading as a first start.
// Only the fields that moved or were renamed sit here.
// Everything else keeps its key inside its group and the ordinary decode already found it,
// so this is a second pass over the same bytes rather than a second struct for the whole file.
type flat struct {
	RelayHost  *string `json:"relayHost"`
	RelayPort  *int    `json:"relayPort"`
	RtspPort   *int    `json:"rtspPort"`
	WebrtcPort *int    `json:"webrtcPort"`
	RtmpPort   *int    `json:"rtmpPort"`
	HlsPort    *int    `json:"hlsPort"`

	Transport           *string `json:"transport"`
	Codec               *string `json:"codec"`
	Mode                *string `json:"mode"`
	Chroma              *string `json:"chroma"`
	ColorRange          *string `json:"colorRange"`
	Fps                 *int    `json:"fps"`
	Cq                  *int    `json:"cq"`
	BitrateM            *int    `json:"bitrateM"`
	MaxrateM            *int    `json:"maxrateM"`
	VbvMs               *int    `json:"vbvMs"`
	Gop                 *int    `json:"gop"`
	Bframes             *int    `json:"bframes"`
	Effort              *string `json:"effort"`
	Capture             *string `json:"capture"`
	Audio               *string `json:"audio"`
	AudioCodec          *string `json:"audioCodec"`
	DrmMap              *string `json:"drmMap"`
	Monitor             *int    `json:"monitor"`
	CaptureMemory       *string `json:"captureMemory"`
	SrtPublishLatencyMs *int    `json:"srtPublishLatencyMs"`
	RtspPublishProtocol *string `json:"rtspPublishProtocol"`
	UplinkMbps          *int    `json:"uplinkMbps"`
	OutputResolution    *string `json:"outputResolution"`

	// gridTransport was the grid window's leg and upgrades to the tile receiver's,
	// that being the leg a receiving pipeline was given under it.
	// watchTransport, the player's, upgrades to nothing:
	// a player is opened per press on a leg the call names, so no field carries it.
	GridTransport      *string `json:"gridTransport"`
	RtspWatchProtocol  *string `json:"rtspWatchProtocol"`
	SrtWatchLatencyMs  *int    `json:"srtWatchLatencyMs"`
	RtspWatchLatencyMs *int    `json:"rtspWatchLatencyMs"`
}

// decodeFlat reads a settings file written before the three groups,
// false for one written after them.
//
// A file is the old shape where it carries "relayHost",
// the one key the old shape always wrote and no group ever writes.
// Presence rather than emptiness: a relay host is a string a user may legitimately have cleared,
// and a test on its value would read an edited new file as an old one.
//
// Every field is a pointer for the same reason.
// A key the old file did not carry is one the defaults answer,
// and a zero taken from an absent key would set a frame rate of zero or a port of none,
// where the old build had neither.
func decodeFlat(data []byte) (Settings, bool) {
	var f flat
	if err := json.Unmarshal(data, &f); err != nil {
		return Settings{}, false
	}
	if f.RelayHost == nil {
		return Settings{}, false
	}

	s := Defaults()
	set(&s.Relay.Host, f.RelayHost)
	set(&s.Relay.SrtPort, f.RelayPort)
	set(&s.Relay.RtspPort, f.RtspPort)
	set(&s.Relay.WebrtcPort, f.WebrtcPort)
	set(&s.Relay.RtmpPort, f.RtmpPort)
	set(&s.Relay.HlsPort, f.HlsPort)

	set(&s.Publish.Transport, f.Transport)
	set(&s.Publish.FlatCodec, f.Codec)
	set(&s.Publish.Mode, f.Mode)
	set(&s.Publish.Chroma, f.Chroma)
	set(&s.Publish.ColorRange, f.ColorRange)
	set(&s.Publish.Fps, f.Fps)
	set(&s.Publish.Cq, f.Cq)
	set(&s.Publish.BitrateM, f.BitrateM)
	set(&s.Publish.MaxrateM, f.MaxrateM)
	set(&s.Publish.VbvMs, f.VbvMs)
	set(&s.Publish.Gop, f.Gop)
	set(&s.Publish.Bframes, f.Bframes)
	set(&s.Publish.Effort, f.Effort)
	set(&s.Publish.Capture, f.Capture)
	s.Publish.AudioSources = audioSourcesOf(f.Audio, s.Publish.AudioSources)
	set(&s.Publish.AudioCodec, f.AudioCodec)
	set(&s.Publish.DrmMap, f.DrmMap)
	set(&s.Publish.Monitor, f.Monitor)
	set(&s.Publish.CaptureMemory, f.CaptureMemory)
	set(&s.Publish.SrtPublishLatencyMs, f.SrtPublishLatencyMs)
	set(&s.Publish.RtspPublishProtocol, f.RtspPublishProtocol)
	set(&s.Publish.UplinkMbps, f.UplinkMbps)
	set(&s.Publish.OutputResolution, f.OutputResolution)

	set(&s.Viewer.TileWatchTransport, f.GridTransport)
	set(&s.Viewer.RtspWatchProtocol, f.RtspWatchProtocol)
	set(&s.Viewer.SrtWatchLatencyMs, f.SrtWatchLatencyMs)
	set(&s.Viewer.RtspWatchLatencyMs, f.RtspWatchLatencyMs)
	return s, true
}

// set writes a stored value over a default, leaving the default where the file carried no such key.
func set[T any](into *T, stored *T) {
	assert.IsNotNil(into, "a stored value is written into a field")

	if stored != nil {
		*into = *stored
	}
}

// migrate upgrades a decoded settings object to the schema this build reads.
// It renames the pre-rate-control modes ("latency" to "cbr", "quality" to "crf"),
// moves a value onto the one that answers where it named something the app does not address,
// and fills the keys a file was written without, so a file from an older build stays usable.
// Run over the working settings and over every saved preset.
//
// Idempotent: a second pass over its own output renames nothing, the old mode names being gone,
// moves nothing, a moved value not matching what it moved from,
// and fills nothing, every key it fills being non-empty by then.
func migrate(s Settings) Settings {
	d := Defaults()
	s.Relay = migrateRelay(s.Relay, d.Relay)
	s.Publish = migratePublish(s.Publish, d.Publish)
	s.Viewer = migrateViewer(s.Viewer, d.Viewer)
	return s
}

// The RTSP and RTMP ports of a relay that terminated neither leg itself.
const (
	cleartextRtspPort = 8554
	cleartextRtmpPort = 1935
)

// migrateRelay fills the listener ports a file written before a transport was registered lacks,
// and moves the two a relay stopped binding onto the listeners that answer in their place.
// No transport is reachable on port zero, so a missing port is no value a user chose.
func migrateRelay(r, d Relay) Relay {
	fillNum(&r.SrtPort, d.SrtPort)
	fillNum(&r.RtspPort, d.RtspPort)
	fillNum(&r.WebrtcPort, d.WebrtcPort)
	fillNum(&r.RtmpPort, d.RtmpPort)
	fillNum(&r.HlsPort, d.HlsPort)
	fillNum(&r.MoqPort, d.MoqPort)

	// Every relay terminates TLS on these two legs and binds no cleartext listener at all
	// (deploy/mediamtx-groups.yml, rtspEncryption),
	// and both addresses are spelled rtsps and rtmps whatever port they carry (internal/transport).
	// A file naming a cleartext port addresses nothing,
	// and reaching for a port with no listener behind it is a timeout rather than a refusal,
	// so the publish spends its whole connect window before saying so.
	// These two numbers alone:
	// a relay binds its TLS listeners wherever it is told to,
	// and a second one on the same host takes ports somebody chose (cmd/soak/scripts/start.sh).
	replaceNum(&r.RtspPort, cleartextRtspPort, d.RtspPort)
	replaceNum(&r.RtmpPort, cleartextRtmpPort, d.RtmpPort)

	// DisplayName is left alone.
	// An empty one is a state and not a gap:
	// this machine has no name,
	// and joining a group asks for one rather than proceeding under a filled-in default.
	// A name is claimed first-come inside a group,
	// so a default would send every machine to claim the same one,
	// and hand all but the first a refusal on a name nobody chose (internal/membership).

	assert.Assert(r.SrtPort > 0 && r.RtspPort > 0 && r.WebrtcPort > 0 && r.RtmpPort > 0 && r.HlsPort > 0 && r.MoqPort > 0,
		"an upgraded relay names a port for every listener",
		r.SrtPort, r.RtspPort, r.WebrtcPort, r.RtmpPort, r.HlsPort, r.MoqPort)
	return r
}

// migratePublish upgrades one publish group, a stored preset being one of those.
func migratePublish(p, d Publish) Publish {
	switch p.Mode {
	case "latency":
		p.Mode = "cbr"
	case "quality":
		p.Mode = "crf"
	}
	// A rate at or below zero prices nothing and encodes nothing, and the file is the user's to edit,
	// so the stored value is repaired here rather than carried into the form.
	fillNum(&p.BitrateM, d.BitrateM)
	// A zero ceiling is a value and not an absence: the encode bounded by nothing,
	// which the form offers as an entry of its own beside the ceiling's own band
	// (api/proto/screenshare/v1/form.proto, CONTROL_KIND_NUMBER_SELECT).
	// A file naming no ceiling at all keeps the default already, the decode starting from Defaults,
	// so filling here would reach the reader who answered rather than the file that never asked.
	// A VbvMs of zero is left standing for the same reason, being the encoder's own buffer default.
	// A file from before the per-hop latency split lacks this key,
	// and zero disables SRT's retransmit window entirely.
	fillNum(&p.SrtPublishLatencyMs, d.SrtPublishLatencyMs)
	// The relay negotiates the larger of its window and this one,
	// so a stored figure below the floor names a window the hop does not run at,
	// and the form would show it as if it did.
	if p.SrtPublishLatencyMs < SrtRelayFloorMs {
		p.SrtPublishLatencyMs = SrtRelayFloorMs
	}
	// The publish leg's protocol was fixed before it was a field,
	// so a file from then names none and the transport refuses the publish over the empty value.
	fillText(&p.RtspPublishProtocol, d.RtspPublishProtocol)
	// A file written while the encode was one field carries an encoder name under the old key,
	// which the ordinary decode never reads:
	// the pair replaced it and neither half spells a codec.
	// That name addresses one row, so the row's own columns are what the two fields become,
	// and a stored stream keeps encoding as it did.
	// A name no row carries is dropped rather than split,
	// which would write a format nothing produces beside an encoder no family answers to.
	//
	// The old key wins over the pair rather than filling it, the pair being the default either way:
	// the group decode starts from Defaults and the flat one does too.
	if p.FlatCodec != "" {
		if c, ok := capabilities.Get(p.FlatCodec); ok {
			p.Format, p.Encoder = c.Format, c.Encoder()
		}
		p.FlatCodec = ""
	}
	// Either half left empty takes what a fresh installation holds,
	// a dropped name among the ways to get there.
	// An empty format names no bitstream and an empty encoder no encode,
	// so neither is a value somebody chose,
	// and the form walks the pair on from here wherever this machine cannot run it.
	fillText(&p.Format, d.Format)
	fillText(&p.Encoder, d.Encoder)
	// A file written while the second track was one source carries that name under the old key,
	// which the ordinary decode never reads:
	// the field is a list and the two shapes have no reading in common.
	// The one source becomes the one entry, at unity gain and unmuted,
	// so a stored stream keeps recording what it did.
	// A file written before the option at all carries neither,
	// and gets the empty list a fresh installation has.
	p.AudioSources = audioSourcesOf(&p.FlatAudio, p.AudioSources)
	p.FlatAudio = ""
	// A file written before the audio codec became a setting names none,
	// and both engines refuse a track whose codec no table row carries.
	// Opus is what those builds encoded,
	// so filling it keeps a stored stream publishing the track it did,
	// rather than starting it on a codec the file never chose.
	fillText(&p.AudioCodec, d.AudioCodec)
	// The DRM download strategy is matched against a table by the builders that read it,
	// and a value the table does not name is rejected.
	// Filling the key an older file lacks keeps that rejection about a value the user chose.
	fillText(&p.DrmMap, d.DrmMap)
	// A file written before the ladders became settings names no step,
	// and every builder refuses a value its ladder does not carry.
	// The codec's own default for the mode is what those builds spent,
	// so filling it keeps a stored stream encoding as it did.
	if p.Effort == "" || p.Tune == "" {
		effort, tune := LadderSteps(p.Codec(), p.Mode)
		fillText(&p.Effort, effort)
		fillText(&p.Tune, tune)
	}
	// A file written before the frame memory option names none,
	// and every engine refuses a value its table does not carry.
	// The table's default is the value every pair satisfies,
	// so filling it keeps a stored stream publishing as it did:
	// a pair with no GPU path resolves to the system memory it used, and a pair with one takes it.
	fillText(&p.CaptureMemory, d.CaptureMemory)
	// A file written before the pointer became a setting names no mode,
	// and the builders reject a value no table carries.
	// The default is what those builds did,
	// every backend but kmsgrab drawing the pointer into the frames,
	// so filling it keeps a stored stream looking as it did.
	fillText(&p.Cursor, d.Cursor)

	assert.Assert(p.FlatCodec == "", "an upgraded publish carries no pre-pair codec key", p.FlatCodec)
	assert.Assert(p.FlatAudio == "", "an upgraded publish carries no pre-list audio key", p.FlatAudio)
	assert.Assert(p.Mode != "latency" && p.Mode != "quality",
		"an upgraded publish names a rate control the tables carry", p.Mode)
	return p
}

// migrateViewer fills the watch knobs a file written before each of them lacks.
// No zero here is a value a receiver takes:
// no leg to decode from, no RTP lower transport to negotiate, no retransmit window,
// no jitter buffer and no render chain.
func migrateViewer(v, d Viewer) Viewer {
	fillText(&v.TileWatchTransport, d.TileWatchTransport)
	fillText(&v.RtspWatchProtocol, d.RtspWatchProtocol)
	fillNum(&v.SrtWatchLatencyMs, d.SrtWatchLatencyMs)
	fillNum(&v.RtspWatchLatencyMs, d.RtspWatchLatencyMs)
	fillText(&v.RenderChain, d.RenderChain)
	// The card draws one of three routes and has no reading for a fourth,
	// so an empty key and a hand-edited one are repaired together:
	// a file written before the toggle became a setting names none,
	// and the file is the user's to edit.
	// The local route is what those builds drew, and it costs no reader slot at the relay.
	if !ValidPreviewRoute(v.PreviewRoute) {
		v.PreviewRoute = d.PreviewRoute
	}

	assert.Assert(v.TileWatchTransport != "" && v.RenderChain != "" && ValidPreviewRoute(v.PreviewRoute),
		"an upgraded viewer names a tile leg, a render chain and a preview route",
		v.TileWatchTransport, v.RenderChain, v.PreviewRoute)
	return v
}

// fillText writes the default over a string a stored file did not carry, empty standing for absent.
// Each call site above states why empty is no value the user could have chosen for that field.
func fillText(into *string, def string) {
	assert.IsNotNil(into, "a default is written into a field")

	if *into == "" {
		*into = def
	}
}

// replaceNum writes the default over one stored figure and leaves every other value standing.
// For a figure that addressed something once and addresses nothing now,
// which fillNum's absence test does not reach.
func replaceNum(into *int, stale, def int) {
	assert.IsNotNil(into, "a replacement is written into a field")

	if *into == stale {
		*into = def
	}
}

// fillNum is fillText for a figure, filling anything at or below zero rather than zero alone.
// Every number it is called on is a port, a window or a ceiling,
// none of which carries a meaning at or below zero a file could have stored on purpose.
func fillNum(into *int, def int) {
	assert.IsNotNil(into, "a default is written into a field")

	if *into <= 0 {
		*into = def
	}
}

// audioSourcesOf turns the one source name an older file carried into a list,
// and leaves a list that is already there alone.
//
// The stored list wins wherever it has entries:
// a file carrying both was written by a build with the list,
// so the old key is what that build left behind rather than a second opinion about what to record.
// The absent source becomes the empty list, the same stream with no second track,
// spelled the way the field spells it.
func audioSourcesOf(flat *string, stored []AudioSource) []AudioSource {
	if len(stored) > 0 || flat == nil || *flat == "" || *flat == audioSourceNone {
		return stored
	}
	return Recording(*flat)
}
