package gstrun

import (
	"strings"

	"github.com/go-gst/go-gst/pkg/gst"
)

// Narrowing the encoder input.
//
// A capsfilter that states alternatives fixates to the first of them whatever the frames
// carry. That is the only answer a launcher can give, because nothing in a launched
// pipeline knows what the capture turned out to be, and it is the wrong answer for a
// publish: a surface with a transfer characteristic of its own would be converted into the
// leading row and the stream would carry a colour the desktop never had.
//
// So before anything negotiates, the alternatives are narrowed to the ones whose
// colorimetry names the transfer the capture is producing. The rule carries no domain
// knowledge and no colour science: which colours a publish accepts is the parent's table,
// written into the description it handed over; which of them the surface carries is the
// capture's own answer; and this is the one place both are in hand.
//
// A capture that states no transfer at all narrows nothing, so the leading row stands.
// That is deliberate rather than a fallback: a standard-range surface is what caps carrying
// no transfer mean, and the parent leads its table with the standard-range row.

// colorimetryField is the caps field the narrowing reads and writes.
const colorimetryField = "colorimetry"

// narrowToSurface arranges for the pipeline's alternatives to be narrowed to the colour the
// capture produces, once, before the frames reach anything that would convert them.
//
// The hook is a probe on each source's own src pad, taken before the pipeline plays. A caps
// event passes that pad on its way downstream, so the narrowing lands ahead of the
// negotiation it is about rather than after an encoder has already been told a colour.
//
// Sources are found by shape, as everything else here finds them: an element with no sink
// pad is one producing frames from outside the pipeline, and naming the capture instead
// would put one string in this package and in whichever backend built the description.
func narrowToSurface(pipeline gst.Pipeline) {
	for v := range pipeline.IterateSources().Values() {
		el, ok := v.(gst.Element)
		if !ok {
			continue
		}
		pad := el.GetStaticPad("src")
		if pad == nil {
			continue
		}
		pad.AddProbe(gst.PadProbeTypeEventDownstream, func(_ gst.Pad, info *gst.PadProbeInfo) gst.PadProbeReturn {
			event := info.GetEvent()
			if event == nil || event.GetType() != gst.EventCaps {
				return gst.PadProbeOK
			}
			narrowFilters(pipeline, transferOf(event.ParseCaps()))
			// One answer per run: what the capture negotiated does not change under a
			// playing pipeline, and a second narrowing would be the same edit twice.
			return gst.PadProbeRemove
		})
	}
}

// narrowFilters drops from every capsfilter the structures whose colorimetry names another
// transfer than the one the capture carries.
//
// A filter is left alone in three cases, all of which mean there is nothing to choose
// between: the capture states no transfer, the filter states one structure, and no
// structure matches. The last is what keeps this from producing an empty capsfilter, which
// would fail the run with a negotiation error in place of the refusal the parent makes on
// the same caps, in words that name both ends.
func narrowFilters(pipeline gst.Pipeline, transfer string) {
	if transfer == "" {
		return
	}
	for v := range pipeline.IterateAllByElementFactoryName("capsfilter").Values() {
		el, ok := v.(gst.Element)
		if !ok {
			continue
		}
		caps, ok := el.ObjectProperty("caps").(*gst.Caps)
		if !ok || caps == nil || caps.GetSize() < 2 {
			continue
		}
		if narrowed := matching(caps, transfer); narrowed != nil {
			el.SetObjectProperty("caps", narrowed)
		}
	}
}

// matching is caps holding the structures whose colorimetry carries this transfer, and nil
// where none does.
//
// Each is copied with its own caps features, because the feature is what decides which pads
// can link at all: a structure that arrived naming device memory and left naming none would
// pin the frames back into the system-memory round trip the whole path exists to avoid.
func matching(caps *gst.Caps, transfer string) *gst.Caps {
	var out *gst.Caps
	for i := uint(0); i < caps.GetSize(); i++ {
		s := caps.GetStructure(i)
		if s == nil || TransferOfColorimetry(s.GetString(colorimetryField)) != transfer {
			continue
		}
		if out == nil {
			out = caps.CopyNth(i)
			continue
		}
		out.AppendStructureFull(s.Copy(), caps.GetFeatures(i))
	}
	return out
}

// transferOf is the transfer characteristic the caps carry, and the empty string for caps
// that name none or name more than one structure. The capture produces one format, so a
// caps with several structures is one nothing has negotiated yet.
func transferOf(caps *gst.Caps) string {
	if caps == nil || caps.GetSize() != 1 {
		return ""
	}
	s := caps.GetStructure(0)
	if s == nil {
		return ""
	}
	return TransferOfColorimetry(s.GetString(colorimetryField))
}

// TransferOfColorimetry is the transfer characteristic in one colorimetry value, as
// GstVideoTransferFunction nicks it, and the empty string where the value carries none.
//
// The field takes two forms and both have to answer alike, because the narrowing holds one
// against the other: a capture negotiates one of GStreamer's names for a common
// combination, and a capsfilter pins four numbers separated by colons - range, matrix,
// transfer, primaries. Answering with the number for one and the nick for the other would
// make every comparison between them false, and the surface's own colour would never be the
// row that won.
//
// It is exported because the parent reads the same field off the caps this child reports,
// and a second spelling of "which part of that string is the transfer" is one that can
// disagree with this one about what a surface is.
func TransferOfColorimetry(value string) string {
	if value == "" {
		return ""
	}
	if named, ok := namedTransfers[value]; ok {
		return named
	}
	if parts := strings.Split(value, ":"); len(parts) == 4 {
		if nick, ok := transferNicks[parts[2]]; ok {
			return nick
		}
		return parts[2]
	}
	return value
}

// namedTransfers is the transfer characteristic behind each colorimetry name GStreamer
// prints in place of the four components.
var namedTransfers = map[string]string{
	"bt601":      "bt601",
	"bt709":      "bt709",
	"bt2020":     "bt2020-10",
	"smpte240m":  "smpte240m",
	"sRGB":       "srgb",
	"bt2100-pq":  "smpte2084",
	"bt2100-hlg": "arib-std-b67",
}

// transferNicks is the GstVideoTransferFunction enum as the colorimetry field spells it in
// each of its two forms: the value a capsfilter pins, and the nick everything else prints.
var transferNicks = map[string]string{
	"1":  "gamma10",
	"2":  "gamma18",
	"3":  "gamma20",
	"4":  "gamma22",
	"5":  "bt709",
	"6":  "smpte240m",
	"7":  "srgb",
	"8":  "gamma28",
	"9":  "log100",
	"10": "log316",
	"11": "bt2020-12",
	"12": "adobergb",
	"13": "bt2020-10",
	"14": "smpte2084",
	"15": "arib-std-b67",
	"16": "bt601",
}
