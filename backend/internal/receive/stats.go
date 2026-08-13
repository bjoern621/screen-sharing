package receive

import "time"

// StatGroup is one element's counters under the element's own keys.
//
// Nothing here is labelled or formatted.
// A key is the element's, and its wording is the reader's (api/proto/screenshare/v1/text.proto).
type StatGroup struct {
	// Factory is the element factory, e.g. "rtpjitterbuffer".
	Factory string
	// Element is the pipeline name, e.g. "rtpjitterbuffer0", which tells the jitterbuffers of a
	// muxed stream apart.
	Element string
	Values  []StatValue
}

// StatValue is one counter of a StatGroup, under the element's own field name.
// Counts, per-second rates and millisecond figures all arrive as float64: no counter here reaches
// the magnitude where that loses a digit.
type StatValue struct {
	Key   string
	Value float64
}

// Stats is what a receiver reads off its running pipeline: the encoded stream it receives, the
// pixels it decodes, what the sink does with them, and the counters the transport's own elements
// keep.
// A figure the pipeline has not negotiated or measured stays zero, and a reader prints that as
// unknown rather than as a number.
//
// Every field here is a monotonic counter or a description, never a rate.
// Bitrate and fps are derived by the reader from two polls' deltas, so the interval stays the
// reader's business.
// The per-second figures a transport element keeps itself arrive inside Groups (statsources.go).
type Stats struct {
	// Encoded video, off the video decoder's sink pad.
	Codec         string // GStreamer's description, e.g. "H.265 (Main 4:4:4 profile)"
	Profile       string
	Level         string
	VideoBytes    uint64
	VideoFrames   uint64
	Keyframes     uint64
	SinceKeyframe time.Duration

	// Decoded video, off the decoder's source pad.
	Width, Height int
	Format        string // pixel format of the raw frames, e.g. "Y444_10LE"
	Depth         int    // bits per component
	Subsampling   string // chroma subsampling, e.g. "4:2:0"
	Colorimetry   string
	// Transfer is the transfer characteristic out of Colorimetry, the one part of it a viewer acts
	// on: two of those curves carry more range than a standard display shows and every other one
	// describes a standard-range picture.
	// Read here rather than by whoever holds the string, so one reading answers for the publish
	// child, the encoder refusal and this side alike (internal/colour).
	Transfer       string
	ChromaSite     string
	PixelAspect    string
	Interlace      string
	FPSNum, FPSDen int

	// The decoder the pipeline picked, and what the sink did with its frames.
	Decoder string // decoder factory, "" until the pipeline picks one
	// Hardware is where the decoding ran, and says nothing about where the frames went afterwards:
	// a hardware decoder that downloads its own output into system memory reports true.
	// DecodeMemory answers that.
	Hardware bool
	// What the sink takes, off its own input caps.
	// The size is worth having beside the decoded one: the two differ by exactly the scaling the
	// chain did for the window drawing the frames.
	RenderFormat      string
	RenderColorimetry string
	RenderWidth       int
	RenderHeight      int
	Frames            uint64 // pulled out of the sink and handed on
	Rendered          uint64 // taken by the sink, off its own counter
	Dropped           uint64 // thrown away by the sink for arriving late

	// The render chain the receiver built and what its two ends negotiated.
	//
	// What a chain promises about memory and colour follows from which chain it is, so Chain carries
	// the name and nothing beside it.
	// The two memory fields are the memory features the caps carry, verbatim, on the decoder's
	// output and on the sink's input: they are the measurement those promises are judged against.
	// Both are "" until the pads negotiate.
	Chain        string
	DecodeMemory string
	RenderMemory string
	// ToneMap is whether the pipeline was built with the rung that rolls an HDR stream down into the
	// range a standard display shows.
	// What was built, not what was asked for: a machine with no rung builds without one
	// (tonemap.go).
	ToneMap bool

	// Timing, off the pipeline's own latency and position queries.
	Live       bool
	LatencyMin time.Duration
	LatencyMax time.Duration
	Position   time.Duration // running time reached, which a stall freezes
	Uptime     time.Duration // wall clock since the pipeline started

	// Audio, zero until an audio pad turns up and the branch is built.
	AudioCodec    string
	AudioDecoder  string
	AudioFormat   string
	AudioRate     int
	AudioChannels int
	AudioBytes    uint64

	// Transport counters, one group per statSources element the pipeline holds.
	Groups []StatGroup
}

// MemorySystem is the memory feature of frames held in ordinary system memory, the one value
// DecodeMemory and RenderMemory carry that is not a device's.
// Caps that name no feature at all mean it too.
const MemorySystem = "memory:SystemMemory"

// DecodeOnDevice reports whether the decoder left its frames in a device's memory.
// Memory nothing has negotiated reads as not a device's: an unknown is not a claim.
func (s Stats) DecodeOnDevice() bool { return onDevice(s.DecodeMemory) }

// RenderOnDevice reports whether the frames were still in a device's memory at the sink.
func (s Stats) RenderOnDevice() bool { return onDevice(s.RenderMemory) }

// onDevice is the reading of a memory feature both ends answer through.
func onDevice(memory string) bool {
	return memory != "" && memory != MemorySystem
}
