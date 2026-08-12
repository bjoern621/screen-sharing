package publish

import (
	"slices"
	"strconv"

	"bjoernblessin.de/screenshare/internal/gstrun"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The GStreamer half of the live table: which element a write addresses, which property
// each codec's bitrate travels in, and what a whole live state looks like on the wire.
// Which fields are live at all is live.go.

// gstEncoderName is what the encoder element is called in every pipeline this engine
// builds, so a property write can name it.
//
// One name for every codec: which element wears it is the codec's business, and a parent
// addressing "the encoder" is addressing whichever one this pipeline has.
const gstEncoderName = "enc"

// gstLiveBitrate is the property the bitrate travels in on each codec's element, and the
// unit that element counts it in.
//
// It is a table rather than a derivation because the two engines spell one figure four
// ways: kbit on the x26x, nvcodec, va and qsv elements, bits per second on the libvpx and
// rav1e ones. TestTheLivePropertiesAreWhatTheBuildersSpend holds every row to what the
// mapping for that codec actually writes, so a row that drifts fails there rather than
// setting a rate an element reads as something else entirely.
//
// A codec absent here is one whose encoder takes no rate while it runs, which is what
// makes the bitrate not live for it: the row is the whole of the claim.
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

// gstBitrateProperty is one element's bitrate knob: what it is called and whether it
// counts bits per second rather than kbit.
type gstBitrateProperty struct {
	name      string
	perSecond bool
}

// value renders a settings bitrate as this property takes it.
func (p gstBitrateProperty) value(mbps int) string {
	if p.perSecond {
		return strconv.Itoa(mbps * 1_000_000)
	}
	return strconv.Itoa(mbps * 1000)
}

// gstLiveBitrateCodecs is every codec whose element takes a rate while it runs, sorted so
// the rule this fills reads the same on every build.
func gstLiveBitrateCodecs() []string {
	out := make([]string, 0, len(gstLiveBitrate))
	for codec := range gstLiveBitrate {
		out = append(out, codec)
	}
	slices.Sort(out)
	return out
}

// gstLiveBitrateWrite is what a running pipeline is told to hold these settings' bitrate.
//
// It is the row's own write and asks nothing about whether the field is live: that is the
// row's when, and a write reached with the mode or the codec it is gated on would be
// answering a question twice.
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

// gstLiveState is what a running pipeline should be holding for these settings.
//
// The state is whole every time, which is what lets the child converge to it rather than
// track what it was told before: sending it twice changes nothing the second time, and one
// that never arrived cannot leave the pipeline on a value nobody chose.
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

// gstLiveGainWrite is what a running pipeline is told to hold every source at.
//
// One write per branch the mixer already has, addressed by the volume element's own name,
// which is what makes a level reach exactly the source it belongs to. A muted source is one
// at zero, so nothing else is written for it: mute and gain are one value to an element that
// multiplies (settings.AudioSource.Volume).
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
