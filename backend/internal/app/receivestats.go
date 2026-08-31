package app

import (
	"time"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/receive"
	"bjoernblessin.de/screenshare/internal/wire"
)

// receiveStatsInterval is how often the poll reads the running decodes.
// The publish side's progress cadence:
// a diagnostic moving at two speeds on two screens of one app is one a reader learns twice.
const receiveStatsInterval = time.Second

// startReceiveStatsPoll begins the sampling of the running decodes.
//
// Idempotent through sync.Once, as startRelayPoll is:
// a second call joins the loop already running rather than starting one that samples twice as often
// and halves the interval the deltas are divided by.
func (a *App) startReceiveStatsPoll() {
	a.receiveStatsOnce.Do(func() { go a.pollReceiveStats() })
}

// stopReceiveStatsPoll ends the sampling, and is safe with none running:
// the channel closes once, and a loop yet to start finds it closed at its first select.
func (a *App) stopReceiveStatsPoll() {
	a.receiveStatsStopOnce.Do(func() { close(a.receiveStatsStop) })
}

// pollReceiveStats samples every running decode until receiveStatsStop closes.
//
// The previous sample lives here rather than on the App:
// this goroutine is the only writer and the only reader,
// so no other path can slip a sample in between two ticks,
// shortening the interval a delta is divided by.
//
// The wait comes before the sample.
// Nothing is decoding at process start,
// and the first tick after a decode opens is the first with two readings to subtract anyway.
func (a *App) pollReceiveStats() {
	assert.Assert(receiveStatsInterval > 0, "a sampling interval is positive")

	ticker := time.NewTicker(receiveStatsInterval)
	defer ticker.Stop()

	previous := map[StreamRef]receive.Stats{}

	// Quiet to begin with, so a process that has never decoded anything announces nothing:
	// the empty announcement below takes a decode's counters off a shell holding them,
	// and before the first decode there are none to take off.
	quiet := true

	for {
		select {
		case <-a.receiveStatsStop:
			return
		case <-ticker.C:
		}

		sample := a.sampleReceiveStats(previous)

		// A tick with nothing decoding is announced once and not again:
		// repeating it every second would wake every subscriber to say nothing happened.
		if len(sample) == 0 {
			if quiet {
				continue
			}
			quiet = true
		} else {
			quiet = false
		}

		a.emit(wire.ReceiveStatsEvent(sample))
	}
}

// sampleReceiveStats reads every running decode and derives the rates against the previous reading,
// which it then replaces.
//
// procMu is held across the reads, as ReceiveState holds it:
// stats are read off a running pipeline,
// and a receiver a stop took out of the map meanwhile is one whose pipeline is being torn down.
//
// Ended decodes drop out of previous here.
// Nothing else prunes it, and it would otherwise hold a reading per decode this process ever ran.
func (a *App) sampleReceiveStats(previous map[StreamRef]receive.Stats) []wire.ReceiveStreamStats {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	for ref := range previous {
		if _, running := a.receivers[ref]; !running {
			delete(previous, ref)
		}
	}

	// A budget's publishing stages are this machine's own leg,
	// and belong to a decode only where that decode is of the stream this machine sends.
	// Read once for the whole sample: a.run is the same run for every decode in it.
	published, publishing := a.publishedPathLocked()

	out := make([]wire.ReceiveStreamStats, 0, len(a.receivers))
	for ref, receiver := range a.receivers {
		stats := receiver.Stats()
		last, seen := previous[ref]
		previous[ref] = stats

		sample := receiveStatsOf(ref, stats, last, seen)
		own := publishDelay{}
		if published != "" && ref.Name == published {
			own = publishing
		}
		sample.Delay = receiveDelayOf(stats, last, seen, own)
		out = append(out, sample)
	}

	assert.Assert(len(out) == len(a.receivers), "a sample per running decode", len(out), len(a.receivers))
	return out
}

