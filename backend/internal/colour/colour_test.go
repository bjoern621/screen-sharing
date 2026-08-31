package colour

import "testing"

// Both forms answer alike, callers holding one against the other: a capture negotiates a name,
// a capsfilter pins four numbers.
// Swept over every curve the enum carries.
func TestBothSpellingsOfOneCurveAnswerAlike(t *testing.T) {
	for value, want := range namedTransfers {
		if got := TransferOfColorimetry(value); got != want {
			t.Errorf("the colorimetry named %q carries transfer %q, want %q", value, got, want)
		}
	}
	for number, want := range transferNicks {
		// "range:matrix:transfer:primaries", third read, others any legal value.
		value := "1:1:" + number + ":1"
		if got := TransferOfColorimetry(value); got != want {
			t.Errorf("the colorimetry %q carries transfer %q, want %q", value, got, want)
		}
	}
}

// No colorimetry, no transfer, so "" is a readable answer rather than a gap.
// Every caller acts on it: a capture stating none narrows nothing,
// a decode stating none is standard range.
func TestAnUnstatedColorimetryCarriesNoTransfer(t *testing.T) {
	if got := TransferOfColorimetry(""); got != "" {
		t.Errorf("an empty colorimetry carries transfer %q", got)
	}
	if IsHDR(TransferOfColorimetry("")) {
		t.Error("a signal stating no colorimetry reads as HDR")
	}
}

// An unknown value is answered with itself, never a guess.
// A curve a later GStreamer adds to the enum reaches a reader as whatever it is called,
// and nothing promotes it to HDR for being unfamiliar.
func TestAnUnknownColorimetryAnswersWithItself(t *testing.T) {
	for _, value := range []string{"bt2100-something", "1:1:99:1", "nonsense"} {
		got := TransferOfColorimetry(value)
		if got == "" {
			t.Errorf("the colorimetry %q was read as carrying no transfer", value)
		}
		if IsHDR(got) {
			t.Errorf("the colorimetry %q was read as HDR", value)
		}
	}
}

func TestOnlyTheTwoBT2100CurvesAreHDR(t *testing.T) {
	for _, value := range []string{"bt2100-pq", "bt2100-hlg"} {
		if !IsHDR(TransferOfColorimetry(value)) {
			t.Errorf("the colorimetry %q is not read as HDR", value)
		}
	}
	for _, value := range []string{"bt601", "bt709", "bt2020", "sRGB", "smpte240m"} {
		if IsHDR(TransferOfColorimetry(value)) {
			t.Errorf("the colorimetry %q is read as HDR", value)
		}
	}

	// Names resolve to the nicks the constants spell, so a caller can hold a transfer read off
	// a pipeline against them.
	if TransferOfColorimetry("bt2100-pq") != TransferPQ || TransferOfColorimetry("bt2100-hlg") != TransferHLG {
		t.Error("the two HDR names resolve to transfers the constants do not spell")
	}
}
