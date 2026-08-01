package gstreamer

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/screenshare-nativegrid/internal/player"
)

// Stats reads the running pipeline: the caps on three pads (the decoder's input,
// the decoded frames leaving it, the format and size the sink takes), the sink's
// own render counters, a latency and a position query, and the counters of the
// transport's elements. Nothing is cached, so a field stays zero for exactly as
// long as the pipeline has not learned it.
func (r *receiver) Stats() player.Stats {
	s := player.Stats{
		Frames: r.frames.Load(),
		Uptime: time.Since(r.started),

		VideoBytes:  r.video.bytes.Load(),
		VideoFrames: r.video.frames.Load(),
		Keyframes:   r.video.keyframes.Load(),
		AudioBytes:  r.audio.bytes.Load(),

		Chain:      r.chain.name,
		ChainGPU:   r.chain.device != "",
		ChainExact: r.chain.colour == ColourStated,
	}
	if ns := r.video.lastKey.Load(); ns > 0 {
		s.SinceKeyframe = time.Since(time.Unix(0, ns))
	}

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

// readEncoded describes the stream as it arrives, off the video decoder's input
// caps.
func readEncoded(s *player.Stats, dec gst.Element) {
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

// readDecoded describes the decoded frames, off the decoder's own output pad.
// That pad carries what the decoder produced, and everything downstream of it is this side's own
// doing: the picture size a stream sends is not the size a tile scaled it to.
//
// It is the decoder's pad rather than the next element's because every video decoder has one,
// whichever chain it sits in and whichever bin decodebin built it inside, and because the memory
// feature on it is where the decoder put the frames rather than where something after it moved them.
func readDecoded(s *player.Stats, dec gst.Element) {
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
	s.ChromaSite = st.GetString("chroma-site")
	if n, d, ok := st.GetFraction("pixel-aspect-ratio"); ok {
		s.PixelAspect = fmt.Sprintf("%d:%d", n, d)
	}
	s.Interlace = st.GetString("interlace-mode")
	if n, d, ok := st.GetFraction("framerate"); ok {
		s.FPSNum, s.FPSDen = int(n), int(d)
	}
	// A stream nobody decodes is raw on the wire, so the decoded caps name it:
	// pbutils describes video/x-raw too.
	if s.Codec == "" {
		s.Codec = codecDescription(caps)
	}
}

// readRender describes what the sink does with the frames: the memory they are
// in when they reach it, the format and size it takes, and its own count of what
// it rendered and what it threw away for arriving past its deadline, which the
// paintable's count cannot tell apart.
func (r *receiver) readRender(s *player.Stats) {
	if caps := padCaps(r.sink, "sink"); caps != nil {
		s.RenderMemory = memoryOf(caps)
		st := caps.GetStructure(0)
		s.Render = strings.TrimSpace(st.GetString("format") + " " + st.GetString("colorimetry"))
		// The size shows only where the scaler took the frames down to a tile.
		// That is the case where the format alone would read as the decoded picture in
		// another layout.
		w, wok := st.GetInt("width")
		h, hok := st.GetInt("height")
		if wok && hok && (int(w) != s.Width || int(h) != s.Height) {
			s.Render = strings.TrimSpace(fmt.Sprintf("%s %dx%d", s.Render, w, h))
		}
	}
	st, ok := r.sink.ObjectProperty("stats").(*gst.Structure)
	if !ok || st == nil {
		return
	}
	if v, ok := st.GetUint64("rendered"); ok {
		s.Rendered = v
	}
	if v, ok := st.GetUint64("dropped"); ok {
		s.Dropped = v
	}
}

// readTiming answers the two queries a running pipeline carries: the latency
// window it configured and whether it runs live, and the running time it has
// reached, which a stall freezes.
func (r *receiver) readTiming(s *player.Stats) {
	q := gst.NewQueryLatency()
	if r.pipeline.Query(q) {
		live, lo, hi := q.ParseLatency()
		s.Live = live
		if lo != gst.ClockTimeNone {
			s.LatencyMin = time.Duration(lo)
		}
		if hi != gst.ClockTimeNone {
			s.LatencyMax = time.Duration(hi)
		}
	}
	if pos, ok := r.pipeline.QueryPosition(gst.FormatTime); ok && pos > 0 {
		s.Position = time.Duration(pos)
	}
}

// readAudio describes the audio branch: the encoded track off the decoder's
// input, the raw one off audioconvert's.
func readAudio(s *player.Stats, dec, convert gst.Element) {
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
