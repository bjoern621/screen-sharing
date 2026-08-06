package publish

import (
	"bufio"
	"io"
	"os"
	"regexp"
	"strconv"
	"sync/atomic"
	"time"
)

// gstStatsName names the progressreport element counting encoded frames;
// gstCaptureName names the one counting the frames the screen really produced,
// which the capture backend places ahead of anything that repeats or paces them;
// gstMeterName names the tee the sink branch continues from.
const (
	gstStatsName   = "stats"
	gstCaptureName = "capture"
	gstMeterName   = "meter"
)

// gstCaptureProbe is the progressreport element a capture backend splices in to
// count what the source produced. It is built here rather than in the backend so
// both halves of the wire format stay one decision, as for the encoded counter.
//
// One argument per token, like every other element list here: gst-launch parses
// its argv token by token, so an element and its properties in a single argument
// is a parse error rather than an element.
var gstCaptureProbe = []string{
	"progressreport", "name=" + gstCaptureName, "update-freq=1", "format=buffers", "do-query=false",
}

// gstProgressLine matches one progressreport line: the element that printed it,
// the running time it last saw and its cumulative buffer count. A line reads
// "stats (00:00:07): 141 buffers"; the query form of the element pads the time
// differently, hence the optional spaces.
var gstProgressLine = regexp.MustCompile(`^(` + gstStatsName + `|` + gstCaptureName + `) \(\s*(\d+):\s*(\d+):\s*(\d+)\):\s+(\d+) buffers`)

// gstProgressElements returns the instrumentation buildPipeline splices in
// between the parser and the sink, the pair gstMeter reads. It sits next to the
// parser because both halves of the wire format are one decision: the element
// properties here produce the lines gstProgressLine matches.
//
// progressreport counts buffers rather than querying a position, because no
// element upstream of an encoded stream answers a byte or time query, and it
// prints the count and the pipeline running time to stdout once a second.
//
// The tee exists because no GStreamer element reports byte throughput: the
// second branch hands a copy of the encoded video to an fdsink on a pipe the app
// weighs. The branch cannot hold up the encode path, since its queue leaks and
// its sink neither synchronizes to the clock nor prerolls. Its bytes are the
// video elementary stream, so the figures come out below ffmpeg's, which counts
// the muxed stream with its audio track and container overhead.
func gstProgressElements(meterFd string) []string {
	return []string{
		"progressreport", "name=" + gstStatsName, "update-freq=1", "format=buffers", "do-query=false",
		"!", "tee", "name=" + gstMeterName,
		"!", "queue", "max-size-buffers=8", "leaky=downstream",
		"!", "fdsink", "fd=" + meterFd, "sync=false", "async=false",
		gstMeterName + ".", "!",
	}
}

// gstMeter turns what a gst-launch child can report into the same Stats samples
// the ffmpeg engine emits. GStreamer has no counterpart to ffmpeg's -progress
// stream, so two elements in the pipeline carry the raw counts: a progressreport
// prints the encoded frame count and the running time once a second, and an
// fdsink writes a second copy of the encoded video into a pipe this type counts.
// Frames arrive as text and bytes as data, so the sample is joined here: each
// progress line takes the byte counter as it stands.
//
// Stats.Drop stays zero because nothing on the encode path discards a frame.
// The one intentional drop in the graph is the single-slot leaky queue ahead of
// videoconvert, which only discards a damage frame a newer one supersedes.
type gstMeter struct {
	onStats func(Stats)
	bytes   atomic.Int64
	// captured is the newest cumulative count from the capture backend's rate
	// probe. It is atomic for the same reason bytes is: the parse goroutine and
	// the counting goroutine are not the same one.
	captured atomic.Int64
	// haveCaptured records that a capture line arrived at all, which is what
	// tells a probe reporting a genuine zero rate apart from a pipeline that
	// carries no probe to report one.
	haveCaptured atomic.Bool
	// r is the read end of the meter pipe, w the end the child inherits.
	r, w *os.File
	// now reads the wall clock the per-second figures are measured against.
	now func() time.Time

	// Previous and first sample, for the deltas the derived figures need. Only
	// parse touches them, from the one goroutine that reads stdout.
	prevFrames   int
	prevBytes    int64
	prevCaptured int
	prevWall     time.Time
	startRun     float64
	startWall    time.Time
	havePrev     bool
}

