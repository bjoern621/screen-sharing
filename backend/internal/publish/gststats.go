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

	"bjoernblessin.de/screenshare/internal/gstrun"
)

// gstStatsName names the progressreport element counting encoded frames, gstCaptureName the one
// counting the frames the screen produced, and gstTeeName the tee every extra branch and the sink
// itself continue from.
// The capture backend splices its probe in ahead of anything that repeats or paces frames.
const (
	gstStatsName   = "stats"
	gstCaptureName = "capture"
	gstTeeName     = "meter"
	// gstShedName names the queue ahead of the encoder, which is counted at both ends so what it drops
	// is a figure rather than a silence (gstpipeline.go, gstrun/delay.go).
	gstShedName = "shed"
)

// gstCaptureProbe is the progressreport element a capture backend splices in to count what
// the source produced.
// Built here rather than in the backend, so both halves of the wire format stay one decision,
// as for the encoded counter.
//
// One argument per token, like every element list here: gst-launch parses its argv token by token,
// so an element and its properties in one argument is a parse error rather than an element.
var gstCaptureProbe = []string{
	"progressreport", "name=" + gstCaptureName, "update-freq=1", "format=buffers", "do-query=false",
}

// gstProgressLine matches one progressreport line: which element printed it, the running time it
// last saw, and its cumulative buffer count.
// A line reads "stats (00:00:07): 141 buffers".
// The element's query form pads the time differently, which the optional spaces cover.
var gstProgressLine = regexp.MustCompile(`^(` + gstStatsName + `|` + gstCaptureName + `) \(\s*(\d+):\s*(\d+):\s*(\d+)\):\s+(\d+) buffers`)

// gstProgressElement is the encoded-frame counter buildPipeline splices in between the parser and
// the tee, and the line gstMeter reads.
// Beside the pattern that reads it, the properties set here and that pattern being one wire format.
//
// progressreport counts buffers rather than querying a position: no element upstream of an encoded
// stream answers a byte or time query.
// It prints the count and the pipeline running time to stdout once a second.
var gstProgressElement = []string{
	"progressreport", "name=" + gstStatsName, "update-freq=1", "format=buffers", "do-query=false",
}

// gstMeterTap is the branch that weighs the encoded stream, no GStreamer element reporting byte
// throughput: a copy of the encoded video goes to a tcpclientsink on a loopback socket this process
// counts.
// The branch cannot hold up the encode path, its queue leaking and its sink neither synchronizing
// to the clock nor prerolling.
// Its bytes are the video elementary stream, so the figures read below ffmpeg's, which weighs
// the muxed stream with its audio track and container overhead.
//
// A socket rather than an inherited descriptor, Windows inheriting none: os/exec supports
// ExtraFiles on Unix alone, and a child handed one there fails to start at all.
// Both platforms carry a socket, so the meter has one wire format rather than one per operating
// system.
func gstMeterTap(meterPort string) []string {
	assert.Assert(meterPort != "", "a meter branch names the port it reports to")

	return []string{
		"queue", "max-size-buffers=8", "leaky=downstream",
		"!", "tcpclientsink", "host=" + gstMeterHost, "port=" + meterPort, "sync=false", "async=false",
	}
}

// gstTapElements returns the tee and the branches taking a copy of the encoded stream off it,
// ending in the reference the trunk resumes from.
// Empty for a run that taps nothing, what a rendered command is.
//
// The trunk is a branch of the tee like the others rather than the element the tee was linked into,
// so every branch, the muxer's included, starts from the tee by name.
// A second tap then arrives without the first one's shape changing.
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
// Loopback alone: the branch carries a copy of the user's screen, and its only intended peer
// is the child this process spawned.
const gstMeterHost = "127.0.0.1"

// gstMeter turns what a gst-launch child can report into the Stats samples the ffmpeg engine emits.
// GStreamer has no counterpart to ffmpeg's -progress stream, so two elements carry the raw counts:
// a progressreport prints the encoded frame count and the running time once a second,
// and a tcpclientsink writes a second copy of the encoded video to a loopback socket this type
// counts.
// Frames arrive as text and bytes as data, so the sample is joined here: each progress line takes
// the byte counter as it stands.
//
// Stats.Drop is what the shed ahead of the encoder threw away, counted at both its ends
// by the child (gstEncodeQueue, gstrun/delay.go).
// The leaky queue on the damage path keeps no count and needs none: it drops a frame a newer one
// supersedes, which costs the stream nothing.
type gstMeter struct {
	onStats func(Stats)
	bytes   atomic.Int64
	// captured is the newest cumulative count from the capture backend's rate probe.
	// Atomic for the same reason bytes is: the parse goroutine and the counting goroutine are not one
	// goroutine.
	captured atomic.Int64
	// haveCaptured records that a capture line arrived at all, which tells a probe reporting a genuine
	// zero rate apart from a pipeline carrying no probe.
	haveCaptured atomic.Bool
	// ln accepts the one connection the child's tcpclientsink opens, and conn is that connection once
	// it arrives.
	// conn is held so close can end the read the counting goroutine is parked in, and mu guards
	// the pair against a close landing while the accept is still outstanding.
	mu     sync.Mutex
	ln     net.Listener
	conn   net.Conn
	closed bool
	// delayMu guards the newest delay reading and whether one has arrived at all.
	// A mutex rather than the atomics beside it, a reading being a struct whose halves are read
	// together: the line reader writes it and the parse goroutine reads it, which are two goroutines
	// (gsthdr.go).
	delayMu   sync.Mutex
	delay     gstrun.Delay
	haveDelay bool
	// now reads the wall clock the per-second figures are measured against.
	now func() time.Time

	// Previous and first sample, for the deltas the derived figures need.
	// Touched by parse alone, from the one goroutine that reads stdout.
	prevFrames   int
	prevBytes    int64
	prevCaptured int
	prevWall     time.Time
	startRun     float64
	startWall    time.Time
	havePrev     bool
	// The delay reading the previous sample was taken against, and whether there was one.
	// A mean transit is one interval's summed delay over that interval's frames, so it needs
	// the reading before it as much as a rate does.
	prevDelay     gstrun.Delay
	havePrevDelay bool
}

