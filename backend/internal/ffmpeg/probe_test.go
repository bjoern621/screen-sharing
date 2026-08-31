package ffmpeg

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

// probeTimeout bounds one test encode.
// Two frames at 320x240 return well under a second, so it catches only an encoder that takes
// the frames and emits nothing.
const probeTimeout = 20 * time.Second

// probeStream is a stream any ffmpeg build encodes: the software encoder they all carry,
// on a screen grabber this engine runs.
func probeStream() settings.Settings {
	s := settings.Defaults()
	s.Publish.Capture = "x11grab"
	s.Publish.UseCodec("libx264")
	s.Publish.Mode = "crf"
	s.Publish.Chroma = "yuv420p"
	s.Publish.Fps = 30
	// The defaults hold the default codec's step, and this row's ladder has no such step.
	// Clearing is what the repair does in the app,
	// and it leaves the builder on the step this row declares.
	s.Publish.Effort, s.Publish.Tune = "", ""
	return s
}

// Both the lavfi graphs and the encoder arguments are wire formats,
// so a filter spelled wrong fails a measurement where the publish it predicts would have run.
// Hence the real binary, on both ends of the content range.
func TestEncodeProbeAgainstFfmpeg(t *testing.T) {
	exe, err := FindExe("ffmpeg")
	if err != nil {
		t.Skipf("ffmpeg not installed: %v", err)
	}

	for _, tc := range []struct {
		name  string
		heavy bool
	}{
		{"heavy", true},
		{"light", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args, err := BuildEncodeProbeArgs(probeStream(), 320, 240, 2, tc.heavy)
			if err != nil {
				t.Fatalf("building the probe: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
			defer cancel()
			out, err := exec.CommandContext(ctx, exe, args...).CombinedOutput()
			if err != nil {
				t.Fatalf("ffmpeg %s\n%v\n%s", strings.Join(args, " "), err, out)
			}
		})
	}
}

// The ceiling says whether a measurement found the encoder or the generator,
// which holds only while it generates the frames the encode does and stops at the muxer.
func TestProbeCeilingAgainstFfmpeg(t *testing.T) {
	exe, err := FindExe("ffmpeg")
	if err != nil {
		t.Skipf("ffmpeg not installed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	args := BuildProbeCeilingArgs(320, 240, 30, 2, true)
	if out, err := exec.CommandContext(ctx, exe, args...).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg %s\n%v\n%s", strings.Join(args, " "), err, out)
	}
}

// A figure about the encoder the settings name holds only while the probe's encoder half
// is the publish command's own.
func TestEncodeProbeCarriesTheRunsEncoder(t *testing.T) {
	s := probeStream()
	probe, err := BuildEncodeProbeArgs(s, 320, 240, 2, true)
	if err != nil {
		t.Fatalf("building the probe: %v", err)
	}
	enc, err := encoderArgs(s, gopFor(s))
	if err != nil {
		t.Fatalf("building the encoder: %v", err)
	}
	if !strings.Contains(strings.Join(probe, " "), strings.Join(enc, " ")) {
		t.Errorf("the probe encodes with something other than the run's encoder\nprobe:   %s\nencoder: %s",
			strings.Join(probe, " "), strings.Join(enc, " "))
	}
}

// The transport is no part of what an encoder costs, so a leg that cannot carry the stream leaves
// the measurement standing.
func TestEncodeProbeIgnoresTheTransport(t *testing.T) {
	s := probeStream()
	s.Publish.Transport = "hls"
	if _, err := BuildEncodeProbeArgs(s, 320, 240, 2, true); err != nil {
		t.Errorf("the probe refused a measurement over a transport it does not use: %v", err)
	}
}

// A combination the form starts and the probe refuses costs nothing visible until the frames
// are already being discarded.
func TestEveryPublishableCombinationCanBeProbed(t *testing.T) {
	for _, c := range capabilities.Codecs {
		if _, gap := c.EngineGap(capabilities.EngineFfmpeg); !c.Implemented || gap {
			continue
		}
		for _, chroma := range c.EngineChromas(capabilities.EngineFfmpeg) {
			for _, mode := range capabilities.Modes {
				s := probeStream()
				s.Publish.UseCodec(c.Name)
				s.Publish.Chroma, s.Publish.Mode = chroma, mode
				if _, err := BuildPublishArgs(s, nil); err != nil {
					continue
				}
				if _, err := BuildEncodeProbeArgs(s, 320, 240, 2, true); err != nil {
					t.Errorf("%s at %s/%s publishes but cannot be probed: %v",
						c.Name, chroma, mode, err)
				}
			}
		}
	}
}

// No frames, or a picture with no size, is an Entwicklungsfehler and panics rather than returning.
func TestEncodeProbeAssertsItsBounds(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		width, height, frames int
	}{
		{"no width", 0, 240, 2},
		{"no height", 320, 0, 2},
		{"no frames", 320, 240, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("the probe accepted a bound it cannot encode")
				}
			}()
			_, _ = BuildEncodeProbeArgs(probeStream(), tc.width, tc.height, tc.frames, true)
		})
	}
}
