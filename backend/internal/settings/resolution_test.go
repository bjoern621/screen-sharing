package settings

import "testing"

// Empty is a value and not an absence,
// which is what the second answer keeps a caller from mistaking for a zero size to scale around.
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

// Writing and reading are each other's inverse.
// A value the option list wrote and the parser refused is a control offering a refusal.
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

// An option list builds every entry's value with FormatSize.
func TestFormatSizeWritesTheSpellingParseSizeReads(t *testing.T) {
	if got, want := FormatSize(1280, 720), "1280x720"; got != want {
		t.Errorf("FormatSize(1280, 720) = %q, want %q", got, want)
	}
}

// Each refusal names which part failed, whoever has to repair the value being the one reading it.
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
		// An odd side is refused rather than rounded,
		// every chroma subsampling here needing an even picture.
		{"odd width", "1921x1080"},
		{"odd height", "1920x1081"},
	}
	for _, tc := range cases {
		if _, err := ParseSize(tc.value); err == nil {
			t.Errorf("%s: ParseSize(%q) was accepted", tc.name, tc.value)
		}
	}
}

// A size a stored file carries and nothing can parse is an Umgebungsfehler,
// so it reaches the caller as an error rather than an assert.
func TestAMalformedStoredResolutionReachesTheCallerAsAnError(t *testing.T) {
	s := Publish{OutputResolution: "not-a-size"}

	if _, _, err := s.OutputSize(); err == nil {
		t.Error("a settings file naming an unparseable output resolution was accepted")
	}
}
