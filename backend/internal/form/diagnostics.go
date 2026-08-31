package form

import (
	"slices"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
)

// warningAnchors is every field key a diagnostic may anchor on.
// A diagnostic about the combination rather than about one control carries the empty key instead.
//
// An anchor no shell has a widget for renders the diagnostic nowhere and reports nothing,
// so every anchor a rule below writes is held against this list.
var warningAnchors = []string{
	KeyName, KeyRelayHost, KeyRelayTls, KeyGroupKey, KeySrtPort, KeyAPIPort, KeyRtspPort, KeyWebrtcPort,
	KeyRtmpPort, KeyHlsPort, KeyMoqPort,
	KeyTransport, KeyFormat, KeyEncoder, KeyMode, KeyChroma, KeyColorRange, KeyFps, KeyCq,
	KeyBitrateM, KeyMaxrateM, KeyVbvMs, KeyGop, KeyBframes, KeyEffort, KeyTune,
	KeyCapture, KeyAudioSource, KeyAudioSourceDevice, KeyAudioSourceGain, KeyAudioSourceMute,
	KeyAudioCodec, KeyDrmMap, KeyMonitor, KeyCaptureMemory,
	KeySrtPublishLatencyMs, KeySrtWatchLatencyMs,
	KeyRtspPublishProtocol, KeyRtspWatchProtocol,
	KeyUplinkMbps,
	KeyOutputResolution,
}

// diagnostics is everything worth saying about one draft, ranked,
// and derived on every resolve rather than held.
//
// The rank is the backend's judgement and not a shell's.
// An error means the settings cannot be published as they stand, and turns Form.publishable false;
// a warning means the stream runs and something about it is likely to disappoint;
// an info is worth knowing.
// Within a rank the order is the gathering order below, which runs from what refuses the publish
// outward to what only describes it.
//
// No refusal is restated here.
// publish.Command runs every check a publish is refused for, on whichever engine owns the selected
// capture backend, so its failure is the one authority on whether these settings can run.
// A second evaluation here would enable the start button on settings the publish refuses,
// the first time an engine gained a check this package did not know about.
func diagnostics(d Deps, s settings.Settings, est *screensharev1.Estimate) []*screensharev1.Diagnostic {
	out := make([]*screensharev1.Diagnostic, 0, 8)

	// The refusal itself is prose and rides on Summary.command_error;
	// the diagnostic states only that there is one.
	// It is an operational failure rather than a fact about the domain,
	// and the same text crosses as a gRPC status once
	// the publish is attempted (docs/ipc-api.md, "Errors").
	if _, reason := formCommand(s); reason != "" {
		out = append(out, diagnosticFor(screensharev1.Severity_SEVERITY_ERROR, "", say(publishRefused)))
	}
	out = append(out, diagnosticsAboutTheAudience(s)...)
	out = append(out, diagnosticsAboutTheLine(s, est)...)
	out = append(out, diagnosticsAboutTheCapture(d, s)...)
	out = append(out, diagnosticsAboutTheViewer(s)...)
	out = append(out, diagnosticsAboutThePrediction(d, s, est)...)

	// Highest rank first, stable, so equal ranks keep the gathering order above.
	// An unstable sort would move diagnostics under the reader
	// between two keystrokes that changed neither of them.
	slices.SortStableFunc(out, func(a, b *screensharev1.Diagnostic) int {
		return int(b.GetSeverity()) - int(a.GetSeverity())
	})

	for _, w := range out {
		assert.Assert(w.GetSeverity() != screensharev1.Severity_SEVERITY_UNSPECIFIED,
			"a diagnostic is ranked by what ignoring it costs", w.GetText().GetCode())
		assert.IsNotNil(w.GetText(), "a diagnostic says what it is about", w.GetFieldKey())
		assert.Assert(w.GetFieldKey() == "" || slices.Contains(warningAnchors, w.GetFieldKey()),
			"a diagnostic anchors on a declared field or on none", w.GetFieldKey())
	}
	return out
}

func diagnosticFor(severity screensharev1.Severity, fieldKey string, t *screensharev1.Text) *screensharev1.Diagnostic {
	return &screensharev1.Diagnostic{Severity: severity, FieldKey: fieldKey, Text: t}
}

