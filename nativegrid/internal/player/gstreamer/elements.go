package gstreamer

import (
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/logger"
)

// The markers of a factory's GStreamer class the player reads. A class is a
// slash-separated path, e.g. "Codec/Decoder/Video/Hardware".
const (
	klassDecoder  = "Decoder"
	klassHardware = "Hardware"
)

// trackByKlass routes a decoder to the track it drives, off the media type in
// its class. A decoder for neither drives no track and is ignored.
var trackByKlass = []struct {
	klass string
	track func(r *receiver) *decodeTrack
}{
	{klass: "Video", track: func(r *receiver) *decodeTrack { return &r.video }},
	{klass: "Audio", track: func(r *receiver) *decodeTrack { return &r.audio }},
}

// decodeTrack is what a player learns about one elementary stream: the decoder
// the pipeline picked for it, and the counters a probe on that decoder's input
// fills. The probe sits on the input, so the bytes it counts are the encoded
// stream and not the far larger decoded frames.
//
// The counters are atomic because the probe runs on a streaming thread while the
// overlay polls from the UI loop; the element fields are guarded by the
// receiver's mutex.
type decodeTrack struct {
	bytes     atomic.Uint64
	frames    atomic.Uint64
	keyframes atomic.Uint64
	// lastKey is when the last keyframe arrived, as unix nanoseconds, and 0
	// before the first one.
	lastKey atomic.Int64

	dec      gst.Element
	factory  string
	hardware bool
}

// elementStats pairs an element found in the pipeline with the fields of its
// "stats" structure the overlay shows.
type elementStats struct {
	element gst.Element
	fields  []statField
}

// onElement classifies one element of the pipeline: the video and audio decoders
// by their factory class, and the transport elements statSources names. It runs
// for the elements parse-launch built and for every element a bin adds later, so
// it stays idempotent.
func (r *receiver) onElement(e gst.Element) {
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
			r.trackStats(e, src.fields)
		}
	}
}

// trackDecoder records the decoder of one track and starts counting the encoded
// stream it is handed. The first decoder wins: the pipeline builds one per
// elementary stream, so a second video decoder would be a second video stream
// nothing renders.
func (r *receiver) trackDecoder(t *decodeTrack, e gst.Element, factory string, hardware bool) {
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

	// A decoder whose input pad only appears later keeps its counters at zero
	// rather than guessing a pad; the caps rows do not depend on them.
	pad := e.GetStaticPad("sink")
	if pad == nil {
		return
	}
	pad.AddProbe(gst.PadProbeTypeBuffer, func(_ gst.Pad, info *gst.PadProbeInfo) gst.PadProbeReturn {
		buf := info.GetBuffer()
		if buf == nil {
			return gst.PadProbeOK
		}
		t.bytes.Add(uint64(buf.GetSize()))
		t.frames.Add(1)
		// A buffer that is not a delta unit decodes on its own. For video that is
		// a keyframe; for audio it is every frame, which is why only the video row
		// shows the count.
		if !buf.HasFlags(gst.BufferFlagDeltaUnit) {
			t.keyframes.Add(1)
			t.lastKey.Store(time.Now().UnixNano())
		}
		return gst.PadProbeOK
	})
}

// trackStats remembers a transport element whose counters the overlay shows,
// keyed by pipeline name so an element seen twice is added once.
func (r *receiver) trackStats(e gst.Element, fields []statField) {
	r.mu.Lock()
	defer r.mu.Unlock()

	known := slices.ContainsFunc(r.stats, func(s elementStats) bool {
		return s.element.GetName() == e.GetName()
	})
	if known {
		return
	}
	r.stats = append(r.stats, elementStats{element: e, fields: fields})
	logger.Debugf("stream %q reports the counters of %s", r.name, e.GetName())
}
