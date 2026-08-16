package app

import (
	"testing"

	"bjoernblessin.de/screenshare/internal/receive"
)

// Tone mapping is part of what a decode is built from, so asking for it again has to mean "is this
// already the case" and never "build it again".
//
// The trap is a machine that cannot tone-map at all.
// A decode there is built without the rung whatever it was asked for, so a caller comparing its own
// request against what ran finds the two different on every call and tears down a pipeline that was
// already everything it can be.
// WillToneMap is what they are compared through: it answers what a request builds rather than what
// it asked for.

// aDecodeOf is one real decode of a synthetic picture, on the chain that needs no display.
//
// A real pipeline rather than a stand-in, because what is asked is what a run was built with:
// a double would answer with whatever it was handed, the reading this is here to rule out.
func aDecodeOf(t *testing.T, toneMap bool) *receive.Receiver {
	t.Helper()

	r, err := receive.New(
		receive.Stream{Name: "probe", Transport: "none", Source: "videotestsrc is-live=true", Raw: true},
		receive.Open{Chain: "cpu", ToneMap: toneMap},
		receive.Events{})
	if err != nil {
		t.Fatalf("opening a decode of a synthetic picture: %v", err)
	}
	t.Cleanup(func() { r.Stop() })
	return r
}

func withDecode(t *testing.T, ref StreamRef, toneMap bool) *App {
	t.Helper()

	return &App{receivers: map[StreamRef]*receive.Receiver{ref: aDecodeOf(t, toneMap)}}
}

// TestAskingAgainForTheToneMappingThatRanRebuildsNothing covers what a viewer produces every time it
// repeats a call, and the loop a machine with no rung would otherwise run.
func TestAskingAgainForTheToneMappingThatRanRebuildsNothing(t *testing.T) {
	ref := StreamRef{Name: "bob", Transport: "rtsp"}

	for _, asked := range []bool{false, true} {
		a := withDecode(t, ref, asked)
		wanted := receive.WillToneMap(asked)

		if !a.receiving(ref, wanted) {
			t.Errorf("a decode built having asked for %t reads as absent when asked for again", asked)
		}
		if replaced := a.replacedReceiver(ref, wanted); replaced != nil {
			t.Errorf("asking again for %t tore down a decode that already answers it", asked)
		}
	}
}

// TestTheOtherAnswerIsADifferentDecode means something only where the machine has a rung:
// there the two answers are two pipelines, and the running one is handed back to be stopped rather
// than left beside its replacement.
func TestTheOtherAnswerIsADifferentDecode(t *testing.T) {
	if !receive.ToneMapping().Available {
		t.Skip("this machine has no element that rolls an HDR stream down")
	}

	ref := StreamRef{Name: "bob", Transport: "rtsp"}
	a := withDecode(t, ref, false)

	if a.receiving(ref, true) {
		t.Error("a decode built without the rung reads as one that tone-maps")
	}
	replaced := a.replacedReceiver(ref, true)
	if replaced == nil {
		t.Fatal("asking for tone mapping left the decode that does not tone-map running")
	}
	replaced.Stop()

	// Taken out of the set by the call that handed it back, so nothing is left keyed by a pair whose
	// pipeline is being torn down.
	if a.receiving(ref, false) {
		t.Error("the replaced decode is still the one this pair is keyed to")
	}
}
