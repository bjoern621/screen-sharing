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

// probeTimeout bounds one test encode. Two frames at 320x240 return in well under a
// second, so this only catches an encoder that takes the frames and emits nothing.
const probeTimeout = 20 * time.Second

// probeStream is a stream the probe builder can be exercised on: the software encoder
// every ffmpeg build carries, on a screen grabber this engine runs.
func probeStream() settings.Stream {
	s := settings.Defaults()
	s.Capture = "x11grab"
	s.Codec = "libx264"
	s.Mode = "crf"
	s.Chroma = "yuv420p"
	s.Fps = 30
	return s
}

// The lavfi sources and the encoder arguments are both wire formats: a filter spelled
// wrong is a measurement that fails where the publish it predicts would have run. So
// this runs the real thing on both ends of the content range.
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

// The ceiling command is what says whether a measurement found the encoder or the
// source, so it has to generate the same frames the encode does and stop at the muxer.
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

// The probe measures the encoder the settings name, so the encoder half of what it
// builds has to be the publish command's own.
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

// The transport is no part of what an encoder costs, so a leg that cannot carry the
// codec has to leave the measurement alone.
func TestEncodeProbeIgnoresTheTransport(t *testing.T) {
	s := probeStream()
	s.Transport = "hls"
	if _, err := BuildEncodeProbeArgs(s, 320, 240, 2, true); err != nil {
		t.Errorf("the probe refused a measurement over a transport it does not use: %v", err)
	}
}

// Whatever this engine can publish, it has to be able to measure. A combination the
// form lets a user start and the probe refuses is one whose cost stays invisible until
// the frames are already being discarded.
func TestEveryPublishableCombinationCanBeProbed(t *testing.T) {
	for _, c := range capabilities.Codecs {
		if _, gap := c.EngineGap(capabilities.EngineFfmpeg); !c.Implemented || gap {
			continue
		}
		for _, chroma := range c.EngineChromas(capabilities.EngineFfmpeg) {
			for _, mode := range capabilities.Modes {
				s := probeStream()
				s.Codec, s.Chroma, s.Mode = c.Name, chroma, mode
				if _, err := BuildPublishArgs(s); err != nil {
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

// A run of no frames or of a picture with no size is a caller's mistake, not a user's.
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
