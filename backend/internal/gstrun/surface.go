package gstrun

import (
	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/colour"
	"bjoernblessin.de/screenshare/internal/padprobe"
)

// Narrowing the encoder input.
//
// A capsfilter stating alternatives fixates to the first of them whatever the frames carry.
// That is the only answer a launcher can give, nothing in a launched pipeline knowing what
// the capture negotiated, and it is the wrong one for a publish: a surface with a transfer
// characteristic of its own is converted into the leading row, and the stream then carries a colour
// the desktop never had.
//
// So the alternatives are narrowed, before anything negotiates, to the ones whose colorimetry names
// the transfer the capture produces.
// No domain knowledge and no colour science here: which colours a publish accepts is the parent's
// table, written into the description it handed over, which of them the surface carries
// is the capture's own answer, and this is the one place both are in hand.
//
// A capture stating no transfer narrows nothing and the leading row stands.
// Not a fallback: caps carrying no transfer mean a standard-range surface, and the parent leads
// its table with the standard-range row.

// colorimetryField is the caps field the narrowing reads and writes.
const colorimetryField = "colorimetry"

// narrowToSurface narrows the pipeline's alternatives to the colour the capture produces, once,
// before the frames reach anything that would convert them.
//
// The hook is a probe on each source's own src pad, taken before the pipeline plays.
// A caps event crosses that pad on its way downstream, so the narrowing lands ahead
// of the negotiation it is about rather than after an encoder has been told a colour.
//
// Sources are found by shape, as everywhere else here: an element with no sink pad produces frames
// from outside the pipeline, and a name would live both here and in whichever backend built
// the description.
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
			event := padprobe.Event(info)
			if event == nil || event.GetType() != gst.EventCaps {
				return gst.PadProbeOK
			}
			narrowFilters(pipeline, transferOf(event.ParseCaps()))
			// One answer per run: what the capture negotiated does not change under a playing pipeline, and
			// a second narrowing is the same edit twice.
			return gst.PadProbeRemove
		})
	}
}

// narrowFilters drops from every capsfilter the structures whose colorimetry names a transfer other
// than the capture's.
//
// A filter is left alone where there is nothing to choose between: the capture states no transfer,
// the filter states one structure, or no structure matches.
// The last keeps this from producing an empty capsfilter, which would fail the run
// with a negotiation error in place of the refusal the parent makes on the same caps, naming both
// ends.
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

// matching is caps holding the structures whose colorimetry carries this transfer, nil where none
// does.
//
// Each structure is copied with its own caps features, the feature deciding which pads can link
// at all: one that arrived naming device memory and left naming none pins the frames back
// into the system-memory round trip the whole path exists to avoid.
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

// transferOf is the transfer characteristic these caps carry, empty for caps naming none or naming
// more than one structure.
// The capture produces one format, so several structures are caps nothing has negotiated.
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
