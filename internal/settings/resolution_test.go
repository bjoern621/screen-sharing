package settings

import "testing"

// The empty setting is the capture's own size reaching the encoder unscaled.
// It is a value and not an absence, and the two answers are separated so a caller cannot mistake
// "no scaling" for a zero size and build a scaler around it.
func TestAnEmptyOutputResolutionIsTheCapturesOwnSize(t *testing.T) {
	s := Publish{}

	size, scaled, err := s.OutputSize()
	if err != nil {
		t.Fatalf("OutputSize() = %v, want the unscaled answer", err)
	}
	if scaled {
		t.Error("an empty output resolution reads as scaled")
	}
	if size != (Size{}) {
		t.Errorf("OutputSize() = %v, want the zero size beside a false", size)
	}
}

func TestASetOutputResolutionParsesToItsTwoFigures(t *testing.T) {
	s := Publish{OutputResolution: "1920x1080"}

	size, scaled, err := s.OutputSize()
	if err != nil {
		t.Fatalf("OutputSize() = %v, want a size", err)
	}
	if !scaled {
		t.Error("a set output resolution reads as unscaled")
	}
	if size != (Size{Width: 1920, Height: 1080}) {
		t.Errorf("OutputSize() = %v, want 1920x1080", size)
	}
}

// A size crosses the wire as a string and comes back through the same spelling,
// so the two directions have to be each other's inverse: a value the option list wrote and the
// parser then refused would be a control offering what the publish rejects.
func TestASizeSurvivesFormattingAndParsing(t *testing.T) {
	for _, want := range []Size{{Width: 1920, Height: 1080}, {Width: 640, Height: 360}, {Width: 2560, Height: 1440}} {
		got, err := ParseSize(want.String())
		if err != nil {
			t.Errorf("ParseSize(%q) = %v, want the size it was written from", want.String(), err)
			continue
		}
		if got != want {
			t.Errorf("ParseSize(%q) = %v, want %v", want.String(), got, want)
		}
	}
}

// FormatSize is what an option list builds an entry's value with, and it has to write the same
// spelling the parser reads.
func TestFormatSizeWritesTheSpellingParseSizeReads(t *testing.T) {
	if got, want := FormatSize(1280, 720), "1280x720"; got != want {
		t.Errorf("FormatSize(1280, 720) = %q, want %q", got, want)
	}
}

// Every value that reaches the parser was written by this side, so a malformed one is a caller that
// made it up.
// Each refusal names which part failed, because the caller that made it up is the one reading the
// message.
func TestAMalformedOutputResolutionIsRefused(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"no separator", "1920"},
		{"no width", "x1080"},
		{"no height", "1920x"},
		{"not a number", "wide x tall"},
		{"below the floor", "8x8"},
		// Every chroma subsampling this app encodes in needs an even picture, so an odd side is refused
		// rather than rounded: a size the run silently changed is a size no form can show back.
		{"odd width", "1921x1080"},
		{"odd height", "1920x1081"},
	}
	for _, tc := range cases {
		if _, err := ParseSize(tc.value); err == nil {
			t.Errorf("%s: ParseSize(%q) was accepted", tc.name, tc.value)
		}
	}
}

// A stored settings file carrying a size nothing can parse is an environment condition rather than
// a bug, so it travels as an error and the run refuses with it named.
func TestAMalformedStoredResolutionReachesTheCallerAsAnError(t *testing.T) {
	s := Publish{OutputResolution: "not-a-size"}

	if _, _, err := s.OutputSize(); err == nil {
		t.Error("a settings file naming an unparseable output resolution was accepted")
	}
}
