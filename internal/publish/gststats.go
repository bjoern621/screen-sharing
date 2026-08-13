package publish

import (
	"bufio"
	"io"
	"net"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
)

// gstStatsName names the progressreport element counting encoded frames; gstCaptureName names the
// one counting the frames the screen really produced, which the capture backend places ahead of
// anything that repeats or paces them; gstTeeName names the tee every extra branch and the sink
// itself continue from.
const (
	gstStatsName   = "stats"
	gstCaptureName = "capture"
	gstTeeName     = "meter"
)

// gstCaptureProbe is the progressreport element a capture backend splices in to count what the
// source produced.
// It is built here rather than in the backend so both halves of the wire format stay one decision,
// as for the encoded counter.
//
// One argument per token, like every other element list here: gst-launch parses its argv token by
// token, so an element and its properties in a single argument is a parse error rather than an
// element.
var gstCaptureProbe = []string{
	"progressreport", "name=" + gstCaptureName, "update-freq=1", "format=buffers", "do-query=false",
}

// gstProgressLine matches one progressreport line: the element that printed it,
// the running time it last saw and its cumulative buffer count.
// A line reads "stats (00:00:07): 141 buffers"; the query form of the element pads the time
// differently, hence the optional spaces.
var gstProgressLine = regexp.MustCompile(`^(` + gstStatsName + `|` + gstCaptureName + `) \(\s*(\d+):\s*(\d+):\s*(\d+)\):\s+(\d+) buffers`)

// gstProgressElement is the counter buildPipeline splices in between the parser and the tee,
// the line gstMeter reads.
// It sits next to the pattern that reads it because both halves of the wire format are one
// decision: the element properties here produce the lines gstProgressLine matches.
//
// progressreport counts buffers rather than querying a position, because no element upstream of an
// encoded stream answers a byte or time query, and it prints the count and the pipeline running
// time to stdout once a second.
var gstProgressElement = []string{
	"progressreport", "name=" + gstStatsName, "update-freq=1", "format=buffers", "do-query=false",
}

// gstMeterTap is the branch that weighs the encoded stream, because no GStreamer element reports
// byte throughput: it hands a copy of the encoded video to a tcpclientsink on a loopback socket the
// app counts.
// The branch cannot hold up the encode path, since its queue leaks and its sink neither
// synchronizes to the clock nor prerolls.
// Its bytes are the video elementary stream, so the figures come out below ffmpeg's,
// which counts the muxed stream with its audio track and container overhead.
//
// A socket rather than an inherited descriptor because Windows inherits none:
// os/exec supports ExtraFiles on Unix alone, and a child handed one there fails to start at all.
// The one mechanism both platforms carry is the one both use, so the meter has a single wire format
// rather than one per operating system.
func gstMeterTap(meterPort string) []string {
	assert.Assert(meterPort != "", "a meter branch names the port it reports to")

	return []string{
		"queue", "max-size-buffers=8", "leaky=downstream",
		"!", "tcpclientsink", "host=" + gstMeterHost, "port=" + meterPort, "sync=false", "async=false",
	}
}

// gstTapElements returns the tee and the branches that take a copy of the encoded stream off it,
// ending in the reference the trunk resumes from.
// It is empty for a run that taps nothing, which is what a rendered command is.
//
// The trunk is a branch of the tee like the others rather than the element the tee was linked into,
// so which branch is which is one rule: every branch, the muxer's included,
// starts from the tee by name.
// That is also what lets a second tap be added without the first one's shape changing.
func gstTapElements(taps [][]string) []string {
	if len(taps) == 0 {
		return nil
	}

	out := []string{"tee", "name=" + gstTeeName}
	for _, tap := range taps {
		assert.Assert(len(tap) > 0, "a tap off the encoded stream yields elements")

		out = append(out, "!")
		out = append(out, tap...)
		out = append(out, gstTeeName+".")
	}
	return append(out, "!")
}

// gstMeterHost is the address the meter listens on and the child connects back to.
// Loopback alone: the branch carries a copy of the user's screen, and the only peer it is meant for
// is the child this process just spawned.
const gstMeterHost = "127.0.0.1"

// gstMeter turns what a gst-launch child can report into the same Stats samples the ffmpeg engine
// emits.
// GStreamer has no counterpart to ffmpeg's -progress stream, so two elements in the pipeline carry
// the raw counts: a progressreport prints the encoded frame count and the running time once a
// second, and a tcpclientsink writes a second copy of the encoded video to a loopback socket this
// type counts.
// Frames arrive as text and bytes as data, so the sample is joined here: each progress line takes
// the byte counter as it stands.
//
// Stats.Drop stays zero because nothing on the encode path discards a frame.
// The one intentional drop in the graph is the single-slot leaky queue ahead of videoconvert,
// which only discards a damage frame a newer one supersedes.
type gstMeter struct {
	onStats func(Stats)
	bytes   atomic.Int64
	// captured is the newest cumulative count from the capture backend's rate probe.
	// It is atomic for the same reason bytes is: the parse goroutine and the counting goroutine are
	// not the same one.
	captured atomic.Int64
	// haveCaptured records that a capture line arrived at all, which is what tells a probe reporting a
	// genuine zero rate apart from a pipeline that carries no probe to report one.
	haveCaptured atomic.Bool
	// ln accepts the one connection the child's tcpclientsink opens, and conn is that connection once
	// it arrives.
	// conn is held so close can end the read the counting goroutine is parked in,
	// and mu guards the pair against a close that lands while the accept is still outstanding.
	mu     sync.Mutex
	ln     net.Listener
	conn   net.Conn
	closed bool
	// now reads the wall clock the per-second figures are measured against.
	now func() time.Time

	// Previous and first sample, for the deltas the derived figures need.
	// Only parse touches them, from the one goroutine that reads stdout.
	prevFrames   int
	prevBytes    int64
	prevCaptured int
	prevWall     time.Time
	startRun     float64
	startWall    time.Time
	havePrev     bool
}

