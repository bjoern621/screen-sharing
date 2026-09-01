package receive

import (
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/framestamp"
)

// A stamped position reads back as the fraction of the picture it was written as.
func TestPointerOfAStampedFrame(t *testing.T) {
	var track decodeTrack
	at := time.Unix(1_700_000_000, 5)
	track.holdPointer(framestamp.Stamp{
		At:       at,
		Pointer:  framestamp.PointerHere,
		PointerX: framestamp.PointerWhole / 4,
		PointerY: framestamp.PointerWhole,
	})

	got, carried := track.pointer()
	if !carried {
		t.Fatal("a stamped frame carries no position")
	}
	if !got.Visible {
		t.Error("a stamped position reads as not visible")
	}
	if got.X < 0.24 || got.X > 0.26 || got.Y != 1 {
		t.Errorf("read %v,%v, want about 0.25,1", got.X, got.Y)
	}
	if !got.At.Equal(at) {
		t.Errorf("read %v, want %v, the moment the frame was stamped", got.At, at)
	}
}

// A publish sending no position leaves a viewer with nothing to draw,
// which is a different answer from a pointer that has left the captured screen.
func TestPointerOfAPublishSendingNone(t *testing.T) {
	var track decodeTrack
	track.holdPointer(framestamp.Stamp{At: time.Now(), Pointer: framestamp.PointerNone})

	if _, carried := track.pointer(); carried {
		t.Error("a frame carrying no position reported one")
	}
}

// A pointer off the captured screen is reported as not visible rather than at its last position.
func TestPointerOffTheCapturedScreen(t *testing.T) {
	var track decodeTrack
	track.holdPointer(framestamp.Stamp{
		At:       time.Now(),
		Pointer:  framestamp.PointerHere,
		PointerX: framestamp.PointerWhole,
	})
	track.holdPointer(framestamp.Stamp{At: time.Now(), Pointer: framestamp.PointerAway})

	got, carried := track.pointer()
	if !carried {
		t.Fatal("a frame reporting the pointer away carries no answer")
	}
	if got.Visible {
		t.Error("a pointer off the captured screen reads as visible")
	}
}

// A track nothing has stamped carries no position, every frame of a stream this app did not
// publish being one.
func TestPointerOfAnUnstampedTrack(t *testing.T) {
	var track decodeTrack
	if _, carried := track.pointer(); carried {
		t.Error("an unstamped track reported a position")
	}
}
