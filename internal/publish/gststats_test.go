package publish

import (
	"math"
	"os/exec"
	"strings"
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/settings"
)

// The parser takes the cumulative counts off a progress line and ignores every
// other line gst-launch prints. The first line has no predecessor, so only the
// cumulative figures are set and every per-interval one is marked unmeasured;
// the second carries the deltas, measured against the wall clock: 30 frames and
// 250 kB in half a second is 60 fps at 4 Mbps. This pipeline carries no rate
// probe, so the capture rate stays unmeasured throughout rather than reading as
// a zero rate.
func TestGstMeterSamples(t *testing.T) {
	wall := time.Unix(0, 0)
	var got []Stats
	m := &gstMeter{
		onStats: func(s Stats) { got = append(got, s) },
		now:     func() time.Time { return wall },
	}

	m.bytes.Store(250_000)
	m.parse(strings.NewReader("stats (00:00:01): 30 buffers\n"))

	wall = wall.Add(500 * time.Millisecond)
	m.bytes.Store(500_000)
	m.parse(strings.NewReader(
		"Setting pipeline to PLAYING ...\n" +
			"progressreport0 (00:00:02): 60 buffers\n" + // another element's report
			"stats (00:00:02): 60 buffers\n"))

	if len(got) != 2 {
		t.Fatalf("got %d samples, want 2 (one per stats line)", len(got))
	}

	first, second := got[0], got[1]
	wantFirst := Stats{
		Frame: 30, SizeKiB: 250_000.0 / 1024, TimeSec: 1, AvgMbps: 2,
		Missing: Missing{Fps: true, InstMbps: true, Speed: true, CaptureFps: true},
	}
	if first != wantFirst {
		t.Errorf("first sample = %+v, want %+v", first, wantFirst)
	}
	wantSecond := Stats{
		Frame:    60,
		Fps:      60,
		SizeKiB:  500_000.0 / 1024,
		TimeSec:  2,
		Speed:    2, // one second of media in half a second of wall clock
		InstMbps: 4,
		AvgMbps:  2,
		Missing:  Missing{CaptureFps: true},
	}
	if second != wantSecond {
		t.Errorf("second sample = %+v, want %+v", second, wantSecond)
	}
}

// The instrumentation splits the encoded stream between the parser and the
// muxer, and the displayed command carries none of it.
func TestGstProgressElementsPlacement(t *testing.T) {
	s := settings.Defaults()
	s.Publish.Capture = "portal"
	s.Publish.Transport = "srt"
	// The default planar RGB has no encoder element on this engine, which the form
	// repairs to 4:4:4 before a portal publish; this test is about element order.
	s.Publish.Chroma = "yuv444p"

	capture := []string{"videotestsrc"}
	plain, err := buildPipeline(s, capture, "")
	if err != nil {
		t.Fatal(err)
	}
	if line := strings.Join(plain, " "); strings.Contains(line, "progressreport") ||
		strings.Contains(line, "tcpclientsink") {
		t.Errorf("pipeline built without a meter port carries instrumentation: %s", line)
	}

	metered, err := buildPipeline(s, capture, "54321")
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(metered, " ")
	parser := strings.Index(line, "h264parse")
	report := strings.Index(line, "progressreport")
	meterBranch := strings.Index(line, "tcpclientsink host=127.0.0.1 port=54321")
	sinkBranch := strings.Index(line, gstMeterName+". !")
	mux := strings.Index(line, "mpegtsmux")
	if !(parser < report && report < meterBranch && meterBranch < sinkBranch && sinkBranch < mux) {
		t.Errorf("instrumentation out of order between parser and muxer: %s", line)
	}
	// gst-launch links no pair of unnamed request pads, which is what the tee
	// and the muxer expose, so the sink branch carries a queue even with audio
	// off.
	if !strings.Contains(line[sinkBranch:mux], "queue") {
		t.Errorf("no queue between the meter tee and the muxer: %s", line)
	}
}

// A pipeline stall leaves the running time behind the wall clock, which is what
// the speed figure reports.
func TestGstMeterSpeedBelowRealtime(t *testing.T) {
	wall := time.Unix(0, 0)
	var last Stats
	m := &gstMeter{
		onStats: func(s Stats) { last = s },
		now:     func() time.Time { return wall },
	}

	m.parse(strings.NewReader("stats (00:00:01): 60 buffers\n"))
	wall = wall.Add(4 * time.Second)
	m.parse(strings.NewReader("stats (00:00:03): 120 buffers\n"))

	if math.Abs(last.Speed-0.5) > 0.001 {
		t.Errorf("speed = %v, want 0.5", last.Speed)
	}
}

