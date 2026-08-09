package form

import (
	"math"
	"strconv"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The quality model, ported from the Wails frontend's util/estimate.ts.
//
// The anchor is the one measured figure the whole prediction rests on: bits per pixel
// per frame for H.264 4:2:0 at CQ 23 on mixed content. Every other combination is
// priced as a ratio against it, so a codec, a pixel format or a quantizer moves the
// figure without any of them carrying a bitrate of its own.
//
// The quantizer scale is the anchor's own, which is the H.26x encoders' 51 points. A
// codec that counts further (libvpx VP9 reaches 63, a raw quantizer index 127 or 255)
// has its target placed on the anchor scale before the exponent is taken, since the
// same number is a different quality per encoder.
const (
	estimateAnchorBpp   = 0.07
	estimateAnchorCq    = 23
	estimateAnchorCqMax = 51
	// estimateCqStep is how many quantizer points on the anchor scale halve or double
	// the rate.
	estimateCqStep = 6
)

// The spread the content puts around the nominal figure, from a static desktop to
// heavy motion. The contract carries one bitrate rather than a range, so the nominal
// is what estimate reports and the high end is what a diagnostic prices the burst from:
// constant quality sets no bitrate bound, and a line sized for the average stalls on
// the first window drag.
const (
	estimateMotionLow  = 0.4
	estimateMotionHigh = 2.5
)

// The lossless spread, as a fraction of the raw rate: a near-static screen compresses
// hard and heavy motion nears the uncompressed picture.
//
// estimateLosslessTypical is the midpoint, and it exists because the contract has one
// figure where the model has two ends. It is derived from the ends rather than
// measured on its own, so a revision of either end moves it with them.
const (
	estimateLosslessLow     = 0.06
	estimateLosslessHigh    = 0.55
	estimateLosslessTypical = (estimateLosslessLow + estimateLosslessHigh) / 2
)

// estimateEfficiency is the relative coding efficiency of each bitstream format: the
// bits it spends for the quality H.264 reaches at 1.0.
//
// It follows the format and not the encoder family, because it is a property of what
// the bitstream may express: hevc_nvenc and libx265 spend the same bits for the same
// quality by definition of the format they both produce. That is why it is keyed by
// Codec.Format rather than by Codec.Name, and why a codec added to the capability
// table needs no row here as long as it produces a format that has one.
var estimateEfficiency = map[string]float64{
	"h264": 1.0,
	"hevc": 0.6,
	"av1":  0.5,
	"vp9":  0.6,
	"vp8":  1.0,
}

// estimateChromaCost is what one pixel of a pixel format costs, coded and raw.
type estimateChromaCost struct {
	// weight is the detail the format carries relative to 4:2:0 at 1.0, which is what
	// the encoder spends the extra bits on.
	weight float64
	// rawBpp is the uncompressed bits per pixel per frame, which is what the capture
	// produces before anything codes it.
	rawBpp float64
}

// estimateChromas is the cost of each pixel format the codec table declares.
//
// The set is closed: capabilities.Codecs names these five and no other, so a chroma
// absent here is a settings value that no codec can encode, and the prediction is
// withheld rather than priced against a guessed weight. Predicting nothing is the
// honest answer for settings the publish path refuses anyway.
var estimateChromas = map[string]estimateChromaCost{
	"gbrp":    {weight: 2.0, rawBpp: 24},
	"yuv444p": {weight: 1.5, rawBpp: 24},
	"yuv422p": {weight: 1.25, rawBpp: 16},
	"yuv420p": {weight: 1.0, rawBpp: 12},
	"p010le":  {weight: 1.2, rawBpp: 15},
}

// estimate predicts what the settings cost before anything runs.
//
// Three figures, and the middle one is what makes the other two legible: the raw rate
// is what the capture produces, the bitrate is what the encoder is predicted to leave
// of it, and the headroom is what the stated uplink has left over. It is a prediction
// and not a promise - the real rate is content-dependent - which is why the measured
// figures arrive separately once a stream is live.
//
// Nothing here reads a device or spawns anything: it is arithmetic over the settings,
// the capability tables and the monitor list it was handed, so a shell may call the
// resolve it belongs to on every keystroke.
//
// It answers nil where an input the figure rests on is unresolved. A monitor the
// enumeration does not contain is the common case and is an environment condition
// rather than a bug: the machine's outputs changed under a stored settings file, or
// the platform has no enumerator here. The rest of the form still resolves, the
// summary carries no estimate, and a diagnostic says the picture size is missing.
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

// estimateCoded prices the encoded rate for the rate-control mode in force, and
// reports false where the mode leaves it unpriceable.
//
// The mode decides which question is even being asked, which is why this is not one
// formula. Three of the modes aim at a bitrate the user set, so the prediction is that
// target: the encoder holds it, and predicting a quality-model figure instead would
// contradict the number in the field beside it. The other two are quality-driven and
// spend whatever the picture costs, so they are priced from the picture.
func estimateCoded(s settings.Settings, pixelRate, raw float64, chroma estimateChromaCost) (float64, bool) {
	switch s.Publish.Mode {
	case capabilities.ModeCbr, capabilities.ModeAbr, capabilities.ModeVbr:
		// The target, not the ceiling. A VBR ceiling is the burst the encoder is allowed
		// rather than the rate it aims at, and reporting it here would read every VBR
		// configuration as costing its worst second. The burst is priced where it
		// matters instead, as a diagnostic against the line (estimatePeak).
		return float64(s.Publish.BitrateM), true
	case capabilities.ModeLossless:
		return raw * estimateLosslessTypical, true
	case capabilities.ModeCrf:
		return estimateConstantQuality(s, pixelRate, chroma)
	}
	// The mode is the user's string and arrives from a settings file this app did not
	// necessarily write, so a value outside the table is answered rather than asserted.
	// The publish path refuses it with its own reason, which is the sentence worth
	// showing.
	return 0, false
}

// estimateConstantQuality prices a crf encode from the anchor: the quantizer target
// placed on the anchor scale, the codec's coding efficiency and the chroma's weight.
func estimateConstantQuality(s settings.Settings, pixelRate float64, chroma estimateChromaCost) (float64, bool) {
	c, known := capabilities.Get(s.Publish.Codec)
	if !known {
		return 0, false
	}
	efficiency, priced := estimateEfficiency[c.Format]
	// Format is the capability table's own value, not the user's, so a format with no
	// efficiency is a row added to one table and not the other.
	assert.Assert(priced, "every published bitstream format states a coding efficiency", c.Format)

	// The quantizer is placed on the scale the model is calibrated against. The scale
	// is the running engine's, since the two engines set different properties and one
	// may count further than the other. A codec whose scale this engine declares none
	// for leaves the number where it stands, there being no ratio to convert it by.
	cq := float64(s.Publish.Cq)
	if engine, err := publish.EngineFor(s.Publish.Capture); err == nil {
		if scale := c.CqMaxOn(engine); scale > 0 {
			cq = float64(s.Publish.Cq) * estimateAnchorCqMax / float64(scale)
		}
	}

	bpp := estimateAnchorBpp * math.Pow(2, (estimateAnchorCq-cq)/estimateCqStep) * efficiency * chroma.weight
	return pixelRate * bpp / 1e6, true
}

// estimateSpread is what the content can move the rate to either side of the
// prediction, and false for a mode that has no second figure to state.
//
// It is derived from the estimate rather than computed a second time, so the spread
// and the prediction cannot disagree about what the settings produce. The high end is
// the one a diagnostic is priced from: a line sized for the average stalls on the first
// window drag, and the drop is the transport's rather than the encoder's.
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
		// The ceiling is what separates VBR from ABR, so the mode reaches an encoder
		// only where there is a property for it: an encoder that takes no ceiling
		// carries a mode gap and cannot be in VBR here at all. A ceiling under the
		// target is not a burst.
		if s.Publish.MaxrateM > s.Publish.BitrateM {
			return float64(s.Publish.BitrateM), float64(s.Publish.MaxrateM), true
		}
	}
	// CBR is held at the target every second, and ABR averages toward it with nothing
	// declared above it, so neither has a second figure to state.
	return 0, 0, false
}

// estimateMonitor finds the monitor the settings name in the enumeration the capture
// crops to, so the prediction is priced from the same picture the publish would send.
func estimateMonitor(d Deps, s settings.Settings) (display.Monitor, bool) {
	for _, m := range d.Monitors {
		if m.Index == s.Publish.Monitor {
			return m, true
		}
	}
	return display.Monitor{}, false
}

// estimateFigure renders a rate as the frontend did: tenths under ten, whole numbers
// above, so a slow combination keeps the digit that distinguishes it and a fast one is
// not given a precision the model does not have.
func estimateFigure(mbps float64) string {
	if mbps >= 10 {
		return strconv.Itoa(int(math.Round(mbps)))
	}
	return strconv.FormatFloat(mbps, 'f', 1, 64)
}
