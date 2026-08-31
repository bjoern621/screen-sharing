package screensrc

import (
	"strings"
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/platform"
)

// An index no machine running this test has an output for,
// which is what makes the heads below the same everywhere.
// What a head does with an enumerated index differs per machine.
// What it does with one the enumeration does not carry is this package's own answer,
// the shape asserted here.
const unenumerated = 4096

// init asserts the same invariant at load.
// A build failing at load fails every test with one message,
// and this one names the row that is wrong.
func TestEverySessionSourceHasAHead(t *testing.T) {
	for _, s := range sessions {
		if len(Head(s.element, unenumerated, true)) == 0 {
			t.Errorf("the %s/%s session reads screens with %q and nothing builds a head for it",
				s.os, s.display, s.element)
		}
	}
}

// Which sessions show one screen, and what the others say instead.
// Stated here as well as in the table, the rows being the whole of the feature's reach:
// a row added or dropped changes what a wizard on that platform offers.
func TestWhichSessionsReadOneScreen(t *testing.T) {
	for name, tc := range map[string]struct {
		platform platform.Info
		element  string
	}{
		"windows reads an output through Desktop Duplication": {
			platform: platform.Info{OS: "windows"},
			element:  D3D11,
		},
		"an X11 session crops the X screen": {
			platform: platform.Info{OS: "linux", Display: "x11"},
			element:  XImage,
		},
		// The portal is Wayland's only way to a screen and answers with whatever the picker was told,
		// so there is no picture of one named output to take.
		"a Wayland session has no way to read one output": {
			platform: platform.Info{OS: "linux", Display: "wayland"},
		},
		"a headless Linux session has no screen at all": {
			platform: platform.Info{OS: "linux"},
		},
		// avfvideosrc chooses its own display,
		// which is also why the monitor setting never reaches the macOS publish pipeline.
		"macOS hands the app whichever screen it likes": {
			platform: platform.Info{OS: "darwin"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			element, gap := Session(tc.platform)

			if element != tc.element {
				t.Errorf("session %+v reads screens with %q, want %q", tc.platform, element, tc.element)
			}
			if (gap == nil) != (tc.element != "") {
				t.Fatalf("session %+v answered element %q and gap %v, and exactly one of the two is the answer",
					tc.platform, element, gap)
			}
			if gap == nil {
				return
			}
			if gap.GetCode() != screensharev1.TextCode_TEXT_CODE_NO_MONITOR_PREVIEW {
				t.Errorf("the gap states %v rather than that there is no monitor preview", gap.GetCode())
			}
			// The sentence is the shell's, written per platform,
			// so the statement carries which platform it is about.
			if len(gap.GetArgs()) == 0 {
				t.Error("the gap names no session, so a shell could only write one sentence for every machine that has none")
			}
		})
	}
}

// The invariant the package exists for: one head,
// so the picture a screen is offered by carries the element and the rectangle the stream would.
func TestThePreviewReadsTheHeadThePublishPipelineIsBuiltFrom(t *testing.T) {
	for _, tc := range []struct {
		platform platform.Info
		element  string
	}{
		{platform.Info{OS: "windows"}, D3D11},
		{platform.Info{OS: "linux", Display: "x11"}, XImage},
	} {
		source, err := PreviewSource(tc.platform, unenumerated)
		if err != nil {
			t.Fatalf("session %+v reads screens and would not build a preview: %v", tc.platform, err)
		}

		head := strings.Join(Head(tc.element, unenumerated, true), " ")
		if !strings.HasPrefix(source, head) {
			t.Errorf("the preview of a screen on %+v does not begin with the head the publish pipeline uses:\n preview: %s\n head:    %s",
				tc.platform, source, head)
		}
	}
}

// Both bounds are what keep a preview at the cost of a wizard tile:
// the default render chain on Linux writes no size bound,
// so a preview that did not reduce its own frames would upload whole desktops.
func TestAPreviewIsPacedAndReduced(t *testing.T) {
	source, err := PreviewSource(platform.Info{OS: "linux", Display: "x11"}, unenumerated)
	if err != nil {
		t.Fatalf("an X11 session reads screens and would not build a preview: %v", err)
	}

	for _, want := range []string{"framerate=5/1", "videoscale", "width=[1,640],height=[1,360]"} {
		if !strings.Contains(source, want) {
			t.Errorf("the preview fragment holds no %q: %s", want, source)
		}
	}
}

// Refused rather than served a fragment that would not build,
// an Umgebungsfehler the control service turns into a status.
func TestASessionThatReadsNoScreenBuildsNoPreview(t *testing.T) {
	if _, err := PreviewSource(platform.Info{OS: "linux", Display: "wayland"}, 0); err == nil {
		t.Error("a Wayland session cannot read one screen apart from another and must not yield a preview fragment")
	}
}

// Uncropped rather than a guessed rectangle, which is the answer x11grab gives.
// The Windows head names the index whatever the enumeration knows,
// the index being that enumeration's own.
func TestAnUnenumeratedIndexLeavesTheHeadUncropped(t *testing.T) {
	x11 := strings.Join(Head(XImage, unenumerated, true), " ")
	if strings.Contains(x11, "startx=") {
		t.Errorf("an index no output carries produced a crop rectangle: %s", x11)
	}

	windows := strings.Join(Head(D3D11, unenumerated, true), " ")
	if !strings.Contains(windows, "monitor-index=4096") {
		t.Errorf("the Windows head drops the index it is given: %s", windows)
	}
}
