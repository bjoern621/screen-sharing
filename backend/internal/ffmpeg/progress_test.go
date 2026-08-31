package ffmpeg

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

// progressBlock renders one -progress block from the fields a test sets,
// keeping ffmpeg's own spelling and the terminating "progress=" line.
func progressBlock(fields ...string) string {
	return strings.Join(fields, "\n") + "\nprogress=continue\n"
}

// newTestParser returns a parser whose clock the test advances by hand,
// putting the per-interval figures on a known interval.
func newTestParser(wall *time.Time, got *[]Stats) *progressParser {
	return &progressParser{
		onStats: func(s Stats) { *got = append(*got, s) },
		now:     func() time.Time { return *wall },
	}
}

// The counters in a block are cumulative and the per-interval figures come off the previous block.
// A first block has no predecessor and carries no rate.
// The second is measured against the wall clock: 30 frames
// and 250 kB in half a second is 60 fps at 4 Mbps.
func TestParseProgressSamples(t *testing.T) {
	wall := time.Unix(0, 0)
	var got []Stats
	p := newTestParser(&wall, &got)

	p.parse(strings.NewReader(progressBlock(
		"frame=30",
		"fps=30.00",
		"bitrate=2000.0kbits/s",
		"total_size=250000",
		"out_time_us=1000000",
		"dup_frames=0",
		"drop_frames=0",
		"speed=1.0x",
	)))

	wall = wall.Add(500 * time.Millisecond)
	p.parse(strings.NewReader(progressBlock(
		"frame=60",
		"fps=30.00",
		"bitrate=2000.0kbits/s",
		"total_size=500000",
		"out_time_us=2000000",
		"dup_frames=7",
		"drop_frames=2",
		"speed=0.5x",
	)))

	if len(got) != 2 {
		t.Fatalf("got %d samples, want 2 (one per progress block)", len(got))
	}

	first, second := got[0], got[1]
	wantFirst := Stats{
		Frame:   30,
		SizeKiB: 250_000.0 / 1024,
		TimeSec: 1,
		Speed:   1,
		AvgMbps: 2,
		Missing: Missing{Fps: true, CaptureFps: true, InstMbps: true, TransitMs: true, LinkMs: true, RttMs: true},
	}
	if first != wantFirst {
		t.Errorf("first sample = %+v, want %+v", first, wantFirst)
	}
	wantSecond := Stats{
		Frame:    60,
		Fps:      60,
		SizeKiB:  500_000.0 / 1024,
		TimeSec:  2,
		Speed:    0.5,
		Dup:      7,
		Drop:     2,
		InstMbps: 4,
		AvgMbps:  2,
		Missing:  Missing{CaptureFps: true, TransitMs: true, LinkMs: true, RttMs: true},
	}
	if second != wantSecond {
		t.Errorf("second sample = %+v, want %+v", second, wantSecond)
	}
}

// Fps is the rate over the last interval and not over the run.
// The fixture is a collapse, which is where the two part: ffmpeg's own fps field still reports
// the run's 60 while the frame counter advances by five in a second.
func TestParseProgressFpsIsPerInterval(t *testing.T) {
	wall := time.Unix(0, 0)
	var got []Stats
	p := newTestParser(&wall, &got)

	p.parse(strings.NewReader(progressBlock("frame=600", "fps=60.00", "total_size=1000")))
	wall = wall.Add(time.Second)
	p.parse(strings.NewReader(progressBlock("frame=605", "fps=60.00", "total_size=2000")))

	last := got[len(got)-1]
	if last.Missing.Fps {
		t.Fatal("second sample reports no fps, want the rate over the interval")
	}
	if math.Abs(last.Fps-5) > 0.001 {
		t.Errorf("fps = %v, want 5, the frames the interval added", last.Fps)
	}
}

// "N/A" is ffmpeg holding no figure yet, which is not a figure of zero.
// The block after it has no byte baseline either, so its bitrate stays unmeasured rather
// than counting the whole output as one interval's worth.
func TestParseProgressUnmeasuredFields(t *testing.T) {
	wall := time.Unix(0, 0)
	var got []Stats
	p := newTestParser(&wall, &got)

	p.parse(strings.NewReader(progressBlock(
		"frame=0",
		"bitrate=N/A",
		"total_size=N/A",
		"out_time_us=N/A",
		"speed=N/A",
	)))
	wall = wall.Add(time.Second)
	p.parse(strings.NewReader(progressBlock(
		"frame=60",
		"bitrate=2000.0kbits/s",
		"total_size=1000000",
		"out_time_us=1000000",
		"speed=1.0x",
	)))
	wall = wall.Add(time.Second)
	p.parse(strings.NewReader(progressBlock(
		"frame=120",
		"bitrate=2000.0kbits/s",
		"total_size=1500000",
		"out_time_us=2000000",
		"speed=1.0x",
	)))

	if len(got) != 3 {
		t.Fatalf("got %d samples, want 3", len(got))
	}

	want := Missing{
		Fps: true, CaptureFps: true, SizeKiB: true, TimeSec: true, Speed: true, InstMbps: true,
		AvgMbps: true, TransitMs: true, LinkMs: true, RttMs: true,
	}
	if got[0].Missing != want {
		t.Errorf("first sample missing set = %+v, want %+v", got[0].Missing, want)
	}
	if !got[1].Missing.InstMbps {
		t.Errorf("bitrate = %v Mbps across the block with no byte total, want unmeasured", got[1].InstMbps)
	}
	if got[2].Missing.InstMbps || math.Abs(got[2].InstMbps-4) > 0.001 {
		t.Errorf("bitrate = %v Mbps (missing %v), want 4 from the first pair of byte totals",
			got[2].InstMbps, got[2].Missing.InstMbps)
	}
}

// The two states a UI has to tell apart stay apart on the wire: an unmeasured figure is null
// and a measured zero is 0.
func TestStatsMarshalJSONMissingIsNull(t *testing.T) {
	blob, err := json.Marshal(Stats{
		Frame:   12,
		Speed:   0,
		AvgMbps: 3,
		Missing: Missing{Fps: true, CaptureFps: true, InstMbps: true, TransitMs: true, LinkMs: true, RttMs: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	var wire map[string]any
	if err := json.Unmarshal(blob, &wire); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"fps", "captureFps", "instMbps"} {
		if wire[key] != nil {
			t.Errorf("%s = %v, want null for a figure no engine measured", key, wire[key])
		}
	}
	if wire["speed"] != float64(0) {
		t.Errorf("speed = %v, want a measured 0", wire["speed"])
	}
	if wire["avgMbps"] != float64(3) {
		t.Errorf("avgMbps = %v, want 3", wire["avgMbps"])
	}
}
