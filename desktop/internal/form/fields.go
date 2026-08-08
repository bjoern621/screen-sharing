package form

import (
	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

// fieldTable is one row per settings field: what it edits, how it is drawn, what its
// number means, and how its value, options and range are read out of a draft.
//
// It is a table and not a switch for the reason docs/development-principles.md gives:
// these are static facts, the same on every resolve, and a rule spread through logic is
// one that goes stale where a table does not. The order is the order a shell renders in,
// grouped as groups.go groups them, so reading the table top to bottom is reading the
// screen top to bottom.
//
// Every row states its availability nowhere. A field's fixed facts do not change between
// resolves and its availability changes on every one, which is why the two are two tables
// (form.go) and why nothing here decides what is greyed.
//
// What a row does NOT state is its name and the paragraph behind it. A control's label
// and its help text are the surface's, looked up by the key below
// (api/proto/screenshare/v1/text.proto): they are written for a column width, a tone and
// a reading level this package cannot see, and a form that shipped them would be
// deciding all three for every shell at once.

// The ends a numeric control is offered between.
//
// None of them is a limit anything enforces: the encoder's own ceilings come off the
// capability table below and everything else here is a sane end for a slider and a guard
// against a typed digit too many. A value outside a range still resolves - Field.value is
// whatever the settings hold, and a draft that carries more than the encoder takes is
// refused by capabilities.Validate with the encoder's own sentence, which is a better
// message than a control that silently clamps.
const (
	fieldPortFloor   = 1
	fieldPortCeiling = 65535

	// fieldFpsCeiling is above every panel this app has met. The frame rate is not bound
	// to a monitor's refresh: capturing above it is legal and yields duplicate frames,
	// which is a diagnostic and not a refusal.
	fieldFpsCeiling = 1000

	// fieldRateCeiling is the end of both megabit fields. A codec that takes less says so
	// through capabilities.BitrateLimitOn, which fieldBitrateBounds reads.
	fieldRateCeiling = 10000
	// fieldUplinkCeiling covers a 100 Gbit/s line, which is past any uplink this is
	// weighed against.
	fieldUplinkCeiling = 100000

	fieldVbvCeiling    = 10000
	fieldGopCeiling    = 6000
	fieldBframeCeiling = 16

	// The latency windows are swept rather than typed, so they carry a step. The floor is
	// above zero because settings.Load reads a non-positive latency as unset and replaces
	// it with the default, so a zero would not survive being written.
	fieldLatencyFloor   = 20
	fieldLatencyCeiling = 8000
	fieldLatencyStep    = 10

	// fieldAnchorCq is the 51-point scale every quantizer figure stated
	// codec-independently counts on: the H.26x encoders' own. It is the range a codec
	// declaring no scale on this engine is offered within, since pricing an unknown scale
	// on some other one would clamp a 255-point target to a fifth of its range.
	fieldAnchorCq = 51
)

var fieldTable = []field{
	// The stream: the one field that depends on nothing else, and the only setting other
	// people see. Where it is carried is the relay's group, at the far end of the table.
	{
		key:     KeyName,
		group:   GroupStream,
		control: screensharev1.ControlKind_CONTROL_KIND_TEXT,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return stringValue(s.Name) },
	},

	// The capture: what is grabbed, how much of it, and how it reaches the encoder.
	{
		key:     KeyCapture,
		group:   GroupSource,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return stringValue(s.Capture) },
		options: optionCaptures,
	},
	{
		key:     KeyMonitor,
		group:   GroupSource,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return number(s.Monitor) },
		options: optionMonitors,
	},
	{
		// The row the contract led. It was declared here with no settings field behind it,
		// disabled with the reason that the pipeline had no scaling stage; the stage landed
		// and the row is now an ordinary one. The ladder it offers did not change in the
		// move, which was the claim the contract-first order was making.
		//
		// What can still refuse a scaled value is the frame path rather than the field: an
		// encoder reading captured surfaces with no filter between has nothing on it that
		// resizes, and availability greys the scaled entries there (availability.go).
		key:     KeyOutputResolution,
		group:   GroupSource,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return stringValue(s.OutputResolution) },
		options: optionOutputResolutions,
	},
	{
		// The one control that is both. The rate is a number the whole range accepts, and
		// it is also a short list of answers - a film's, a game's, each panel's - that a
		// reader should not have to remember to type, so the row carries a ladder and the
		// ends it may be typed past.
		key:     KeyFps,
		group:   GroupSource,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER_SELECT,
		unit:    screensharev1.Unit_UNIT_FRAMES_PER_SECOND,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return number(s.Fps) },
		options: optionFpsPresets,
		bounds:  fieldFpsBounds,
	},
	{
		key:     KeyCaptureMemory,
		group:   GroupSource,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return stringValue(s.CaptureMemory) },
		options: optionCaptureMemories,
	},
	{
		// The hidden case of docs/field-availability.md: a backend implementation knob whose
		// help describes a mechanism a user on any other capture backend has no reason to
		// read. It is a row here all the same, because hiding is availability's verdict and
		// a control the table never named is one no verdict can reach.
		key:     KeyDrmMap,
		group:   GroupSource,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return stringValue(s.DrmMap) },
		options: optionDrmMaps,
	},

	// The encode: which encoder, in which format, and how it spends bits over time.
	{
		key:     KeyCodec,
		group:   GroupQuality,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return stringValue(s.Codec) },
		options: optionCodecs,
	},
	{
		key:     KeyChroma,
		group:   GroupQuality,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return stringValue(s.Chroma) },
		options: optionChromas,
	},
	{
		key:     KeyColorRange,
		group:   GroupQuality,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return stringValue(s.ColorRange) },
		options: optionColorRanges,
	},
	{
		key:     KeyEncPreset,
		group:   GroupQuality,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return stringValue(s.EncPreset) },
		options: optionEncPresets,
	},
	{
		// The one radio: five choices carrying a paragraph each, which is what
		// CONTROL_KIND_RADIO is reserved for. Every other closed set here is a select.
		key:     KeyMode,
		group:   GroupQuality,
		control: screensharev1.ControlKind_CONTROL_KIND_RADIO,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return stringValue(s.Mode) },
		options: optionModes,
	},
	{
		key:     KeyCq,
		group:   GroupQuality,
		control: screensharev1.ControlKind_CONTROL_KIND_SLIDER,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return number(s.Cq) },
		bounds:  fieldCqBounds,
	},
	{
		key:     KeyBitrateM,
		group:   GroupQuality,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		unit:    screensharev1.Unit_UNIT_MEGABITS_PER_SECOND,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return number(s.BitrateM) },
		bounds:  fieldBitrateBounds,
	},
	{
		key:     KeyMaxrateM,
		group:   GroupQuality,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		unit:    screensharev1.Unit_UNIT_MEGABITS_PER_SECOND,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return number(s.MaxrateM) },
		bounds:  fieldMaxrateBounds,
	},
	{
		key:     KeyVbvMs,
		group:   GroupQuality,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		unit:    screensharev1.Unit_UNIT_MILLISECONDS,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return number(s.VbvMs) },
		bounds:  fieldVbvBounds,
	},
	{
		key:     KeyGop,
		group:   GroupQuality,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		unit:    screensharev1.Unit_UNIT_FRAMES,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return number(s.Gop) },
		bounds:  fieldGopBounds,
	},
	{
		key:     KeyBframes,
		group:   GroupQuality,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		unit:    screensharev1.Unit_UNIT_FRAMES,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return number(s.Bframes) },
		bounds:  fieldBframeBounds,
	},

	// The second track: where it comes from, and what codes it.
	{
		key:     KeyAudio,
		group:   GroupAudio,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return stringValue(s.Audio) },
		options: optionAudioSources,
	},
	{
		key:     KeyAudioCodec,
		group:   GroupAudio,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return stringValue(s.AudioCodec) },
		options: optionAudioCodecs,
	},

	// The publish leg: how the stream leaves this machine, and what the line can carry.
	{
		key:     KeyTransport,
		group:   GroupTransport,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return stringValue(s.Transport) },
		options: optionPublishTransports,
	},
	{
		key:     KeySrtPublishLatencyMs,
		group:   GroupTransport,
		control: screensharev1.ControlKind_CONTROL_KIND_SLIDER,
		unit:    screensharev1.Unit_UNIT_MILLISECONDS,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return number(s.SrtPublishLatencyMs) },
		bounds:  fieldLatencyBounds,
	},
	{
		key:     KeyRtspPublishProtocol,
		group:   GroupTransport,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return stringValue(s.RtspPublishProtocol) },
		options: optionRtspProtocols,
	},
	{
		key:     KeyUplinkMbps,
		group:   GroupTransport,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		unit:    screensharev1.Unit_UNIT_MEGABITS_PER_SECOND,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return number(s.UplinkMbps) },
		bounds:  fieldUplinkBounds,
	},

	// The watch leg: how a stream comes back, once per viewer that can reach it.
	{
		key:     KeyWatchTransport,
		group:   GroupWatch,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return stringValue(s.WatchTransport) },
		options: optionWatchTransports,
	},
	{
		key:     KeySrtWatchLatencyMs,
		group:   GroupWatch,
		control: screensharev1.ControlKind_CONTROL_KIND_SLIDER,
		unit:    screensharev1.Unit_UNIT_MILLISECONDS,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return number(s.SrtWatchLatencyMs) },
		bounds:  fieldLatencyBounds,
	},
	{
		key:     KeyRtspWatchProtocol,
		group:   GroupWatch,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return stringValue(s.RtspWatchProtocol) },
		options: optionRtspProtocols,
	},

	// The relay: which machine carries the stream, then one number per listener it serves
	// it on. The address leads because the ports are that machine's - a port answered
	// against no host is a number about nothing - and which of them is read follows from a
	// leg chosen further up rather than from anything here.
	{
		key:     KeyRelayHost,
		group:   GroupRelay,
		control: screensharev1.ControlKind_CONTROL_KIND_TEXT,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return stringValue(s.RelayHost) },
	},
	{
		key:     KeyRelayPort,
		group:   GroupRelay,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return number(s.RelayPort) },
		bounds:  fieldPortBounds,
	},
	{
		key:     KeyRtspPort,
		group:   GroupRelay,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return number(s.RtspPort) },
		bounds:  fieldPortBounds,
	},
	{
		key:     KeyWebrtcPort,
		group:   GroupRelay,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return number(s.WebrtcPort) },
		bounds:  fieldPortBounds,
	},
	{
		key:     KeyRtmpPort,
		group:   GroupRelay,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return number(s.RtmpPort) },
		bounds:  fieldPortBounds,
	},
	{
		key:     KeyHlsPort,
		group:   GroupRelay,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return number(s.HlsPort) },
		bounds:  fieldPortBounds,
	},
	{
		key:     KeyMoqPort,
		group:   GroupRelay,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return number(s.MoqPort) },
		bounds:  fieldPortBounds,
	},
	{
		key:     KeyAPIPort,
		group:   GroupRelay,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		value:   func(s settings.Stream) *screensharev1.FieldValue { return number(s.ApiPort) },
		bounds:  fieldPortBounds,
	},
}

