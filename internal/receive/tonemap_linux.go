package receive

// The rung a Linux viewer rolls an HDR stream down with.
//
// vapostproc's hdr-tone-mapping asks the VA driver for its tone-mapping filter, which fits
// the luminance range the stream carries into the range a standard display shows. It is the
// only element within reach that does that, and the alternative was measured rather than
// assumed: the software converter's gamma-mode converts the curve alone, and it normalizes
// PQ against the format's ten thousand nits rather than the display's hundred, so a mid-grey
// PQ frame through it came out at a fifth of the code value it went in at. A picture that
// dark is not a tone map, and offering it as one would be a conversion that promises a
// correct picture and delivers a worse one than it replaced.
//
// The rung is one element because vapostproc takes and hands back either VA memory or system
// memory: it needs no upload of its own and links to whatever the chain after it begins
// with.
//
// Whether the driver has the filter is not the same question as whether the element
// registers, and the element is the one this side can ask. A driver without it converts
// nothing and the picture is the one that would have been drawn anyway, which is why the
// stream's own transfer characteristic is reported beside the choice rather than replaced by
// it.
var toneMapping = toneMapRung{
	needs:    []string{"vapostproc"},
	elements: []string{"vapostproc hdr-tone-mapping=true"},
}
