package player

import "time"

// StatGroup is one element's counters, labelled and formatted by the player: the
// overlay prints the rows it is handed, so a transport reports whatever its
// elements know without the tile learning about them.
type StatGroup struct {
	// Name is the element's pipeline name, e.g. "rtpjitterbuffer0", which also
	// tells the jitterbuffers of a muxed stream apart.
	Name string
	Rows []StatRow
}

// StatRow is one labelled counter of a StatGroup.
type StatRow struct {
	Label string
	Value string
}

// Stats is what a player can read off its running pipeline for the tile's stats
// overlay: the encoded stream it receives, the pixels it decodes, what the sink
// does with them, and the counters the transport's own elements keep. A figure
// the pipeline has not negotiated or measured yet stays zero, and the overlay
// prints it as unknown rather than as a number.
//
// Rates are not in here. The player reports monotonic counters and the overlay
// derives bitrate and fps from their deltas, so the poll interval stays the
// overlay's business.
type Stats struct {
	// Encoded video, off the video decoder's input.
	Codec         string // the backend's own description, e.g. "H.265 (Main 4:4:4 profile)"
	Profile       string
	Level         string
	VideoBytes    uint64
	VideoFrames   uint64
	Keyframes     uint64
	SinceKeyframe time.Duration

	// Decoded video, off the caps the decoder hands the converter.
	Width, Height  int
	Format         string // raw pixel format, e.g. "Y444_10LE"
	Depth          int    // bits per component
	Subsampling    string // chroma subsampling in J:a:b notation
	Colorimetry    string
	ChromaSite     string
	PixelAspect    string
	Interlace      string
	FPSNum, FPSDen int

	// Decode and render.
	Decoder  string // decoder factory name, "" until the pipeline picks one
	Hardware bool   // the decoder decodes on the GPU
	Render   string // pixel format and colorimetry the sink takes
	Frames   uint64 // frames the paintable has put on screen
	Rendered uint64 // frames the sink rendered, off the sink's own counters
	Dropped  uint64 // frames the sink dropped for arriving late

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
