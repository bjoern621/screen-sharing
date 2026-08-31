package receive

import (
	"strings"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// Tone mapping is the one thing a viewer decides about an HDR stream.
//
// A stream carrying one of the BT.2100 curves carries more range than a standard display shows.
// Drawn as it is, it is wrong in the direction the curve leans:
// the frames reach a window labelled sRGB and carrying PQ samples,
// every chain here converting matrix and range and applying no transfer function at all.
// Rolling that range down into the display's is a conversion of its own.
//
// A choice rather than something done to every HDR stream, and per tile rather than per machine.
// A reader comparing what a stream carries against what a display shows wants an untouched picture,
// a reader watching it wants the mapped one,
// and both can be on screen at once through two tiles of the same stream.
//
// Not a setting either: a preference kept per stream path would outlive the stream it was made
// about, so a path that stops carrying HDR would carry a stale choice nobody can find.
// The choice lives with the decode, which is exactly as long as it means anything.
//
// PQ is rolled down and HLG is not.
// PQ is absolute, so an untouched PQ picture is wrong by the ratio between the display's peak and
// the format's ten thousand nits, the failure this rung exists for.
// HLG is display-referred and its lower range tracks a standard gamma curve,
// the property it was designed around,
// so an HLG stream drawn as it arrives is approximately right and is left alone rather than given
// PQ's inverse.

// toneMapName is what the rung's shader element is called in the launch line.
//
// The GLSL is written into it after the parse rather than carried in the line,
// for the reason the fit capsfilter's caps are: a launch line is parsed as one string,
// and a shader carries the newlines its preprocessor directives need and the quotes and separators
// the parser reads as syntax.
const toneMapName = "tonemap"

// toneMapRung is one way to roll an HDR stream down:
// the launch-line fragment that goes between the decoder and the chain,
// and the GLSL a shader element in it is given.
//
// One rung for every chain rather than a column on each chain row.
// A rung takes whatever the decoder produced and hands on a standard-range picture,
// the input every chain already converts,
// so which rung this machine has is the platform's answer and not the row's.
type toneMapRung struct {
	// name is what the rung is called in a log line.
	// An identifier and never a sentence, the words a reader sees being the shell's.
	name string
	// needs are the element factories the fragment is built from.
	// What an offer that cannot be taken names, the parse deciding whether it can be taken.
	needs []string
	// elements are the pieces of the launch line, joined with " ! ".
	elements []string
	// shader is the GLSL given to the element named toneMapName,
	// and "" on a rung that builds no shader.
	shader string
}

// declared separates a rung from the zero value a machine with none resolves to.
func (r toneMapRung) declared() bool {
	return len(r.elements) > 0
}

// fragment is the rung's own text inside a launch line.
func (r toneMapRung) fragment() string {
	return strings.Join(r.elements, " ! ")
}

// buildable reports whether gst_parse accepts this rung's fragment on this machine.
//
// The fragment is parsed rather than its factories looked up,
// a fragment naming properties as well as elements and either being a way for it not to build.
// vapostproc is the case that decided it: the element registers wherever a VA driver does,
// and hdr-tone-mapping is a property only a driver whose VA implementation carries the tone-mapping
// filter has.
// Mesa's radeonsi carries the driver and not the filter,
// so a registry lookup answers yes on a machine where the parse then fails,
// and the decode fails with it rather than falling back.
//
// The reading is the same operation the pipeline performs, which stops the two from disagreeing.
func (r toneMapRung) buildable() error {
	assert.Assert(r.declared(), "a rung that is probed is one with elements to build", r.name)

	_, err := gst.ParseBinFromDescription(r.fragment(), true)
	return err
}

// toneMapping is the rung this machine rolls an HDR stream down with:
// the first declared rung that builds here, and the zero rung where none of them does.
//
// Read through on every call rather than settled once,
// the registry and the driver behind it being a fact this process reads and never owns.
func toneMapping() toneMapRung {
	// The registry decides what builds and it exists once the library is up,
	// which a caller asking before it has opened anything has not done.
	initGStreamer()

	for _, r := range toneMapRungs {
		if err := r.buildable(); err != nil {
			logger.Debugf("the %q tone-map rung does not build on this machine: %v", r.name, err)
			continue
		}
		return r
	}
	return toneMapRung{}
}

// toneMapMissing names the first factory the last declared rung needs and this GStreamer does not
// register, and "" where every one of them registers.
//
// The last rung is read: the list runs from the rung that needs the most of a machine to the one
// that needs the least,
// so what the last one is missing is what a machine with no rung at all lacks.
// A machine registering every factory and still building none is left unnamed,
// what failed there being a property rather than an element and nothing a reader can install.
func toneMapMissing() string {
	if len(toneMapRungs) == 0 {
		return ""
	}
	return missingFactory(toneMapRungs[len(toneMapRungs)-1].needs)
}

// WillToneMap is whether a decode that asks for tone mapping is built with a rung.
//
// What a caller compares a running decode against, rather than its own request.
// A machine with no rung builds without one,
// so a caller holding its own request against what ran would find the two different every time and
// rebuild the same pipeline forever.
func WillToneMap(want bool) bool {
	return want && toneMapping().declared()
}

// toneMapFor is the rung a decode is built with, and the line that says one was dropped.
//
// A rung this machine does not have is dropped rather than refused, for the reason a chain falls
// back: what is missing is an element or a driver feature, an Umgebungsfehler,
// and a stream drawn in the range it was coded in is a picture where a refusal is none.
// The answer is what the receive state then reports,
// so a tile shows what ran rather than what was asked for.
func toneMapFor(stream string, want bool) toneMapRung {
	assert.Assert(stream != "", "a decode being built for is one with a name")

	if !want {
		return toneMapRung{}
	}
	if r := toneMapping(); r.declared() {
		return r
	}
	logger.Warnf("stream %q was asked to be tone-mapped and no rung on this machine rolls an HDR stream down, so it is drawn as it arrives", stream)
	return toneMapRung{}
}

// ToneMap is what this machine can do about an HDR stream, as a viewer offers the choice.
//
// Nothing above this package names GStreamer,
// so which element rolls the range down stays here and only the reading of it crosses.
type ToneMap struct {
	// Available is whether a decode that asks to tone-map is built with a rung that does.
	// A decode that asks anyway on a machine that cannot is built without one and reports so,
	// the same fallback a render chain makes.
	Available bool
	// MissingElement names the first factory the most portable rung needs
	// and this GStreamer does not register.
	// Empty on a machine that can tone-map,
	// and empty too where every factory registers and the rung failed on a property,
	// the pair Available tells apart.
	MissingElement string
}

// ToneMapping is what this machine offers a viewer that is watching an HDR stream.
func ToneMapping() ToneMap {
	// The registry answers what builds only once the library is up,
	// and a form can ask what it may offer before a stream is ever received.
	initGStreamer()

	offer := ToneMap{Available: toneMapping().declared()}
	if !offer.Available {
		offer.MissingElement = toneMapMissing()
	}
	// An offer that can be taken names nothing missing,
	// the contract every offer in this app keeps and the one a reader reads the pair by.
	assert.Assert(!offer.Available || offer.MissingElement == "",
		"a machine that tone-maps names no missing element", offer.MissingElement)
	return offer
}