// warningPeaks is where a mode's burst diagnostic anchors: the control that moves the burst.
// Why the rate spreads follows from the mode, which the statement carries,
// so a row holds the anchor alone.
//
// A mode absent from the table has no burst above its prediction,
// the answer estimateSpread gives for it.
var warningPeaks = map[string]string{
	capabilities.ModeCrf:      KeyCq,
	capabilities.ModeLossless: KeyMode,
	capabilities.ModeVbr:      KeyMaxrateM,
}

// diagnosticsAboutTheAudience says who will be able to watch,
// where that is more than the people the user handed a key to.
//
// A warning and never a refusal: publishing without a group is a choice this app carries out,
// and the stream is authenticated and encrypted either way (docs/network-architecture.md).
// What it is not is private, and that is the one thing nothing else on the screen says.
//
// Only on a relay somebody named, an unnamed one having no service to draw a group
// from and no prefix to publish under either.
func diagnosticsAboutTheAudience(s settings.Settings) []*screensharev1.Diagnostic {
	if _, hasService := s.Relay.GroupService(); !hasService || s.Relay.GroupKey != "" {
		return nil
	}
	return []*screensharev1.Diagnostic{
		diagnosticFor(screensharev1.Severity_SEVERITY_WARNING, KeyGroupKey, say(streamIsPublic)),
	}
}

// diagnosticsAboutTheLine holds the prediction against the uplink the user stated, both Mbit/s.
//
// Neither end of this failure announces itself: a stream the line cannot carry is not encoded
// smaller, the send queue grows, the transport drops what it cannot ship,
// and the viewer sees a stall rather than a lower quality.
func diagnosticsAboutTheLine(s settings.Settings, est *screensharev1.Estimate) []*screensharev1.Diagnostic {
	if s.Publish.UplinkMbps <= 0 {
		return []*screensharev1.Diagnostic{
			diagnosticFor(screensharev1.Severity_SEVERITY_INFO, KeyUplinkMbps, say(noUplinkStated)),
		}
	}
	if est == nil {
		return nil
	}

	var out []*screensharev1.Diagnostic
	if est.GetHeadroomMbps() < 0 {
		out = append(out, diagnosticFor(screensharev1.Severity_SEVERITY_WARNING, KeyUplinkMbps,
			say(uplinkBelowPrediction,
				argBitrateMbps(est.GetBitrateMbps()), argUplinkMbps(s.Publish.UplinkMbps))))
		return out
	}

	// A prediction the line carries says nothing about the second the picture moves,
	// and the modes that spread are the ones whose prediction is an average.
	// The anchor is the control that sets the burst rather than the uplink,
	// that being the end the user moves.
	low, high, spreads := estimateSpread(s, est)
	if !spreads || high <= float64(s.Publish.UplinkMbps) {
		return out
	}
	anchor, declared := warningPeaks[s.Publish.Mode]
	assert.Assert(declared, "a mode whose rate spreads states where its burst belongs", s.Publish.Mode)
	out = append(out, diagnosticFor(screensharev1.Severity_SEVERITY_WARNING, anchor,
		say(burstAboveUplink,
			argLowMbps(low), argHighMbps(high), argUplinkMbps(s.Publish.UplinkMbps), argMode(s.Publish.Mode))))
	return out
}

// diagnosticsAboutTheCapture says where this machine gives the pipeline less than the settings ask.
func diagnosticsAboutTheCapture(d Deps, s settings.Settings) []*screensharev1.Diagnostic {
	var out []*screensharev1.Diagnostic

	// Above the panel's own rate the capture has no new picture to hand over,
	// and the encoder codes at the target anyway: the stream carries the frame count the form promises
	// and none of the smoothness it implies.
	// The anchor is the target, that being the end that fixes it.
	if m, found := estimateMonitor(d, s); found && m.RefreshHz > 0 && s.Publish.Fps > m.RefreshHz {
		out = append(out, diagnosticFor(screensharev1.Severity_SEVERITY_WARNING, KeyFps,
			say(fpsAboveRefresh, argFps(s.Publish.Fps), argRefreshHz(m.RefreshHz))))
	}

	// An engine whose own tooling is missing was never probed, so no codec on it carries a verdict
	// and nothing greys for being absent from this machine.
	// The form then looks unrestricted and the failure moves from a greyed option to the launch.
	if engine, err := publish.EngineFor(s.Publish.Capture); err == nil {
		if _, unprobed := d.Encoders.Unprobed[engine]; unprobed {
			out = append(out, diagnosticFor(screensharev1.Severity_SEVERITY_INFO, KeyCapture,
				say(engineNotProbed,
					argEngine(engine), argCause(say(engineToolingMissing, argEngine(engine))))))
		}
	}
	return out
}

