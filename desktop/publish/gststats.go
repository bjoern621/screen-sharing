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

// gstStatsName names the progressreport element whose lines the parser keys on;
// gstMeterName names the tee the sink branch continues from.
const (
	gstStatsName = "stats"
	gstMeterName = "meter"
)

// gstProgressLine matches one progressreport line: the pipeline running time the
// element last saw and the cumulative buffer count, one buffer per encoded
// frame. A line reads "stats (00:00:07): 141 buffers"; the query form of the
// element pads the time differently, hence the optional spaces.
var gstProgressLine = regexp.MustCompile(`^` + gstStatsName + ` \(\s*(\d+):\s*(\d+):\s*(\d+)\):\s+(\d+) buffers`)

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
	// r is the read end of the meter pipe, w the end the child inherits.
	r, w *os.File
	// now reads the wall clock the per-second figures are measured against.
	now func() time.Time

	// Previous and first sample, for the deltas the derived figures need. Only
	// parse touches them, from the one goroutine that reads stdout.
	prevFrames int
	prevBytes  int64
	prevWall   time.Time
	startRun   float64
	startWall  time.Time
	havePrev   bool
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

// parse reads the child's stdout and emits one sample per progress line, and
// returns when the stream ends.
func (m *gstMeter) parse(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		f := gstProgressLine.FindStringSubmatch(scanner.Text())
		if f == nil {
			continue
		}
		runSec := float64(atoi(f[1])*3600 + atoi(f[2])*60 + atoi(f[3]))
		m.sample(atoi(f[4]), runSec)
	}
}

// sample derives one Stats from the cumulative counts of a progress line and the
// byte counter, against the previous line.
func (m *gstMeter) sample(frames int, runSec float64) {
	now := m.now()
	total := m.bytes.Load()

	stats := Stats{
		Frame:   frames,
		SizeKiB: float64(total) / 1024,
		TimeSec: runSec,
	}
	if runSec > 0 {
		stats.AvgMbps = float64(total) * 8 / runSec / 1_000_000
	}
	if m.havePrev {
		if d := now.Sub(m.prevWall).Seconds(); d > 0 {
			stats.Fps = float64(frames-m.prevFrames) / d
			stats.InstMbps = float64(total-m.prevBytes) * 8 / d / 1_000_000
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
	m.prevFrames, m.prevBytes, m.prevWall, m.havePrev = frames, total, now, true

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
