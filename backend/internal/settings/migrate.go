package settings

import (
	"encoding/json"

	"bjoernblessin.de/go-utils/util/assert"
)

// flat is the settings shape from before the three groups: one object with every key at the top
// level, the leg spelled into the field names that needed it.
//
// Kept so a stored file survives the split rather than reading as a first start.
// Only the fields that moved or were renamed sit here; everything else keeps its key inside its
// group and the ordinary decode already found it, which is why this is a second pass over the same
// bytes rather than a second struct for the whole file.
type flat struct {
	RelayHost  *string `json:"relayHost"`
	RelayPort  *int    `json:"relayPort"`
	ApiPort    *int    `json:"apiPort"`
	RtspPort   *int    `json:"rtspPort"`
	WebrtcPort *int    `json:"webrtcPort"`
	RtmpPort   *int    `json:"rtmpPort"`
	HlsPort    *int    `json:"hlsPort"`

	Name                *string `json:"name"`
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

	// watchTransport was the one watch leg and upgrades to the player's.
	// gridTransport was the grid window's and upgrades to the tile receiver's, that being the leg a
	// receiving pipeline was given under it.
	WatchTransport     *string `json:"watchTransport"`
	GridTransport      *string `json:"gridTransport"`
	RtspWatchProtocol  *string `json:"rtspWatchProtocol"`
	SrtWatchLatencyMs  *int    `json:"srtWatchLatencyMs"`
	RtspWatchLatencyMs *int    `json:"rtspWatchLatencyMs"`
}

// decodeFlat reads a settings file written before the three groups, false for one written after
// them.
//
// A file is the old shape where it carries "relayHost", the one key the old shape always wrote and
// no group ever writes.
// Presence rather than emptiness: a relay host is a string a user may legitimately have cleared,
// and a test on its value would read an edited new file as an old one.
//
// Every field is a pointer for the same reason.
// A key the old file did not carry is one the defaults answer, and a zero taken from an absent key
// would set a frame rate of zero or a port of none where the old build had neither.
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
	set(&s.Relay.ApiPort, f.ApiPort)
	set(&s.Relay.RtspPort, f.RtspPort)
	set(&s.Relay.WebrtcPort, f.WebrtcPort)
	set(&s.Relay.RtmpPort, f.RtmpPort)
	set(&s.Relay.HlsPort, f.HlsPort)

	set(&s.Publish.Name, f.Name)
	set(&s.Publish.Transport, f.Transport)
	set(&s.Publish.Codec, f.Codec)
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

	set(&s.Viewer.PlayerWatchTransport, f.WatchTransport)
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
// It renames the pre-rate-control modes ("latency" to "cbr", "quality" to "crf") and fills the keys
// added since a file was written, so a file from an older build stays usable.
// Run over the working settings and over every saved preset.
//
// Idempotent: a second pass over its own output renames nothing, the old mode names being gone, and
// fills nothing, every key it fills being non-empty by then.
func migrate(s Settings) Settings {
	d := Defaults()
	s.Relay = migrateRelay(s.Relay, d.Relay)
	s.Publish = migratePublish(s.Publish, d.Publish)
	s.Viewer = migrateViewer(s.Viewer, d.Viewer)
	return s
}