// receiveStatsOf turns one reading into the sample a shell draws.
//
// Counters, sizes and the negotiated figures are the reading as it stands.
// A rate is derived against the reading before it,
// absent rather than zero where there is no interval to take it over:
// absent is unmeasured, and a measured zero is a stream carrying nothing.
//
// last is the previous reading and seen whether there was one.
// Both are needed: a decode on its first tick has no previous reading,
// and a rebuilt decode has one belonging to a pipeline that is gone.
func receiveStatsOf(ref StreamRef, now, last receive.Stats, seen bool) wire.ReceiveStreamStats {
	out := wire.ReceiveStreamStats{
		Stream: wire.StreamRef{StreamName: ref.Name, Transport: ref.Transport},

		Codec:       now.Codec,
		Profile:     now.Profile,
		Level:       now.Level,
		VideoBytes:  now.VideoBytes,
		VideoFrames: now.VideoFrames,
		Keyframes:   now.Keyframes,

		Width:       now.Width,
		Height:      now.Height,
		PixelFormat: now.Format,
		Depth:       now.Depth,
		Subsampling: now.Subsampling,
		Colorimetry: now.Colorimetry,
		Transfer:    now.Transfer,
		ChromaSite:  now.ChromaSite,
		PixelAspect: now.PixelAspect,
		Interlace:   now.Interlace,
		FPSNum:      now.FPSNum,
		FPSDen:      now.FPSDen,

		Decoder:           now.Decoder,
		Hardware:          now.Hardware,
		DecodeMemory:      now.DecodeMemory,
		RenderMemory:      now.RenderMemory,
		Chain:             now.Chain,
		ToneMap:           now.ToneMap,
		RenderFormat:      now.RenderFormat,
		RenderColorimetry: now.RenderColorimetry,
		RenderWidth:       now.RenderWidth,
		RenderHeight:      now.RenderHeight,
		Frames:            now.Frames,
		Rendered:          now.Rendered,
		Dropped:           now.Dropped,

		Live:   now.Live,
		Uptime: now.Uptime.Seconds(),

		AudioCodec:    now.AudioCodec,
		AudioDecoder:  now.AudioDecoder,
		AudioFormat:   now.AudioFormat,
		AudioRate:     now.AudioRate,
		AudioChannels: now.AudioChannels,
		AudioBytes:    now.AudioBytes,

		Groups: receiveStatGroupsOf(now.Groups),
	}

	if now.SinceKeyframe > 0 {
		seconds := now.SinceKeyframe.Seconds()
		out.SinceKeyframe = &seconds
	}
	if now.LatencyMin > 0 || now.LatencyMax > 0 {
		lo, hi := msOf(now.LatencyMin), msOf(now.LatencyMax)
		out.LatencyMin, out.LatencyMax = &lo, &hi
	}
	if now.Position > 0 {
		seconds := now.Position.Seconds()
		out.Position = &seconds
	}

	// The interval is the difference between the pipeline's own two uptimes, not the ticker's period.
	// A tick the scheduler held back would otherwise divide a real delta by a nominal interval,
	// reporting a rate the decode never ran at.
	elapsed := (now.Uptime - last.Uptime).Seconds()
	if !seen || elapsed <= 0 {
		// A pipeline rebuilt under one ref restarts its uptime and its counters,
		// so the reading before it describes a pipeline that is gone.
		// No rate is the honest answer for that tick, and the next has two readings of one run.
		return out
	}

	out.VideoMbps = perSecond(now.VideoBytes, last.VideoBytes, elapsed, 8.0/1e6)
	out.VideoFPS = perSecond(now.VideoFrames, last.VideoFrames, elapsed, 1)
	out.RenderFPS = perSecond(now.Rendered, last.Rendered, elapsed, 1)
	out.DiscardedFPS = perSecond(heldBack(now), heldBack(last), elapsed, 1)
	out.AudioKbps = perSecond(now.AudioBytes, last.AudioBytes, elapsed, 8.0/1e3)
	return out
}

// heldBack is what the decoder took in and did not hand on:
// frames discarded answering QoS, plus the few held at this instant.
//
// A rate over two of these is the discard alone,
// a decoder's own depth being a constant handful that stands in both readings and cancels.
// Zero rather than a negative where the output pad is the one ahead,
// the two counters being read one after the other off a running pipeline.
func heldBack(s receive.Stats) uint64 {
	if s.VideoDecoded >= s.VideoFrames {
		return 0
	}
	return s.VideoFrames - s.VideoDecoded
}

// receiveStatGroupsOf carries the transport's own counters over, in the pipeline's element order.
func receiveStatGroupsOf(groups []receive.StatGroup) []wire.ReceiveStatGroup {
	if len(groups) == 0 {
		return nil
	}

	out := make([]wire.ReceiveStatGroup, 0, len(groups))
	for _, g := range groups {
		values := make([]wire.ReceiveStatValue, 0, len(g.Values))
		for _, v := range g.Values {
			values = append(values, wire.ReceiveStatValue{Key: v.Key, Value: v.Value})
		}
		out = append(out, wire.ReceiveStatGroup{Factory: g.Factory, Element: g.Element, Values: values})
	}
	return out
}

// perSecond is one monotonic counter's rate over elapsed seconds, scaled into the reported unit:
// bytes to Mbit/s at 8.0/1e6, frames to a frame rate at 1.
//
// nil where the counter went backwards, which is the pipeline behind it replaced:
// a rebuild rather than a measurement of zero.
// The interval is the caller's to have checked.
func perSecond(now, last uint64, elapsed, scale float64) *float64 {
	assert.Assert(elapsed > 0, "a rate is taken over a positive interval", elapsed)

	if now < last {
		return nil
	}

	rate := float64(now-last) / elapsed * scale
	return &rate
}

// msOf is a duration in milliseconds,
// the unit the contract states latencies in because the settings ask for them in it.
func msOf(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
