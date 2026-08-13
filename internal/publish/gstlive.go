package publish

import (
	"slices"
	"strconv"

	"bjoernblessin.de/screenshare/internal/gstrun"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The GStreamer half of the live table: the element a write addresses, and the property each codec
// takes a rate in.
// Which fields are live at all is live.go.

// gstEncoderName is the name every pipeline here gives its encoder element, so a property write can
// address it without knowing which element the codec resolved to.
const gstEncoderName = "enc"

// gstLiveBitrate is the property each codec's encoder element takes a rate in, and the unit that
// element counts in.
// Property name and unit vary independently across the elements, which is why it is a table:
// "bitrate" or "target-bitrate", kbit or bits per second.
// TestTheLivePropertiesAreWhatTheBuildersSpend holds every row to what that codec's mapping writes.
//
// A codec absent here takes no rate while it runs, and the row is the whole of the claim that the
// bitrate is not live for it.
var gstLiveBitrate = map[string]gstBitrateProperty{
	"libx264":    {name: "bitrate", perSecond: false},
	"libx265":    {name: "bitrate", perSecond: false},
	"h264_nvenc": {name: "bitrate", perSecond: false},
	"hevc_nvenc": {name: "bitrate", perSecond: false},
	"av1_nvenc":  {name: "bitrate", perSecond: false},
	"libvpx":     {name: "target-bitrate", perSecond: true},
	"libvpx-vp9": {name: "target-bitrate", perSecond: true},
	"libaom-av1": {name: "target-bitrate", perSecond: false},
	"libsvtav1":  {name: "target-bitrate", perSecond: false},
	"librav1e":   {name: "bitrate", perSecond: true},
	"h264_vaapi": {name: "bitrate", perSecond: false},
	"hevc_vaapi": {name: "bitrate", perSecond: false},
	"av1_vaapi":  {name: "bitrate", perSecond: false},
	"vp9_vaapi":  {name: "bitrate", perSecond: false},
	"vp8_vaapi":  {name: "bitrate", perSecond: false},
	"h264_qsv":   {name: "bitrate", perSecond: false},
	"hevc_qsv":   {name: "bitrate", perSecond: false},
	"av1_qsv":    {name: "bitrate", perSecond: false},
	"vp9_qsv":    {name: "bitrate", perSecond: false},
}

// gstBitrateProperty is one element's rate knob: the property name, and whether it counts bits per
// second rather than kbit.
type gstBitrateProperty struct {
	name      string
	perSecond bool
}

// value renders a bitrate in Mbit/s as this property takes it.
func (p gstBitrateProperty) value(mbps int) string {
	if p.perSecond {
		return strconv.Itoa(mbps * 1_000_000)
	}
	return strconv.Itoa(mbps * 1000)
}

// gstLiveBitrateCodecs is every codec whose element takes a rate while it runs, sorted so the rule
// it fills reads the same on every build.
func gstLiveBitrateCodecs() []string {
	out := make([]string, 0, len(gstLiveBitrate))
	for codec := range gstLiveBitrate {
		out = append(out, codec)
	}
	slices.Sort(out)
	return out
}

// gstLiveBitrateWrite is what a running pipeline is told to hold these settings' bitrate.
// Whether the field is live at all is the row's when, so nothing is asked about it twice here.
func gstLiveBitrateWrite(s settings.Settings) []gstrun.Property {
	property, mapped := gstLiveBitrate[s.Publish.Codec]
	if !mapped {
		return nil
	}
	return []gstrun.Property{{
		Element: gstEncoderName,
		Name:    property.name,
		Value:   property.value(s.Publish.BitrateM),
	}}
}

// gstLiveState is the whole state a running pipeline should hold for these settings, never a diff.
// The child converges to it, so a second send changes nothing and one that never arrived cannot
// leave the pipeline on a value nobody chose.
func gstLiveState(s settings.Settings) gstrun.LiveState {
	var state gstrun.LiveState
	for _, key := range LiveFields(s) {
		f, ok := liveFieldFor(key)
		if !ok {
			continue
		}
		state.Properties = append(state.Properties, f.write(s)...)
	}
	return state
}

// gstLiveGainWrite is the level every source is held at, one write per branch the mixer already
// has, addressed by that branch's own volume element.
// Mute is a level to an element that multiplies, so a muted source writes zero and carries nothing
// else (settings.AudioSource.Volume).
func gstLiveGainWrite(s settings.Settings) []gstrun.Property {
	recorded := s.Publish.Recorded()
	out := make([]gstrun.Property, 0, len(recorded))
	for i, a := range recorded {
		out = append(out, gstrun.Property{
			Element: gstAudioVolumeName(i),
			Name:    "volume",
			Value:   strconv.FormatFloat(a.Volume(), 'f', 3, 64),
		})
	}
	return out
}