// The instrumentation is a wire format shared with GStreamer: the element
// properties gstProgressElements sets have to produce the lines the parser
// matches, and the tcpclientsink has to reach the meter's listener. Both hold
// only against a real gst-launch, so this runs one.
func TestGstMeterAgainstGstLaunch(t *testing.T) {
	if _, err := exec.LookPath(GstExe); err != nil {
		t.Skipf("%s not installed", GstExe)
	}
	if err := exec.Command("gst-inspect-1.0", "--exists", "x264enc").Run(); err != nil {
		t.Skip("x264enc plugin not installed")
	}

	samples := make(chan Stats, 8)
	meter, err := newGstMeter(func(s Stats) {
		select {
		case samples <- s:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer meter.close()

	// videotestsrc stands in for the portal capture, which this pipeline needs
	// none of: the meter reaches the app over loopback either way.
	args := []string{
		"videotestsrc", "is-live=true",
		"!", "video/x-raw,format=I420,width=320,height=240,framerate=30/1",
		"!", "x264enc", "bitrate=2000", "pass=cbr", "tune=zerolatency",
		"!", "h264parse",
		"!",
	}
	args = append(args, gstProgressElements(meter.port())...)
	args = append(args, "fakesink")

	handle, err := supervise(superviseConfig{
		exe:         GstExe,
		args:        args,
		tag:         "gstmeter-test",
		parseStdout: meter.parse,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Stop()

	// progressreport prints once a second and needs a second of data first.
	var latest Stats
	deadline := time.After(15 * time.Second)
	for latest.InstMbps == 0 {
		select {
		case latest = <-samples:
		case <-deadline:
			t.Fatal("no progress sample with a bitrate within 15s")
		}
	}

	if latest.Frame < 20 {
		t.Errorf("frame count = %d, want the encoded frames of at least a second", latest.Frame)
	}
	if latest.Fps < 20 || latest.Fps > 40 {
		t.Errorf("fps = %v, want about the 30 fps the source produces", latest.Fps)
	}
	if latest.InstMbps < 0.5 || latest.InstMbps > 8 {
		t.Errorf("bitrate = %v Mbps, want about the 2 Mbps the encoder targets", latest.InstMbps)
	}
}

// The two counters answer different questions, and the sample carries both. A
// portal capture whose screen changed five times while the encoder emitted sixty
// frames has to report five, because that is what a viewer sees; reporting the
// encoder's sixty would hand the pacing target back as a measurement.
//
// The capture element prints on its own second, out of step with the encoded one,
// so a capture line must record rather than sample: two samples a second, one of
// them with an unchanged count, would halve every rate the card shows.
func TestGstMeterCaptureRateIsSeparateFromTheEncodedRate(t *testing.T) {
	wall := time.Unix(0, 0)
	var got []Stats
	m := &gstMeter{
		onStats: func(s Stats) { got = append(got, s) },
		now:     func() time.Time { return wall },
	}

	m.parse(strings.NewReader(
		"capture (00:00:01): 5 buffers\n" +
			"stats (00:00:01): 60 buffers\n"))
	wall = wall.Add(time.Second)
	m.parse(strings.NewReader(
		"capture (00:00:02): 10 buffers\n" +
			"stats (00:00:02): 120 buffers\n"))

	if len(got) != 2 {
		t.Fatalf("got %d samples, want 2: a capture line records, it does not sample", len(got))
	}
	if got[1].Fps != 60 {
		t.Errorf("encoded rate = %v, want 60", got[1].Fps)
	}
	if got[1].CaptureFps != 5 {
		t.Errorf("capture rate = %v, want 5", got[1].CaptureFps)
	}
}

// A pipeline built without the rate probe prints no capture line, and an
// unmeasured rate reads zero rather than borrowing the encoded one.
func TestGstMeterCaptureRateIsZeroWithoutTheProbe(t *testing.T) {
	wall := time.Unix(0, 0)
	var got []Stats
	m := &gstMeter{
		onStats: func(s Stats) { got = append(got, s) },
		now:     func() time.Time { return wall },
	}
	m.parse(strings.NewReader("stats (00:00:01): 60 buffers\n"))
	wall = wall.Add(time.Second)
	m.parse(strings.NewReader("stats (00:00:02): 120 buffers\n"))

	if got[1].CaptureFps != 0 {
		t.Errorf("capture rate = %v, want 0 for a pipeline with no probe", got[1].CaptureFps)
	}
}

// The same divergence against a running pipeline rather than synthetic lines. A
// source producing five pictures a second behind an imagefreeze paced to thirty
// is the portal path's shape: the encoder emits thirty, the screen produced five,
// and only the probe ahead of the pacer can tell the two apart.
func TestGstMeterCaptureRateAgainstGstLaunch(t *testing.T) {
	if _, err := exec.LookPath(GstExe); err != nil {
		t.Skipf("%s not installed", GstExe)
	}
	if err := exec.Command("gst-inspect-1.0", "--exists", "x264enc").Run(); err != nil {
		t.Skip("x264enc plugin not installed")
	}

	samples := make(chan Stats, 8)
	meter, err := newGstMeter(func(s Stats) {
		select {
		case samples <- s:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer meter.close()

	const sourceFps, pacedFps = 5, 30
	args := []string{
		"videotestsrc", "is-live=true",
		"!", "video/x-raw,format=I420,width=320,height=240,framerate=5/1",
		"!",
	}
	args = append(args, gstCaptureProbe...)
	args = append(args,
		"!", "imagefreeze", "is-live=true", "allow-replace=true",
		"!", "video/x-raw,framerate=30/1",
		"!", "x264enc", "bitrate=2000", "pass=cbr", "tune=zerolatency",
		"!", "h264parse",
		"!",
	)
	args = append(args, gstProgressElements(meter.port())...)
	args = append(args, "fakesink")

	handle, err := supervise(superviseConfig{
		exe:         GstExe,
		args:        args,
		tag:         "gstcapturerate-test",
		parseStdout: meter.parse,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Stop()

	var latest Stats
	deadline := time.After(20 * time.Second)
	for latest.CaptureFps == 0 {
		select {
		case latest = <-samples:
		case <-deadline:
			t.Fatal("no progress sample carrying a capture rate within 20s")
		}
	}

	if math.Abs(latest.Fps-pacedFps) > 10 {
		t.Errorf("encoded rate = %v, want about the %d fps the pacer holds", latest.Fps, pacedFps)
	}
	if math.Abs(latest.CaptureFps-sourceFps) > 3 {
		t.Errorf("capture rate = %v, want about the %d fps the source produces", latest.CaptureFps, sourceFps)
	}
	if latest.CaptureFps >= latest.Fps {
		t.Errorf("capture rate %v must read below the encoded rate %v: the pacer repeats frames the source never sent",
			latest.CaptureFps, latest.Fps)
	}
}

// An unmeasured capture rate and a measured zero are different readings. A
// pipeline with no probe carries no capture line at all, so the figure is marked
// missing; one whose probe reports no new pictures reports zero, which is the
// starved capture worth seeing.
func TestGstMeterMarksAnUnprobedCaptureRateMissing(t *testing.T) {
	sampleTwice := func(lines ...string) Stats {
		wall := time.Unix(0, 0)
		var got []Stats
		m := &gstMeter{
			onStats: func(s Stats) { got = append(got, s) },
			now:     func() time.Time { return wall },
		}
		m.parse(strings.NewReader(lines[0]))
		wall = wall.Add(time.Second)
		m.parse(strings.NewReader(lines[1]))
		return got[len(got)-1]
	}

	unprobed := sampleTwice("stats (00:00:01): 60 buffers\n", "stats (00:00:02): 120 buffers\n")
	if !unprobed.Missing.CaptureFps {
		t.Error("a pipeline with no rate probe must mark the capture rate missing")
	}

	starved := sampleTwice(
		"capture (00:00:01): 7 buffers\nstats (00:00:01): 60 buffers\n",
		"capture (00:00:02): 7 buffers\nstats (00:00:02): 120 buffers\n")
	if starved.Missing.CaptureFps {
		t.Error("a probe reporting no new pictures is a measurement, not a missing figure")
	}
	if starved.CaptureFps != 0 {
		t.Errorf("capture rate = %v, want 0: the screen produced nothing in that second", starved.CaptureFps)
	}
}
