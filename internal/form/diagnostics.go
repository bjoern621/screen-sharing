package form

import (
	"slices"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
)

// warningAnchors is every control a diagnostic may be shown beside.
//
// A diagnostic either belongs to one field or to the combination, and the second is an
// empty key rather than a key nobody declares. A key invented here would give a shell
// an anchor with no widget under it, and the diagnostic would render nowhere at all,
// which is the silent half of the failure this list exists to catch.
var warningAnchors = []string{
	KeyName, KeyRelayHost, KeyGroupKey, KeySrtPassphrase, KeySrtPort, KeyAPIPort, KeyRtspPort, KeyWebrtcPort,
	KeyRtmpPort, KeyHlsPort,
	KeyTransport, KeyCodec, KeyMode, KeyChroma, KeyColorRange, KeyFps, KeyCq,
	KeyBitrateM, KeyMaxrateM, KeyVbvMs, KeyGop, KeyBframes, KeyEffort, KeyTune,
	KeyCapture, KeyAudioSource, KeyAudioSourceDevice, KeyAudioSourceGain, KeyAudioSourceMute,
	KeyAudioCodec, KeyDrmMap, KeyMonitor, KeyCaptureMemory,
	KeySrtPublishLatencyMs, KeySrtWatchLatencyMs,
	KeyRtspPublishProtocol, KeyRtspWatchProtocol,
	KeyUplinkMbps,
	KeyPlayerWatchTransport,
	KeyOutputResolution,
}

