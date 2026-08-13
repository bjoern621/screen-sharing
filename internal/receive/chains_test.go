package receive

import (
	"strings"
	"testing"
)

// TestChainsAreWellFormed holds the table to its own invariants. A row is read by name, offered to
// a picker by label and tip, and built into a launch line, so one missing any of those becomes an
// unusable offer instead of failing where it is used.
func TestChainsAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range chains {
		if c.name == "" {
			t.Error("a chain carries no name")
		}
		if seen[c.name] {
			t.Errorf("two chains are named %q", c.name)
		}
		seen[c.name] = true
		if c.label == "" {
			t.Errorf("chain %q carries no label", c.name)
		}
		if c.tip == "" {
			t.Errorf("chain %q explains nothing", c.name)
		}
		if c.colour == "" {
			t.Errorf("chain %q says nothing about its colour", c.name)
		}
	}
}

// TestChainNeedsAreTheElementsItBuilds holds the availability check to the launch line: a factory a
// row builds from and does not name is one resolve never looks for, leaving the chain to fail at
// parse time on a machine without it.
func TestChainNeedsAreTheElementsItBuilds(t *testing.T) {
	for _, c := range chains {
		for _, need := range c.needs {
			found := false
			for _, e := range c.elements {
				if e == need || strings.HasPrefix(e, need+" ") {
					found = true
				}
			}
			if !found {
				t.Errorf("chain %q needs %s and does not build it", c.name, need)
			}
		}
	}
}

// TestChainBoundsWhereItNamesAFilter pins the pair a render size is written through: the row's
// elements name the filter and the row's caps go into it. Either alone is a size nothing bounds, or
// caps nothing carries.
func TestChainBoundsWhereItNamesAFilter(t *testing.T) {
	for _, c := range chains {
		names := strings.Contains(strings.Join(c.elements, " ! "), "name="+fitName)
		if names != (c.fitCaps != "") {
			t.Errorf("chain %q names a fit filter (%t) but carries fit caps (%t)", c.name, names, c.fitCaps != "")
		}
	}
}

// TestDeviceChainsKeepTheirMemoryFeature pins the defect this table exists against: a device chain
// whose size bound names no memory feature downloads every frame the moment a tile reports the size
// it draws. The feature belongs in the caps the filter is written with and not only in the ones it
// was parsed with.
//
// A chain bounding nothing has no such caps and answers on the element half alone. Not bounding is
// a device chain's option and not an oversight: the bound pays where a conversion costs its output
// pixels, and on the GPU whole frames can beat renegotiating for tile-sized ones.
func TestDeviceChainsKeepTheirMemoryFeature(t *testing.T) {
	for _, c := range chains {
		if c.device == "" {
			continue
		}
		feature := "(" + c.device + ")"
		if c.fitCaps != "" && !strings.Contains(c.fitCaps, feature) {
			t.Errorf("chain %q asks for %s and bounds itself with %q", c.name, c.device, c.fitCaps)
		}
		if !strings.Contains(strings.Join(c.elements, " ! "), feature) {
			t.Errorf("chain %q asks for %s and no element of it pins that memory", c.name, c.device)
		}
	}
}

// TestChainFitTakesWidthThenHeight pins the order of the template's two figures, which no caller
// can see and a swap would letterbox in silence.
func TestChainFitTakesWidthThenHeight(t *testing.T) {
	for _, c := range chains {
		caps := c.fit(320, 240)
		if c.fitCaps == "" {
			if caps != "" {
				t.Errorf("chain %q bounds itself with %q and names no filter", c.name, caps)
			}
			continue
		}
		if !strings.Contains(caps, "width=[1,320]") || !strings.Contains(caps, "height=[1,240]") {
			t.Errorf("chain %q bounded to 320x240 is %q", c.name, caps)
		}
		if strings.Contains(caps, "%!") {
			t.Errorf("chain %q takes figures its template does not hold: %q", c.name, caps)
		}
	}
}

