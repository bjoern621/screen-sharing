package form

import (
	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/text"
)

// The statement vocabulary this package builds its answers out of.
//
// Every reason, note and diagnostic below is one code and the identifiers it is about,
// never a sentence (api/proto/screenshare/v1/text.proto). These wrappers exist so a rule
// reads as the fact it is stating - say(codecCannotEncodeChroma, argCodec(...),
// argChroma(...)) - rather than as three lines of enum spelling, and so that an argument
// is named once here instead of at each of the forty-odd sites that fill it.
//
// The naming is deliberate on both halves. A code constant is named for what is true,
// not for the control it greys: the same fact greys a pixel format on one screen and
// explains a diagnostic on another, and a name taken from one of the two would read as a
// lie on the other.

// say is one statement. It is text.Of under this package's own name, so a rule states a
// fact in one call and nothing here has to spell the constructor's package.
func say(code screensharev1.TextCode, args ...*screensharev1.TextArg) *screensharev1.Text {
	return text.Of(code, args...)
}

// The codes, named for the fact each states.
const (
	captureWrongOS         = screensharev1.TextCode_TEXT_CODE_CAPTURE_WRONG_OS
	captureTakesNoMonitor  = screensharev1.TextCode_TEXT_CODE_CAPTURE_TAKES_NO_MONITOR
	monitorNotEnumerated   = screensharev1.TextCode_TEXT_CODE_MONITOR_NOT_ENUMERATED
	scaledFromSource       = screensharev1.TextCode_TEXT_CODE_SCALED_FROM_SOURCE
	engineToolingMissing   = screensharev1.TextCode_TEXT_CODE_ENGINE_TOOLING_MISSING
	engineHasNoPublishSink = screensharev1.TextCode_TEXT_CODE_ENGINE_HAS_NO_PUBLISH_SINK
	engineNotProbed        = screensharev1.TextCode_TEXT_CODE_ENGINE_NOT_PROBED
	probeNoDevice          = screensharev1.TextCode_TEXT_CODE_PROBE_NO_DEVICE
	probeNoBuild           = screensharev1.TextCode_TEXT_CODE_PROBE_NO_BUILD
	probeFailed            = screensharev1.TextCode_TEXT_CODE_PROBE_FAILED
	codecNotImplemented    = screensharev1.TextCode_TEXT_CODE_CODEC_NOT_IMPLEMENTED

	transportCarriesNoCodec   = screensharev1.TextCode_TEXT_CODE_TRANSPORT_CARRIES_NO_CODEC
	legCarriesNoAudioCodec    = screensharev1.TextCode_TEXT_CODE_LEG_CARRIES_NO_AUDIO_CODEC
	engineHasNoAudioEncoder   = screensharev1.TextCode_TEXT_CODE_ENGINE_HAS_NO_AUDIO_ENCODER
	noViewerReceivesOver      = screensharev1.TextCode_TEXT_CODE_NO_VIEWER_RECEIVES_OVER
	relayServesNoFormatOver   = screensharev1.TextCode_TEXT_CODE_RELAY_SERVES_NO_FORMAT_OVER
	renderChainElementMissing = screensharev1.TextCode_TEXT_CODE_RENDER_CHAIN_ELEMENT_MISSING
	codecCodesNoRGB           = screensharev1.TextCode_TEXT_CODE_CODEC_CODES_NO_RGB
	codecCannotEncodeChroma   = screensharev1.TextCode_TEXT_CODE_CODEC_CANNOT_ENCODE_CHROMA
	rgbIsFullRange            = screensharev1.TextCode_TEXT_CODE_RGB_IS_FULL_RANGE
	decodesInHardware         = screensharev1.TextCode_TEXT_CODE_DECODES_IN_HARDWARE
	decodesOnCPU              = screensharev1.TextCode_TEXT_CODE_DECODES_ON_CPU
	decodesInHardwarePartly   = screensharev1.TextCode_TEXT_CODE_DECODES_IN_HARDWARE_PARTLY
	pairHasNoDeviceMemory     = screensharev1.TextCode_TEXT_CODE_PAIR_HAS_NO_DEVICE_MEMORY
	pairConvertsOnDevice      = screensharev1.TextCode_TEXT_CODE_PAIR_CONVERTS_ON_DEVICE
	pairTradesColour          = screensharev1.TextCode_TEXT_CODE_PAIR_TRADES_COLOUR
	exactColourReach          = screensharev1.TextCode_TEXT_CODE_EXACT_COLOUR_REACH
	devicePathHasNoScaler     = screensharev1.TextCode_TEXT_CODE_DEVICE_PATH_HAS_NO_SCALER
	drmMapUnusedOnDevice      = screensharev1.TextCode_TEXT_CODE_DRM_MAP_UNUSED_ON_DEVICE
	cqOnlyInConstantQuality   = screensharev1.TextCode_TEXT_CODE_CQ_ONLY_IN_CONSTANT_QUALITY
	bitrateNotInMode          = screensharev1.TextCode_TEXT_CODE_BITRATE_NOT_IN_MODE
	maxrateOnlyInConstrained  = screensharev1.TextCode_TEXT_CODE_MAXRATE_ONLY_IN_CONSTRAINED_VBR
	vbvOnlyInBoundedModes     = screensharev1.TextCode_TEXT_CODE_VBV_ONLY_IN_BOUNDED_MODES
	bframesOffInMode          = screensharev1.TextCode_TEXT_CODE_BFRAMES_OFF_IN_MODE
	bframesOnlyOnFamilies     = screensharev1.TextCode_TEXT_CODE_BFRAMES_ONLY_ON_FAMILIES
	codecTakesNoEffortLadder  = screensharev1.TextCode_TEXT_CODE_CODEC_TAKES_NO_EFFORT_LADDER
	effortPinnedByMode        = screensharev1.TextCode_TEXT_CODE_EFFORT_PINNED_BY_MODE
	codecTakesNoTuneLadder    = screensharev1.TextCode_TEXT_CODE_CODEC_TAKES_NO_TUNE_LADDER
	tunePinnedByMode          = screensharev1.TextCode_TEXT_CODE_TUNE_PINNED_BY_MODE
	audioCodecNeedsSource     = screensharev1.TextCode_TEXT_CODE_AUDIO_CODEC_NEEDS_SOURCE
	audioTrackCodedAt         = screensharev1.TextCode_TEXT_CODE_AUDIO_TRACK_CODED_AT
	vaapiCeilingBound         = screensharev1.TextCode_TEXT_CODE_VAAPI_CEILING_BOUND

	publishRefused         = screensharev1.TextCode_TEXT_CODE_PUBLISH_REFUSED
	noUplinkStated         = screensharev1.TextCode_TEXT_CODE_NO_UPLINK_STATED
	uplinkBelowPrediction  = screensharev1.TextCode_TEXT_CODE_UPLINK_BELOW_PREDICTION
	burstAboveUplink       = screensharev1.TextCode_TEXT_CODE_BURST_ABOVE_UPLINK
	fpsAboveRefresh        = screensharev1.TextCode_TEXT_CODE_FPS_ABOVE_REFRESH
	monitorNotPriced       = screensharev1.TextCode_TEXT_CODE_MONITOR_NOT_PRICED
	noPictureToPrice       = screensharev1.TextCode_TEXT_CODE_NO_PICTURE_TO_PRICE
	compressionRatio       = screensharev1.TextCode_TEXT_CODE_COMPRESSION_RATIO
	settingsStoreUnreadble = screensharev1.TextCode_TEXT_CODE_SETTINGS_STORE_UNREADABLE
	presetStoreUnreadable  = screensharev1.TextCode_TEXT_CODE_PRESET_STORE_UNREADABLE
)

