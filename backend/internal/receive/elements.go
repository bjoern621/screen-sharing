package receive

import (
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/padprobe"
)

// The markers read out of a factory's GStreamer class, a slash-separated path, e.g.
// "Codec/Decoder/Video/Hardware".
const (
	klassDecoder  = "Decoder"
	klassHardware = "Hardware"
)

// trackByKlass sends a decoder to the track it drives, going by the media type in its class. A
// decoder for neither drives no track and is passed over.
var trackByKlass = []struct {
	klass string
	track func(r *Receiver) *decodeTrack
}{
	{klass: "Video", track: func(r *Receiver) *decodeTrack { return &r.video }},
	{klass: "Audio", track: func(r *Receiver) *decodeTrack { return &r.audio }},
}

// decodeTrack is what a receiver learns about one elementary stream: the decoder the pipeline
// picked for it, and the counters filled by a probe on that decoder's input. Sitting on the input
// is what makes the counted bytes the encoded stream rather than the far larger decoded frames.
//
// Atomic counters, the probe running on a streaming thread while a reader polls from elsewhere.
// The element fields are guarded by the receiver's mutex.
type decodeTrack struct {
	bytes     atomic.Uint64
	frames    atomic.Uint64
	keyframes atomic.Uint64
	// decoded counts what left the decoder, against frames counting what went in.
	// The gap is what the decoder swallowed answering QoS, which no element keeps a counter for.
	decoded atomic.Uint64
	// lastKey is the last keyframe's arrival in unix nanoseconds, 0 before the first one.
	lastKey atomic.Int64

	dec      gst.Element
	factory  string
	hardware bool
}

// elementStats pairs an element found in the pipeline with the table entry stating which of its
// "stats" fields are shown and how they read.
type elementStats struct {
	element gst.Element
	source  statSource
}

// onElement classifies one element of the pipeline: video and audio decoders by factory class, and
// the transport elements statSources names. It runs over what parse-launch built and over every
// element a bin adds afterwards, so it stays idempotent.
func (r *Receiver) onElement(e gst.Element) {
	f := e.GetFactory()
	if f == nil {
		return
	}
	factory := f.GetName()
	klass := f.GetMetadata("klass")

	if strings.Contains(klass, klassDecoder) {
		for _, t := range trackByKlass {
			if strings.Contains(klass, t.klass) {
				r.trackDecoder(t.track(r), e, factory, strings.Contains(klass, klassHardware))
				break
			}
		}
	}
	for _, src := range statSources {
		if src.factory == factory {
			r.trackStats(e, src)
		}
	}
}

// trackDecoder records one track's decoder and starts counting the encoded stream handed to it.
// First decoder wins: the pipeline builds one per elementary stream, so a second video decoder
// would be a second video stream nothing renders.
func (r *Receiver) trackDecoder(t *decodeTrack, e gst.Element, factory string, hardware bool) {
	r.mu.Lock()
	seen := t.dec != nil
	if !seen {
		t.dec, t.factory, t.hardware = e, factory, hardware
	}
	r.mu.Unlock()
	if seen {
		return
	}
	logger.Debugf("stream %q decodes through %s (hardware=%t)", r.name, factory, hardware)

	// Counters stay at zero where a decoder's input pad appears later, rather than a pad being
	// guessed at. The caps rows do not read them.
	pad := e.GetStaticPad("sink")
	if pad == nil {
		return
	}
	pad.AddProbe(gst.PadProbeTypeBuffer, func(_ gst.Pad, info *gst.PadProbeInfo) gst.PadProbeReturn {
		buf := padprobe.Buffer(info)
		if buf == nil {
			return gst.PadProbeOK
		}
		t.bytes.Add(uint64(buf.GetSize()))
		t.frames.Add(1)
		// A buffer carrying no delta-unit flag decodes on its own: a keyframe in video, and every
		// frame in audio, which is why the count is shown on the video row alone.
		if !buf.HasFlags(gst.BufferFlagDeltaUnit) {
			t.keyframes.Add(1)
			t.lastKey.Store(time.Now().UnixNano())
		}
		return gst.PadProbeOK
	})

	// The other end of the same element, so the two counts bracket it and their difference is what
	// it took in and never handed on.
	// Read as a rate rather than as a total, because a decoder holds a few frames at all times and
	// that depth is constant: it cancels between two readings and would stand as a permanent
	// discard count in a running total (internal/app, discardedOf).
	if out := e.GetStaticPad("src"); out != nil {
		out.AddProbe(gst.PadProbeTypeBuffer, func(_ gst.Pad, info *gst.PadProbeInfo) gst.PadProbeReturn {
			if padprobe.Buffer(info) != nil {
				t.decoded.Add(1)
			}
			return gst.PadProbeOK
		})
	}
}

// trackStats remembers a transport element whose counters are reported, keyed by pipeline name so
// an element met twice is added once.
func (r *Receiver) trackStats(e gst.Element, src statSource) {
	r.mu.Lock()
	defer r.mu.Unlock()

	known := slices.ContainsFunc(r.stats, func(s elementStats) bool {
		return s.element.GetName() == e.GetName()
	})
	if known {
		return
	}
	r.stats = append(r.stats, elementStats{element: e, source: src})
	logger.Debugf("stream %q reports the counters of %s", r.name, e.GetName())
}
