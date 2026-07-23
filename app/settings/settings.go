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
	Name         string `json:"name"`
	RelayHost    string `json:"relayHost"`
	RelayPort    int    `json:"relayPort"`
	ApiPort      int    `json:"apiPort"`
	Transport    string `json:"transport"`  // registry key, e.g. "srt"
	Codec        string `json:"codec"`      // hevc_nvenc h264_nvenc av1_nvenc libx264
	Mode         string `json:"mode"`       // lossless quality latency
	Chroma       string `json:"chroma"`     // gbrp yuv444p yuv420p p010le
	ColorRange   string `json:"colorRange"` // pc tv (ignored for gbrp, inherently full range)
	Fps          int    `json:"fps"`
	Cq           int    `json:"cq"`        // quality mode: constant-quality value, lower = better
	BitrateM     int    `json:"bitrateM"`  // Mbps: quality = burst ceiling, latency = CBR target
	Gop          int    `json:"gop"`       // keyframe interval in frames, 0 = auto (2*fps)
	Bframes      int    `json:"bframes"`   // 0 recommended (B-frames save nothing in lossless mode)
	EncPreset    string `json:"encPreset"` // nvenc p1..p7
	Capture      string `json:"capture"`   // ddagrab gdigrab (Windows), x11grab kmsgrab (Linux)
	Monitor      int    `json:"monitor"`   // ddagrab output_idx
	// SRT latency windows PER HOP. Glass-to-glass delay is the SUM of both
	// (plus encode/decode): publisher→relay and relay→viewer are independent
	// SRT links, each holding packets for its own retransmit window.
	SrtPublishLatencyMs int `json:"srtPublishLatencyMs"`
	SrtWatchLatencyMs   int `json:"srtWatchLatencyMs"`
	UplinkMbps   int    `json:"uplinkMbps"` // user's known upload capacity, used for warnings only
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
		Transport: "srt", Codec: "hevc_nvenc", Mode: "lossless", Chroma: "gbrp",
		ColorRange: "pc", Fps: 60, Cq: 19, BitrateM: 150, Gop: 0, Bframes: 0,
		EncPreset: "p7", Capture: capture, Monitor: 0,
		SrtPublishLatencyMs: 300, SrtWatchLatencyMs: 1200, // sum ≈ glass-to-glass budget
		UplinkMbps: 50,
	}
}

// configPath returns the settings file path, creating the directory if needed.
func configPath() string {
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

	return filepath.Join(dir, configFileName)
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

	return s
}

// Save persists the settings.
func Save(s Stream) error {
	data, err := json.MarshalIndent(s, "", "  ")
	assert.IsNil(err, "marshalling a plain settings struct cannot fail")

	return os.WriteFile(configPath(), data, 0o644)
}
