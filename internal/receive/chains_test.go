package receive

import (
	"strings"
	"testing"
)

// TestChainsAreWellFormed holds the table's own invariants. Every row is read by
// name, offered to a picker by label and tip, and built into a launch line, so a
// row missing any of those renders an unusable offer rather than failing where it
// is used.
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

// TestChainNeedsAreTheElementsItBuilds keeps the availability check from drifting
// away from the launch line: a factory a row is built from and does not name is a
// factory resolve never checks for, and the chain then fails at parse time on a
// machine that does not have it.
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

// TestChainBoundsWhereItNamesAFilter pins the pair the render size is written
// through: the row's elements name the filter and the row's caps are what goes into
// it. One without the other is a size nothing bounds or caps nothing carries.
func TestChainBoundsWhereItNamesAFilter(t *testing.T) {
	for _, c := range chains {
		names := strings.Contains(strings.Join(c.elements, " ! "), "name="+fitName)
		if names != (c.fitCaps != "") {
			t.Errorf("chain %q names a fit filter (%t) but carries fit caps (%t)", c.name, names, c.fitCaps != "")
		}
	}
}

// TestDeviceChainsKeepTheirMemoryFeature is the bug this table replaced: a device
// chain whose size bound names no memory feature downloads every frame the moment a
// tile reports the size it draws. The feature has to be in the caps the filter is
// written with, not only in the ones it was parsed with.
//
// A chain that bounds nothing has no such caps and is held to the element half alone.
// Not bounding is a device chain's option rather than an oversight: the bound pays for
// itself where the conversion costs its output pixels, and a chain converting on the
// GPU can be cheaper converting whole frames than renegotiating for tile-sized ones.
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

// TestChainFitTakesWidthThenHeight pins the order of the two figures in the
// template, which a caller cannot see and a swap would silently letterbox.
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

// TestChainLaunchIsComplete covers what every chain carries whether or not its row
// says so: the decoder the audio branch hangs off, the queue between the two
// threads, and the sink the receiver reads back by name.
func TestChainLaunchIsComplete(t *testing.T) {
	for _, c := range chains {
		line := c.launch("videotestsrc", false)
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

		// A raw source is the same line without the decoder: everything the chain
		// carries is still there, and the one element that has nothing to do is gone.
		raw := c.launch("videotestsrc", true)
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

// TestRenderSinkNamesItself pins the one thing the receiver cannot do without: it
// reads the sink back out of the parsed pipeline by name, so a sink that names
// itself anything else parses and then cannot be found.
func TestRenderSinkNamesItself(t *testing.T) {
	if !strings.Contains(renderSink, "name="+sinkName) {
		t.Errorf("the sink %q does not name itself %q", renderSink, sinkName)
	}
}

// TestDefaultChainConvertsOnTheDevice is the table's contract with a viewer that
// never picked a chain, asserted at init and stated here as the reason the table is
// ordered the way it is.
//
// Two claims rather than one, and neither is about which chain is named. It has to
// convert, because a chain that states no colour at all leaves the window mapping an
// unknown transfer function to BT.709 and washing out every shadow; and it has to keep
// its frames on the device, because the frame channel exports a handle to device
// memory and a default that converted in system memory would make every tile cost a
// download.
//
// What it does not have to do is state an exact colour, and that is the platform's
// answer rather than the table's: on Windows the only exportable memory is Direct3D
// 11's, whose converter may run through a video processor the caps do not describe
// (DefaultChain).
func TestDefaultChainConvertsOnTheDevice(t *testing.T) {
	c := chainNamed(DefaultChain)
	if c.colour == ColourUnstated {
		t.Errorf("the default chain %q converts nothing", DefaultChain)
	}
	if c.device == "" {
		t.Errorf("the default chain %q converts in system memory, so no frame of it can be exported", DefaultChain)
	}
}

// TestUnconvertedChainConvertsNothing is what keeps resolve's rule meaningful: the
// chain it must never fall back to is the one that hands the frames over as they
// are. It pins no format and no memory, asks for no element of its own, and cannot
// be bounded to a tile's size, because every one of those would be a conversion.
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

// TestChainMemoryFeaturesReadAsMemory keeps the features the chains ask for in the
// spelling memoryOf reads back off a pad, since the receive state compares the two.
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
