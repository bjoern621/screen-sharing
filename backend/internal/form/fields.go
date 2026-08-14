package form

import (
	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

// fieldTable is one row per control: what it edits, how it is drawn, what its number means,
// and how its value, options and range are read out of a draft.
//
// A table and not a switch, for the reason docs/development-principles.md gives:
// these are static facts, the same on every resolve.
// The order is the order a shell renders in, grouped as groups.go groups them.
//
// No row states its availability.
// A row's facts are the same on every resolve and availability changes on every one,
// which is why the two are two tables (form.go).
//
// No row states its label or its help text either.
// Both are the surface's, looked up by key (api/proto/screenshare/v1/text.proto),
// and written for a column width, a tone and a reading level this package cannot see.

// A malformed row is an Entwicklungsfehler and fails at load.
//
// The render pass asserts the same facts about the row it is drawing (form.go),
// which catches a broken row only on a draft that reaches it:
// a control the current combination hides is one nothing would have looked at.
// Every row is checked here whether or not anything draws it.
func init() {
	seen := make(map[string]bool, len(fieldTable))
	for i := range fieldTable {
		f := &fieldTable[i]

		assert.Assert(f.key != "", "a field is addressed by a key")
		assert.Assert(f.group != "", "a field is drawn under a heading", f.key)
		assert.Assert(!seen[f.key], "a field is declared once", f.key)
		seen[f.key] = true

		// The draft or one entry of the audio source list, never both and never neither:
		// a control with no value is one a shell cannot render.
		assert.Assert((f.value != nil) != (f.itemValue != nil),
			"a field reads its value from exactly one place", f.key, f.repeat)
		assert.Assert(f.repeat == (f.itemValue != nil),
			"a repeated field is the one that reads an entry", f.key, f.repeat)
	}
}

// The ends a numeric control is offered between.
//
// None of them is a limit anything enforces: an encoder's own ceiling arrives as a rule,
// and everything here is a sane end for a slider and a guard against a typed digit too many.
// A value outside a range still resolves, Field.value being whatever the settings hold,
// and a draft above what the encoder takes is refused by capabilities.Validate,
// in the encoder's own words rather than clamped in silence.
const (
	fieldPortFloor   = 1
	fieldPortCeiling = 65535

	// fieldFpsCeiling sits past any panel's refresh.
	// The frame rate is not bound to a monitor's:
	// capturing above it is legal and yields duplicate frames,
	// which the form states as a diagnostic rather than as a wall.
	fieldFpsCeiling = 1000

	// fieldRateCeiling is the end of both megabit fields.
	// A codec that takes less says so as a rule, which narrows it in the modes that send a target.
	fieldRateCeiling = 10000
	// fieldUplinkCeiling is a 100 Gbit/s line, past any uplink a prediction is weighed against.
	fieldUplinkCeiling = 100000

	fieldVbvCeiling    = 10000
	fieldGopCeiling    = 6000
	fieldBframeCeiling = 16

	// The latency windows, in ms.
	// They are swept rather than typed, so they carry a step.
	// The floor is above zero because settings.Load reads a non-positive latency as unset,
	// and replaces it with the default.
	fieldLatencyFloor   = 20
	fieldLatencyCeiling = 8000
	fieldLatencyStep    = 10

	// fieldGainStep is in percent, which is the unit the gain setting counts in.
	fieldGainStep = 5
)

var fieldTable = []field{
	// The stream: the one field that depends on nothing else, and the only setting other people see.
	// Which relay carries it is the group at the far end of the table.
	{
		key:     KeyName,
		group:   GroupStream,
		control: screensharev1.ControlKind_CONTROL_KIND_TEXT,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return stringValue(s.Publish.Name) },
	},

	// The capture: what is grabbed, how much of it, and how the frames reach the encoder.
	{
		key:     KeyCapture,
		group:   GroupSource,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return stringValue(s.Publish.Capture) },
		options: optionCaptures,
	},
	{
		key:     KeyMonitor,
		group:   GroupSource,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return number(s.Publish.Monitor) },
		options: optionMonitors,
	},
	{
		// What refuses a scaled value is the frame path rather than this field:
		// an encoder reading captured surfaces with no filter between has nothing on it that resizes,
		// and availability greys the scaled entries there (availability.go).
		key:     KeyOutputResolution,
		group:   GroupSource,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return stringValue(s.Publish.OutputResolution) },
		options: optionOutputResolutions,
	},
	{
		// Both controls at once: the rate takes any number in the range,
		// and the answers worth remembering (a film's, a game's, a panel's) sit beside it as a ladder,
		// so the row carries options and bounds together.
		key:     KeyFps,
		group:   GroupSource,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER_SELECT,
		unit:    screensharev1.Unit_UNIT_FRAMES_PER_SECOND,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return number(s.Publish.Fps) },
		options: optionFpsPresets,
		bounds:  fieldFpsBounds,
	},
	{
		key:     KeyCaptureMemory,
		group:   GroupSource,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return stringValue(s.Publish.CaptureMemory) },
		options: optionCaptureMemories,
	},
	{
		// The pointer sits with the capture rather than with the encode,
		// because what it can do follows from the capture backend alone:
		// every value is offered everywhere and greyed per backend with what that backend is missing.
		key:     KeyCursor,
		group:   GroupSource,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return stringValue(s.Publish.Cursor) },
		options: optionCursors,
	},
	{
		// The hidden treatment of docs/field-availability.md: a knob of the kmsgrab scanout path,
		// whose help describes a mechanism no user on another capture backend has reason to read.
		// It is a row here all the same, hiding being availability's verdict,
		// and a control this table never named being one no verdict can reach.
		key:     KeyDrmMap,
		group:   GroupSource,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return stringValue(s.Publish.DrmMap) },
		options: optionDrmMaps,
	},

	// The encode: which encoder, which format, and how the bits are spent over time.
	{
		key:     KeyCodec,
		group:   GroupQuality,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return stringValue(s.Publish.Codec) },
		options: optionCodecs,
	},
	{
		key:     KeyChroma,
		group:   GroupQuality,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return stringValue(s.Publish.Chroma) },
		options: optionChromas,
	},
	{
		key:     KeyColorRange,
		group:   GroupQuality,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return stringValue(s.Publish.ColorRange) },
		options: optionColorRanges,
	},
	{
		key:     KeyEffort,
		group:   GroupQuality,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return stringValue(s.Publish.Effort) },
		options: optionEfforts,
	},
	{
		// Beside the effort step, the two being one decision read twice: how hard the encoder works,
		// and what it works towards.
		// A ladder the codec declares none of greys the control naming that codec,
		// which is the answer the step gets too.
		key:     KeyTune,
		group:   GroupQuality,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return stringValue(s.Publish.Tune) },
		options: optionTunes,
	},
	{
		// The one radio, CONTROL_KIND_RADIO being for a closed set whose entries carry a paragraph each.
		// Every other closed set here is a select.
		key:     KeyMode,
		group:   GroupQuality,
		control: screensharev1.ControlKind_CONTROL_KIND_RADIO,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return stringValue(s.Publish.Mode) },
		options: optionModes,
	},
	{
		key:     KeyCq,
		group:   GroupQuality,
		control: screensharev1.ControlKind_CONTROL_KIND_SLIDER,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return number(s.Publish.Cq) },
		bounds:  fieldCqBounds,
	},
	{
		key:     KeyBitrateM,
		group:   GroupQuality,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		unit:    screensharev1.Unit_UNIT_MEGABITS_PER_SECOND,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return number(s.Publish.BitrateM) },
		bounds:  fieldBitrateBounds,
	},
	{
		key:     KeyMaxrateM,
		group:   GroupQuality,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		unit:    screensharev1.Unit_UNIT_MEGABITS_PER_SECOND,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return number(s.Publish.MaxrateM) },
		bounds:  fieldMaxrateBounds,
	},
	{
		key:     KeyVbvMs,
		group:   GroupQuality,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		unit:    screensharev1.Unit_UNIT_MILLISECONDS,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return number(s.Publish.VbvMs) },
		bounds:  fieldVbvBounds,
	},
	{
		key:     KeyGop,
		group:   GroupQuality,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		unit:    screensharev1.Unit_UNIT_FRAMES,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return number(s.Publish.Gop) },
		bounds:  fieldGopBounds,
	},
	{
		key:     KeyBframes,
		group:   GroupQuality,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		unit:    screensharev1.Unit_UNIT_FRAMES,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return number(s.Publish.Bframes) },
		bounds:  fieldBframeBounds,
	},

	// The second track: what it is mixed from, and what codes it.
	//
	// The controls of one entry are drawn once per entry,
	// plus once for the row a reader grows the list by.
	// That trailing row makes adding a source an ordinary settings write through an ordinary control:
	// picking a kind on it writes an entry the list did not have,
	// and setting a kind back to none is what takes one off.
	// Neither needs an effect on the contract, and neither lets a shell decide anything.
	{
		key:         KeyAudioSource,
		group:       GroupAudio,
		control:     screensharev1.ControlKind_CONTROL_KIND_SELECT,
		repeat:      true,
		itemValue:   func(a settings.AudioSource) *screensharev1.FieldValue { return stringValue(a.Source) },
		itemOptions: optionAudioKinds,
	},
	{
		key:         KeyAudioSourceDevice,
		group:       GroupAudio,
		control:     screensharev1.ControlKind_CONTROL_KIND_SELECT,
		repeat:      true,
		itemValue:   func(a settings.AudioSource) *screensharev1.FieldValue { return stringValue(a.Device) },
		itemOptions: optionAudioDevices,
	},
	{
		key:       KeyAudioSourceGain,
		group:     GroupAudio,
		control:   screensharev1.ControlKind_CONTROL_KIND_SLIDER,
		unit:      screensharev1.Unit_UNIT_PERCENT,
		repeat:    true,
		itemValue: func(a settings.AudioSource) *screensharev1.FieldValue { return number(a.Gain) },
		bounds:    fieldGainBounds,
	},
	{
		key:       KeyAudioSourceMute,
		group:     GroupAudio,
		control:   screensharev1.ControlKind_CONTROL_KIND_TOGGLE,
		repeat:    true,
		itemValue: func(a settings.AudioSource) *screensharev1.FieldValue { return flag(a.Mute) },
	},
	{
		key:     KeyAudioCodec,
		group:   GroupAudio,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return stringValue(s.Publish.AudioCodec) },
		options: optionAudioCodecs,
	},

	// The publish leg: how the stream leaves this machine, and what the line carries.
	{
		key:     KeyTransport,
		group:   GroupTransport,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return stringValue(s.Publish.Transport) },
		options: optionPublishTransports,
	},
	{
		key:     KeySrtPublishLatencyMs,
		group:   GroupTransport,
		control: screensharev1.ControlKind_CONTROL_KIND_SLIDER,
		unit:    screensharev1.Unit_UNIT_MILLISECONDS,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return number(s.Publish.SrtPublishLatencyMs) },
		bounds:  fieldLatencyBounds,
	},
	{
		key:     KeyRtspPublishProtocol,
		group:   GroupTransport,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return stringValue(s.Publish.RtspPublishProtocol) },
		options: optionRtspProtocols,
	},
	{
		key:     KeyUplinkMbps,
		group:   GroupTransport,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		unit:    screensharev1.Unit_UNIT_MEGABITS_PER_SECOND,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return number(s.Publish.UplinkMbps) },
		bounds:  fieldUplinkBounds,
	},

	// The watch leg: how a stream comes back, per receiver that can reach it.
	{
		key:     KeyPlayerWatchTransport,
		group:   GroupWatch,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return stringValue(s.Viewer.PlayerWatchTransport) },
		options: optionWatchTransports,
	},
	{
		key:     KeySrtWatchLatencyMs,
		group:   GroupWatch,
		control: screensharev1.ControlKind_CONTROL_KIND_SLIDER,
		unit:    screensharev1.Unit_UNIT_MILLISECONDS,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return number(s.Viewer.SrtWatchLatencyMs) },
		bounds:  fieldLatencyBounds,
	},
	{
		key:     KeyRtspWatchProtocol,
		group:   GroupWatch,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return stringValue(s.Viewer.RtspWatchProtocol) },
		options: optionRtspProtocols,
	},
	// The tile receiver's own controls.
	// Its leg is a field of its own because a receive pipeline reaches protocols no player URL
	// expresses, and the rest are knobs of a receiving pipeline alone:
	// an external player buffers by reorder queue rather than by time,
	// and none of them builds a chain of elements at all.
	{
		key:     KeyTileWatchTransport,
		group:   GroupWatch,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return stringValue(s.Viewer.TileWatchTransport) },
		options: optionTileWatchTransports,
	},
	{
		key:     KeyRtspWatchLatencyMs,
		group:   GroupWatch,
		control: screensharev1.ControlKind_CONTROL_KIND_SLIDER,
		unit:    screensharev1.Unit_UNIT_MILLISECONDS,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return number(s.Viewer.RtspWatchLatencyMs) },
		bounds:  fieldLatencyBounds,
	},
	{
		key:     KeyRenderChain,
		group:   GroupWatch,
		control: screensharev1.ControlKind_CONTROL_KIND_SELECT,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return stringValue(s.Viewer.RenderChain) },
		options: optionRenderChains,
	},

	// The relay: which machine carries the stream, then one port per listener it serves on.
	// The address leads the ports because they are that machine's,
	// a port answered against no host being a number about nothing,
	// and which port is read follows from the leg chosen further up.
	{
		// The group is where every stream of this machine lives on the relay.
		//
		// Text and not a list, the key being a secret somebody was handed: the key service draws it,
		// whatever distributes it hands it over, and groups are not enumerable by design,
		// possession of the key being the whole of membership
		// (docs/plan.md, "Groups, auth and encryption").
		key:     KeyGroupKey,
		group:   GroupRelay,
		control: screensharev1.ControlKind_CONTROL_KIND_TEXT,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return stringValue(s.Relay.GroupKey) },
	},
	{
		// Whether the relay's HTTP legs are reached through a TLS proxy,
		// which is one fact about the deployment rather than one per listener:
		// the proxy terminates for the relay and the group service alike,
		// under one name on the standard port.
		//
		// A toggle and not a scheme dropdown, there being two deployments and no third:
		// a relay on the internet has a proxy in front of every HTTP leg,
		// and one on a trusted network has none.
		key:     KeyRelayTls,
		group:   GroupRelay,
		control: screensharev1.ControlKind_CONTROL_KIND_TOGGLE,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return flag(s.Relay.Tls()) },
	},
	{
		// The passphrase the relay keys its SRT listener with, that leg being the one no proxy wraps:
		// UDP with no TLS, so what protects the packets is a value both ends hold.
		// Empty is a relay that takes none, which is every relay on a trusted network.
		key:     KeySrtPassphrase,
		group:   GroupRelay,
		control: screensharev1.ControlKind_CONTROL_KIND_TEXT,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return stringValue(s.Relay.SrtPassphrase) },
	},
	{
		key:     KeyRelayHost,
		group:   GroupRelay,
		control: screensharev1.ControlKind_CONTROL_KIND_TEXT,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return stringValue(s.Relay.Host) },
	},
	{
		key:     KeySrtPort,
		group:   GroupRelay,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return number(s.Relay.SrtPort) },
		bounds:  fieldPortBounds,
	},
	{
		key:     KeyRtspPort,
		group:   GroupRelay,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return number(s.Relay.RtspPort) },
		bounds:  fieldPortBounds,
	},
	{
		key:     KeyWebrtcPort,
		group:   GroupRelay,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return number(s.Relay.WebrtcPort) },
		bounds:  fieldPortBounds,
	},
	{
		key:     KeyRtmpPort,
		group:   GroupRelay,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return number(s.Relay.RtmpPort) },
		bounds:  fieldPortBounds,
	},
	{
		key:     KeyHlsPort,
		group:   GroupRelay,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return number(s.Relay.HlsPort) },
		bounds:  fieldPortBounds,
	},
	{
		key:     KeyAPIPort,
		group:   GroupRelay,
		control: screensharev1.ControlKind_CONTROL_KIND_NUMBER,
		value:   func(s settings.Settings) *screensharev1.FieldValue { return number(s.Relay.ApiPort) },
		bounds:  fieldPortBounds,
	},
}

