package publish

import (
	"bytes"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/gstrun"
)

// The caps are the whole of the verdict, so the readings are held to real caps strings: the named
// colorimetries a capture negotiates, the four-component form a capsfilter pins,
// and caps carrying no colorimetry at all.
func TestAnHdrCaptureIsReadOffTheCapsAndNeverGuessed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		caps  string
		hdr   bool
		trans string
	}{
		{
			name:  "a PQ surface",
			caps:  "video/x-raw, format=(string)P010_10LE, colorimetry=(string)bt2100-pq",
			hdr:   true,
			trans: "smpte2084",
		},
		{
			name:  "an HLG surface",
			caps:  "video/x-raw, format=(string)P010_10LE, colorimetry=(string)bt2100-hlg",
			hdr:   true,
			trans: "arib-std-b67",
		},
		{
			name:  "an ordinary desktop",
			caps:  "video/x-raw, format=(string)BGRx, colorimetry=(string)bt709",
			hdr:   false,
			trans: "bt709",
		},
		{
			// Wide primaries with a standard-range curve, the case the transfer characteristic exists to
			// separate: reading the primaries would publish a wide-gamut SDR desktop as HDR.
			name:  "a wide-gamut SDR surface",
			caps:  "video/x-raw, format=(string)BGRx, colorimetry=(string)bt2020",
			hdr:   false,
			trans: "bt2020-10",
		},
		{
			// The form a capsfilter pins: range:matrix:transfer:primaries.
			// It reads back in the nicks the named form uses, which is what lets the child hold one against
			// the other when it narrows the encoder input to the surface's colour.
			name:  "the four-component form",
			caps:  "video/x-raw, format=(string)P010_10LE, colorimetry=(string)1:6:14:7",
			hdr:   true,
			trans: "smpte2084",
		},
		{
			name:  "the four-component form at standard range",
			caps:  "video/x-raw, format=(string)I420, colorimetry=(string)2:3:5:1",
			hdr:   false,
			trans: "bt709",
		},
		{
			// Caps carrying no colour are SDR: guessing upward tags a desktop PQ that never carried it.
			name:  "caps that say nothing about colour",
			caps:  "video/x-raw, format=(string)I420, width=(int)64",
			hdr:   false,
			trans: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := captureTransfer(tc.caps); got != tc.trans {
				t.Errorf("transfer = %q, want %q", got, tc.trans)
			}
			if got := hdrCapture(tc.caps); got != tc.hdr {
				t.Errorf("hdr = %v, want %v", got, tc.hdr)
			}
		})
	}
}

// An HDR capture in eight bits stops the publish, and the refusal names both ends,
// what the capture turned out to carry and what the settings asked to code it as,
// either one being a way out.
func TestAnHdrCaptureRefusesTheEightBitFormats(t *testing.T) {
	const pq = "video/x-raw, format=(string)P010_10LE, colorimetry=(string)bt2100-pq"

	for _, chroma := range []string{"yuv420p", "yuv444p", "yuv422p", "gbrp"} {
		s := baseStream()
		s.Publish.Chroma = chroma

		err := hdrRefusal(s, pq)
		if err == nil {
			t.Errorf("an HDR capture was published as %s, which carries eight bits", chroma)
			continue
		}
		for _, end := range []string{"smpte2084", chroma, tenBitChroma} {
			if !strings.Contains(err.Error(), end) {
				t.Errorf("the refusal for %s does not name %s: %v", chroma, end, err)
			}
		}
	}

	s := baseStream()
	s.Publish.Chroma = tenBitChroma
	if err := hdrRefusal(s, pq); err != nil {
		t.Errorf("an HDR capture in ten bits was refused: %v", err)
	}
}

// The rule is about a surface carrying more than the format can, and every combination here carries
// less, so an SDR capture is refused nothing whatever it is coded as.
func TestAnSdrCaptureIsRefusedNothing(t *testing.T) {
	for _, chroma := range []string{"yuv420p", "gbrp", tenBitChroma} {
		s := baseStream()
		s.Publish.Chroma = chroma
		if err := hdrRefusal(s, "video/x-raw, colorimetry=(string)bt709"); err != nil {
			t.Errorf("an SDR capture coded as %s was refused: %v", chroma, err)
		}
	}
}

// Each kind of line on the child's output reaches its own reader and no other.
// The meter is nil here, a run nobody asked progress from, which still reports what it captured.
func TestTheChildsOutputSplitsCapsFromProgress(t *testing.T) {
	output := strings.Join([]string{
		"progressreport0 (00:00:01): 60 buffers",
		gstrun.CapsPrefix + "video/x-raw, colorimetry=(string)bt2100-pq",
		"progressreport0 (00:00:02): 120 buffers",
	}, "\n") + "\n"

	var seen []string
	gstReadChild(strings.NewReader(output), nil, func(caps string) {
		seen = append(seen, caps)
	}, nil)

	if len(seen) != 1 {
		t.Fatalf("the caps reader saw %d lines, want the one the child reported: %v", len(seen), seen)
	}
	if !hdrCapture(seen[0]) {
		t.Errorf("the caps reached the reader as %q, which no longer reads as HDR", seen[0])
	}
}

// Through the runner the publish child runs: a source negotiating PQ is reported as PQ,
// so the verdict rests on what a capture says rather than on a string written in a test.
func TestAPqPipelineReportsItselfAsHdr(t *testing.T) {
	caps := runnerCaps(t, "videotestsrc num-buffers=2 ! video/x-raw,format=P010_10LE,width=64,height=64,colorimetry=bt2100-pq ! fakesink")

	if !hdrCapture(caps) {
		t.Errorf("a PQ pipeline reported %q, which does not read as HDR", caps)
	}
	s := baseStream()
	s.Publish.Chroma = "yuv420p"
	if hdrRefusal(s, caps) == nil {
		t.Error("a PQ capture in eight bits was not refused")
	}
}

// The same for the surface an ordinary desktop is: reported, and refused nothing.
func TestAnOrdinaryPipelineReportsItselfAsSdr(t *testing.T) {
	caps := runnerCaps(t, "videotestsrc num-buffers=2 ! video/x-raw,format=I420,width=64,height=64,colorimetry=bt709 ! fakesink")

	if hdrCapture(caps) {
		t.Errorf("a BT.709 pipeline reported %q, which reads as HDR", caps)
	}
}

// runnerCaps plays one pipeline through the runner the publish child runs and returns the caps it
// reported.
// videotestsrc rather than a capture backend: the reading is what is under test,
// and a run needing a screen or a portal consent would test the machine.
func runnerCaps(t *testing.T, description string) string {
	t.Helper()

	var out bytes.Buffer
	if err := gstrun.Run(t.Context(), description, &out); err != nil {
		t.Fatalf("running %q: %v", description, err)
	}
	for _, line := range strings.Split(out.String(), "\n") {
		if caps, ok := strings.CutPrefix(line, gstrun.CapsPrefix); ok {
			return caps
		}
	}
	t.Fatalf("the run reported no caps: %q", out.String())
	return ""
}
