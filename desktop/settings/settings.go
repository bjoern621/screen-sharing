// Package settings persists every user-controllable aspect of the stream.
//
// Settings are stored as JSON in the user's config directory
// (os.UserConfigDir: %APPDATA% on Windows, XDG_CONFIG_HOME/~/.config on Linux).
// The JSON tags double as the wire format between Go backend and frontend.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

const configDirName = "screenshare"
const configFileName = "settings.json"

// Stream holds every user-controllable aspect of the stream.
type Stream struct {
	Name       string `json:"name"`
	RelayHost  string `json:"relayHost"`
	RelayPort  int    `json:"relayPort"`  // UDP port of the relay's SRT listener
	ApiPort    int    `json:"apiPort"`    // TCP port of the relay's HTTP API
	RtspPort   int    `json:"rtspPort"`   // TCP port of the relay's RTSP listener
	WebrtcPort int    `json:"webrtcPort"` // TCP port of the relay's WebRTC/WHIP+WHEP HTTP listener
	RtmpPort   int    `json:"rtmpPort"`   // TCP port of the relay's RTMP listener
	HlsPort    int    `json:"hlsPort"`    // TCP port of the relay's HLS HTTP listener
	Transport  string `json:"transport"`  // publish leg (publisher to relay): registry key, e.g. "srt"
	Codec      string `json:"codec"`      // ffmpeg encoder name, a row of capabilities.Codecs
	Mode       string `json:"mode"`       // rate control: cbr vbr abr crf lossless
	Chroma     string `json:"chroma"`     // gbrp yuv444p yuv420p p010le
	ColorRange string `json:"colorRange"` // pc tv (ignored for gbrp, inherently full range)
	Fps        int    `json:"fps"`
	Cq         int    `json:"cq"`        // crf mode: constant-quality value, lower = better
	BitrateM   int    `json:"bitrateM"`  // Mbps: target for cbr/vbr/abr
	MaxrateM   int    `json:"maxrateM"`  // Mbps: vbr burst ceiling above the target
	VbvMs      int    `json:"vbvMs"`     // VBV/rate buffer in ms for cbr/vbr, 0 = encoder default
	Gop        int    `json:"gop"`       // keyframe interval in frames, 0 = auto (2*fps)
	Bframes    int    `json:"bframes"`   // lossy modes only; adds reorder latency
	EncPreset  string `json:"encPreset"` // nvenc p1..p7
	Capture    string `json:"capture"`   // ddagrab gdigrab (Windows), x11grab kmsgrab (Linux)
	Audio      string `json:"audio"`     // none desktop (desktop = monitor of the default sink via PulseAudio/PipeWire)
	DrmMap     string `json:"drmMap"`    // kmsgrab DRM download strategy: auto vaapi vulkan none
	Monitor    int    `json:"monitor"`   // ddagrab output_idx
	// SRT latency windows PER HOP. Glass-to-glass delay is the SUM of both
	// (plus encode/decode): publisher→relay and relay→viewer are independent
	// SRT links, each holding packets for its own retransmit window.
	SrtPublishLatencyMs int `json:"srtPublishLatencyMs"`
	SrtWatchLatencyMs   int `json:"srtWatchLatencyMs"`
	// RTSP watch leg (relay to viewer). RtspWatchProtocol is the RTP lower
	// transport every RTSP viewer negotiates, "tcp" (interleaved over the RTSP
	// connection) or "udp" (a port pair per track). RtspWatchLatencyMs sizes
	// rtspsrc's jitter buffer in milliseconds and reaches the native grid
	// alone: ffplay and mpv buffer by reorder queue rather than by time, which
	// is not the same knob under another name. The publish leg interleaves over
	// TCP unconditionally and reads neither field.
	RtspWatchLatencyMs int    `json:"rtspWatchLatencyMs"`
	RtspWatchProtocol  string `json:"rtspWatchProtocol"`
	UplinkMbps         int    `json:"uplinkMbps"` // user's known upload capacity, used for warnings only
	// WatchTransport is the watch leg (relay to viewer): the transport a Watch
	// click receives over. Independent of Transport, the publish leg, since the
	// relay re-serves every stream on all its listeners, so the two legs of one
	// stream can run different protocols.
	WatchTransport string `json:"watchTransport"`
}

