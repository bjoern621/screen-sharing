package gstreamer

import (
	"strings"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare-nativegrid/internal/player"
)

// The caps features this side reads memory out of.
//
// A feature list says what a buffer is beyond its media type. Only a "memory:"
// feature names where the frames live; a "meta:" feature names something riding
// along with them and says nothing about memory either way.
//
// System memory is player.MemorySystem, because that is the value the overlay
// reads the same way and one spelling of it is enough. Caps carrying no feature at
// all mean it, which is why the absence of a feature reads as a value here rather
// than as a gap.
const (
	memoryPrefix = "memory:"
	// memoryUnknown is what caps nobody has negotiated report. It is empty so a
	// row of the overlay stays missing rather than reading wrong while a pad is
	// still settling.
	memoryUnknown = ""
)

// memoryOf names where the frames on a pad live, off the memory feature of the
// caps it negotiated.
//
// The feature list belongs to a structure and negotiated caps hold one, so the
// answer is read off the first. A list that names no memory is system memory:
// that is what GStreamer means by leaving the feature out.
func memoryOf(caps *gst.Caps) string {
	if caps == nil || caps.GetSize() == 0 {
		return memoryUnknown
	}
	features := caps.GetFeatures(0)
	if features == nil {
		return player.MemorySystem
	}
	for i := range features.GetSize() {
		if f := features.GetNth(i); strings.HasPrefix(f, memoryPrefix) && f != player.MemorySystem {
			return f
		}
	}
	return player.MemorySystem
}

// verifyMemory checks the memory the chain asks for against the memory the filter
// it asks on negotiated. It runs once a frame has been rendered, which is the
// point where every pad of the chain has caps.
//
// A chain that asked for device memory and got system memory is an
// Umgebungsfehler rather than a broken contract: the elements existed, the parser
// linked them, and what they then agreed on between themselves is theirs. The
// stream plays, more slowly than the chain promised, and the overlay's memory row
// shows the same thing this line says.
func (r *receiver) verifyMemory() {
	if r.chain.device == "" || r.fit == nil {
		return
	}
	got := memoryOf(padCaps(r.fit, "src"))
	if got == memoryUnknown || got == r.chain.device {
		return
	}
	logger.Warnf("stream %q renders through the %s chain, which asks for %s, but its filter negotiated %s",
		r.name, r.chain.name, r.chain.device, got)
}