// The range builders.
// A row carries one where its control takes a range, which is every number and every slider.

// fieldPortBounds spans every port a listener can bind.
func fieldPortBounds(Deps, settings.Settings) *screensharev1.NumericRange {
	return bounded(fieldPortFloor, fieldPortCeiling, 1)
}

func fieldFpsBounds(Deps, settings.Settings) *screensharev1.NumericRange {
	return bounded(1, fieldFpsCeiling, 1)
}

// fieldCqBounds is the quantizer scale the selected codec counts on,
// under the engine behind the selected capture backend.
//
// The control is offered within the widest scale the table declares and narrowed by the rules,
// so the number a slider stops at and the number a publish refuses above are one answer.
// A codec the table declares no scale for narrows nothing and keeps the widest,
// which is what declaring none means: an unwired family counts on whatever its builder sets,
// and pricing it on another encoder's scale would clamp a target to a fifth of its range.
func fieldCqBounds(d Deps, s settings.Settings) *screensharev1.NumericRange {
	low, high := verdictsOf(d, s).Bounds(KeyCq, 0, capabilities.WidestCqScale())
	return bounded(low, high, 1)
}

// fieldBitrateBounds narrows to the codec's own ceiling where a rule states one.
// An encoder with a ceiling refuses the encode rather than clamping,
// so a target above it is a publish that dies at launch,
// and the range is where that is cheapest to say.
//
// The rule binds in the modes that send the encoder a target and nowhere else.
func fieldBitrateBounds(d Deps, s settings.Settings) *screensharev1.NumericRange {
	low, high := verdictsOf(d, s).Bounds(KeyBitrateM, 0, fieldRateCeiling)
	return bounded(low, high, 1)
}