// Defaults returns the settings a fresh installation starts with.
// The capture backend is chosen per OS.
func Defaults() Stream {
	host, err := os.Hostname()
	if err != nil {
		host = "me"
	}

	capture := "ddagrab"
	if runtime.GOOS != "windows" {
		capture = "x11grab"
	}

	return Stream{
		Name: host, RelayHost: "127.0.0.1", RelayPort: 8890, ApiPort: 9997,
		RtspPort: 8554, WebrtcPort: 8889, RtmpPort: 1935, HlsPort: 8888,
		Transport: "srt", Codec: "hevc_nvenc", Mode: "lossless", Chroma: "gbrp",
		ColorRange: "pc", Fps: 60, Cq: 19, BitrateM: 150, MaxrateM: 200, VbvMs: 0,
		Gop: 0, Bframes: 0,
		EncPreset: "p7", Capture: capture, DrmMap: "auto", Monitor: 0, Audio: "none",
		SrtPublishLatencyMs: 300, SrtWatchLatencyMs: 1200, // sum ≈ glass-to-glass budget
		// rtspsrc defaults to 2000 ms of jitter buffer, two seconds of display
		// delay above what a LAN needs. TCP because the UDP alternative loses
		// its port pair to NAT and never retransmits.
		RtspWatchLatencyMs: 200, RtspWatchProtocol: "tcp",
		UplinkMbps:     50,
		WatchTransport: "srt",
	}
}

// configDir returns the config directory, creating it if needed.
func configDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		logger.Warnf("Cannot determine user config directory, falling back to working directory: %v", err)
		base = "."
	}

	dir := filepath.Join(base, configDirName)
	err = os.MkdirAll(dir, 0o755)
	if err != nil {
		logger.Warnf("Cannot create config directory %s: %v", dir, err)
	}

	return dir
}

// configPath returns the settings file path.
func configPath() string {
	return filepath.Join(configDir(), configFileName)
}

// Load reads the persisted settings.
// A missing file silently yields Defaults() - first start is not an error.
func Load() Stream {
	s := Defaults()

	data, err := os.ReadFile(configPath())
	if err != nil {
		return s
	}

	err = json.Unmarshal(data, &s)
	if err != nil {
		logger.Warnf("Settings file is corrupt, using defaults: %v", err)
		return Defaults()
	}

	// Migration guard: settings files from before the per-hop latency split
	// lack these keys; zero would disable SRT's retransmit window entirely.
	if s.SrtPublishLatencyMs <= 0 {
		s.SrtPublishLatencyMs = Defaults().SrtPublishLatencyMs
	}
	if s.SrtWatchLatencyMs <= 0 {
		s.SrtWatchLatencyMs = Defaults().SrtWatchLatencyMs
	}
	// Settings files from before the audio option lack the key.
	if s.Audio == "" {
		s.Audio = "none"
	}
	// A settings file written before a transport was registered lacks that
	// listener's port, and no transport can be reached on port zero.
	if s.RtspPort <= 0 {
		s.RtspPort = Defaults().RtspPort
	}
	if s.WebrtcPort <= 0 {
		s.WebrtcPort = Defaults().WebrtcPort
	}
	if s.RtmpPort <= 0 {
		s.RtmpPort = Defaults().RtmpPort
	}
	if s.HlsPort <= 0 {
		s.HlsPort = Defaults().HlsPort
	}

	return migrateStream(s)
}

// migrateStream upgrades a decoded settings object to the current schema. It
// renames the pre-rate-control modes (latency/quality became cbr/crf) and fills
// fields added with the VBR and ABR modes. Applied to the working settings and
// to every saved preset, so a file written by an older build stays usable.
func migrateStream(s Stream) Stream {
	switch s.Mode {
	case "latency":
		s.Mode = "cbr"
	case "quality":
		s.Mode = "crf"
	}
	// A zero ceiling would leave VBR no room above the target; default it. VbvMs
	// zero is a valid value (the encoder's own buffer default), so it is left.
	if s.MaxrateM <= 0 {
		s.MaxrateM = Defaults().MaxrateM
	}
	// Files from before the watch transport was persisted lack the key.
	if s.WatchTransport == "" {
		s.WatchTransport = Defaults().WatchTransport
	}
	// Files from before the RTSP watch knobs lack their keys, and neither zero
	// value is one a receiver can be given: no jitter buffer at all, and no RTP
	// lower transport to negotiate.
	if s.RtspWatchLatencyMs <= 0 {
		s.RtspWatchLatencyMs = Defaults().RtspWatchLatencyMs
	}
	if s.RtspWatchProtocol == "" {
		s.RtspWatchProtocol = Defaults().RtspWatchProtocol
	}
	return s
}

// Save persists the settings.
func Save(s Stream) error {
	data, err := json.MarshalIndent(s, "", "  ")
	assert.IsNil(err, "marshalling a plain settings struct cannot fail")

	return os.WriteFile(configPath(), data, 0o644)
}
