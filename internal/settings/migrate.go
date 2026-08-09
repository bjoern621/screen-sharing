package settings

import "encoding/json"

// flat is the one settings shape every build before the three groups wrote: one
// object with every key at the top level, the leg spelled into the field names that
// needed it.
//
// It is kept so a stored file survives the split rather than reading as a first start.
// Only the fields that moved or were renamed are here; everything else keeps its key
// inside its group, so the ordinary decode already found it - which is why this is a
// second pass over the same bytes rather than a whole second struct.
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
	EncPreset           *string `json:"encPreset"`
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

	// WatchTransport was the one watch leg, which the player keeps; GridTransport was
	// the GTK grid window's, which the tile receiver inherits, since it is the leg a
	// receiving pipeline was already being given.
	WatchTransport     *string `json:"watchTransport"`
	GridTransport      *string `json:"gridTransport"`
	RtspWatchProtocol  *string `json:"rtspWatchProtocol"`
	SrtWatchLatencyMs  *int    `json:"srtWatchLatencyMs"`
	RtspWatchLatencyMs *int    `json:"rtspWatchLatencyMs"`
}

// decodeFlat reads a settings file written before the three groups, and reports false
// for one written after them.
//
// A file is the old shape when it carries the one key the old shape always wrote and
// no group ever will. Presence rather than emptiness: a relay host is a string a user
// can legitimately have cleared, and a test on its value would read an edited new file
// as an old one.
//
// Every field is a pointer for the same reason. A key the old file did not carry is a
// key the defaults answer, and a zero taken from an absent key would set a frame rate
// of zero or a port of none where the old build had neither.
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
	set(&s.Publish.EncPreset, f.EncPreset)
	set(&s.Publish.Capture, f.Capture)
	set(&s.Publish.Audio, f.Audio)
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

// set writes a stored value over a default, and leaves the default where the file
// carried no such key.
func set[T any](into *T, stored *T) {
	if stored != nil {
		*into = *stored
	}
}

// migrate upgrades a decoded settings object to the current schema. It renames the
// pre-rate-control modes (latency/quality became cbr/crf) and fills fields added since
// the file was written. Applied to the working settings and to every saved preset, so
// a file written by an older build stays usable.
func migrate(s Settings) Settings {
	d := Defaults()
	s.Relay = migrateRelay(s.Relay, d.Relay)
	s.Publish = migratePublish(s.Publish, d.Publish)
	s.Viewer = migrateViewer(s.Viewer, d.Viewer)
	return s
}

// migrateRelay fills the listeners a file written before a transport was registered
// lacks. No transport can be reached on port zero, so a missing port is not a value
// the user chose.
func migrateRelay(r, d Relay) Relay {
	fillNum(&r.SrtPort, d.SrtPort)
	fillNum(&r.ApiPort, d.ApiPort)
	fillNum(&r.RtspPort, d.RtspPort)
	fillNum(&r.WebrtcPort, d.WebrtcPort)
	fillNum(&r.RtmpPort, d.RtmpPort)
	fillNum(&r.HlsPort, d.HlsPort)
	return r
}

// migratePublish upgrades one publish group, which is also what a stored preset is.
func migratePublish(p, d Publish) Publish {
	switch p.Mode {
	case "latency":
		p.Mode = "cbr"
	case "quality":
		p.Mode = "crf"
	}
	// A zero ceiling would leave VBR no room above the target; default it. VbvMs
	// zero is a valid value (the encoder's own buffer default), so it is left.
	fillNum(&p.MaxrateM, d.MaxrateM)
	// Files from before the per-hop latency split lack this key; zero would disable
	// SRT's retransmit window entirely.
	fillNum(&p.SrtPublishLatencyMs, d.SrtPublishLatencyMs)
	// The publish leg's protocol was fixed before it was a field, so a file from then
	// names none and the transport refuses the publish over the empty value.
	fillText(&p.RtspPublishProtocol, d.RtspPublishProtocol)
	// Settings files from before the audio option lack the key.
	fillText(&p.Audio, d.Audio)
	// A file written before the audio codec became a setting names none, and both
	// engines refuse an audio track whose codec no table row carries. Opus is what
	// those builds encoded, so filling it keeps a stored stream publishing the track it
	// always did rather than starting it on a codec the file never chose.
	fillText(&p.AudioCodec, d.AudioCodec)
	// The DRM download strategy and the encoder preset are both matched against a
	// table by the builders that read them, and both reject a value the table does
	// not name. Filling the key a file written before the option lacks is what keeps
	// that rejection about a value the user chose.
	fillText(&p.DrmMap, d.DrmMap)
	fillText(&p.EncPreset, d.EncPreset)
	// A file written before the frame memory option names none, and every engine
	// refuses a value its table does not carry. The table's own default is the value
	// every pair satisfies, so filling it keeps a stored stream publishing exactly as
	// it did: a pair with no GPU path resolves to the same system memory it always
	// used, and one that has a path takes it.
	fillText(&p.CaptureMemory, d.CaptureMemory)
	return p
}

// migrateViewer fills the watch knobs a file written before each of them lacks. None
// of the zero values is one a receiver can be given: no leg to open, no RTP lower
// transport to negotiate, no retransmit window, no jitter buffer and no render chain.
func migrateViewer(v, d Viewer) Viewer {
	fillText(&v.PlayerWatchTransport, d.PlayerWatchTransport)
	fillText(&v.TileWatchTransport, d.TileWatchTransport)
	fillText(&v.RtspWatchProtocol, d.RtspWatchProtocol)
	fillNum(&v.SrtWatchLatencyMs, d.SrtWatchLatencyMs)
	fillNum(&v.RtspWatchLatencyMs, d.RtspWatchLatencyMs)
	fillText(&v.RenderChain, d.RenderChain)
	return v
}

// fillText writes the default over a string a stored file did not carry. Empty is what
// "did not carry" means for every field it is called on, which is why each call site
// above says why empty is not a value the user could have chosen there.
func fillText(into *string, def string) {
	if *into == "" {
		*into = def
	}
}

// fillNum is fillText for a figure, and takes anything at or below zero rather than
// zero alone: every number it is called on is a port, a window or a ceiling, and none
// of those has a meaning at zero or below that a file could have stored on purpose.
func fillNum(into *int, def int) {
	if *into <= 0 {
		*into = def
	}
}
