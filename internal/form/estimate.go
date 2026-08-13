package form

import (
	"math"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The quality model, resting on one measured figure:
// the bits a pixel of H.264 4:2:0 costs per frame at CQ 23 on mixed content.
// Every other combination is priced as a ratio against that anchor,
// so a codec, a pixel format or a quantizer moves the prediction without carrying a bitrate itself.
//
// The quantizer scale is the anchor's own, the H.26x encoders' 51 points.
// A codec that counts further, libvpx VP9 to 63 or a raw quantizer index to 127 or 255,
// has its target placed on that scale before the exponent is taken,
// since the same number is a different quality per encoder.
const (
	estimateAnchorBpp   = 0.07
	estimateAnchorCq    = 23
	estimateAnchorCqMax = 51
	// estimateCqStep is the quantizer points, on the anchor scale, that halve or double the rate.
	estimateCqStep = 6
)

// What content moves the rate to, as a multiple of the nominal figure:
// a static desktop at the low end, heavy motion at the high end.
// The contract carries one bitrate rather than a range,
// so estimate reports the nominal and a diagnostic prices the burst from the high end:
// constant quality sets no bitrate bound, and a line sized for the average stalls on a window drag.
const (
	estimateMotionLow  = 0.4
	estimateMotionHigh = 2.5
)

// The lossless spread, as a fraction of the raw rate:
// a near-static screen compresses hard and heavy motion nears the uncompressed picture.
//
// estimateLosslessTypical is the midpoint,
// which the contract needs because it carries one figure where the model has two ends.
// It is derived from the ends rather than measured, so a revision of either end moves it.
const (
	estimateLosslessLow     = 0.06
	estimateLosslessHigh    = 0.55
	estimateLosslessTypical = (estimateLosslessLow + estimateLosslessHigh) / 2
)

// estimateEfficiency is the bits each bitstream format spends for the quality H.264 reaches at 1.0.
//
// Keyed by Codec.Format and not by Codec.Name,
// because it is a property of what the bitstream may express:
// hevc_nvenc and libx265 spend the same bits for the same quality.
// A codec added to the capability table needs no row here as long as its format has one.
var estimateEfficiency = map[string]float64{
	"h264": 1.0,
	"hevc": 0.6,
	"av1":  0.5,
	"vp9":  0.6,
	"vp8":  1.0,
}

// estimateChromaCost is what one pixel of a pixel format costs, coded and raw.
type estimateChromaCost struct {
	// weight is the detail the format carries against 4:2:0 at 1.0,
	// which is what the encoder spends the extra bits on.
	weight float64
	// rawBpp is the bits one pixel costs per frame uncompressed, as the capture hands it over.
	rawBpp float64
}

// estimateChromas is one row per pixel format the codec table declares.
//
// The set is closed against capabilities.Codecs, so a chroma absent here is one no codec encodes:
// the prediction is withheld rather than priced against a guessed weight,
// which is the honest answer for settings the publish path refuses anyway.
var estimateChromas = map[string]estimateChromaCost{
	"gbrp":    {weight: 2.0, rawBpp: 24},
	"yuv444p": {weight: 1.5, rawBpp: 24},
	"yuv422p": {weight: 1.25, rawBpp: 16},
	"yuv420p": {weight: 1.0, rawBpp: 12},
	"p010le":  {weight: 1.2, rawBpp: 15},
}

// estimate predicts what the settings cost before anything runs.
//
// Three figures, all Mbit/s: the raw rate the capture produces,
// the bitrate the encoder is predicted to leave of it,
// and the headroom the stated uplink has over that.
// It is a prediction and not a promise, the real rate being content-dependent,
// which is why the measured figures arrive separately once a stream is live.
//
// It reads no device and spawns nothing, being arithmetic over the draft,
// the capability tables and the monitor list it was handed,
// so a shell may call the resolve it belongs to on every keystroke.
//
// nil where an input the figure rests on is unresolved.
// A monitor the enumeration does not hold is the common case,
// and an environment condition rather than a bug:
// the machine's outputs changed under a stored settings file, or the platform has no enumerator.
// The rest of the form still resolves, the summary carries no estimate,
// and a diagnostic says the picture has no size.
func estimate(d Deps, s settings.Settings) *screensharev1.Estimate {
	m, found := estimateMonitor(d, s)
	if !found || m.Width <= 0 || m.Height <= 0 || s.Publish.Fps <= 0 {
		return nil
	}
	chroma, known := estimateChromas[s.Publish.Chroma]
	if !known {
		return nil
	}

	pixelRate := float64(m.Width) * float64(m.Height) * float64(s.Publish.Fps)
	raw := pixelRate * chroma.rawBpp / 1e6

	coded, priced := estimateCoded(s, pixelRate, raw, chroma)
	if !priced {
		return nil
	}

	est := &screensharev1.Estimate{
		BitrateMbps:  coded,
		RawMbps:      raw,
		HeadroomMbps: float64(s.Publish.UplinkMbps) - coded,
	}

	assert.Assert(est.GetRawMbps() > 0, "a captured picture costs something to carry uncompressed", est.GetRawMbps())
	assert.Assert(est.GetBitrateMbps() >= 0, "a predicted bitrate is a rate", est.GetBitrateMbps())
	assert.Assert(est.GetHeadroomMbps() == float64(s.Publish.UplinkMbps)-est.GetBitrateMbps(),
		"the headroom is what the stated uplink has left over", est.GetHeadroomMbps())
	return est
}

// estimateCoded prices the coded rate in Mbit/s for the rate-control mode in force,
// and reports false where the mode leaves it unpriceable.
//
// The mode decides which question is asked, which is why this is not one formula.
// A mode aiming at a bitrate the user set is predicted at that target: the encoder holds it,
// and a quality-model figure beside the number in the field would contradict it.
// A quality-driven mode spends whatever the picture costs and is priced from the picture.
func estimateCoded(s settings.Settings, pixelRate, raw float64, chroma estimateChromaCost) (float64, bool) {
	switch s.Publish.Mode {
	case capabilities.ModeCbr, capabilities.ModeAbr, capabilities.ModeVbr:
		// The target, not the ceiling.
		// A VBR ceiling is the burst the encoder is allowed rather than the rate it aims at,
		// so reporting it here would read every VBR configuration as costing its worst second.
		// The burst is priced against the line instead (estimateSpread).
		return float64(s.Publish.BitrateM), true
	case capabilities.ModeLossless:
		return raw * estimateLosslessTypical, true
	case capabilities.ModeCrf:
		return estimateConstantQuality(s, pixelRate, chroma)
	}
	// The mode arrives from a settings file this app did not necessarily write.
	// A value outside the table is therefore an Umgebungsfehler: answered with no price,
	// never asserted, and refused by the publish path with its own reason.
	return 0, false
}

// estimateConstantQuality prices a crf encode from the anchor:
// the quantizer target placed on the anchor scale, times the codec's coding efficiency,
// times the chroma's weight.
func estimateConstantQuality(s settings.Settings, pixelRate float64, chroma estimateChromaCost) (float64, bool) {
	c, known := capabilities.Get(s.Publish.Codec)
	if !known {
		return 0, false
	}
	efficiency, priced := estimateEfficiency[c.Format]
	// Format is the capability table's value and not the user's,
	// so a missing efficiency is a row added to one table and not the other.
	assert.Assert(priced, "every published bitstream format states a coding efficiency", c.Format)

	// The scale is the running engine's,
	// since the two engines set different properties and one may count further than the other.
	// A codec whose scale this engine declares none for leaves the number where it stands,
	// there being no ratio to convert it by.
	cq := float64(s.Publish.Cq)
	if engine, err := publish.EngineFor(s.Publish.Capture); err == nil {
		if scale := c.CqMaxOn(engine); scale > 0 {
			cq = float64(s.Publish.Cq) * estimateAnchorCqMax / float64(scale)
		}
	}

	bpp := estimateAnchorBpp * math.Pow(2, (estimateAnchorCq-cq)/estimateCqStep) * efficiency * chroma.weight
	return pixelRate * bpp / 1e6, true
}

// estimateSpread is what content can move the rate to, either side of the prediction and in Mbit/s,
// and false for a mode with no second figure to state.
//
// It is derived from the estimate rather than priced again,
// so the spread and the prediction cannot disagree about what the settings produce.
// The high end is what a diagnostic is priced from,
// the drop on a line sized for the average being the transport's rather than the encoder's.
func estimateSpread(s settings.Settings, est *screensharev1.Estimate) (low, high float64, spreads bool) {
	if est == nil {
		return 0, 0, false
	}
	switch s.Publish.Mode {
	case capabilities.ModeCrf:
		return est.GetBitrateMbps() * estimateMotionLow, est.GetBitrateMbps() * estimateMotionHigh, true
	case capabilities.ModeLossless:
		return est.GetRawMbps() * estimateLosslessLow, est.GetRawMbps() * estimateLosslessHigh, true
	case capabilities.ModeVbr:
		// A ceiling under the target is not a burst.
		// An encoder that takes no ceiling gaps the mode rather than greying the field,
		// since VBR without a ceiling is ABR (docs/domain-model.md).
		if s.Publish.MaxrateM > s.Publish.BitrateM {
			return float64(s.Publish.BitrateM), float64(s.Publish.MaxrateM), true
		}
	}
	// CBR holds the target every second and ABR averages toward it with nothing declared above it,
	// so neither states a second figure.
	return 0, 0, false
}

// estimateMonitor is the monitor the draft names, out of the enumeration the capture crops to,
// so the prediction is priced from the picture the publish would send.
func estimateMonitor(d Deps, s settings.Settings) (display.Monitor, bool) {
	for _, m := range d.Monitors {
		if m.Index == s.Publish.Monitor {
			return m, true
		}
	}
	return display.Monitor{}, false
}
