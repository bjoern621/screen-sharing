// Package report bundles this machine's facts and run logs
// and delivers the bundle to the group service beside the relay (internal/groupsvc, POST /reports).
//
// What leaves: report.json with the facts below, the settings with their secrets blanked
// (settings.Redacted), and the newest run logs, each tail-capped.
// The group key, the Discord link and the member secrets never ride.
package report

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/gpu"
	"bjoernblessin.de/screenshare/internal/platform"
)

// The two ways a report leaves this machine.
const (
	// KindManual is a report a reader asked for.
	KindManual = "manual"
	// KindCrash is a report about an earlier run's traceback, sent on start (UnreportedCrash).
	KindCrash = "crash"
)

// probeTimeout bounds each tool-version probe.
// A tool that hangs costs a report its version line rather than the send.
const probeTimeout = 5 * time.Second

// Facts is report.json: what this machine is, for reading the logs against.
type Facts struct {
	Kind    string `json:"kind"` // KindManual or KindCrash
	Version string `json:"version"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	// Display is the Linux display server, "x11" or "wayland", empty elsewhere.
	Display string `json:"display,omitempty"`
	// Distro is the distribution's own name for itself, off /etc/os-release.
	Distro string `json:"distro,omitempty"`
	// GPU is the VA driver this machine encodes through, empty fields where none answered.
	GPU gpu.Info `json:"gpu"`
	// Tool versions as each names itself, first line, empty where a tool is absent.
	GStreamer string `json:"gstreamer,omitempty"`
	FFmpeg    string `json:"ffmpeg,omitempty"`
	// SentAt is this machine's clock, RFC 3339 UTC.
	SentAt string `json:"sentAt"`
}

// Gather reads the facts.
// Costs up to a few seconds on the tool probes, which a report a reader waits on affords.
func Gather(version, kind string) Facts {
	return Facts{
		Kind:      kind,
		Version:   version,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		Display:   platform.Detect().Display,
		Distro:    distro(),
		GPU:       gpu.Detect(),
		GStreamer: firstLineOf("gst-inspect-1.0", "--version"),
		FFmpeg:    ffmpegVersion(),
		SentAt:    time.Now().UTC().Format(time.RFC3339),
	}
}

// distro is the Linux distribution's own name for itself, empty elsewhere.
func distro() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	content, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(content), "\n") {
		if value, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
			return strings.Trim(value, `"`)
		}
	}
	return ""
}

// ffmpegVersion probes the ffmpeg the publish engine would run,
// resolved the way a publish resolves it, bundled copy first.
func ffmpegVersion() string {
	exe, err := ffmpeg.FindExe("ffmpeg")
	if err != nil {
		return ""
	}
	return firstLineOf(exe, "-version")
}

// firstLineOf runs one probe and answers its first line,
// empty where the tool is absent or silent.
func firstLineOf(exe string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, exe, args...).Output()
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(out), "\n")
	return strings.TrimSpace(line)
}
