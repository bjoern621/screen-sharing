package rules

import (
	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// The axes a rule may match on: the vocabulary, declared once, so a rule names a fact by the same
// identifier every other consumer knows it by.
//
// An axis that is a settings field carries that field's key, so a rule matching on the codec and a
// form control the same rule greys are the one identifier on both sides.
// The keys are spelled here rather than imported because this package is the one every other domain
// package registers into and may therefore import nothing of theirs; the form holds the two lists
// to each other in a test, so a rename that reaches only one of them fails there rather than by
// binding nothing.
//
// The rest are derived facts, which are exactly what the old Gap could not express:
// the publish engine is not a settings field but follows from the capture backend,
// and it had a column of its own on every gap because there was nowhere else to put it.
// A derived axis carries no field key, because there is no control to name.
const (
	AxisCodec      = "publish.codec"
	AxisChroma     = "publish.chroma"
	AxisMode       = "publish.mode"
	AxisColorRange = "publish.color_range"
	AxisCapture    = "publish.capture"
	AxisTransport  = "publish.publish_transport"
	AxisAudioCodec = "publish.audio_codec"
	AxisMemory     = "publish.capture_memory"
	AxisCursor     = "publish.cursor"
	AxisBitrateM   = "publish.bitrate_mbps"
	AxisCq         = "publish.cq"

	// FieldAudioGain is a control a rule lands on and no rule matches on.
	// Not every control is a fact a configuration reads: the level one entry of the audio list runs at
	// is something a rule states liveness about, and nothing binds under it.
	// It is spelled here for the reason the axes are, since the form and the publish engine both name
	// it and a second spelling is one that can disagree.
	FieldAudioGain = "publish.audio_sources[].gain"

	AxisEngine  = "engine"
	AxisFamily  = "codec.family"
	AxisFormat  = "codec.format"
	AxisOS      = "platform.os"
	AxisDisplay = "platform.display"
)

// Kind is what an axis reads as, which decides how a match against it is written and which of the
// two halves of a Value carries it.
type Kind int

const (
	// KindText is an identifier out of a closed set: a codec name, a pixel format, an engine.
	KindText Kind = iota + 1
	// KindNumber is a quantity, matched by range rather than by membership.
	KindNumber
)

// Axis is one fact a rule may name.
type Axis struct {
	// Name is the identifier, which is the settings field key where the axis is a field.
	Name string
	// Kind is what it reads as.
	Kind Kind
	// Arg is the argument a reason carries this axis under, so a statement naming the axis gets the
	// current value attached without the rule spelling it.
	// An axis with no argument is one no sentence has a slot for, which is asserted against rather
	// than dropped: a fact worth constraining on is a fact worth naming in the refusal.
	Arg screensharev1.TextArgName
}

// axes is the vocabulary, in domain order.
//
// The order is read rather than decorative: a reason attaches the axes its rule matched on in this
// order, so two rules matching the same facts produce their identifiers in the same sequence and a
// shell reading them meets the subject before the qualifier.
var axes = []Axis{
	{Name: AxisCapture, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_CAPTURE},
	{Name: AxisEngine, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_ENGINE},
	{Name: AxisCodec, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_CODEC},
	{Name: AxisFamily, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_FAMILY},
	{Name: AxisFormat, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_FORMAT},
	{Name: AxisChroma, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_CHROMA},
	{Name: AxisColorRange, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_COLOR_RANGE},
	{Name: AxisMode, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_MODE},
	{Name: AxisMemory, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_MEMORY},
	{Name: AxisCursor, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_CURSOR},
	{Name: AxisTransport, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_TRANSPORT},
	{Name: AxisAudioCodec, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_AUDIO_CODEC},
	{Name: AxisOS, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_OS},
	{Name: AxisDisplay, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_DISPLAY},
	{Name: AxisBitrateM, Kind: KindNumber, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_BITRATE_MBPS},
	{Name: AxisCq, Kind: KindNumber, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_CQ},
}

// The table describes one vocabulary, so a duplicate name or an axis with nothing to carry its
// value is an Entwicklungsfehler rather than a condition to survive.
func init() {
	seen := make(map[string]bool, len(axes))
	for _, a := range axes {
		assert.Assert(a.Name != "", "an axis carries an identifier")
		assert.Assert(!seen[a.Name], "an axis is declared once", a.Name)
		assert.Assert(a.Kind == KindText || a.Kind == KindNumber, "an axis states what it reads as", a.Name)
		assert.Assert(a.Arg != screensharev1.TextArgName_TEXT_ARG_NAME_UNSPECIFIED,
			"an axis names the argument a reason carries it under", a.Name)
		seen[a.Name] = true
	}
}

// Declared returns the axis this name declares, and false for a name the vocabulary does not carry.
func Declared(name string) (Axis, bool) {
	for _, a := range axes {
		if a.Name == name {
			return a, true
		}
	}
	return Axis{}, false
}

// Axes is the vocabulary in domain order, for a caller assembling the facts or reporting on what
// may be matched.
func Axes() []Axis {
	return axes
}

// Value is one axis's reading.
// Which half carries it is the axis's Kind, and reading the other half is asserted against rather
// than answered with a zero.
type Value struct {
	text string
	num  int
	kind Kind
}

// TextValue is the reading of a text axis.
func TextValue(s string) Value {
	return Value{text: s, kind: KindText}
}

// NumberValue is the reading of a numeric axis.
func NumberValue(n int) Value {
	return Value{num: n, kind: KindNumber}
}

// Text is the identifier this value carries.
func (v Value) Text() string {
	assert.Assert(v.kind == KindText, "an identifier is read off a text axis", int(v.kind))
	return v.text
}

// Number is the quantity this value carries.
func (v Value) Number() int {
	assert.Assert(v.kind == KindNumber, "a quantity is read off a numeric axis", int(v.kind))
	return v.num
}

// Facts is one configuration as the axes read it.
//
// It is assembled by the caller that holds the settings and the machine's answers,
// which is what keeps this package free of both.
// Every axis a registered rule names has to be present: an absent fact would make the rule bind
// nothing, which reads on screen as a combination that is allowed rather than as a question nobody
// answered.
type Facts map[string]Value
