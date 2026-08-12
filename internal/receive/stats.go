package receive

import "time"

// StatGroup is one element's counters, as the element names them.
//
// Nothing here is labelled, formatted or explained, and that is the boundary rather
// than an omission: a counter's key is the element's and a counter's wording is the
// reader's (api/proto/screenshare/v1/text.proto). This package reports which elements
// keep counters worth reading and what those counters say.
type StatGroup struct {
	// Factory is the element's factory, e.g. "rtpjitterbuffer", which says what kind
	// of thing is counting.
	Factory string
	// Element is the element's pipeline name, e.g. "rtpjitterbuffer0", which tells the
	// jitterbuffers of a muxed stream apart.
	Element string
	Values  []StatValue
}

// StatValue is one counter of a StatGroup: the element's own field name and what it
// reads. Counts, rates and millisecond figures all arrive as a float64, since none of
// these elements keeps a counter that reaches where that costs a digit.
type StatValue struct {
	Key   string
	Value float64
}

// Stats is what a receiver can read off its running pipeline: the encoded stream
// it receives, the pixels it decodes, what the sink does with them, and the
// counters the transport's own elements keep. A figure the pipeline has not
// negotiated or measured yet stays zero, and a reader prints it as unknown rather
// than as a number.
//
// Rates are not in here. The receiver reports monotonic counters and the reader
// derives bitrate and fps from their deltas, so the poll interval stays the
// reader's business.
type Stats struct {
	// Encoded video, off the video decoder's input.
	Codec         string // GStreamer's own description, e.g. "H.265 (Main 4:4:4 profile)"
	Profile       string
	Level         string
	VideoBytes    uint64
	VideoFrames   uint64
	Keyframes     uint64
	SinceKeyframe time.Duration

	// Decoded video, off the caps the decoder hands the converter.
	Width, Height int
	Format        string // raw pixel format, e.g. "Y444_10LE"
	Depth         int    // bits per component
	Subsampling   string // chroma subsampling in J:a:b notation
	Colorimetry   string
	// Transfer is the transfer characteristic inside Colorimetry, which is the one part
	// of it a viewer acts on: two of those curves carry more range than a standard
	// display shows and every other one describes a standard-range picture. It is read
	// here rather than by whoever holds the string, so one reading answers for the
	// publish child, the encoder refusal and this side alike (internal/colour).
	Transfer       string
	ChromaSite     string
	PixelAspect    string
	Interlace      string
	FPSNum, FPSDen int

	// Decode and render.
	Decoder string // decoder factory name, "" until the pipeline picks one
	// Hardware says where the decoding ran, and nothing about where the frames
	// went afterwards: a hardware decoder that downloads its own output into
	// system memory reports true. DecodeMemory is what answers that.
	Hardware bool
	// What the sink takes, off its own input caps. The size is worth having beside
	// the decoded one: the two differ by exactly the scaling the chain did for the
	// window drawing the frames.
	RenderFormat      string
	RenderColorimetry string
	RenderWidth       int
	RenderHeight      int
	Frames            uint64 // frames pulled out of the sink and handed on
	Rendered          uint64 // frames the sink took, off its own counters
	Dropped           uint64 // frames the sink dropped for arriving late

	// The render chain the receiver built and what its two ends negotiated.
	//
	// Chain is the chain's name, and what that chain promises about memory and
	// colour follows from which one it is rather than being reported beside it. The
	// two memory fields are the memory features the caps carry, verbatim, on the
	// decoder's output and on the sink's input: they are the measurement those
	// promises are judged against, and both are "" until the pads negotiate.
	Chain        string
	DecodeMemory string
	RenderMemory string
	// ToneMap is whether the pipeline was built with the rung that rolls an HDR stream
	// down into the range a standard display shows. It is what was built and not what was
	// asked for: a machine with no rung builds without one (tonemap.go).
	ToneMap bool

	// Pipeline timing.
	Live       bool
	LatencyMin time.Duration
	LatencyMax time.Duration
	Position   time.Duration // stream running time, which a stalled pipeline freezes
	Uptime     time.Duration // wall time since the pipeline started

	// Audio, zero until the audio branch is up.
	AudioCodec    string
	AudioDecoder  string
	AudioFormat   string
	AudioRate     int
	AudioChannels int
	AudioBytes    uint64

	// Transport counters, one group per element that keeps them.
	Groups []StatGroup
}

// MemorySystem is the memory feature of frames held in ordinary system memory. It
// is the one value DecodeMemory and RenderMemory can carry that is not a device's,
// and caps that name no feature at all mean it too.
const MemorySystem = "memory:SystemMemory"

// DecodeOnDevice reports whether the decoder left its frames in a device's memory.
// Memory nothing has negotiated is not a device's: an unknown is not a claim.
func (s Stats) DecodeOnDevice() bool { return onDevice(s.DecodeMemory) }

// RenderOnDevice reports whether the frames were still in a device's memory when
// they reached the sink.
func (s Stats) RenderOnDevice() bool { return onDevice(s.RenderMemory) }

// onDevice is the one reading of a memory feature both ends share.
func onDevice(memory string) bool {
	return memory != "" && memory != MemorySystem
}
