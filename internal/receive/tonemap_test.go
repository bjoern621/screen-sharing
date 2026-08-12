package receive

import (
	"strings"
	"testing"
)

// The rung is declared per platform, so every case here holds a relationship rather than a
// value: what this machine can do is the machine's answer, and a test that pinned it would
// pass on the machine it was written on and nowhere else.

// TestToneMapRungBuildsWhatItNeeds is the availability check kept from drifting away from
// the launch line, which is the same pair TestChainNeedsAreTheElementsItBuilds holds for a
// chain: a factory the rung is built from and does not name is one nothing checks for, and
// the pipeline then fails at parse time on a machine without it.
func TestToneMapRungBuildsWhatItNeeds(t *testing.T) {
	for _, need := range toneMapping.needs {
		found := false
		for _, e := range toneMapping.elements {
			if e == need || strings.HasPrefix(e, need+" ") {
				found = true
			}
		}
		if !found {
			t.Errorf("the tone-map rung needs %s and does not build it", need)
		}
	}
	if len(toneMapping.elements) > 0 && len(toneMapping.needs) == 0 {
		t.Errorf("the tone-map rung builds %v and names no factory to check for", toneMapping.elements)
	}
}

// TestToneMapRungSitsBetweenTheDecoderAndTheChain pins where the rung goes, which is the one
// thing about it a reader cannot see from the table. Behind the chain's converter the frames
// are already labelled sRGB, so a rolloff there would read its input off a label that no
// longer describes the samples.
func TestToneMapRungSitsBetweenTheDecoderAndTheChain(t *testing.T) {
	if len(toneMapping.elements) == 0 {
		t.Skip("this platform declares no tone-map rung")
	}
	rung := toneMapping.elements[0]

	for _, c := range chains {
		line := c.launch(Stream{Source: "videotestsrc"}, Open{ToneMap: true})
		at := strings.Index(line, rung)
		if at < 0 {
			t.Errorf("the %q line was asked to tone-map and carries no rung: %s", c.name, line)
			continue
		}
		if decoder := strings.Index(line, "decodebin"); decoder < 0 || decoder > at {
			t.Errorf("the %q line tone-maps at %d and decodes at %d: %s", c.name, at, decoder, line)
		}
		if first := strings.Index(line, c.elements[0]); first >= 0 && first < at {
			t.Errorf("the %q line tone-maps after its own conversion: %s", c.name, line)
		}
	}
}

// TestADecodeThatAsksForNoToneMappingBuildsNoRung is the other half, and the one that
// matters on every machine: a rung in a line nobody asked for it in is a conversion of every
// standard-range stream on the grid.
func TestADecodeThatAsksForNoToneMappingBuildsNoRung(t *testing.T) {
	for _, c := range chains {
		line := c.launch(Stream{Source: "videotestsrc"}, Open{})
		for _, e := range toneMapping.elements {
			if strings.Contains(line, e) {
				t.Errorf("the %q line carries %q and was asked for no tone mapping: %s", c.name, e, line)
			}
		}
	}
}

// TestTheToneMapOfferSaysWhatIsMissing is the contract every offer in this app keeps: an
// offer that cannot be taken names what is absent, and one that can names nothing.
//
// A platform that declares no rung is the third case and it names nothing either, because
// nothing is missing where nothing was declared. The two are told apart by Available, which
// is why both fields are reported rather than the string alone.
func TestTheToneMapOfferSaysWhatIsMissing(t *testing.T) {
	offer := ToneMapping()

	if offer.Available && offer.MissingElement != "" {
		t.Errorf("this machine tone-maps and reports %q missing", offer.MissingElement)
	}
	if len(toneMapping.elements) == 0 {
		if offer.Available {
			t.Error("a platform that declares no rung offers tone mapping")
		}
		if offer.MissingElement != "" {
			t.Errorf("a platform that declares no rung names %q as missing", offer.MissingElement)
		}
	}
}

// TestAMachineWithNoRungDrawsTheStreamAsItArrives holds the fallback: what a decode was
// built with is what the state reports, so a viewer is never told a conversion happened that
// this GStreamer has no element for.
func TestAMachineWithNoRungDrawsTheStreamAsItArrives(t *testing.T) {
	if got := toneMapFor("a stream", false); got {
		t.Error("a decode that asked for no tone mapping was built with the rung")
	}
	if got, want := toneMapFor("a stream", true), ToneMapping().Available; got != want {
		t.Errorf("a decode that asked to tone-map was built with the rung: %t, want %t", got, want)
	}
}