// The argument constructors, one per substitution the statements above take. Each names
// the axis its value belongs to, which is what a surface looks the value up in.

func argCapture(v string) *screensharev1.TextArg {
	return text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_CAPTURE, v)
}

func argEngine(v string) *screensharev1.TextArg {
	return text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_ENGINE, v)
}

func argOtherEngine(v string) *screensharev1.TextArg {
	return text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_OTHER_ENGINE, v)
}

func argTransport(v string) *screensharev1.TextArg {
	return text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_TRANSPORT, v)
}

// argValue is the settings value a statement is about, where no argument of its own
// axis fits: a render chain is not a codec, a chroma or a transport, and giving it one
// of those names would say it was.
func argValue(v string) *screensharev1.TextArg {
	return text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_VALUE, v)
}

// argElement is a GStreamer element factory, which nobody picks: it names what this
// machine is missing rather than a choice that was made.
func argElement(v string) *screensharev1.TextArg {
	return text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_ELEMENT, v)
}

func argCodec(v string) *screensharev1.TextArg {
	return text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_CODEC, v)
}

func argFormat(v string) *screensharev1.TextArg {
	return text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_FORMAT, v)
}

func argFamily(v string) *screensharev1.TextArg {
	return text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_FAMILY, v)
}

func argChroma(v string) *screensharev1.TextArg {
	return text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_CHROMA, v)
}

func argMode(v string) *screensharev1.TextArg {
	return text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_MODE, v)
}

func argMemory(v string) *screensharev1.TextArg {
	return text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_MEMORY, v)
}

