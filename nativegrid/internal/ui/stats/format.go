package stats

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"bjoernblessin.de/screenshare-nativegrid/internal/player"
)

// separator joins two related figures on one row.
const separator = " · "

// join puts the parts that have a value on one row, which is how two related
// figures share a line.
func join(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, separator)
}

// joinTip stacks the parts of one tooltip, separated by a blank line: a value the
// card had to ellipsize above the explanation of the row it sits on. The settings
// form appends its availability notes the same way.
func joinTip(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "\n\n")
}

// mbps converts a byte delta over an interval into megabits per second.
func mbps(bytes uint64, seconds float64) float64 {
	if seconds <= 0 {
		return 0
	}
	return float64(bytes) * 8 / seconds / 1e6
}

func fpsText(fps float64) string {
	return fmt.Sprintf("%.1f", fps)
}

func sizeText(w, h int) string {
	if w == 0 || h == 0 {
		return ""
	}
	return fmt.Sprintf("%d×%d", w, h)
}

// rateText keeps a caps framerate a fraction: a stream that negotiated 30000/1001
// did not negotiate 29.97.
func rateText(num, den int) string {
	if num == 0 || den == 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d", num, den)
}

func depthText(depth int) string {
	if depth == 0 {
		return ""
	}
	return fmt.Sprintf("%d bit", depth)
}

// aspectText marks a pixel aspect ratio, so a square-pixel stream does not read as
// a ratio of something else on the row it shares with the scan mode.
func aspectText(par string) string {
	if par == "" {
		return ""
	}
	return "par " + par
}

// siteText qualifies the chroma siting, which caps write bare, e.g. "mpeg2".
func siteText(site string) string {
	if site == "" {
		return ""
	}
	return "site " + site
}

// levelText marks an H.26x level, so the row reads as a level and not as a second
// version number.
func levelText(level string) string {
	if level == "" {
		return ""
	}
	return "L" + level
}

// decoderText names the decoder and whether it decodes on the GPU, the one thing a
// factory name does not say.
func decoderText(s player.Stats) string {
	if s.Decoder == "" {
		return ""
	}
	if s.Hardware {
		return s.Decoder + " (hardware)"
	}
	return s.Decoder + " (software)"
}

// chainText names the render chain and what it claims about the colour it
// produces, which is the choice behind picking one chain over another.
func chainText(s player.Stats) string {
	if s.Chain == "" {
		return ""
	}
	if s.ChainExact {
		return s.Chain + " (colour stated)"
	}
	return s.Chain + " (colour unstated)"
}

// pathText is the verdict on what happened to the frames between the decoder and
// the sink.
func pathText(s player.Stats) string {
	p, ok := pathOf(s)
	if !ok {
		return ""
	}
	return p.label
}

// pathTip explains the verdict the path row shows, since the four verdicts mean
// four different things.
func pathTip(s player.Stats) string {
	p, ok := pathOf(s)
	if !ok {
		return ""
	}
	return p.tip
}

// memoryText is where each end of the chain holds its frames, as the caps spell it
// rather than as a summary: the row is the evidence the path row's verdict is read
// from, so a reader can check the verdict against it.
//
// Both ends have to have negotiated. One feature alone does not say which end it
// belongs to.
func memoryText(s player.Stats) string {
	if s.DecodeMemory == "" || s.RenderMemory == "" {
		return ""
	}
	return s.DecodeMemory + " → " + s.RenderMemory
}

// bitrateText is an encoded stream's measured rate and the bytes it has carried. A
// stream nobody decodes has no encoded side to count, and the row stays away.
func bitrateText(rate float64, bytes uint64) string {
	if bytes == 0 {
		return ""
	}
	return fmt.Sprintf("%.1f Mbps%s%s", rate, separator, byteSize(bytes))
}

// byteSize renders a byte count in binary units, the unit growing with the value so
// a long watch reads as GiB instead of as ten digits.
func byteSize(b uint64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	v, i := float64(b), 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", b)
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}

// keyframeText is the keyframe count and the age of the last one, which is the GOP
// length as it actually arrives rather than as the encoder was asked for.
func keyframeText(s player.Stats) string {
	if s.Keyframes == 0 {
		return ""
	}
	if s.SinceKeyframe == 0 {
		return strconv.FormatUint(s.Keyframes, 10)
	}
	return fmt.Sprintf("%d%s%.1f s ago", s.Keyframes, separator, s.SinceKeyframe.Seconds())
}

// framesText is what the sink did with the frames it was handed. The paintable's
// own count stands in until the sink reports, since it cannot tell a dropped frame
// from one that was never sent. The drop count carries its share of the frames the
// sink was handed, because a raw count means nothing without the run it happened
// over: 200 drops is a stall on a short watch and a rounding error on a long one.
func framesText(s player.Stats) string {
	if s.Rendered == 0 && s.Dropped == 0 {
		if s.Frames == 0 {
			return ""
		}
		return fmt.Sprintf("%d painted", s.Frames)
	}
	handed := s.Rendered + s.Dropped
	share := 100 * float64(s.Dropped) / float64(handed)
	return fmt.Sprintf("%d rendered%s%d dropped (%.1f%%)", s.Rendered, separator, s.Dropped, share)
}

// codedText counts the encoded frames the decoder was handed, which runs slightly
// ahead of the rendered count while frames are in flight and well ahead of it when
// the queue leaks.
func codedText(s player.Stats) string {
	if s.VideoFrames == 0 {
		return ""
	}
	return fmt.Sprintf("%d frames", s.VideoFrames)
}

// latencyText is the latency window the pipeline configured and whether it runs
// live at all, the two answers a latency query carries.
func latencyText(s player.Stats) string {
	if s.LatencyMin == 0 && s.LatencyMax == 0 {
		return ""
	}
	out := shortDuration(s.LatencyMin)
	if s.LatencyMax > s.LatencyMin {
		out += " / " + shortDuration(s.LatencyMax)
	}
	if s.Live {
		out += separator + "live"
	}
	return out
}

// audioFormatText is the audio branch's raw format: sample format, rate and channel
// count.
func audioFormatText(s player.Stats) string {
	if s.AudioFormat == "" {
		return ""
	}
	out := s.AudioFormat
	if s.AudioRate > 0 {
		out += fmt.Sprintf(" %g kHz", float64(s.AudioRate)/1000)
	}
	switch s.AudioChannels {
	case 0:
	case 1:
		out += " mono"
	case 2:
		out += " stereo"
	default:
		out += fmt.Sprintf(" %dch", s.AudioChannels)
	}
	return out
}

// shortDuration renders a duration at second resolution, with sub-second figures to
// the millisecond so a 200 ms latency does not read as zero.
func shortDuration(d time.Duration) string {
	switch {
	case d <= 0:
		return ""
	case d < time.Second:
		return fmt.Sprintf("%d ms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1f s", d.Seconds())
	default:
		return d.Round(time.Second).String()
	}
}