// newGstMeter opens the loopback socket the child's tcpclientsink connects to and starts counting
// what arrives on it.
//
// The listener is up before the caller has an argument to put in a pipeline, let alone a child
// to run it, so the connection the child opens on start always finds a peer.
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
// here and no run can collide with another's.
func (m *gstMeter) port() string {
	assert.IsNotNil(m.ln, "a meter that is asked for its port is listening")
	addr, ok := m.ln.Addr().(*net.TCPAddr)
	assert.Assert(ok, "a tcp listener has a tcp address", m.ln.Addr().String())
	return strconv.Itoa(addr.Port)
}

// count takes the child's connection and drains it.
// The payload is a copy of what the sink ships, so it is weighed and discarded, never inspected.
//
// The listener closes on the first connection, a run making exactly one.
// Left open, a later pipeline's sink could land on this meter's port and have its bytes counted
// against the run that opened it.
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

// hold records the accepted connection so close can end the read, and reports whether the meter
// is still open.
// A meter closed while the accept was outstanding takes no connection, nothing else closing it.
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

// parse reads the child's stdout and emits one sample per encoded progress line, returning when
// the stream ends.
//
// A capture line is recorded rather than sampled.
// Both elements print once a second but not in step, so emitting on either would give two samples
// a second with one of the two counts unchanged.
// The encoded line is the one carrying the byte counter, so it stays the sample point and reads
// whatever the capture counter last said.
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

// timing fills in what the child measured about the delay through its pipeline, and marks each
// figure missing where nothing measured it.
//
// The transit is a mean over the interval between two readings rather than over the run, the split
// every rate here is taken under: an encoder that slows down halfway shows it in the next sample
// instead of being averaged out by the minutes before it.
// A reading with none before it yields no mean, and neither does one whose counters went backwards,
// which is a child relaunched under one meter.
func (m *gstMeter) timing(stats *Stats) {
	delay, reported := m.readDelay()
	if !reported {
		stats.Missing.TransitMs, stats.Missing.LinkMs, stats.Missing.RttMs = true, true, true
		return
	}

	// A total since the run started, like the frame count beside it, so a reader watching it climb
	// is reading a shortfall that is still happening.
	if delay.Dropped != nil {
		stats.Drop = int(*delay.Dropped)
	}

	if delay.LinkMs != nil {
		stats.LinkMs = *delay.LinkMs
	} else {
		stats.Missing.LinkMs = true
	}
	if delay.RttMs != nil {
		stats.RttMs = *delay.RttMs
	} else {
		stats.Missing.RttMs = true
	}

	prev, havePrev := m.prevDelay, m.havePrevDelay
	m.prevDelay, m.havePrevDelay = delay, true

	// No frames across the interval is nothing to time rather than a frame that took no time, so it
	// reads as absent like the other two.
	if !havePrev || delay.Frames <= prev.Frames || delay.TransitNs < prev.TransitNs {
		stats.Missing.TransitMs = true
		return
	}
	stats.TransitMs = float64(delay.TransitNs-prev.TransitNs) / float64(delay.Frames-prev.Frames) / 1e6
}

// takeDelay records the newest reading the child reported, for the next sample to measure against.
func (m *gstMeter) takeDelay(d gstrun.Delay) {
	m.delayMu.Lock()
	defer m.delayMu.Unlock()
	m.delay, m.haveDelay = d, true
}

// readDelay is the newest reading and whether one has arrived.
func (m *gstMeter) readDelay() (gstrun.Delay, bool) {
	m.delayMu.Lock()
	defer m.delayMu.Unlock()
	return m.delay, m.haveDelay
}

// sample derives one Stats from a progress line's cumulative counts and the byte counter,
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
	m.timing(&stats)
	// A per-interval figure has no value on the first line of a run, and this engine measures no
	// capture rate at all unless the backend placed the probe.
	// Both are unmeasured rather than zero, zero being the reading that marks a stalled encoder.
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
		// progressreport prints whole seconds, so the same ratio taken between two lines would swing
		// by a whole step whenever the two clocks fall on opposite sides of a second boundary.
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
// Safe to call more than once: a start that fails closes the meter on the way out, and a start
// that succeeds closes it again when the child exits.
// A nil meter, which is a start with no OnStats callback, closes nothing, and so does one built
// without a listener.
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

// atoi is the value of a digit group the progress-line pattern already matched, so the conversion
// cannot fail.
func atoi(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}
