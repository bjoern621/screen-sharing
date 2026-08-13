package colour

import "testing"

// The two forms of the field have to answer alike, because callers hold one against the other:
// a capture negotiates a name and a capsfilter pins four numbers.
// This is the case that says so for every curve the enum carries, rather than for the two anybody
// remembered.
func TestBothSpellingsOfOneCurveAnswerAlike(t *testing.T) {
	for value, want := range namedTransfers {
		if got := TransferOfColorimetry(value); got != want {
			t.Errorf("the colorimetry named %q carries transfer %q, want %q", value, got, want)
		}
	}
	for number, want := range transferNicks {
		// A capsfilter pins range, matrix, transfer and primaries.
		// The other three are what they are: only the third is read.
		value := "1:1:" + number + ":1"
		if got := TransferOfColorimetry(value); got != want {
			t.Errorf("the colorimetry %q carries transfer %q, want %q", value, got, want)
		}
	}
}

// A signal carrying no colorimetry at all carries no transfer, which is what makes "no transfer" a
// readable answer rather than a gap.
// Every caller acts on it: a capture stating none narrows nothing, and a decode stating none is
// standard range.
func TestAnUnstatedColorimetryCarriesNoTransfer(t *testing.T) {
	if got := TransferOfColorimetry(""); got != "" {
		t.Errorf("an empty colorimetry carries transfer %q", got)
	}
	if IsHDR(TransferOfColorimetry("")) {
		t.Error("a signal stating no colorimetry reads as HDR")
	}
}

// A value this table has never seen is answered with itself rather than with a guess.
// It is the case a new GStreamer produces: a curve added to the enum reaches a reader as whatever
// it is called, and nothing here promotes it to HDR on the strength of being unfamiliar.
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

// The verdict is the two BT.2100 curves and nothing else.
// Everything else in the enum describes a standard-range picture whatever its primaries are,
// which is why a wide-gamut SDR desktop is not HDR and the primaries are read nowhere.
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

	// The nicks the two names resolve to are the ones the constants spell, which is what lets a caller
	// compare a transfer it read off a pipeline against them.
	if TransferOfColorimetry("bt2100-pq") != TransferPQ || TransferOfColorimetry("bt2100-hlg") != TransferHLG {
		t.Error("the two HDR names resolve to transfers the constants do not spell")
	}
}
