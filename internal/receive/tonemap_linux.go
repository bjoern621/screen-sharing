package receive

// The rungs a Linux viewer rolls an HDR stream down with, in the order they are tried.
//
// vapostproc's hdr-tone-mapping asks the VA driver for its own tone-mapping filter, which
// fits the luminance the stream carries into the luminance a standard display shows. It is
// first because it is the one conversion here that silicon built for it performs, and
// because it is one element: vapostproc takes and hands back either VA memory or system
// memory, so it needs no upload of its own and links to whatever the chain after it begins
// with.
//
// Whether the driver has the filter is a different question from whether the element
// registers, and it is the question that decides this list. The element registers wherever a
// VA driver loads at all; hdr-tone-mapping is a property GStreamer adds only where the
// driver reports VAProcFilterHighDynamicRangeToneMapping. Mesa's radeonsi reports it on no
// generation, so on an AMD card the element is there and the property is not, and a rung
// chosen by looking the factory up would build a launch line the parser rejects. That is why
// a rung is probed by parsing it (tonemap.go, buildable) and why there is a second rung
// behind this one at all.
//
// The software converter is not among them and was measured rather than assumed.
// videoconvert gamma-mode=remap converts the curve, on every platform and with no rung at
// all, but it normalizes PQ against the format's ten thousand nits rather than the display's
// hundred, and a mid-grey PQ frame through it comes out at a fifth of the code value it went
// in at. A darker picture is not a tone map.
var toneMapRungs = []toneMapRung{
	{
		name:     "vapostproc",
		needs:    []string{"vapostproc"},
		elements: []string{"vapostproc hdr-tone-mapping=true"},
	},
	glToneMapRung,
}
