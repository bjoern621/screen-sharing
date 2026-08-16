package publish

import (
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/ffmpeg"
)

// The ffmpeg engine reports no byte total on a leg whose muxer states none, RTP and the tee among
// them, so every rate derived from one is missing for the whole run.
// The meter weighs the packets the tee copies to it and fills exactly those figures.
func TestTheMeterFillsTheRatesTheMuxerReportsNone(t *testing.T) {
	wall := time.Unix(1000, 0)
	m := &ffmpegMeter{now: func() time.Time { return wall }, start: wall}

	// Half a megabyte over the first second: 4 Mbit/s, and no interval to measure against yet.
	wall = wall.Add(time.Second)
	m.bytes.Store(500_000)
	first := m.fill(ffmpeg.Stats{Missing: ffmpeg.Missing{InstMbps: true, AvgMbps: true, SizeKiB: true}})

	if first.Missing.AvgMbps {
		t.Error("the run's own average is missing where the meter has weighed the whole run")
	}
	if got := first.AvgMbps; got < 3.9 || got > 4.1 {
		t.Errorf("the run averaged %.2f Mbit/s, want 4.00", got)
	}
	if !first.Missing.InstMbps {
		t.Error("an interval rate was reported off the first sample, which has no interval behind it")
	}
	if first.Missing.SizeKiB || first.SizeKiB < 488 || first.SizeKiB > 489 {
		t.Errorf("the run wrote %.0f KiB, want about 488", first.SizeKiB)
	}

	// Another half megabyte over the next second.
	wall = wall.Add(time.Second)
	m.bytes.Store(1_000_000)
	second := m.fill(ffmpeg.Stats{Missing: ffmpeg.Missing{InstMbps: true, AvgMbps: true, SizeKiB: true}})

	if second.Missing.InstMbps {
		t.Fatal("the interval rate is missing where two samples bracket an interval")
	}
	if got := second.InstMbps; got < 3.9 || got > 4.1 {
		t.Errorf("the last interval ran at %.2f Mbit/s, want 4.00", got)
	}
}

// A figure the encoder measured is the encoder's.
// The meter weighs the video the tee copies to it, where ffmpeg weighs the muxed stream it writes,
// so overwriting a reported figure would swap one measurement for a narrower one.
func TestTheMeterLeavesAReportedRateAlone(t *testing.T) {
	wall := time.Unix(1000, 0)
	m := &ffmpegMeter{now: func() time.Time { return wall }, start: wall}

	wall = wall.Add(time.Second)
	m.bytes.Store(500_000)
	got := m.fill(ffmpeg.Stats{InstMbps: 12, AvgMbps: 11, SizeKiB: 9})

	if got.InstMbps != 12 || got.AvgMbps != 11 || got.SizeKiB != 9 {
		t.Errorf("a measured sample came back as %.0f/%.0f/%.0f, want 12/11/9 left alone",
			got.InstMbps, got.AvgMbps, got.SizeKiB)
	}
}