// diagnostics is everything worth saying about the settings as a whole.
//
// The ranking is what a shell renders from, so it is the backend's: an error means the
// settings cannot be published as they stand and is what turns Form.publishable false,
// a warning means the stream runs and something about it is likely to disappoint, and
// an info is worth knowing. Within a rank the order is the order they are gathered in,
// which runs from what refuses the publish outward to what merely describes it.
//
// The refusals are not restated here. Every check a publish is refused for already
// runs inside publish.Command - capabilities.Validate, transport.ValidatePublish,
// capabilities.ValidateAudio, transport.ValidatePublishAudio and
// transport.ValidatePublishSettings, on whichever engine owns the selected capture
// backend - so the command's own failure is the one authority on whether these
// settings can run. Calling those validators again beside it would be a second
// statement of the same rule, and the first time an engine gained a check the form did
// not know about, the start button would enable on settings the publish refuses.
func diagnostics(d Deps, s settings.Settings, est *screensharev1.Estimate) []*screensharev1.Diagnostic {
	out := make([]*screensharev1.Diagnostic, 0, 8)

	// The builder's own refusal is the one raw string on this contract that a
	// diagnostic points at rather than carries: it is an operational failure, and the
	// same text crosses as a gRPC status when the publish is attempted. What the
	// diagnostic states is that there is one, and Summary.command_error is where it is
	// (api/proto/screenshare/v1/text.proto).
	if _, reason := formCommand(s); reason != "" {
		out = append(out, diagnosticFor(screensharev1.Severity_SEVERITY_ERROR, "", say(publishRefused)))
	}
	out = append(out, diagnosticsAboutTheLine(s, est)...)
	out = append(out, diagnosticsAboutTheCapture(d, s)...)
	out = append(out, diagnosticsAboutTheViewer(s)...)
	out = append(out, diagnosticsAboutThePrediction(d, s, est)...)

	// Ranked by what ignoring it costs, and stable within a rank so the gathering order
	// above is what a shell shows. A sort that reordered equal severities would move
	// diagnostics under the user between two keystrokes that changed neither of them.
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

// diagnosticFor is the one diagnostic constructor, so every diagnostic carries a rank, a
// statement and an anchor rather than being assembled field by field at each site.
func diagnosticFor(severity screensharev1.Severity, fieldKey string, t *screensharev1.Text) *screensharev1.Diagnostic {
	return &screensharev1.Diagnostic{Severity: severity, FieldKey: fieldKey, Text: t}
}

// warningPeaks is where a mode's burst belongs: the control the user changes to move
// it. Why the rate spreads follows from the mode, which the statement carries, so this
// table holds the anchor alone.
//
// A table rather than a switch because it is one static fact about three modes. A mode
// absent from it has no burst above its prediction, which is the same thing
// estimateSpread says of it.
var warningPeaks = map[string]string{
	capabilities.ModeCrf:      KeyCq,
	capabilities.ModeLossless: KeyMode,
	capabilities.ModeVbr:      KeyMaxrateM,
}

// diagnosticsAboutTheLine holds the predicted rate against the uplink the user stated.
//
// It is worth stating at all because neither end of this failure announces itself. A
// stream the line cannot carry is not encoded smaller; the send queue grows and the
// transport drops what it cannot ship, and what the viewer sees is a stall rather than
// a lower quality.
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
	// and the modes that have a burst are exactly the ones whose prediction is an
	// average. The burst is anchored on the control that sets it rather than on the
	// uplink, since that is the end the user moves.
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

// diagnosticsAboutTheCapture says where the machine will not give the pipeline what the
// settings ask of it.
func diagnosticsAboutTheCapture(d Deps, s settings.Settings) []*screensharev1.Diagnostic {
	var out []*screensharev1.Diagnostic

	// A frame rate above the monitor's own is a rate the capture cannot produce new
	// pictures for. The encoder still codes at the target, so the stream carries the
	// frame count the form promises and none of the smoothness it implies, which is the
	// disappointment worth naming: the fix is the target, not the encoder.
	if m, found := estimateMonitor(d, s); found && m.RefreshHz > 0 && s.Publish.Fps > m.RefreshHz {
		out = append(out, diagnosticFor(screensharev1.Severity_SEVERITY_WARNING, KeyFps,
			say(fpsAboveRefresh, argFps(s.Publish.Fps), argRefreshHz(m.RefreshHz))))
	}

	// An engine whose own tooling is missing was not probed at all, so no codec on it
	// carries a verdict and nothing is greyed for being absent from this machine. That
	// is worth knowing precisely because the form looks unrestricted: the failure moves
	// from the greyed option to the launch.
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
// The verdict is capabilities.Decoders' and is not restated: every hardware decoder is
// 4:2:0 with 10-bit where the format has a Main-10 equivalent, and the two exceptions
// are HEVC's Range Extensions profiles on NVDEC and on Intel. Which of those a choice
// lands on is a table lookup, and what a family's limit is follows from that family and
// the format, both of which the statement carries.
//
// It is a cost and never an error. Every format has a software decoder, so no stream is
// undecodable; the question is only whether a viewer spends cores on it.
func diagnosticsAboutTheViewer(s settings.Settings) []*screensharev1.Diagnostic {
	decode, err := capabilities.DecodeOf(s.Publish.Codec, s.Publish.Chroma)
	if err != nil {
		// An unknown codec has no format to decode. The publish path refuses it with its
		// own reason, and a second statement about the viewer would bury it.
		return nil
	}
	format := decode.Software.Format

	// The families that do not decode this pair agree in substance and differ in vendor,
	// so one is quoted and the rest are counted: a paragraph of four would bury the
	// verdict they all support.
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

// diagnosticsAboutThePrediction says what the estimate is, or why there is none.
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

	// The raw rate is what makes the rest legible: a compression ratio is the one figure
	// that says what the encoder is actually being asked to do, and it is the reason the
	// contract carries the uncompressed rate beside the predicted one. Both travel as
	// figures, so the surface decides whether it shows the ratio, at what precision, and
	// whether a zero prediction - a bitrate target of zero, which has no ratio - stops
	// the sentence at the two rates.
	return []*screensharev1.Diagnostic{
		diagnosticFor(screensharev1.Severity_SEVERITY_INFO, "",
			say(compressionRatio, argRawMbps(est.GetRawMbps()), argBitrateMbps(est.GetBitrateMbps()))),
	}
}

// warningFamilies names the decode families a pair reaches, in table order and once
// each, for a statement that says which viewers a choice reaches in hardware.
func warningFamilies(decoders []capabilities.Decoder) []string {
	out := make([]string, 0, len(decoders))
	for _, d := range decoders {
		if !slices.Contains(out, d.Family) {
			out = append(out, d.Family)
		}
	}
	return out
}
