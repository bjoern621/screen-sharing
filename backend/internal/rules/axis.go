package rules

import (
	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// The vocabulary, declared once, so a rule and the form control it greys name a fact by one
// identifier.
//
// An axis that is a settings field carries that field's key.
// The keys are spelled here rather than imported, because every other domain package registers into
// this one and it may import nothing of theirs.
// A form test holds the two lists to each other, so a rename reaching only one of them fails there
// rather than by binding nothing.
//
// The rest are derived facts and carry no field key, there being no control to name: the publish
// engine follows from the capture backend and is no setting of its own.
const (
	AxisFormat     = "publish.format"
	AxisEncoder    = "publish.encoder"
	AxisChroma     = "publish.chroma"
	AxisMode       = "publish.mode"
	AxisColorRange = "publish.color_range"
	AxisTune       = "publish.tune"
	AxisCapture    = "publish.capture"
	AxisTransport  = "publish.publish_transport"
	AxisAudioCodec = "publish.audio_codec"
	AxisMemory     = "publish.capture_memory"
	AxisCursor     = "publish.cursor"
	AxisBitrateM   = "publish.bitrate_mbps"
	AxisCq         = "publish.cq"
	AxisGop        = "publish.gop"

	// FieldAudioGain is a control a rule lands on and nothing matches on: the level one entry of the
	// audio list runs at is a fact rules state liveness about, and no configuration reads it.
	// Spelled here for the reason the axes are, the form and the publish engine both naming it.
	FieldAudioGain = "publish.audio_sources[].gain"

	AxisEngine = "engine"
	AxisFamily = "codec.family"
	// AxisCodec is the row the format and the encoder address between them, as an engine spells it.
	// A derived fact rather than a field: the two controls are what a draft carries and what a
	// greying lands on, and the row is what they resolve to (capabilities.Row).
	AxisCodec   = "codec.name"
	AxisOS      = "platform.os"
	AxisDisplay = "platform.display"

	// The video driver an encode runs through, which a rule binds on where the constraint is the
	// installed driver's rather than the encoder's (internal/gpu).
	// Its release is a figure so a rule can name the version a defect is fixed in and stop binding
	// there.
	AxisGpuDriver        = "gpu.driver"
	AxisGpuModel         = "gpu.model"
	AxisGpuDriverVersion = "gpu.driver_version"
)

// Kind is what an axis reads as: how a match against it is written, and which half of a Value
// carries it.
type Kind int

const (
	// KindText reads as an identifier out of a closed set: "libx264", "yuv420p", "gstreamer".
	KindText Kind = iota + 1
	// KindNumber reads as a quantity, matched by band rather than by membership.
	KindNumber
)

// Axis is one fact a rule may bind under.
type Axis struct {
	// Name is the settings field key where the axis is a field, a bare identifier where it is derived.
	Name string
	Kind Kind
	// Arg is the reason argument this axis rides under, so a statement naming the axis gets the
	// current reading attached without the rule spelling it.
	// Every axis carries one, asserted at load: a fact worth constraining on is worth naming in the
	// refusal.
	Arg screensharev1.TextArgName
}

// axes is the vocabulary, in domain order.
//
// A reason attaches the axes its rule matched on in this order, so one set of facts always yields
// the identifiers in one sequence and a shell reading them meets the subject before the qualifier.
var axes = []Axis{
	{Name: AxisCapture, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_CAPTURE},
	{Name: AxisEngine, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_ENGINE},
	{Name: AxisCodec, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_CODEC},
	{Name: AxisFamily, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_FAMILY},
	{Name: AxisFormat, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_FORMAT},
	{Name: AxisEncoder, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_ENCODER},
	{Name: AxisChroma, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_CHROMA},
	{Name: AxisColorRange, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_COLOR_RANGE},
	{Name: AxisMode, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_MODE},
	{Name: AxisTune, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_TUNE},
	{Name: AxisMemory, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_MEMORY},
	{Name: AxisCursor, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_CURSOR},
	{Name: AxisTransport, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_TRANSPORT},
	{Name: AxisAudioCodec, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_AUDIO_CODEC},
	{Name: AxisOS, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_OS},
	{Name: AxisDisplay, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_DISPLAY},
	{Name: AxisGpuDriver, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_GPU_DRIVER},
	{Name: AxisGpuModel, Kind: KindText, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_GPU_MODEL},
	{Name: AxisBitrateM, Kind: KindNumber, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_BITRATE_MBPS},
	{Name: AxisCq, Kind: KindNumber, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_CQ},
	{Name: AxisGop, Kind: KindNumber, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_GOP_LIMIT_FRAMES},
	{Name: AxisGpuDriverVersion, Kind: KindNumber, Arg: screensharev1.TextArgName_TEXT_ARG_NAME_GPU_DRIVER_VERSION},
}

// One vocabulary, so a duplicate name or an axis with nothing to carry its reading is an
// Entwicklungsfehler and fails at load.
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

// Declared returns the named axis, false for a name outside the vocabulary.
func Declared(name string) (Axis, bool) {
	for _, a := range axes {
		if a.Name == name {
			return a, true
		}
	}
	return Axis{}, false
}

// Axes returns the vocabulary in domain order, for a caller assembling the facts or listing what
// may be matched.
func Axes() []Axis {
	return axes
}

// Value is one axis's reading.
// The axis's Kind decides which half carries it, and reading the other half asserts rather than
// answering a zero.
type Value struct {
	text string
	num  int
	kind Kind
}

func TextValue(s string) Value {
	return Value{text: s, kind: KindText}
}

func NumberValue(n int) Value {
	return Value{num: n, kind: KindNumber}
}

func (v Value) Text() string {
	assert.Assert(v.kind == KindText, "an identifier is read off a text axis", int(v.kind))
	return v.text
}

func (v Value) Number() int {
	assert.Assert(v.kind == KindNumber, "a quantity is read off a numeric axis", int(v.kind))
	return v.num
}

// Facts is one configuration as the axes read it.
//
// The caller holding the settings and the machine's answers assembles it, which is what keeps this
// package free of both.
// Every axis a registered rule names has to be present: an absent one makes the rule bind nothing,
// which reads on screen as a combination the app allows rather than as a question nobody answered.
type Facts map[string]Value
