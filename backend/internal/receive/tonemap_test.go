package receive

import (
	"strings"
	"testing"

	"github.com/go-gst/go-gst/pkg/gst"
	"github.com/go-gst/go-gst/pkg/gstapp"
)

// The rungs are declared per platform and which one resolves is the machine's answer, so every case
// here holds a relationship rather than a value: a test that pinned the rung would pass
// on the machine it was written on and nowhere else.

// TestEveryToneMapRungBuildsWhatItNeeds keeps the availability check from drifting away
// from the launch line, the pair TestChainNeedsAreTheElementsItBuilds holds for a chain.
// A factory a rung is built from and does not name is one nothing can report as missing.
func TestEveryToneMapRungBuildsWhatItNeeds(t *testing.T) {
	for _, r := range toneMapRungs {
		for _, need := range r.needs {
			found := false
			for _, e := range r.elements {
				if e == need || strings.HasPrefix(e, need+" ") {
					found = true
				}
			}
			if !found {
				t.Errorf("the %q rung needs %s and does not build it", r.name, need)
			}
		}
		if len(r.elements) > 0 && len(r.needs) == 0 {
			t.Errorf("the %q rung builds %v and names no factory to check for", r.name, r.elements)
		}
		if r.name == "" {
			t.Errorf("a declared rung building %v carries no name", r.elements)
		}
	}
}

// TestAnOfferedRungIsOneThePipelineCanParse pins the regression the whole probe exists for.
//
// An offer made on the strength of factories registering is an offer vapostproc answers on a VA
// driver that carries no tone-mapping filter and therefore no hdr-tone-mapping property: the offer
// is taken, gst_parse rejects the line, and the decode fails outright rather than falling back
// to no tone mapping.
// An offer this machine reports as available has to be one the parser accepts.
func TestAnOfferedRungIsOneThePipelineCanParse(t *testing.T) {
	rung := toneMapping()
	if !rung.declared() {
		t.Skip("this machine resolves no tone-map rung")
	}
	t.Logf("this machine resolves the %q rung", rung.name)
	for _, r := range toneMapRungs {
		t.Logf("the %q rung parses: %v", r.name, r.buildable())
	}
	if err := rung.buildable(); err != nil {
		t.Errorf("the %q rung resolved and does not parse: %v", rung.name, err)
	}
	if !ToneMapping().Available {
		t.Errorf("the %q rung resolved and the offer reports tone mapping unavailable", rung.name)
	}
}

// TestToneMapRungSitsBetweenTheDecoderAndTheChain pins where the rung goes, the one thing about it
// a reader cannot see from the table.
// Behind the chain's converter the frames are already labelled sRGB, so a rolloff there would read
// its input off a label that has stopped describing the samples.
//
// The rung is matched as one contiguous fragment rather than by its first element, because
// the shader rung and the GL chain share factories: an index taken on glupload alone would find
// whichever of the two came first and prove nothing about the other.
func TestToneMapRungSitsBetweenTheDecoderAndTheChain(t *testing.T) {
	for _, rung := range toneMapRungs {
		fragment := rung.fragment()

		for _, c := range chains {
			line := c.launch(Stream{Source: "videotestsrc"}, rung)
			at := strings.Index(line, fragment)
			if at < 0 {
				t.Errorf("the %q line carries no %q rung: %s", c.name, rung.name, line)
				continue
			}
			if decoder := strings.Index(line, "decodebin"); decoder < 0 || decoder > at {
				t.Errorf("the %q line tone-maps at %d and decodes at %d: %s", c.name, at, decoder, line)
			}
			own := strings.Index(line, strings.Join(c.elements, " ! "))
			if own < 0 || own < at+len(fragment) {
				t.Errorf("the %q line converts at %d and tone-maps at %d: %s", c.name, own, at, line)
			}
		}
	}
}

// TestADecodeThatAsksForNoToneMappingBuildsNoRung is the half that matters on every machine: a rung
// in a line nobody asked for it in is a conversion of every standard-range stream on the grid.
//
// The zero rung is what a decode that asked for none carries, and what a decode on a machine
// that resolves none carries too, so one case covers both.
func TestADecodeThatAsksForNoToneMappingBuildsNoRung(t *testing.T) {
	markers := toneMapMarkers(t)

	for _, c := range chains {
		line := c.launch(Stream{Source: "videotestsrc"}, toneMapRung{})
		for _, marker := range markers {
			if strings.Contains(line, marker) {
				t.Errorf("the %q line carries %q and was asked for no tone mapping: %s", c.name, marker, line)
			}
		}
	}
}

// toneMapMarkers are the fragments only a rung builds, so one of them in a launch line is a rung
// and cannot be anything else.
//
// Two kinds are left out and both would answer for the wrong reason.
// The shader rung shares glupload and glcolorconvert with the GL chain, which builds them whether
// or not anything is tone-mapping.
// A caps fragment is a substring of the caps a chain pins after it, so it matches a line
// that carries no rung at all.
func toneMapMarkers(t *testing.T) []string {
	t.Helper()

	built := map[string]bool{}
	for _, c := range chains {
		for _, e := range c.elements {
			built[e] = true
		}
	}

	var markers []string
	for _, r := range toneMapRungs {
		found := 0
		for _, e := range r.elements {
			if built[e] || strings.HasPrefix(e, "video/") {
				continue
			}
			markers = append(markers, e)
			found++
		}
		// A rung every chain also builds is one this test cannot tell apart from the chain, so it
		// would pass by having nothing to look for.
		if found == 0 {
			t.Errorf("the %q rung builds nothing a chain does not, so its absence cannot be checked", r.name)
		}
	}
	return markers
}

