package screensrc

import (
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/share"
)

// offDesktop is a rectangle no machine running this test has an output for,
// which is what makes the refusal below the same everywhere.
var offDesktop = share.Rect{X: 1 << 20, Y: 1 << 20, Width: 640, Height: 480}

// The X screen is one picture, so a rectangle of it reaches the element as the crop a monitor does.
// endx and endy are inclusive, so the last captured column is the offset plus the width minus one.
func TestTheXHeadCropsToTheRectangleDrawn(t *testing.T) {
	head, err := Head(XImage, share.Target{
		Kind:   share.Region,
		Region: share.Rect{X: 100, Y: 200, Width: 1280, Height: 720},
	})
	if err != nil {
		t.Fatal(err)
	}

	line := strings.Join(head, " ")
	for _, want := range []string{"startx=100", "starty=200", "endx=1379", "endy=919"} {
		if !strings.Contains(line, want) {
			t.Errorf("the head holds no %q: %s", want, line)
		}
	}
}

// A window is the element's other input, read by XID with no crop at all.
func TestTheXHeadReadsAWindowByItsId(t *testing.T) {
	head, err := Head(XImage, share.Target{Kind: share.Window, Window: 4242})
	if err != nil {
		t.Fatal(err)
	}

	line := strings.Join(head, " ")
	if !strings.Contains(line, "xid=4242") {
		t.Errorf("the head names no window: %s", line)
	}
	if strings.Contains(line, "startx=") {
		t.Errorf("a window capture carries a crop rectangle: %s", line)
	}
}

// Desktop Duplication reads outputs alone, so a window needs the Graphics Capture path
// and the head names it rather than leaving the element on its default.
func TestTheWindowsHeadNamesTheApiThatReadsAWindow(t *testing.T) {
	head, err := Head(D3D11, share.Target{Kind: share.Window, Window: 4242})
	if err != nil {
		t.Fatal(err)
	}

	line := strings.Join(head, " ")
	for _, want := range []string{"capture-api=wgc", "window-handle=4242"} {
		if !strings.Contains(line, want) {
			t.Errorf("the head holds no %q: %s", want, line)
		}
	}
}

// Desktop Duplication hands out one output at a time,
// so cropping a rectangle no output holds would publish a picture nobody drew.
func TestTheWindowsHeadRefusesARectangleOffEveryScreen(t *testing.T) {
	if _, err := Head(D3D11, share.Target{Kind: share.Region, Region: offDesktop}); err == nil {
		t.Fatal("a rectangle outside every output built a head")
	}
}

// The X screen is one picture, so the same rectangle is a crop of it and needs no output to hold it.
func TestTheXHeadTakesARectangleTheOutputsDoNotHold(t *testing.T) {
	if _, err := Head(XImage, share.Target{Kind: share.Region, Region: offDesktop}); err != nil {
		t.Fatalf("the X screen takes any rectangle of itself: %v", err)
	}
}
