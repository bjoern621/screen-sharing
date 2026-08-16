package receive

import (
	"strings"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/logger"
)

// The caps features this side reads memory out of.
//
// A "memory:" feature names where the frames live; a "meta:" feature names something riding
// along with them and says nothing about memory.
//
// Caps carrying no feature at all are system memory, so absence reads as MemorySystem rather
// than as a gap, and one spelling of that value serves every reader.
const (
	memoryPrefix = "memory:"
	// memoryUnknown is what caps nobody has negotiated report.
	// Empty, so a receive-state row stays missing rather than wrong while a pad settles.
	memoryUnknown = ""
)

// memoryOf names where the frames on a pad live, off its negotiated caps.
//
// A feature list belongs to a structure and negotiated caps hold one, so the answer comes off
// the first.
// A list naming no memory is system memory, which is what GStreamer means by leaving the feature
// out.
func memoryOf(caps *gst.Caps) string {
	if caps == nil || caps.GetSize() == 0 {
		return memoryUnknown
	}
	features := caps.GetFeatures(0)
	if features == nil {
		return MemorySystem
	}
	for i := range features.GetSize() {
		if f := features.GetNth(i); strings.HasPrefix(f, memoryPrefix) && f != MemorySystem {
			return f
		}
	}
	return MemorySystem
}

// verifyMemory compares the memory the chain asks for with the memory its filter negotiated.
// It runs once a frame has left the sink, the point where every pad of the chain has caps.
//
// A chain that asked for device memory and got system memory is an Umgebungsfehler and not a
// broken contract: the elements existed, the parser linked them, and what they agreed on between
// themselves is theirs.
// The stream plays, more slowly than the chain promised, so this warns and the receive state's
// memory rows carry the same fact.
func (r *Receiver) verifyMemory() {
	r.mu.Lock()
	fit := r.fit
	r.mu.Unlock()
	if r.chain.device == "" || fit == nil {
		return
	}
	got := memoryOf(padCaps(fit, "src"))
	if got == memoryUnknown || got == r.chain.device {
		return
	}
	logger.Warnf("stream %q renders through the %s chain, which asks for %s, but its filter negotiated %s",
		r.name, r.chain.name, r.chain.device, got)
}