// TestTheToneMapOfferSaysWhatIsMissing holds the contract every offer in this app keeps: an offer
// that cannot be taken names what is absent, and one that can names nothing.
func TestTheToneMapOfferSaysWhatIsMissing(t *testing.T) {
	offer := ToneMapping()

	if offer.Available && offer.MissingElement != "" {
		t.Errorf("this machine tone-maps and reports %q missing", offer.MissingElement)
	}
	if offer.MissingElement != "" && offer.Available {
		t.Errorf("this machine names %q missing and offers tone mapping", offer.MissingElement)
	}
}

// TestAMachineWithNoRungDrawsTheStreamAsItArrives holds the fallback: what a decode was built
// with is what the state reports, so a viewer is never told a conversion happened that this machine
// has no rung for.
func TestAMachineWithNoRungDrawsTheStreamAsItArrives(t *testing.T) {
	if got := toneMapFor("a stream", false); got.declared() {
		t.Errorf("a decode that asked for no tone mapping was built with the %q rung", got.name)
	}
	if got, want := toneMapFor("a stream", true).declared(), ToneMapping().Available; got != want {
		t.Errorf("a decode that asked to tone-map was built with a rung: %t, want %t", got, want)
	}
	if got, want := WillToneMap(true), ToneMapping().Available; got != want {
		t.Errorf("WillToneMap reports %t and the offer reports %t", got, want)
	}
	if WillToneMap(false) {
		t.Error("a decode that asked for no tone mapping is reported as tone-mapped")
	}
}

// TestTheShaderRungRendersAPQFrame is the check no string comparison can make: the GLSL is compiled
// by the driver rather than by this build, so a shader that does not compile is a decode that draws
// nothing and a table that says it converts.
//
// A control run without the shader decides what a missing frame means.
// Both lines carry the same elements against the same PQ source and differ only by the shader, so
// a machine with no GL fails both and is skipped, and a machine that renders the control and not
// the shader has a shader that does not compile.
func TestTheShaderRungRendersAPQFrame(t *testing.T) {
	initGStreamer()

	rung := toneMapping()
	if rung.shader == "" {
		t.Skip("this machine resolves no rung that carries a shader")
	}

	const source = "videotestsrc num-buffers=1 ! " +
		"video/x-raw,format=P010_10LE,width=64,height=64,colorimetry=bt2100-pq ! "
	const tail = " ! videoconvert ! video/x-raw,format=RGBA ! appsink name=sink"

	// The control is the rung with the shader element taken out, leaving the uploads and
	// the conversion it shares with the GL chain.
	control := make([]string, 0, len(rung.elements))
	for _, e := range rung.elements {
		if strings.HasPrefix(e, "glshader") {
			continue
		}
		control = append(control, e)
	}
	if !renders(t, source+strings.Join(control, " ! ")+tail, "") {
		t.Skipf("this machine renders no GL pipeline at all, so the %q rung cannot be tried", rung.name)
	}

	if !renders(t, source+rung.fragment()+tail, rung.shader) {
		t.Errorf("the %q rung renders its own elements and not its shader, so the GLSL does not compile", rung.name)
	}
}

// renders runs one launch line to a single frame and reports whether it arrived, writing the shader
// into the rung's element first where there is one.
func renders(t *testing.T, desc, shader string) bool {
	t.Helper()

	el, err := gst.ParseLaunch(desc)
	if err != nil {
		t.Fatalf("this line does not parse: %v: %s", err, desc)
	}
	pipeline, ok := el.(gst.Pipeline)
	if !ok {
		t.Fatalf("this line did not yield a pipeline: %s", desc)
	}
	defer pipeline.SetState(gst.StateNull)

	if shader != "" {
		element := pipeline.GetByName(toneMapName)
		if element == nil {
			t.Fatalf("this line carries a shader and no %s element: %s", toneMapName, desc)
		}
		element.SetObjectProperty("fragment", shader)
	}

	sink, ok := pipeline.GetByName("sink").(gstapp.AppSink)
	if !ok {
		t.Fatalf("this line grew no appsink: %s", desc)
	}
	pipeline.SetState(gst.StatePlaying)

	sample := sink.PullSample()
	if sample == nil {
		return false
	}
	gst.UnsafeSampleUnref(sample)
	return true
}

// TestTheShaderRungNamesTheElementItWritesInto guards the one thing the launch line and
// the receiver have to agree about.
// The GLSL is written into an element found by name after the parse, so a rung carrying a shader
// whose fragment names no such element leaves the shader unwritten and the picture unconverted.
func TestTheShaderRungNamesTheElementItWritesInto(t *testing.T) {
	for _, r := range toneMapRungs {
		if r.shader == "" {
			continue
		}
		if !strings.Contains(r.fragment(), "name="+toneMapName) {
			t.Errorf("the %q rung carries a shader and names no %s element: %s", r.name, toneMapName, r.fragment())
		}
	}
}