// fieldMaxrateBounds takes no codec ceiling.
// A codec's limit bounds the target the encoder is given and not the burst allowed above it,
// so narrowing here would refuse headroom the encoder never sees as a target.
func fieldMaxrateBounds(Deps, settings.Settings) *screensharev1.NumericRange {
	return bounded(0, fieldRateCeiling, 1)
}

// fieldVbvBounds starts at zero, which is a value and not an absence:
// it leaves the encoder's own buffer default standing.
func fieldVbvBounds(Deps, settings.Settings) *screensharev1.NumericRange {
	return bounded(0, fieldVbvCeiling, 1)
}

// fieldGopBounds starts at zero for the same reason:
// zero selects auto, which every builder reads as twice the frame rate.
func fieldGopBounds(Deps, settings.Settings) *screensharev1.NumericRange {
	return bounded(0, fieldGopCeiling, 1)
}

// fieldBframeBounds ends where the codecs do: no encoder here takes a longer reorder chain,
// and a live stream pays for every frame of one in delay.
func fieldBframeBounds(Deps, settings.Settings) *screensharev1.NumericRange {
	return bounded(0, fieldBframeCeiling, 1)
}

// fieldLatencyBounds sizes every retransmit window and jitter buffer, on either leg.
// One builder for all of them, because they are one quantity:
// how long a receiver holds packets before giving up on the ones that have not arrived.
// A range that differed per leg would be a claim about the legs the transports never made.
func fieldLatencyBounds(Deps, settings.Settings) *screensharev1.NumericRange {
	return bounded(fieldLatencyFloor, fieldLatencyCeiling, fieldLatencyStep)
}

// fieldUplinkBounds runs from one megabit up past any line a prediction is weighed against.
func fieldUplinkBounds(Deps, settings.Settings) *screensharev1.NumericRange {
	return bounded(1, fieldUplinkCeiling, 1)
}

// fieldGainBounds runs from silence up to the amplification a quiet microphone needs.
//
// The ceiling is above unity because a source that needs turning up is the case a gain exists for,
// and it is a ceiling because an unbounded multiplier clips every other source out of the mix.
// The step is coarse enough that a swept control lands on round figures.
func fieldGainBounds(Deps, settings.Settings) *screensharev1.NumericRange {
	return bounded(0, settings.GainMax, fieldGainStep)
}