// TestChainLaunchIsComplete covers what a chain carries whether or not its row says so: the decoder
// the audio branch hangs off, the queue between the two threads, and the sink the receiver reads
// back by name.
func TestChainLaunchIsComplete(t *testing.T) {
	for _, c := range chains {
		line := c.launch(Stream{Source: "videotestsrc"}, toneMapRung{})
		for _, want := range []string{
			"videotestsrc",
			"decodebin name=" + decodeName,
			renderQueue,
			renderSink,
		} {
			if !strings.Contains(line, want) {
				t.Errorf("the %q launch line holds no %q: %s", c.name, want, line)
			}
		}

		// A raw source gives the same line minus the decoder: all the chain carries remains, and the
		// one element with nothing to do is gone.
		raw := c.launch(Stream{Source: "videotestsrc", Raw: true}, toneMapRung{})
		if strings.Contains(raw, "decodebin") {
			t.Errorf("the %q raw launch line autoplugs a decoder for frames nothing encoded: %s", c.name, raw)
		}
		for _, want := range []string{"videotestsrc", renderQueue, renderSink} {
			if !strings.Contains(raw, want) {
				t.Errorf("the %q raw launch line holds no %q: %s", c.name, want, raw)
			}
		}
		for _, e := range c.elements {
			if !strings.Contains(line, e) {
				t.Errorf("the %q launch line drops %q", c.name, e)
			}
		}
	}
}

// TestRenderSinkNamesItself pins what the receiver cannot do without: the sink is read back out of
// the parsed pipeline by name, so one naming itself anything else parses and is then unfindable.
func TestRenderSinkNamesItself(t *testing.T) {
	if !strings.Contains(renderSink, "name="+sinkName) {
		t.Errorf("the sink %q does not name itself %q", renderSink, sinkName)
	}
}

// TestDefaultChainConvertsOnTheDevice is the table's contract with a viewer that never picked a
// chain, asserted at init and restated here.
//
// Two claims, neither of them about which chain is named. Converting is one: stating no colour
// leaves the window mapping an unknown transfer function to BT.709 and washing out every shadow.
// Keeping frames on the device is the other: the frame channel exports a handle to device memory,
// so a default converting in system memory costs every tile a download.
//
// Stating an exact colour is not among them, that being the platform's answer and not the table's:
// on Windows the only exportable memory is Direct3D 11's, whose converter may run through a video
// processor the caps do not describe (DefaultChain).
func TestDefaultChainConvertsOnTheDevice(t *testing.T) {
	c := chainNamed(DefaultChain)
	if c.colour == ColourUnstated {
		t.Errorf("the default chain %q converts nothing", DefaultChain)
	}
	if c.device == "" {
		t.Errorf("the default chain %q converts in system memory, so no frame of it can be exported", DefaultChain)
	}
}

// TestUnconvertedChainConvertsNothing keeps resolve's rule meaningful: the chain it must never fall
// back to is the one handing frames over as they are. No format and no memory pinned, no element of
// its own wanted, and no bound to a tile's size, each of which would be a conversion.
func TestUnconvertedChainConvertsNothing(t *testing.T) {
	c := chainNamed(unconvertedChain)
	if c.colour != ColourUnstated {
		t.Errorf("the %q chain produces %s colour, want %s", c.name, c.colour, ColourUnstated)
	}
	if strings.Contains(strings.Join(c.elements, " ! "), "format=") {
		t.Errorf("the %q chain pins a format: %v", c.name, c.elements)
	}
	if len(c.needs) != 0 {
		t.Errorf("the %q chain needs %v beyond the elements every chain has", c.name, c.needs)
	}
	if c.device != "" || c.fitCaps != "" {
		t.Errorf("the %q chain asks for %q and bounds itself with %q", c.name, c.device, c.fitCaps)
	}
}

// TestChainMemoryFeaturesReadAsMemory keeps the features a chain asks for in the spelling memoryOf
// reads back off a pad, the receive state comparing the two.
func TestChainMemoryFeaturesReadAsMemory(t *testing.T) {
	for _, c := range chains {
		if c.device == "" {
			continue
		}
		if !strings.HasPrefix(c.device, memoryPrefix) {
			t.Errorf("chain %q asks for %q, which names no memory", c.name, c.device)
		}
		if c.device == MemorySystem {
			t.Errorf("chain %q asks for %q, which is not a device's memory", c.name, c.device)
		}
	}
}
