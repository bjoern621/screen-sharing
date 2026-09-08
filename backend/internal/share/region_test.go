package share

import "testing"

func TestParseRectReadsTheSettingsSpelling(t *testing.T) {
	r, ok := ParseRect("100,200,1280x720")
	if !ok {
		t.Fatal("a well-formed rectangle parses")
	}
	if r.X != 100 || r.Y != 200 || r.Width != 1280 || r.Height != 720 {
		t.Fatalf("parsed %+v", r)
	}
}

func TestParseRectTakesNegativeOrigins(t *testing.T) {
	r, ok := ParseRect("-1920,-40,1920x1080")
	if !ok {
		t.Fatal("a screen left of the primary one carries negative coordinates")
	}
	if r.X != -1920 || r.Y != -40 {
		t.Fatalf("parsed %+v", r)
	}
}

func TestParseRectRefusesWhatCannotBeCaptured(t *testing.T) {
	for _, in := range []string{
		"",
		"100,200",
		"100,200,1280",
		"100,200,1280x",
		"100,200,0x720",
		"100,200,1280x0",
		"100,200,-4x720",
		"a,200,1280x720",
		"100,200,1280x720,",
	} {
		if _, ok := ParseRect(in); ok {
			t.Errorf("%q parsed", in)
		}
	}
}

func TestRectRoundTripsThroughItsSpelling(t *testing.T) {
	want := "-100,0,640x480"
	r, ok := ParseRect(want)
	if !ok {
		t.Fatalf("%q parses", want)
	}
	if got := r.String(); got != want {
		t.Fatalf("spelled %q", got)
	}
}
