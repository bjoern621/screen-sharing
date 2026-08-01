package stats

import (
	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare-nativegrid/internal/player"
)

// memoryPair is where the two ends of the render chain hold their frames: the
// memory the decoder produced them in, and the memory the sink took them in.
// Those two answers are what a download or an upload between them is read from.
type memoryPair struct {
	decodeOnDevice bool
	sinkOnDevice   bool
}

// path is one verdict on the render path: what happened to the frames between the
// decoder and the sink, and the reading of it the row carries.
type path struct {
	label string
	tip   string
}

// paths is the render path per pair of memories. It is a table because the
// verdict is a static fact about the pair and not a rule to restate wherever the
// question comes up.
//
// A pair says nothing about how the frames were converted, only about which side
// of the bus they crossed on the way to the screen. Where the answer is "system
// memory" it also does not say that no GPU was involved: a hardware decoder
// decodes on the GPU and can download the result itself, and the decoder row is
// the one that reports that.
var paths = map[memoryPair]path{
	{decodeOnDevice: true, sinkOnDevice: true}: {
		label: "no download",
		tip:   "The decoder made the frames in GPU memory and the sink took them there. Nothing crosses the bus between decoding and display.",
	},
	{decodeOnDevice: true, sinkOnDevice: false}: {
		label: "downloaded before the sink",
		tip:   "The decoder made the frames in GPU memory and something between it and the sink moved them into system memory, so every frame crosses the bus once on the way to the screen.",
	},
	{decodeOnDevice: false, sinkOnDevice: true}: {
		label: "uploaded before the sink",
		tip:   "The decoder made the frames in system memory and the chain moved them into GPU memory before the sink, so every frame crosses the bus once, in the other direction.",
	},
	{decodeOnDevice: false, sinkOnDevice: false}: {
		label: "system memory throughout",
		tip:   "No frame is held in GPU memory between decoding and display. A hardware decoder still decoded on the GPU; it downloaded its own output, which is what the decoder row alone does not say.",
	},
}

// pathOf is the verdict on one poll's render path, and false while either end has
// negotiated nothing: an unknown memory is not a claim about where the frames
// went.
func pathOf(s player.Stats) (path, bool) {
	if s.DecodeMemory == "" || s.RenderMemory == "" {
		return path{}, false
	}
	pair := memoryPair{
		decodeOnDevice: s.DecodeOnDevice(),
		sinkOnDevice:   s.RenderOnDevice(),
	}
	p, ok := paths[pair]
	assert.Assert(ok, "a render path per decode and sink memory pair", pair.decodeOnDevice, pair.sinkOnDevice)
	return p, true
}