// The range builders. A row states one where its control takes a range, which is every
// number and every slider and nothing else.

// fieldPortBounds is every port a listener can bind.
func fieldPortBounds(Deps, settings.Stream) *screensharev1.NumericRange {
	return bounded(fieldPortFloor, fieldPortCeiling, 1)
}

// fieldFpsBounds is one frame per second up to past any panel. It is deliberately not
// bound to the monitors' refresh rates: capturing above them is legal and produces
// duplicate frames, which the form says as a diagnostic rather than as a wall.
func fieldFpsBounds(Deps, settings.Stream) *screensharev1.NumericRange {
	return bounded(1, fieldFpsCeiling, 1)
}

// fieldCqBounds is the quantizer scale the selected codec counts on with the engine
// behind the selected capture backend.
//
// It moves with the selection because the scales differ: the H.26x encoders reach 51,
// libvpx and the software AV1 ones 63, and an encoder taking a raw quantizer index counts
// to 127 or 255. A codec the table declares no scale for on this engine is offered the
// 51-point anchor, which is the scale every codec-independent figure in this app is
// stated on.
func fieldCqBounds(_ Deps, s settings.Stream) *screensharev1.NumericRange {
	scale := fieldAnchorCq
	if c, ok := capabilities.Get(s.Codec); ok {
		if declared := c.CqMaxOn(optionEngineOf(s)); declared > 0 {
			scale = declared
		}
	}
	return bounded(0, scale, 1)
}