// newGstMeter opens the loopback socket the child's tcpclientsink connects to and starts counting
// what arrives on it.
//
// The listener is up before the caller has an argument to put in a pipeline,
// let alone a child to run it, so the connection the child opens on start always finds a peer.
func newGstMeter(onStats func(Stats)) (*gstMeter, error) {
	ln, err := net.Listen("tcp", net.JoinHostPort(gstMeterHost, "0"))
	if err != nil {
		return nil, err
	}
	m := &gstMeter{onStats: onStats, ln: ln, now: time.Now}
	go m.count()
	return m, nil
}

// port is the loopback port the pipeline's meter branch is pointed at.
// The kernel picked it when the listener opened, so it is read off the listener rather than chosen
// here, and no run can collide with another's.
func (m *gstMeter) port() string {
	assert.IsNotNil(m.ln, "a meter that is asked for its port is listening")
	addr, ok := m.ln.Addr().(*net.TCPAddr)
	assert.Assert(ok, "a tcp listener has a tcp address", m.ln.Addr().String())
	return strconv.Itoa(addr.Port)
}

// count takes the child's connection and drains it.
// The payload is a copy of what the sink ships, so it is weighed and discarded, never inspected.
//
// The listener closes on the first connection because a run makes exactly one:
// leaving it open would let a later pipeline's sink land on this meter's port and have its bytes
// counted against the run that opened it.
func (m *gstMeter) count() {
	conn, err := m.ln.Accept()
	m.ln.Close()
	if err != nil {
		return
	}
	if !m.hold(conn) {
		conn.Close()
		return
	}

	buf := make([]byte, 64*1024)
	for {
		n, err := conn.Read(buf)
		m.bytes.Add(int64(n))
		if err != nil {
			return
		}
	}
}

// hold records the accepted connection so close can end the read, and reports whether the meter is
// still open.
// A meter closed while the accept was outstanding takes no connection, since nothing would ever
// close it again.
func (m *gstMeter) hold(conn net.Conn) bool {
	assert.IsNotNil(conn, "an accepted connection is not nil")

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return false
	}
	m.conn = conn
	return true
}

// parse reads the child's stdout and emits one sample per encoded progress line,
// and returns when the stream ends.
//
// The capture counter is recorded rather than sampled.
// Both elements print once a second but not in step, so emitting on either would produce two
// samples per second with one of the two counts unchanged; the encoded line is the one that carries
// the byte counter, so it stays the sample point and reads whatever the capture counter last said.
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

// sample derives one Stats from the cumulative counts of a progress line and the byte counter,
// against the previous line.
func (m *gstMeter) sample(frames int, runSec float64) {
	now := m.now()
	total := m.bytes.Load()
	captured := int(m.captured.Load())

	stats := Stats{
		Frame:   frames,
		SizeKiB: float64(total) / 1024,
		TimeSec: runSec,
	}
	// A per-interval figure has no value on the first line of a run, and this engine measures no
	// capture rate at all unless the backend placed the probe.
	// Both are unmeasured rather than zero, which is the reading that marks a stalled encoder.
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
		// progressreport prints whole seconds, so the same ratio taken between two lines would swing by a
		// whole step whenever the two clocks land on opposite sides of a second boundary.
		if d := now.Sub(m.startWall).Seconds(); d > 0 {
			stats.Speed = (runSec - m.startRun) / d
		}
	} else {
		m.startRun, m.startWall = runSec, now
	}
	m.prevFrames, m.prevBytes, m.prevCaptured, m.prevWall, m.havePrev = frames, total, captured, now, true

	m.onStats(stats)
}

// close releases the listener and the child's connection, which ends the counting goroutine.
// It is safe to call more than once, since a start that fails closes the meter on the way out and a
// start that succeeds closes it again when the child exits.
// A nil meter (a start with no OnStats callback) closes nothing, and so does one built without a
// listener.
func (m *gstMeter) close() {
	if m == nil {
		return
	}

	m.mu.Lock()
	m.closed = true
	conn := m.conn
	m.conn = nil
	m.mu.Unlock()

	if m.ln != nil {
		m.ln.Close()
	}
	if conn != nil {
		conn.Close()
	}
}

// atoi returns the integer value of a digit group the progress-line pattern already matched.
func atoi(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}