// newGstMeter creates the pipe the child's fdsink writes to and starts counting
// what arrives on it.
func newGstMeter(onStats func(Stats)) (*gstMeter, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	m := &gstMeter{onStats: onStats, r: r, w: w, now: time.Now}
	go m.count()
	return m, nil
}

// count drains the meter pipe. The payload is a copy of what the sink ships, so
// it is weighed and discarded, never inspected.
func (m *gstMeter) count() {
	buf := make([]byte, 64*1024)
	for {
		n, err := m.r.Read(buf)
		m.bytes.Add(int64(n))
		if err != nil {
			return
		}
	}
}

// parse reads the child's stdout and emits one sample per encoded progress line,
// and returns when the stream ends.
//
// The capture counter is recorded rather than sampled. Both elements print once a
// second but not in step, so emitting on either would produce two samples per
// second with one of the two counts unchanged; the encoded line is the one that
// carries the byte counter, so it stays the sample point and reads whatever the
// capture counter last said.
func (m *gstMeter) parse(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		f := gstProgressLine.FindStringSubmatch(scanner.Text())
		if f == nil {
			continue
		}
		frames := atoi(f[5])
		if f[1] == gstCaptureName {
			m.captured.Store(int64(frames))
			m.haveCaptured.Store(true)
			continue
		}
		runSec := float64(atoi(f[2])*3600 + atoi(f[3])*60 + atoi(f[4]))
		m.sample(frames, runSec)
	}
}

// sample derives one Stats from the cumulative counts of a progress line and the
// byte counter, against the previous line.
func (m *gstMeter) sample(frames int, runSec float64) {
	now := m.now()
	total := m.bytes.Load()
	captured := int(m.captured.Load())

	stats := Stats{
		Frame:   frames,
		SizeKiB: float64(total) / 1024,
		TimeSec: runSec,
	}
	// A per-interval figure has no value on the first line of a run, and this
	// engine measures no capture rate at all unless the backend placed the probe.
	// Both are unmeasured rather than zero, which is the reading that marks a
	// stalled encoder.
	stats.Missing.Fps = !m.havePrev
	stats.Missing.InstMbps = !m.havePrev
	stats.Missing.Speed = !m.havePrev
	stats.Missing.CaptureFps = !m.havePrev || !m.haveCaptured.Load()
	if runSec > 0 {
		stats.AvgMbps = float64(total) * 8 / runSec / 1_000_000
	} else {
		stats.Missing.AvgMbps = true
	}
	if m.havePrev {
		if d := now.Sub(m.prevWall).Seconds(); d > 0 {
			stats.Fps = float64(frames-m.prevFrames) / d
			stats.InstMbps = float64(total-m.prevBytes) * 8 / d / 1_000_000
			stats.CaptureFps = float64(captured-m.prevCaptured) / d
		}
		// Speed measures media time against wall time over the whole run.
		// progressreport prints whole seconds, so the same ratio taken between
		// two lines would swing by a whole step whenever the two clocks land on
		// opposite sides of a second boundary.
		if d := now.Sub(m.startWall).Seconds(); d > 0 {
			stats.Speed = (runSec - m.startRun) / d
		}
	} else {
		m.startRun, m.startWall = runSec, now
	}
	m.prevFrames, m.prevBytes, m.prevCaptured, m.prevWall, m.havePrev = frames, total, captured, now, true

	m.onStats(stats)
}

// closeChildEnd drops the parent's copy of the write end once the child holds its
// own, so the counter sees EOF when the child exits.
func (m *gstMeter) closeChildEnd() {
	if m == nil {
		return
	}
	m.w.Close()
}

// close releases the pipe, which ends the counting goroutine. A nil meter (a
// start with no OnStats callback) closes nothing.
func (m *gstMeter) close() {
	if m == nil {
		return
	}
	m.w.Close()
	m.r.Close()
}

// atoi returns the integer value of a digit group the progress-line pattern
// already matched.
func atoi(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}