// fieldBitrateBounds narrows to the codec's own ceiling where it declares one. An encoder
// with a ceiling refuses the encode rather than clamping, so a target above it is a
// publish that dies at launch and the range is where that is cheapest to say.
func fieldBitrateBounds(_ Deps, s settings.Stream) *screensharev1.NumericRange {
	ceiling := fieldRateCeiling
	if c, ok := capabilities.Get(s.Codec); ok {
		if limit := c.BitrateLimitOn(optionEngineOf(s)); limit > 0 {
			ceiling = limit
		}
	}
	return bounded(0, ceiling, 1)
}

// fieldMaxrateBounds takes no codec ceiling. The capability table's limit is a ceiling on
// the target the encoder is given, not on the burst the user allows above it, so applying
// it here would refuse a headroom the encoder never sees as a target.
func fieldMaxrateBounds(Deps, settings.Stream) *screensharev1.NumericRange {
	return bounded(0, fieldRateCeiling, 1)
}

// fieldVbvBounds starts at zero because zero is a value and not an absence: it leaves the
// encoder's own buffer default standing.
func fieldVbvBounds(Deps, settings.Stream) *screensharev1.NumericRange {
	return bounded(0, fieldVbvCeiling, 1)
}

// fieldGopBounds starts at zero for the same reason: zero selects auto, which every
// builder reads as twice the frame rate.
func fieldGopBounds(Deps, settings.Stream) *screensharev1.NumericRange {
	return bounded(0, fieldGopCeiling, 1)
}

// fieldBframeBounds ends where the codecs do: no encoder here takes a longer reorder
// chain, and a live stream pays for every frame of it in delay.
func fieldBframeBounds(Deps, settings.Stream) *screensharev1.NumericRange {
	return bounded(0, fieldBframeCeiling, 1)
}

// fieldLatencyBounds sizes every retransmit window and jitter buffer, on either leg. One
// builder for all three because they are one quantity - how long a receiver holds packets
// before giving up on the ones that have not arrived - and a range that differed per leg
// would be a claim about the legs the transports never made.
func fieldLatencyBounds(Deps, settings.Stream) *screensharev1.NumericRange {
	return bounded(fieldLatencyFloor, fieldLatencyCeiling, fieldLatencyStep)
}

// fieldUplinkBounds is one megabit up to past any line this is weighed against.
func fieldUplinkBounds(Deps, settings.Stream) *screensharev1.NumericRange {
	return bounded(1, fieldUplinkCeiling, 1)
}
