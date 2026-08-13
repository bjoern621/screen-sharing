package gstrun

import (
	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/colour"
)

// Narrowing the encoder input.
//
// A capsfilter that states alternatives fixates to the first of them whatever the frames carry.
// That is the only answer a launcher can give, because nothing in a launched pipeline knows what
// the capture turned out to be, and it is the wrong answer for a publish: a surface with a transfer
// characteristic of its own would be converted into the leading row and the stream would carry a
// colour the desktop never had.
//
// So before anything negotiates, the alternatives are narrowed to the ones whose colorimetry names
// the transfer the capture is producing.
// The rule carries no domain knowledge and no colour science: which colours a publish accepts is
// the parent's table, written into the description it handed over; which of them the surface
// carries is the capture's own answer; and this is the one place both are in hand.
//
// A capture that states no transfer at all narrows nothing, so the leading row stands.
// That is deliberate rather than a fallback: a standard-range surface is what caps carrying no
// transfer mean, and the parent leads its table with the standard-range row.

// colorimetryField is the caps field the narrowing reads and writes.
const colorimetryField = "colorimetry"

// narrowToSurface arranges for the pipeline's alternatives to be narrowed to the colour the capture
// produces, once, before the frames reach anything that would convert them.
//
// The hook is a probe on each source's own src pad, taken before the pipeline plays.
// A caps event passes that pad on its way downstream, so the narrowing lands ahead of the
// negotiation it is about rather than after an encoder has already been told a colour.
//
// Sources are found by shape, as everything else here finds them: an element with no sink pad is
// one producing frames from outside the pipeline, and naming the capture instead would put one
// string in this package and in whichever backend built the description.
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
			// One answer per run: what the capture negotiated does not change under a playing pipeline,
			// and a second narrowing would be the same edit twice.
			return gst.PadProbeRemove
		})
	}
}

// narrowFilters drops from every capsfilter the structures whose colorimetry names another transfer
// than the one the capture carries.
//
// A filter is left alone in three cases, all of which mean there is nothing to choose between:
// the capture states no transfer, the filter states one structure, and no structure matches.
// The last is what keeps this from producing an empty capsfilter, which would fail the run with a
// negotiation error in place of the refusal the parent makes on the same caps,
// in words that name both ends.
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
			assert.Assert(narrowed.GetSize() > 0, "a narrowed filter still states a format", transfer)
			assert.Assert(narrowed.GetSize() <= caps.GetSize(),
				"narrowing removes alternatives and adds none", narrowed.GetSize(), caps.GetSize())
			el.SetObjectProperty("caps", narrowed)
		}
	}
}

// matching is caps holding the structures whose colorimetry carries this transfer,
// and nil where none does.
//
// Each is copied with its own caps features, because the feature is what decides which pads can
// link at all: a structure that arrived naming device memory and left naming none would pin the
// frames back into the system-memory round trip the whole path exists to avoid.
func matching(caps *gst.Caps, transfer string) *gst.Caps {
	assert.IsNotNil(caps, "a narrowing reads the caps it narrows")
	assert.Assert(transfer != "", "a narrowing names the transfer it keeps")

	var out *gst.Caps
	for i := uint(0); i < caps.GetSize(); i++ {
		s := caps.GetStructure(i)
		if s == nil || colour.TransferOfColorimetry(s.GetString(colorimetryField)) != transfer {
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

// transferOf is the transfer characteristic the caps carry, and the empty string for caps that name
// none or name more than one structure.
// The capture produces one format, so a caps with several structures is one nothing has negotiated
// yet.
func transferOf(caps *gst.Caps) string {
	if caps == nil || caps.GetSize() != 1 {
		return ""
	}
	s := caps.GetStructure(0)
	if s == nil {
		return ""
	}
	return colour.TransferOfColorimetry(s.GetString(colorimetryField))
}
