package app

import (
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/receive"
)

// The rates are what this file computes rather than carries, so they are what there is to test.
// Everything else is a field moving from one struct to another, which the compiler checks.

// TestFirstSampleHasNoRates holds the difference between absent and zero.
// A decode on its first tick has one reading, and one reading is not a rate:
// a zero here would say the stream is carrying nothing.
func TestFirstSampleHasNoRates(t *testing.T) {
	ref := StreamRef{Name: "desk", Transport: "srt"}
	now := receive.Stats{Uptime: 3 * time.Second, VideoBytes: 900_000, VideoFrames: 90}

	got := receiveStatsOf(ref, now, receive.Stats{}, false)

	if got.VideoMbps != nil || got.VideoFPS != nil || got.RenderFPS != nil || got.AudioKbps != nil {
		t.Errorf("a first sample reported a rate: %+v", got)
	}
	if got.VideoBytes != 900_000 {
		t.Errorf("counters = %d, want the reading as it stands", got.VideoBytes)
	}
}

// TestRatesAreTakenOverThePipelinesOwnInterval is why the samples carry an uptime.
// A tick the scheduler held back divides a real delta by the interval that passed.
func TestRatesAreTakenOverThePipelinesOwnInterval(t *testing.T) {
	ref := StreamRef{Name: "desk", Transport: "srt"}
	last := receive.Stats{
		Uptime:     10 * time.Second,
		VideoBytes: 1_000_000, VideoFrames: 600, Rendered: 590, AudioBytes: 8_000,
	}
	now := receive.Stats{
		Uptime:     12 * time.Second,
		VideoBytes: 2_000_000, VideoFrames: 720, Rendered: 700, AudioBytes: 16_000,
	}

	got := receiveStatsOf(ref, now, last, true)

	// 1 MB over 2 s is 4 Mbit/s.
	if got.VideoMbps == nil || *got.VideoMbps != 4 {
		t.Errorf("video_mbps = %v, want 4", got.VideoMbps)
	}
	if got.VideoFPS == nil || *got.VideoFPS != 60 {
		t.Errorf("video_fps = %v, want 60", got.VideoFPS)
	}
	if got.RenderFPS == nil || *got.RenderFPS != 55 {
		t.Errorf("render_fps = %v, want 55", got.RenderFPS)
	}
	if got.AudioKbps == nil || *got.AudioKbps != 32 {
		t.Errorf("audio_kbps = %v, want 32", got.AudioKbps)
	}
}

// TestARebuiltPipelineReportsNoRate covers the decode rebuilt under the same ref,
// which is what turning tone mapping on does: the uptime and every counter restart,
// and the reading before it describes a pipeline that is gone.
func TestARebuiltPipelineReportsNoRate(t *testing.T) {
	ref := StreamRef{Name: "desk", Transport: "srt"}
	last := receive.Stats{Uptime: 90 * time.Second, VideoBytes: 50_000_000, VideoFrames: 5400}
	now := receive.Stats{Uptime: time.Second, VideoBytes: 400_000, VideoFrames: 60}

	got := receiveStatsOf(ref, now, last, true)

	if got.VideoMbps != nil || got.VideoFPS != nil {
		t.Errorf("a rebuilt pipeline reported a rate against the run before it: %+v", got)
	}
}

// TestAStalledCounterReportsZero separates an unmeasured rate from one measured at nothing.
// A pipeline whose bytes stopped moving is receiving nothing, and a reader has to see that.
func TestAStalledCounterReportsZero(t *testing.T) {
	ref := StreamRef{Name: "desk", Transport: "srt"}
	last := receive.Stats{Uptime: 10 * time.Second, VideoBytes: 1_000_000, VideoFrames: 600}
	now := receive.Stats{Uptime: 11 * time.Second, VideoBytes: 1_000_000, VideoFrames: 600}

	got := receiveStatsOf(ref, now, last, true)

	if got.VideoMbps == nil || *got.VideoMbps != 0 {
		t.Errorf("video_mbps = %v, want a measured 0", got.VideoMbps)
	}
	if got.VideoFPS == nil || *got.VideoFPS != 0 {
		t.Errorf("video_fps = %v, want a measured 0", got.VideoFPS)
	}
}

// TestUnnegotiatedFiguresStayAbsent covers the figures an opening pipeline answers no query for.
// Each carries presence, so a shell prints "unknown" rather than a latency window of zero,
// or a stream positioned at its first frame.
func TestUnnegotiatedFiguresStayAbsent(t *testing.T) {
	ref := StreamRef{Name: "desk", Transport: "srt"}

	got := receiveStatsOf(ref, receive.Stats{Uptime: time.Second}, receive.Stats{}, false)

	if got.SinceKeyframe != nil || got.LatencyMin != nil || got.LatencyMax != nil || got.Position != nil {
		t.Errorf("an opening pipeline reported a figure it has not answered for: %+v", got)
	}
}
