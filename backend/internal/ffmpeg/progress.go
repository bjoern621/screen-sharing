package ffmpeg

import (
	"bufio"
	"io"
	"strconv"
	"strings"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
)

// progressParser turns ffmpeg's -progress stream into Stats samples, one per block.
//
// The stream is one "key=value" per line, terminated by "progress=continue" and,
// for the last block of a run, "progress=end".
// ffmpeg writes a block per stats period, half a second unless -stats_period says otherwise,
// so a sample's interval is that period and not a frame time.
//
// Its own fps and bitrate fields are averages over the whole run,
// which a collapse takes minutes to move.
// The per-interval figures are derived here instead, from a block's counters against the previous
// block's, the same measurement the GStreamer engine takes.
type progressParser struct {
	onStats func(Stats)
	// now reads the wall clock the per-interval figures are measured against.
	now func() time.Time

	// The previous block, for the deltas the per-interval figures need.
	// Only sample touches them, from the single goroutine reading the child's stdout.
	prevFrame int
	prevBytes float64
	prevWall  time.Time
	havePrev  bool
	// haveBytes holds the byte baseline apart from the rest: a block whose total_size is N/A leaves
	// none, while its frame counter still does.
	haveBytes bool
}

// parseProgress emits one Stats per block off the pipe ffmpeg's -progress writes to.
func parseProgress(r io.Reader, onStats func(Stats)) {
	assert.IsNotNil(r, "a progress stream is read from a reader")
	assert.IsNotNil(onStats, "a progress stream is parsed for somebody to hand the samples to")

	(&progressParser{onStats: onStats, now: time.Now}).parse(r)
}

// parse consumes the stream and returns when it ends, when the child closes the pipe.
//
// A line that is not a key and a value is skipped rather than asserted on:
// this reads another program's output, so anything unexpected in it is an Umgebungsfehler.
func (p *progressParser) parse(r io.Reader) {
	assert.IsNotNil(r, "a progress stream is read from a reader")
	assert.IsNotNil(p.onStats, "a parser hands its samples to somebody")
	assert.IsNotNil(p.now, "a parser reads the clock its intervals are measured against")

	scanner := bufio.NewScanner(r)
	block := map[string]string{}

	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)

		if key != "progress" {
			block[key] = value
			continue
		}
		p.sample(block)
		block = map[string]string{}
	}
}

// sample derives one Stats from a completed block, against the block before it.
func (p *progressParser) sample(block map[string]string) {
	assert.IsNotNil(block, "a sample is derived from a block")
	assert.IsNotNil(p.now, "a parser reads the clock its intervals are measured against")

	now := p.now()

	frame, _ := number(block["frame"])
	dup, _ := number(block["dup_frames"])
	drop, _ := number(block["drop_frames"])
	sizeBytes, haveBytes := number(block["total_size"])
	timeUs, haveTime := number(block["out_time_us"])
	speed, haveSpeed := number(strings.TrimSuffix(block["speed"], "x"))
	avgKbits, haveAvg := number(strings.TrimSuffix(block["bitrate"], "kbits/s"))

	var fps, instMbps float64
	var haveFps, haveInst bool
	if interval := now.Sub(p.prevWall).Seconds(); p.havePrev && interval > 0 {
		fps, haveFps = float64(int(frame)-p.prevFrame)/interval, true
		// A block with no byte total, or one following a block that had none, spans no byte delta.
		// Taken against a zero it would report the run's whole output as one interval's worth.
		if haveBytes && p.haveBytes {
			instMbps, haveInst = (sizeBytes-p.prevBytes)*8/interval/1_000_000, true
		}
	}

	p.prevFrame, p.prevWall, p.havePrev = int(frame), now, true
	p.prevBytes, p.haveBytes = sizeBytes, haveBytes

	p.onStats(Stats{
		Frame:    int(frame),
		Fps:      fps,
		SizeKiB:  sizeBytes / 1024,
		TimeSec:  timeUs / 1_000_000,
		Speed:    speed,
		Dup:      int(dup),
		Drop:     int(drop),
		InstMbps: instMbps,
		AvgMbps:  avgKbits / 1000,
		Missing: Missing{
			Fps: !haveFps,
			// ffmpeg counts what it encoded and its grabbers pace themselves,
			// so no field of a -progress block says how often the screen changed.
			CaptureFps: true,
			SizeKiB:    !haveBytes,
			TimeSec:    !haveTime,
			Speed:      !haveSpeed,
			InstMbps:   !haveInst,
			AvgMbps:    !haveAvg,
			// ffmpeg reports nothing about how long it held a frame or what its output link settled on,
			// and this side runs no pipeline of its own to read it off.
			TransitMs: true,
			LinkMs:    true,
			RttMs:     true,
		},
	})
}

// number returns a -progress field's value and whether the field carries one.
// ffmpeg writes "N/A" for a figure it has none for yet, a different state from a measured zero.
func number(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
