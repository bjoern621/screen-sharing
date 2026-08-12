package app

import (
	"testing"

	"bjoernblessin.de/screenshare/internal/receive"
)

// Tone mapping is part of what a decode is built from, so asking for it again has to be the
// question "is this already the case" and never "build it again".
//
// The trap it is written against is a machine that cannot tone-map at all. A decode there is
// built without the rung whatever it was asked for, so a caller comparing its own request
// against what ran finds them different on every call and tears down a pipeline that was
// already everything it can be. What the two are compared through is WillToneMap, which
// answers what a request builds rather than what it asked for.

// aDecodeOf is one real decode of a synthetic picture, on the chain that needs no display.
//
// A real pipeline rather than a stand-in, because what is being asked is what a run was
// built with: a double would answer with whatever it was handed, which is the reading this
// is here to rule out.
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

// withDecode is a backend holding one decode of one pair.
func withDecode(t *testing.T, key WatchKey, toneMap bool) *App {
	t.Helper()

	return &App{receivers: map[WatchKey]*receive.Receiver{key: aDecodeOf(t, toneMap)}}
}

// TestAskingAgainForTheToneMappingThatRanRebuildsNothing is the case a viewer produces every
// time it repeats a call, and the one a machine with no rung would otherwise loop on.
func TestAskingAgainForTheToneMappingThatRanRebuildsNothing(t *testing.T) {
	key := WatchKey{Name: "bob", Transport: "rtsp"}

	for _, asked := range []bool{false, true} {
		a := withDecode(t, key, asked)
		wanted := receive.WillToneMap(asked)

		if !a.receiving(key, wanted) {
			t.Errorf("a decode built having asked for %t reads as absent when asked for again", asked)
		}
		if replaced := a.replacedReceiver(key, wanted); replaced != nil {
			t.Errorf("asking again for %t tore down a decode that already answers it", asked)
		}
	}
}

// TestTheOtherAnswerIsADifferentDecode is the half that only means anything where the
// machine has a rung: there the two answers are two pipelines, and the running one is handed
// back to be stopped rather than left beside its replacement.
func TestTheOtherAnswerIsADifferentDecode(t *testing.T) {
	if !receive.ToneMapping().Available {
		t.Skip("this machine has no element that rolls an HDR stream down")
	}

	key := WatchKey{Name: "bob", Transport: "rtsp"}
	a := withDecode(t, key, false)

	if a.receiving(key, true) {
		t.Error("a decode built without the rung reads as one that tone-maps")
	}
	replaced := a.replacedReceiver(key, true)
	if replaced == nil {
		t.Fatal("asking for tone mapping left the decode that does not tone-map running")
	}
	replaced.Stop()

	// Taken out of the set by the same call that handed it back, so nothing is left keyed
	// by a pair whose pipeline is being torn down.
	if a.receiving(key, false) {
		t.Error("the replaced decode is still the one this pair is keyed to")
	}
}
