package receive

import (
	"fmt"
	"slices"
	"time"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/screenshare/internal/colour"
)

// Stats reads the running pipeline: the caps on three pads (the decoder's input, the decoded frames
// leaving it, what the sink takes), the sink's own counters, a latency and a position query, and
// the counters of the transport's elements.
// Nothing is cached, so a field stays zero for exactly as long as the pipeline has not learned it.
func (r *Receiver) Stats() Stats {
	s := Stats{
		Frames: r.frames.Load(),
		Uptime: time.Since(r.started),

		VideoBytes:   r.video.bytes.Load(),
		VideoFrames:  r.video.frames.Load(),
		VideoDecoded: r.video.decoded.Load(),
		Keyframes:    r.video.keyframes.Load(),
		AudioBytes:   r.audio.bytes.Load(),

		Chain:   r.chain.name,
		ToneMap: r.toneMap,
	}
	if ns := r.video.lastKey.Load(); ns > 0 {
		s.SinceKeyframe = time.Since(time.Unix(0, ns))
	}
	transit := r.delay.Read()
	s.Transit, s.TransitFrames, s.TransitPeak = transit.Total, transit.Frames, transit.Peak

	// r.mu guards the fields onElement writes and nothing else here, so the handles are copied
	// under it and the pipeline is queried outside it.
	r.mu.Lock()
	videoDec, audioDec := r.video.dec, r.audio.dec
	s.Decoder, s.Hardware = r.video.factory, r.video.hardware
	s.AudioDecoder = r.audio.factory
	audioConvert := r.audioConvert
	sources := slices.Clone(r.stats)
	r.mu.Unlock()

	readEncoded(&s, videoDec)
	readDecoded(&s, videoDec)
	r.readRender(&s)
	r.readTiming(&s)
	readAudio(&s, audioDec, audioConvert)

	for _, src := range sources {
		if g, ok := statGroup(src); ok {
			s.Groups = append(s.Groups, g)
		}
	}
	return s
}

// readEncoded describes the stream as it arrives, off the video decoder's sink pad.
func readEncoded(s *Stats, dec gst.Element) {
	if dec == nil {
		return
	}
	caps := padCaps(dec, "sink")
	if caps == nil {
		return
	}
	st := caps.GetStructure(0)
	s.Codec = codecDescription(caps)
	s.Profile = st.GetString("profile")
	s.Level = st.GetString("level")
}

// readDecoded describes the decoded frames, off the decoder's source pad.
// That pad carries what the decoder produced, and everything downstream of it is this side's own
// doing: the picture size a stream sends is not the size a tile scaled it to.
//
// The decoder's pad rather than the next element's, because every video decoder has one whichever
// chain it sits in and whichever bin decodebin built it inside, and because its memory feature is
// where the decoder put the frames rather than where something after it moved them.
func readDecoded(s *Stats, dec gst.Element) {
	if dec == nil {
		return
	}
	caps := padCaps(dec, "src")
	if caps == nil {
		return
	}
	s.DecodeMemory = memoryOf(caps)
	st := caps.GetStructure(0)
	if w, ok := st.GetInt("width"); ok {
		s.Width = int(w)
	}
	if h, ok := st.GetInt("height"); ok {
		s.Height = int(h)
	}
	s.Format = st.GetString("format")
	s.Depth, s.Subsampling = pixelShape(s.Format)
	s.Colorimetry = st.GetString("colorimetry")
	s.Transfer = colour.TransferOfColorimetry(s.Colorimetry)
	s.ChromaSite = st.GetString("chroma-site")
	if n, d, ok := st.GetFraction("pixel-aspect-ratio"); ok {
		s.PixelAspect = fmt.Sprintf("%d:%d", n, d)
	}
	s.Interlace = st.GetString("interlace-mode")
	if n, d, ok := st.GetFraction("framerate"); ok {
		s.FPSNum, s.FPSDen = int(n), int(d)
	}
	// A stream nobody decodes is raw on the wire, and the decoded caps are then the ones that name
	// it: pbutils describes video/x-raw too.
	if s.Codec == "" {
		s.Codec = codecDescription(caps)
	}
}

// readRender describes what the sink does with the frames: the memory they are in when they reach
// it, the format and size it takes, and its own count of what it took and what it threw away for
// arriving past its deadline, which the pull count cannot tell apart.
//
// The size is read whether or not it differs from the decoded one.
// Whether a scaler took the picture down for a window is a comparison, and a comparison is a
// reader's to make from two figures rather than this side's to make by leaving one out.
func (r *Receiver) readRender(s *Stats) {
	// A stopped receiver has handed its pipeline back, so there is no sink to read and the figures
	// stay at what the run last reported (Receiver.release).
	r.mu.Lock()
	sink := r.sink
	r.mu.Unlock()
	if sink == nil {
		return
	}

	if caps := padCaps(sink, "sink"); caps != nil {
		s.RenderMemory = memoryOf(caps)
		st := caps.GetStructure(0)
		s.RenderFormat = st.GetString("format")
		s.RenderColorimetry = st.GetString("colorimetry")
		if w, ok := st.GetInt("width"); ok {
			s.RenderWidth = int(w)
		}
		if h, ok := st.GetInt("height"); ok {
			s.RenderHeight = int(h)
		}
	}
	// The counters are the base sink's rather than a property this element invented, which is why an
	// appsink answers them the way the native grid's paintable sink does.
	st := sink.GetStats()
	if st == nil {
		return
	}
	if v, ok := st.GetUint64("rendered"); ok {
		s.Rendered = v
	}
	if v, ok := st.GetUint64("dropped"); ok {
		s.Dropped = v
	}
}

// readTiming answers the two queries a running pipeline carries: the latency window it configured
// and whether it runs live, and the running time it has reached, which a stall freezes.
func (r *Receiver) readTiming(s *Stats) {
	r.mu.Lock()
	pipeline := r.pipeline
	r.mu.Unlock()
	if pipeline == nil {
		return
	}

	q := gst.NewQueryLatency()
	if pipeline.Query(q) {
		live, lo, hi := q.ParseLatency()
		s.Live = live
		if lo != gst.ClockTimeNone {
			s.LatencyMin = time.Duration(lo)
		}
		if hi != gst.ClockTimeNone {
			s.LatencyMax = time.Duration(hi)
		}
	}
	if pos, ok := pipeline.QueryPosition(gst.FormatTime); ok && pos > 0 {
		s.Position = time.Duration(pos)
	}
}

// readAudio describes the audio branch: the encoded track off the decoder's sink pad, the raw one
// off audioconvert's.
func readAudio(s *Stats, dec, convert gst.Element) {
	if dec != nil {
		if caps := padCaps(dec, "sink"); caps != nil {
			s.AudioCodec = codecDescription(caps)
		}
	}
	if convert == nil {
		return
	}
	caps := padCaps(convert, "sink")
	if caps == nil {
		return
	}
	st := caps.GetStructure(0)
	s.AudioFormat = st.GetString("format")
	if v, ok := st.GetInt("rate"); ok {
		s.AudioRate = int(v)
	}
	if v, ok := st.GetInt("channels"); ok {
		s.AudioChannels = int(v)
	}
	if s.AudioCodec == "" {
		s.AudioCodec = codecDescription(caps)
	}
}
