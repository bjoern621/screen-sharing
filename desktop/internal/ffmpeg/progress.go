package ffmpeg

import (
	"bufio"
	"io"
	"strconv"
	"strings"
	"time"
)

// progressParser turns ffmpeg's -progress key=value stream into Stats samples,
// one per block (blocks end with a "progress=" line).
//
// ffmpeg's own fps and bitrate fields are averages over the whole run, so a
// collapse takes minutes to move them. The per-interval figures are derived here
// instead, from the counters in the block against the previous block, which is
// the same measurement the GStreamer engine takes.
type progressParser struct {
	onStats func(Stats)
	// now reads the wall clock the per-interval figures are measured against.
	now func() time.Time

	// Previous block, for the deltas the per-interval figures need. Only sample
	// touches them, from the one goroutine that reads stdout.
	prevFrame int
	prevBytes float64
	prevWall  time.Time
	havePrev  bool
	// haveBytes tracks the byte baseline separately: a block whose total_size is
	// N/A leaves no baseline, while its frame counter still carries one.
	haveBytes bool
}

// parseProgress reads ffmpeg's -progress key=value stream and emits one Stats
// per block.
func parseProgress(r io.Reader, onStats func(Stats)) {
	(&progressParser{onStats: onStats, now: time.Now}).parse(r)
}

// parse consumes the stream and returns when it ends.
func (p *progressParser) parse(r io.Reader) {
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

// sample derives one Stats from a completed progress block, against the previous
// one.
func (p *progressParser) sample(block map[string]string) {
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
		// A block with no byte total, or one after a block that had none, spans no
		// measurable byte delta; taking it against a zero would report the whole
		// output as one interval's worth.
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
			// ffmpeg counts what it encoded, and its grabbers pace themselves, so
			// nothing in a -progress block reports how often the screen changed.
			CaptureFps: true,
			SizeKiB:    !haveBytes,
			TimeSec:    !haveTime,
			Speed:      !haveSpeed,
			InstMbps:   !haveInst,
			AvgMbps:    !haveAvg,
		},
	})
}

// number returns the value of a -progress field and whether the field carries
// one. ffmpeg writes "N/A" for a figure it has no value for yet, which is a
// different state from a measured zero.
func number(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
