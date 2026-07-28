// Package settings persists every user-controllable aspect of the stream.
//
// Settings are stored as JSON in the user's config directory
// (os.UserConfigDir: %APPDATA% on Windows, XDG_CONFIG_HOME/~/.config on Linux).
// The JSON tags double as the wire format between Go backend and frontend.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"bjoernblessin.de/go-utils/util/assert"
)

const configDirName = "screenshare"
const configFileName = "settings.json"

// configDirMode is the permission the config directory is created with.
const configDirMode = 0o755

// storeFileMode is the permission the settings and preset files are written with.
const storeFileMode = 0o644

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

// configDir returns the directory holding the settings and preset files, creating
// it if needed.
//
// A directory that cannot be resolved or created is an error and not a path to
// write into anyway: the working directory it used to fall back to is whatever the
// app was started from, so the files landed somewhere the next launch did not look
// and every setting read as a first start.
func configDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine user config directory: %w", err)
	}

	dir := filepath.Join(base, configDirName)
	if err := os.MkdirAll(dir, configDirMode); err != nil {
		return "", fmt.Errorf("cannot create config directory %s: %w", dir, err)
	}
	return dir, nil
}

// configPath returns the settings file path.
func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

// corruptSuffix names the copy an unusable store file is moved to.
const corruptSuffix = ".corrupt"

// setAside moves a store file that cannot be read or parsed out of the way, and
// returns the reason with the copy's path named.
//
// Both stores are rewritten in full from what their loader returned: the working
// settings on the next field change, the presets on the next save. A file left in
// place is therefore a file the next write replaces, so the values in it are
// renamed out of reach first and the caller has a path to name.
//
// An existing copy is the user's real data from the first failure and is kept: the
// file failing now was written after it, so it holds the defaults rather than
// anything worth a second copy.
func setAside(path string, cause error) error {
	kept := path + corruptSuffix
	if _, err := os.Stat(kept); err == nil {
		return fmt.Errorf("%w - %s holds an earlier unreadable copy and is left untouched", cause, kept)
	}
	if err := os.Rename(path, kept); err != nil {
		return fmt.Errorf("%w - moving it to %s failed: %v", cause, kept, err)
	}
	return fmt.Errorf("%w - it was moved to %s, so the values in it survive the next write", cause, kept)
}

// Load reads the persisted settings, and answers with Defaults() and the reason
// when the stored ones cannot be used. A missing file is not a failure: a first
// start has nothing to read.
//
// A file that exists and cannot be read or parsed is moved aside (setAside) before
// the defaults are handed back, so the run that opens on defaults does not take the
// stored values with it.
func Load() (Stream, error) {
	path, err := configPath()
	if err != nil {
		return Defaults(), err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Defaults(), nil
	}
	if err != nil {
		return Defaults(), setAside(path, fmt.Errorf("cannot read settings file %s: %w", path, err))
	}

	s := Defaults()
	if err := json.Unmarshal(data, &s); err != nil {
		return Defaults(), setAside(path, fmt.Errorf("settings file %s is corrupt: %w", path, err))
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

	return migrateStream(s), nil
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
	// The DRM download strategy and the encoder preset are both matched against a
	// table by the builders that read them, and both reject a value the table does
	// not name. Filling the key a file written before the option lacks is what keeps
	// that rejection about a value the user chose.
	if s.DrmMap == "" {
		s.DrmMap = Defaults().DrmMap
	}
	if s.EncPreset == "" {
		s.EncPreset = Defaults().EncPreset
	}
	return s
}

// Save persists the settings.
func Save(s Stream) error {
	data, err := json.MarshalIndent(s, "", "  ")
	assert.IsNil(err, "marshalling a plain settings struct cannot fail")

	path, err := configPath()
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, storeFileMode)
}
