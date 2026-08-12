package publish

import (
	"strconv"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/gstrun"
	"bjoernblessin.de/screenshare/internal/settings"
)

// What a running pipeline will take, and what it will not.
//
// A publish that changes one of these does not have to become another pipeline: the value
// reaches the encoder over the child's control socket and every viewer keeps watching,
// where a relaunch costs each of them a reconnect. Everything else is a different pipeline
// and says so by changing the rendered command (SamePipeline).

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

// gstLiveState is what a running pipeline should be holding for these settings.
//
// A mode that sends the encoder no rate at all sends no write either: constant quality
// aims at a quantizer and lossless at exactness, so a bitrate written there would be a
// figure the element carries and never spends. The state is whole every time, which is
// what lets the child converge to it rather than track what it was told before.
func gstLiveState(s settings.Settings) gstrun.LiveState {
	property, live := gstLiveBitrate[s.Publish.Codec]
	if !live || !capabilities.TargetsBitrate(s.Publish.Mode) {
		return gstrun.LiveState{}
	}
	return gstrun.LiveState{Properties: []gstrun.Property{{
		Element: gstEncoderName,
		Name:    property.name,
		Value:   property.value(s.Publish.BitrateM),
	}}}
}