// migrateRelay fills the listener ports a file written before a transport was registered lacks.
// No transport is reachable on port zero, so a missing port is no value a user chose.
func migrateRelay(r, d Relay) Relay {
	fillNum(&r.SrtPort, d.SrtPort)
	fillNum(&r.ApiPort, d.ApiPort)
	fillNum(&r.RtspPort, d.RtspPort)
	fillNum(&r.WebrtcPort, d.WebrtcPort)
	fillNum(&r.RtmpPort, d.RtmpPort)
	fillNum(&r.HlsPort, d.HlsPort)
	fillNum(&r.MoqPort, d.MoqPort)

	assert.Assert(r.SrtPort > 0 && r.ApiPort > 0 && r.RtspPort > 0 && r.WebrtcPort > 0 && r.RtmpPort > 0 && r.HlsPort > 0 && r.MoqPort > 0,
		"an upgraded relay names a port for every listener",
		r.SrtPort, r.ApiPort, r.RtspPort, r.WebrtcPort, r.RtmpPort, r.HlsPort, r.MoqPort)
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
	// A zero ceiling leaves VBR no room above the target, so it is defaulted.
	// A VbvMs of zero is a legal value, the encoder's own buffer default, and is left standing.
	fillNum(&p.MaxrateM, d.MaxrateM)
	// A file from before the per-hop latency split lacks this key, and zero disables SRT's retransmit
	// window entirely.
	fillNum(&p.SrtPublishLatencyMs, d.SrtPublishLatencyMs)
	// The publish leg's protocol was fixed before it was a field, so a file from then names none and
	// the transport refuses the publish over the empty value.
	fillText(&p.RtspPublishProtocol, d.RtspPublishProtocol)
	// A file written while the second track was one source carries that source's name under the old
	// key, which the ordinary decode never reads: the field is a list and the two shapes have no
	// reading in common.
	// The one source becomes the one entry, at unity gain and unmuted, so a stored stream keeps
	// recording what it did.
	// A file written before the option at all carries neither and gets the empty list a fresh
	// installation has.
	p.AudioSources = audioSourcesOf(&p.LegacyAudio, p.AudioSources)
	p.LegacyAudio = ""
	// A file written before the audio codec became a setting names none, and both engines refuse a
	// track whose codec no table row carries.
	// Opus is what those builds encoded, so filling it keeps a stored stream publishing the track it
	// did rather than starting it on a codec the file never chose.
	fillText(&p.AudioCodec, d.AudioCodec)
	// The DRM download strategy is matched against a table by the builders that read it, and a value
	// the table does not name is rejected.
	// Filling the key a file written before the option lacks keeps that rejection about a value the
	// user chose.
	fillText(&p.DrmMap, d.DrmMap)
	// A file written before the ladders became settings names no step, and every builder refuses a
	// value its ladder does not carry.
	// The codec's own default for the mode is what those builds spent, so filling it keeps a stored
	// stream encoding as it did.
	if p.Effort == "" || p.Tune == "" {
		effort, tune := LadderSteps(p.Codec, p.Mode)
		fillText(&p.Effort, effort)
		fillText(&p.Tune, tune)
	}
	// A file written before the frame memory option names none, and every engine refuses a value its
	// table does not carry.
	// The table's default is the value every pair satisfies, so filling it keeps a stored stream
	// publishing as it did: a pair with no GPU path resolves to the system memory it used, and a pair
	// with one takes it.
	fillText(&p.CaptureMemory, d.CaptureMemory)
	// A file written before the pointer became a setting names no mode, and the builders reject a
	// value no table carries.
	// The default is what those builds did, every backend but kmsgrab drawing the pointer into the
	// frames, so filling it keeps a stored stream looking as it did rather than starting it without
	// one.
	fillText(&p.Cursor, d.Cursor)

	assert.Assert(p.LegacyAudio == "", "an upgraded publish carries no pre-list audio key", p.LegacyAudio)
	assert.Assert(p.Mode != "latency" && p.Mode != "quality",
		"an upgraded publish names a rate control the tables carry", p.Mode)
	return p
}

// migrateViewer fills the watch knobs a file written before each of them lacks.
// No zero here is a value a receiver takes: no leg to open, no RTP lower transport to negotiate, no
// retransmit window, no jitter buffer and no render chain.
func migrateViewer(v, d Viewer) Viewer {
	fillText(&v.PlayerWatchTransport, d.PlayerWatchTransport)
	fillText(&v.TileWatchTransport, d.TileWatchTransport)
	fillText(&v.RtspWatchProtocol, d.RtspWatchProtocol)
	fillNum(&v.SrtWatchLatencyMs, d.SrtWatchLatencyMs)
	fillNum(&v.RtspWatchLatencyMs, d.RtspWatchLatencyMs)
	fillText(&v.RenderChain, d.RenderChain)

	assert.Assert(v.PlayerWatchTransport != "" && v.TileWatchTransport != "" && v.RenderChain != "",
		"an upgraded viewer names both legs and a render chain",
		v.PlayerWatchTransport, v.TileWatchTransport, v.RenderChain)
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

// fillNum is fillText for a figure, filling anything at or below zero rather than zero alone.
// Every number it is called on is a port, a window or a ceiling, none of which carries a meaning at
// or below zero that a file could have stored on purpose.
func fillNum(into *int, def int) {
	assert.IsNotNil(into, "a default is written into a field")

	if *into <= 0 {
		*into = def
	}
}

// audioSourcesOf turns the one source name an older file carried into a list, and leaves a list
// that is already there alone.
//
// The stored list wins wherever it has entries: a file carrying both was written by a build with
// the list, so the old key is what that build left behind rather than a second opinion about what
// to record.
// The absent source becomes the empty list, the same stream with no second track, spelled the way
// the field spells it.
func audioSourcesOf(legacy *string, stored []AudioSource) []AudioSource {
	if len(stored) > 0 || legacy == nil || *legacy == "" || *legacy == audioSourceNone {
		return stored
	}
	return Recording(*legacy)
}
