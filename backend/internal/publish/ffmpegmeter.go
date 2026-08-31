package publish

import (
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/ffmpeg"
)

// ffmpegMeterHost is the loopback the meter listens on, so the copy never leaves the machine.
const ffmpegMeterHost = "127.0.0.1"

// ffmpegMeter weighs the video the publish child copies to it, so a leg whose muxer reports no byte
// total still answers what the stream costs.
//
// ffmpeg derives its own byte figures from the muxer it writes, and the RTP family reports none:
// a stream published over RTSP reports its frame rate on every sample and its bitrate on none,
// for the whole run.
// The tee already carries the encoded packets to whatever wants them, so this is a second reader
// of the one encode rather than a second encode (ffmpeg.Tap).
//
// The GStreamer engine answers the same question the same way (gstMeter), and both weigh the video
// elementary stream rather than the muxed one, so both read a little under what the leg puts
// on the wire.
type ffmpegMeter struct {
	ln    net.Listener
	bytes atomic.Int64

	// now and start are the clock the rates are measured against, injectable so a test can hold
	// an interval still.
	now   func() time.Time
	start time.Time

	mu        sync.Mutex
	prevBytes int64
	prevAt    time.Time
	havePrev  bool
}

// newFfmpegMeter opens the loopback socket the child's tee slave connects to and starts counting.
//
// The listener is up before the caller has an argument to put in a command, let alone a child
// to run it, so the connection the child opens on start always finds a peer.
func newFfmpegMeter() (*ffmpegMeter, error) {
	ln, err := net.Listen("tcp", net.JoinHostPort(ffmpegMeterHost, "0"))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	m := &ffmpegMeter{ln: ln, now: time.Now, start: now}
	go m.count()
	return m, nil
}

// port is the loopback port the command's meter slave is pointed at.
// The kernel picked it when the listener opened, so no run can collide with another's.
func (m *ffmpegMeter) port() string {
	assert.IsNotNil(m.ln, "a meter that is asked for its port is listening")

	addr, ok := m.ln.Addr().(*net.TCPAddr)
	assert.Assert(ok, "a tcp listener has a tcp address", m.ln.Addr().String())
	return strconv.Itoa(addr.Port)
}

// count takes the child's connection and drains it.
// The payload is a copy of what the encoder produced, so it is weighed and discarded, never read.
//
// The listener closes on the first connection, a run making exactly one.
// Left open, a later run's slave could land on this port and have its bytes counted against the run
// that opened it.
func (m *ffmpegMeter) count() {
	conn, err := m.ln.Accept()
	m.ln.Close()
	if err != nil {
		return
	}
	defer conn.Close()

	buf := make([]byte, 64*1024)
	for {
		n, err := conn.Read(buf)
		m.bytes.Add(int64(n))
		if err != nil {
			return
		}
	}
}

// close stops the meter, whether or not a child ever connected.
func (m *ffmpegMeter) close() {
	if m.ln != nil {
		m.ln.Close()
	}
}

// fill answers the byte figures this sample carries no measurement for, and leaves the rest
// as the encoder reported them.
//
// Only the missing ones: ffmpeg weighs the muxed stream where this weighs the video alone, so
// replacing a reported figure would swap a measurement for a narrower one.
func (m *ffmpegMeter) fill(s ffmpeg.Stats) ffmpeg.Stats {
	assert.IsNotNil(m.now, "a meter reads the clock its rates are measured against")

	now := m.now()
	total := m.bytes.Load()

	m.mu.Lock()
	defer m.mu.Unlock()

	if s.Missing.SizeKiB {
		s.SizeKiB, s.Missing.SizeKiB = float64(total)/1024, false
	}
	if run := now.Sub(m.start).Seconds(); s.Missing.AvgMbps && run > 0 {
		s.AvgMbps, s.Missing.AvgMbps = float64(total)*8/run/1_000_000, false
	}
	if interval := now.Sub(m.prevAt).Seconds(); s.Missing.InstMbps && m.havePrev && interval > 0 {
		s.InstMbps, s.Missing.InstMbps = float64(total-m.prevBytes)*8/interval/1_000_000, false
	}

	m.prevBytes, m.prevAt, m.havePrev = total, now, true
	return s
}

// ffmpegMeterTap is the tee slave the meter counts.
//
// Video alone, the question being what the picture costs, and the data muxer, a container around
// a copy nobody plays being bytes this side would have to weigh and then discount.
// A slave that cannot be reached is dropped rather than failing the run: a meter is what a figure
// on screen comes from, and a stream is what the reader asked for.
func ffmpegMeterTap(port string) ffmpeg.Tap {
	assert.Assert(port != "", "a meter slave names the port it writes to")

	return ffmpeg.Tap{
		Options: []string{"select=v", "f=data", "onfail=ignore"},
		URL:     "tcp://" + net.JoinHostPort(ffmpegMeterHost, port),
	}
}
