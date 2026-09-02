//go:build linux

package gstrun

import (
	"testing"

	"github.com/go-gst/go-gst/pkg/gst"
	"github.com/go-gst/go-gst/pkg/gstvideo"
)

// What pipewiresrc puts a portal's cursor position on a frame as: a region of interest under
// this name, and none at all for a pointer that is not over the captured region.
// The name is the element's own and the reader matches it, so the two spellings are checked
// against each other here rather than against a running compositor.
const cursorRoi = "cursor"

// frame is one buffer with the regions of interest named on it, in order.
func frame(t *testing.T, rois ...string) *gst.Buffer {
	t.Helper()
	gst.Init()

	buffer := gst.NewBufferAllocate(nil, 16, nil)
	if buffer == nil {
		t.Fatal("no buffer allocated")
	}
	for i, roi := range rois {
		gstvideo.BufferAddVideoRegionOfInterestMeta(buffer, roi, uint(10*(i+1)), uint(20*(i+1)), 0, 0)
	}
	return buffer
}

// The position on a frame is the one the cursor region carries.
func TestCursorAtReadsTheCursorRegion(t *testing.T) {
	x, y, over := cursorAt(frame(t, cursorRoi))

	if !over {
		t.Fatal("a frame carrying a cursor region reports no pointer")
	}
	if x != 10 || y != 20 {
		t.Errorf("read %d,%d, want 10,20", x, y)
	}
}

// A pointer that is not over the captured region leaves no cursor on the frame,
// which is the pointer having left rather than a frame that failed to say.
func TestCursorAtOfAFrameCarryingNoCursor(t *testing.T) {
	if _, _, over := cursorAt(frame(t)); over {
		t.Error("a frame carrying no cursor region reported a pointer")
	}
}

// A region of interest counts as a cursor by name alone: an encoder's own regions ride the same
// meta, and reading the first would put the pointer wherever something else asked for bitrate.
func TestCursorAtIgnoresAnotherRegion(t *testing.T) {
	if _, _, over := cursorAt(frame(t, "face")); over {
		t.Error("a region named for something else read as the cursor")
	}

	// Behind one, so the answer is the cursor's own position and not the first region's.
	x, y, over := cursorAt(frame(t, "face", cursorRoi))
	if !over {
		t.Fatal("a cursor region behind another was not found")
	}
	if x != 20 || y != 40 {
		t.Errorf("read %d,%d, want the cursor's 20,40", x, y)
	}
}
