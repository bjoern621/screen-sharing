package publish

import (
	"math"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"bjoernblessin.de/screenshare/settings"
)

// The parser takes the cumulative counts off a progress line and ignores every
// other line gst-launch prints. The first line has no predecessor, so only the
// cumulative figures are set; the second carries the deltas, measured against
// the wall clock: 30 frames and 250 kB in half a second is 60 fps at 4 Mbps.
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
	wantFirst := Stats{Frame: 30, SizeKiB: 250_000.0 / 1024, TimeSec: 1, AvgMbps: 2}
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
	}
	if second != wantSecond {
		t.Errorf("second sample = %+v, want %+v", second, wantSecond)
	}
}

// The instrumentation splits the encoded stream between the parser and the
// muxer, and the displayed command carries none of it.
func TestGstProgressElementsPlacement(t *testing.T) {
	s := settings.Defaults()
	s.Capture = "portal"
	s.Transport = "srt"

	plain, err := buildPipeline(s, "3", "42", "")
	if err != nil {
		t.Fatal(err)
	}
	if line := strings.Join(plain, " "); strings.Contains(line, "progressreport") ||
		strings.Contains(line, "fdsink") {
		t.Errorf("pipeline built without a meter fd carries instrumentation: %s", line)
	}

	metered, err := buildPipeline(s, "3", "42", "4")
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(metered, " ")
	parser := strings.Index(line, "h264parse")
	report := strings.Index(line, "progressreport")
	meterBranch := strings.Index(line, "fdsink fd=4")
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
// matches, and the fdsink has to reach the inherited descriptor. Both hold only
// against a real gst-launch, so this runs one.
func TestGstMeterAgainstGstLaunch(t *testing.T) {
	if _, err := exec.LookPath(gstExe); err != nil {
		t.Skipf("%s not installed", gstExe)
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

	// videotestsrc stands in for the portal capture; the meter fd is 3 because
	// this pipeline inherits no PipeWire remote ahead of it.
	args := []string{
		"videotestsrc", "is-live=true",
		"!", "video/x-raw,format=I420,width=320,height=240,framerate=30/1",
		"!", "x264enc", "bitrate=2000", "pass=cbr", "tune=zerolatency",
		"!", "h264parse",
		"!",
	}
	args = append(args, gstProgressElements("3")...)
	args = append(args, "fakesink")

	handle, err := supervise(superviseConfig{
		exe:         gstExe,
		args:        args,
		tag:         "gstmeter-test",
		extraFiles:  []*os.File{meter.w},
		parseStdout: meter.parse,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Stop()
	meter.closeChildEnd()

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