func argAudioCodec(v string) *screensharev1.TextArg {
	return text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_AUDIO_CODEC, v)
}

func argEffort(v string) *screensharev1.TextArg {
	return text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_EFFORT, v)
}

func argTune(v string) *screensharev1.TextArg {
	return text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_TUNE, v)
}

func argDecodeFamily(v string) *screensharev1.TextArg {
	return text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_DECODE_FAMILY, v)
}

func argDecoder(v string) *screensharev1.TextArg {
	return text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_DECODER, v)
}

func argFamilies(v []string) *screensharev1.TextArg {
	return text.IDs(screensharev1.TextArgName_TEXT_ARG_NAME_FAMILIES, v)
}

func argTransports(v []string) *screensharev1.TextArg {
	return text.IDs(screensharev1.TextArgName_TEXT_ARG_NAME_TRANSPORTS, v)
}

func argAudioCodecs(v []string) *screensharev1.TextArg {
	return text.IDs(screensharev1.TextArgName_TEXT_ARG_NAME_AUDIO_CODECS, v)
}

func argDecodeFamilies(v []string) *screensharev1.TextArg {
	return text.IDs(screensharev1.TextArgName_TEXT_ARG_NAME_DECODE_FAMILIES, v)
}

func argMonitor(v int) *screensharev1.TextArg {
	return text.Num(screensharev1.TextArgName_TEXT_ARG_NAME_MONITOR, int64(v))
}

func argWidth(v int) *screensharev1.TextArg {
	return text.Num(screensharev1.TextArgName_TEXT_ARG_NAME_WIDTH, int64(v))
}

func argHeight(v int) *screensharev1.TextArg {
	return text.Num(screensharev1.TextArgName_TEXT_ARG_NAME_HEIGHT, int64(v))
}

func argFps(v int) *screensharev1.TextArg {
	return text.Num(screensharev1.TextArgName_TEXT_ARG_NAME_FPS, int64(v))
}

func argRefreshHz(v int) *screensharev1.TextArg {
	return text.Num(screensharev1.TextArgName_TEXT_ARG_NAME_REFRESH_HZ, int64(v))
}

func argBitrateMbps(v float64) *screensharev1.TextArg {
	return text.Dec(screensharev1.TextArgName_TEXT_ARG_NAME_BITRATE_MBPS, v)
}

func argMaxrateMbps(v int) *screensharev1.TextArg {
	return text.Num(screensharev1.TextArgName_TEXT_ARG_NAME_MAXRATE_MBPS, int64(v))
}

func argBitrateTarget(v int) *screensharev1.TextArg {
	return text.Num(screensharev1.TextArgName_TEXT_ARG_NAME_BITRATE_MBPS, int64(v))
}

func argUplinkMbps(v int) *screensharev1.TextArg {
	return text.Num(screensharev1.TextArgName_TEXT_ARG_NAME_UPLINK_MBPS, int64(v))
}

func argRawMbps(v float64) *screensharev1.TextArg {
	return text.Dec(screensharev1.TextArgName_TEXT_ARG_NAME_RAW_MBPS, v)
}

func argLowMbps(v float64) *screensharev1.TextArg {
	return text.Dec(screensharev1.TextArgName_TEXT_ARG_NAME_LOW_MBPS, v)
}

func argHighMbps(v float64) *screensharev1.TextArg {
	return text.Dec(screensharev1.TextArgName_TEXT_ARG_NAME_HIGH_MBPS, v)
}

func argRateHz(v int) *screensharev1.TextArg {
	return text.Num(screensharev1.TextArgName_TEXT_ARG_NAME_RATE_HZ, int64(v))
}

func argBitrateKbps(v int) *screensharev1.TextArg {
	return text.Num(screensharev1.TextArgName_TEXT_ARG_NAME_BITRATE_KBPS, int64(v))
}

func argOtherCount(v int) *screensharev1.TextArg {
	return text.Num(screensharev1.TextArgName_TEXT_ARG_NAME_OTHER_COUNT, int64(v))
}

func argCause(v *screensharev1.Text) *screensharev1.TextArg {
	return text.Nested(screensharev1.TextArgName_TEXT_ARG_NAME_CAUSE, v)
}

func argImport(v *screensharev1.Text) *screensharev1.TextArg {
	return text.Nested(screensharev1.TextArgName_TEXT_ARG_NAME_IMPORT, v)
}

func argCost(v *screensharev1.Text) *screensharev1.TextArg {
	return text.Nested(screensharev1.TextArgName_TEXT_ARG_NAME_COST, v)
}

func argReach(v *screensharev1.Text) *screensharev1.TextArg {
	return text.Nested(screensharev1.TextArgName_TEXT_ARG_NAME_REACH, v)
}