// diagnosticsAboutTheViewer says what the pixel format costs the people watching.
//
// Which families decode a codec and chroma pair is capabilities.Decoders' answer,
// read rather than restated, and the statement carries the pair it was read for.
//
// It is a cost and never an error.
// Every format has a software decoder, so no stream is undecodable,
// and the only question is whether a viewer spends cores on it (docs/field-availability.md).
func diagnosticsAboutTheViewer(s settings.Settings) []*screensharev1.Diagnostic {
	decode, err := capabilities.DecodeOf(s.Publish.Codec(), s.Publish.Chroma)
	if err != nil {
		// An unknown codec has no format to decode, and the publish path refuses it with its own reason.
		// A second statement about the viewer would bury that one.
		return nil
	}
	format := decode.Software.Format

	// The families that miss this pair differ in vendor
	// and not in substance, so one is named and the rest counted.
	// Listing all of them would be longer than the verdict they agree on.
	limited, others := "", 0
	if len(decode.Missing) > 0 {
		limited, others = decode.Missing[0].Family, len(decode.Missing)-1
	}

	if len(decode.Hardware) == 0 {
		return []*screensharev1.Diagnostic{
			diagnosticFor(screensharev1.Severity_SEVERITY_WARNING, KeyChroma,
				say(decodesOnCPU,
					argFormat(format), argChroma(s.Publish.Chroma),
					argDecoder(decode.Software.Element), argDecodeFamily(limited))),
		}
	}
	if len(decode.Missing) > 0 {
		return []*screensharev1.Diagnostic{
			diagnosticFor(screensharev1.Severity_SEVERITY_INFO, KeyChroma,
				say(decodesInHardwarePartly,
					argFormat(format), argChroma(s.Publish.Chroma),
					argDecodeFamilies(warningFamilies(decode.Hardware)),
					argDecoder(decode.Software.Element),
					argDecodeFamily(limited), argOtherCount(others))),
		}
	}
	return nil
}

// diagnosticsAboutThePrediction states the prediction, or why there is none.
func diagnosticsAboutThePrediction(d Deps, s settings.Settings, est *screensharev1.Estimate) []*screensharev1.Diagnostic {
	if est == nil {
		if _, found := estimateMonitor(d, s); !found {
			return []*screensharev1.Diagnostic{
				diagnosticFor(screensharev1.Severity_SEVERITY_INFO, KeyMonitor,
					say(monitorNotPriced, argMonitor(s.Publish.Monitor))),
			}
		}
		return []*screensharev1.Diagnostic{
			diagnosticFor(screensharev1.Severity_SEVERITY_INFO, "", say(noPictureToPrice)),
		}
	}

	// The raw rate crosses beside the predicted one so a surface can state the ratio between them,
	// which is the one figure that says what the encoder is being asked to do.
	// Both are figures in Mbit/s, so the precision is the surface's,
	// as is what a zero prediction reads as: a bitrate target of zero has no ratio.
	return []*screensharev1.Diagnostic{
		diagnosticFor(screensharev1.Severity_SEVERITY_INFO, "",
			say(compressionRatio, argRawMbps(est.GetRawMbps()), argBitrateMbps(est.GetBitrateMbps()))),
	}
}

// warningFamilies is the decode families a pair reaches, in table order and once each.
func warningFamilies(decoders []capabilities.Decoder) []string {
	out := make([]string, 0, len(decoders))
	for _, d := range decoders {
		if !slices.Contains(out, d.Family) {
			out = append(out, d.Family)
		}
	}
	return out
}
