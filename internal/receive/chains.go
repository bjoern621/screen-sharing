package receive

import (
	"fmt"
	"slices"
	"strings"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// Colour is how much a render chain says about the colour it produces.
//
// It is the axis on which the chains genuinely differ. Every one of them that
// converts anything ends in RGBA at the sink; what separates them is whether the
// caps that say so are also a description of what the conversion did.
type Colour string

const (
	// ColourStated is a chain whose conversion takes the colour the caps carry and
	// produces the colour the caps state.
	ColourStated Colour = "stated"
	// ColourDriverDecides is a chain that converts through an API the caps do not
	// describe, so the colorimetry it is pinned to is a label rather than a
	// guarantee.
	ColourDriverDecides Colour = "driver"
	// ColourUnstated is a chain that converts nothing, leaving the interpretation
	// of the frames to whoever draws them.
	ColourUnstated Colour = "unstated"
)

// DefaultChain is the chain a stream renders through when nothing chose one.
//
// What it saves is the download. The CPU chain pulls every decoded frame into
// system memory and converts and scales it there, which at 1440p144 in 4:4:4 is
// gigabytes a second per tile against the cores; a device chain converts on the GPU
// and hands the sink a texture the frame channel exports without ever reading it
// (share.go).
//
// It is per platform, and the frame channel is why. A decoded frame reaches the
// window as a shared handle rather than as bytes, and only a chain that leaves its
// frames in the memory this platform's handle names can produce one. On Windows that
// is Direct3D 11 and nothing else: the compositor imports a DXGI shared texture, and
// GStreamer's OpenGL there is WGL, whose textures the shell's ANGLE device cannot
// open. Everywhere else it is the GL chain, which is also the one chain that both
// keeps frames on the GPU and states the colour it produces.
//
// The GL chain earns that place by measurement rather than by keeping frames on the
// GPU: rendered through it and through the CPU chain, flat dark, flat bright and
// gradient content come out bit-identical, and a saturated colour-bar frame differs
// by at most one code value per channel with an average under half of one - shader
// float rounding against videoconvert's fixed point, and no trace of the
// shadow-heavy error a transfer-function mismatch produces. Dark content being
// identical is the whole of the evidence, since washed-out shadows are the failure
// the pinned sRGB caps exist to prevent.
//
// The Windows default states no such thing, and that is the trade the platform
// imposes rather than one taken here: GstD3D11Converter may pass the conversion to
// ID3D11VideoProcessor, whose configuration the caps do not describe. A reader who
// wants the colour stated on Windows picks the CPU chain and pays the download,
// which is a choice the form offers and a fact the receive state reports.
//
// A machine that cannot run it falls back (resolve), so this names the chain to
// prefer rather than one every install has.
const DefaultChain = defaultChain

// unconvertedChain is the chain that converts nothing. It is named here because
// resolve has to know which chain it must not fall back to.
const unconvertedChain = "raw"

// chain is one render chain: the elements a stream's source fragment is completed
// with, and what those elements say about where the frames live and what colour
// they carry.
type chain struct {
	name string
	// label and tip are the wording of record: what the chain is called and what it does
	// and says about its colour. Neither crosses the control API, because the words on
	// screen are the shell's (api/proto/screenshare/v1/text.proto) - they are here
	// because this is where the reasoning behind each row lives, and the shell's copy is
	// written from them.
	label string
	tip   string
	// needs are the element factories this chain is built from beyond the ones
	// every chain has. A GStreamer that registers none of them cannot run the
	// chain, and resolve leaves it out.
	needs []string
	// elements are the launch-line fragments after the source, joined with " ! ".
	elements []string
	// fitCaps is the template SetRenderSize writes into the chain's fit
	// capsfilter, taking width then height.
	//
	// It carries the chain's memory feature. Caps without one pin the frames into
	// system memory, so a device chain written with a bare video/x-raw here
	// downloads every frame at the moment a tile reports the size it draws.
	// Empty on a chain that names no fit capsfilter, which then renders at the size
	// the source sends.
	fitCaps string
	// device is the memory feature the chain asks its converter for, "" for a chain
	// that works in system memory. It is where the chain converts and not
	// necessarily what the sink takes: a chain that converts on the GPU can hand
	// the sink system memory afterwards.
	device string
	colour Colour
}

// The memory features the chains ask for.
const (
	glMemory    = "memory:GLMemory"
	d3d11Memory = "memory:D3D11Memory"
	d3d12Memory = "memory:D3D12Memory"
)

// chains are the render chains this package offers, and the only place a launch
// line is written.
//
// Every one of them decodes through decodebin, which picks the depayloader,
// demuxer and decoder from the stream's caps backed by gst-libav, so a tile
// decodes everything a native ffplay or mpv window decodes, HEVC 4:4:4 and RGB
// included. Every one ends in the appsink the frame channel reads, with a queue in
// front of it that bounds the decode thread against the sink's.
//
// The element directly after decodebin takes video and no audio in every chain,
// which is what keeps parse-launch's delayed linking off the audio pads: their
// caps never match, so only the video pad joins the branch, and an audio pad gets
// its own branch built when it appears (audio.go).
//
// There is no CUDA chain. The nvcodec plugin registers cudaupload and
// cudadownload but neither cudaconvert nor cudaconvertscale, which need nvrtc, so
// CUDA memory can be moved and not converted, and a chain that moves frames
// without converting them is the unconverted chain with an extra copy.
var chains = []chain{
	{
		name:  "cpu",
		label: "System memory (exact colour)",
		// The scaler sits ahead of the conversion because that is the cheaper order
		// on the CPU. A frame is scaled in the format the decoder produced, 1.5 bytes
		// a pixel for the 4:2:0 most streams arrive in, and the conversion to 4 bytes
		// a pixel then runs on the tile's pixel count instead of the source's.
		//
		// The RGBA/sRGB filter is not optional. Without it the sink takes whatever
		// the decoder produced and the window that draws the frames interprets them
		// itself, mapping an unknown transfer function to BT.709 and lifting every
		// shadow of sRGB-encoded screen content. Pinned to RGBA, videoconvert applies
		// matrix and range only (gamma-mode defaults to none) and tags the result
		// sRGB, the same interpretation ffplay uses. 4:4:4 and RGB streams keep full
		// chroma; nothing on this path subsamples.
		tip:   "Scales and converts on the CPU, and states the colour it produces: videoconvert applies matrix and range only and tags the result sRGB. Every frame the decoder made on the GPU is downloaded first.",
		needs: []string{"videoscale", "videoconvert"},
		elements: []string{
			"videoscale n-threads=0",
			"capsfilter name=" + fitName + " caps=video/x-raw",
			"videoconvert n-threads=0",
			"video/x-raw,format=RGBA,colorimetry=sRGB",
		},
		fitCaps: "video/x-raw,width=[1,%d],height=[1,%d]",
		colour:  ColourStated,
	},
	{
		name:  "gl",
		label: "GPU, OpenGL (exact colour)",
		// One pass over the frame on the GPU scales it and converts it: the CPU
		// chain's scale-then-convert order buys nothing where the conversion costs
		// nothing. glcolorscale scales only, so glcolorconvert makes the format ahead
		// of it; the two run in the same GL context on the same texture.
		//
		// glcolorconvert strips colorimetry in transform_caps, which is the declaration
		// that it converts it: it derives matrix and range from the input and applies
		// no transfer function, the same thing videoconvert with gamma-mode=none does.
		// So the sRGB the filter pins is a statement about the conversion.
		//
		// It bounds nothing, and that is the row's shape rather than an omission. The
		// bound exists because a CPU conversion costs its output pixels, so converting
		// at the tile's size instead of the source's is most of what the cpu row saves;
		// a conversion on the GPU costs little enough that the whole frame is cheaper
		// than the renegotiation. Writing the bound mid-stream is also what this chain
		// cannot survive: the reconfigure travels past glcolorscale to the decoder,
		// which cannot answer it, and the pipeline dies with not-negotiated. A window
		// scales the texture at draw time either way, so the tile shows the same
		// picture.
		tip:   "Uploads to the GPU and converts and scales there, handing the sink a GL texture, so no frame crosses the bus. States the colour it produces: the GL converter derives matrix and range from the input and applies no transfer function. Converts whole frames rather than tile-sized ones, which on the GPU is the cheaper of the two.",
		needs: []string{"glupload", "glcolorconvert", "glcolorscale"},
		elements: []string{
			"glupload",
			"glcolorconvert",
			"glcolorscale",
			"video/x-raw(" + glMemory + "),format=RGBA,colorimetry=sRGB",
		},
		device: glMemory,
		colour: ColourStated,
	},
	{
		name:  "d3d11",
		label: "GPU, Direct3D 11 (driver decides colour)",
		// The download this row used to end in is gone, and the frame channel is what
		// removed it. The row was written for the native grid, whose sink was
		// gtk4paintablesink and negotiated GL memory or system memory and no D3D
		// memory at all; appsink takes any memory, and a DXGI shared texture is
		// exported from a Direct3D 11 resource and from nothing else
		// (docs/viewer-architecture.md, "The frame channel"). Downloading here would
		// mean pulling every frame into system memory so that the exporter could push
		// it straight back onto the same GPU.
		//
		// GstD3D11Converter may pass the conversion to ID3D11VideoProcessor, which is
		// configured through an API the caps do not describe. The colorimetry pinned
		// behind it is therefore a label on the frames rather than a guarantee about
		// how they were made.
		tip:   "Uploads to the GPU and scales and converts with Direct3D 11, handing the sink a texture the window imports without a copy. The driver may convert through its video processor, so the colour it produces is labelled rather than guaranteed.",
		needs: []string{"d3d11upload", "d3d11convert"},
		elements: []string{
			"d3d11upload",
			"d3d11convert",
			"capsfilter name=" + fitName + " caps=video/x-raw(" + d3d11Memory + ")",
			"video/x-raw(" + d3d11Memory + "),format=RGBA,colorimetry=sRGB",
		},
		fitCaps: "video/x-raw(" + d3d11Memory + "),width=[1,%d],height=[1,%d]",
		device:  d3d11Memory,
		colour:  ColourDriverDecides,
	},
	{
		name:  "d3d12",
		label: "GPU, Direct3D 12 (driver decides colour)",
		// The same shape and the same reservation as the D3D11 chain, on the newer
		// API: it reaches a decoder that already produces D3D12 memory without the
		// frames passing through D3D11 on the way.
		tip:   "Uploads to the GPU, scales and converts with Direct3D 12, then downloads to system memory. The driver may convert through its video processor, so the colour it produces is labelled rather than guaranteed.",
		needs: []string{"d3d12upload", "d3d12convert", "d3d12download"},
		elements: []string{
			"d3d12upload",
			"d3d12convert",
			"capsfilter name=" + fitName + " caps=video/x-raw(" + d3d12Memory + ")",
			"video/x-raw(" + d3d12Memory + "),format=RGBA,colorimetry=sRGB",
			"d3d12download",
		},
		fitCaps: "video/x-raw(" + d3d12Memory + "),width=[1,%d],height=[1,%d]",
		device:  d3d12Memory,
		colour:  ColourDriverDecides,
	},
	{
		name:  unconvertedChain,
		label: "No conversion (colour left to the window)",
		// Nothing converts between the decoder and the sink, so nothing states a
		// colour and the window that draws the frames interprets them itself: it maps
		// an unknown transfer function to BT.709, which linearizes sRGB-encoded screen
		// content with the wrong curve and lifts every shadow.
		//
		// The one element in it converts nothing. It accepts any video in any memory,
		// which passes the frames through as they are, and refuses audio, which is what
		// keeps parse-launch's delayed linking off the audio pad: with the queue as the
		// first thing after decodebin, a stream whose audio pad appears first would put
		// sound into the video branch and leave the video pad with nowhere to go.
		//
		// It renders at the size the source sends, whatever size the tile draws:
		// there is nothing in it that could scale.
		tip: "Hands the decoded frames over untouched, in whatever format and memory the decoder produced. Nothing states a colour, so the window interprets the frames itself and maps an unknown transfer function to BT.709, which washes out dark content. It also renders at the source's size, since nothing in it scales.",
		elements: []string{
			"capsfilter caps=video/x-raw(ANY)",
		},
		colour: ColourUnstated,
	},
}

func init() {
	// The default is the chain of a viewer that has been told nothing about what to
	// render with, so it has to convert: a chain that states no colour at all leaves
	// the window mapping an unknown transfer function to BT.709, which is a washed-out
	// picture nobody asked for. Whether it also states an exact colour is the
	// platform's answer rather than the table's, for the reason DefaultChain gives.
	//
	// Checked against the table rather than against a resolution: which chains this
	// machine can run is the machine's business, and the table's own claim is
	// checkable anywhere.
	assert.Assert(chainNamed(DefaultChain).colour != ColourUnstated,
		"the default render chain converts what it renders", DefaultChain)
	// The default is also the chain the frame channel is written against, so it has to
	// leave its frames somewhere a handle can name them. A default that converted in
	// system memory would be a viewer whose every tile costs a download the chain
	// table exists to avoid.
	assert.Assert(chainNamed(DefaultChain).device != "",
		"the default render chain keeps its frames on the device", DefaultChain)
}

// chainNamed is the table's row of that name. A name that is not in the table is
// a name this package wrote itself and got wrong.
func chainNamed(name string) chain {
	for _, c := range chains {
		if c.name == name {
			return c
		}
	}
	assert.Never("a named render chain is one the table holds", name)
	return chain{}
}

// resolve picks the chain a name asks for, and falls back where this machine
// cannot run it: to the default, then to the first chain that runs at all.
//
// The unconverted chain is never fallen back to while another one runs. It is the
// answer to "show me the frames as they are", which is a question nobody asks by
// accident.
func resolve(name string) chain {
	// The registry decides what runs, and it exists after the library is up. A
	// caller that resolves a chain before opening a pipeline is the reason this
	// sits here rather than in New.
	initGStreamer()

	if c, ok := available(name); ok {
		return c
	}
	if name != DefaultChain {
		if c, ok := available(DefaultChain); ok {
			return c
		}
	}
	for _, c := range chains {
		if c.colour == ColourUnstated || c.missing() != "" {
			continue
		}
		logger.Warnf("rendering through the %q chain instead", c.name)
		return c
	}
	// Every chain that converts anything needs an element this GStreamer does not
	// register, so the frames reach the sink as they are: a picture whose colour
	// nobody states beats no picture.
	logger.Warnf("no render chain on this GStreamer converts anything, rendering the frames unconverted")
	return chainNamed(unconvertedChain)
}

// available is the named chain, when the table holds it and this GStreamer runs
// it. Both refusals are logged: the chain that then plays is not the one that was
// asked for, and the receive state names the one that did.
func available(name string) (chain, bool) {
	if name == "" {
		return chain{}, false
	}
	for _, c := range chains {
		if c.name != name {
			continue
		}
		if missing := c.missing(); missing != "" {
			logger.Warnf("the %q render chain needs a %s element this GStreamer does not register", name, missing)
			return chain{}, false
		}
		return c, true
	}
	logger.Warnf("no render chain is named %q", name)
	return chain{}, false
}

// missing names the first element factory the chain needs and this GStreamer does
// not register, and "" for a chain that can run.
func (c chain) missing() string {
	for _, need := range c.needs {
		if gst.ElementFactoryFind(need) == nil {
			return need
		}
	}
	return ""
}

// launch is the whole launch line: the stream's own source fragment, the chain's
// elements, and the queue and sink every chain ends in.
// raw is a source that hands over pictures rather than a bitstream, which is what
// takes the decoder out of the line. A screen read off this machine is the case that
// exists: nothing encoded those frames, so there is nothing to autoplug a decoder for
// and the chain converts what the capture element already produced.
func (c chain) launch(source string, raw bool) string {
	parts := make([]string, 0, len(c.elements)+4)
	parts = append(parts, source)
	if !raw {
		parts = append(parts, "decodebin name="+decodeName)
	}
	parts = append(parts, c.elements...)
	parts = append(parts, renderQueue, renderSink)
	return strings.Join(parts, " ! ")
}

// fit is the caps that bound the chain to width x height, and "" for a chain with
// no filter to write them into.
func (c chain) fit(width, height int) string {
	if c.fitCaps == "" {
		return ""
	}
	return fmt.Sprintf(c.fitCaps, width, height)
}

// Chain is one render chain as a form offers it: a name to ask for it back by, and
// whether this machine can run it at all.
//
// Nothing above this package names GStreamer, so the elements a chain is built from
// stay here and only the reading of them crosses. What the chain is called and what it
// does are not here either: those are words, and the words on screen are the shell's.
type Chain struct {
	Name string
	// Available is whether this machine registers the elements the chain needs.
	// MissingElement names the first one it does not on a chain that cannot run, so an
	// offer that cannot be taken says what is missing instead of just being refused.
	Available      bool
	MissingElement string
	// Default marks the chain a stream renders through when nothing chose one.
	//
	// The table states it rather than implying it by position: a default read off
	// the order of this slice is a default that moves whenever the list is
	// reordered for a picker's sake, and the two facts then disagree without either
	// side changing. Exactly one offer carries it.
	Default bool
}

// Chains is what this package offers the form, in the table's order: the default
// first, then the device chains, then the unconverted one.
func Chains() []Chain {
	// The registry answers what runs only once the library is up, and the form can
	// ask what it may offer before a stream is ever received.
	initGStreamer()

	out := make([]Chain, 0, len(chains))
	for _, c := range chains {
		missing := c.missing()
		out = append(out, Chain{
			Name:           c.name,
			Available:      missing == "",
			MissingElement: missing,
			Default:        c.name == DefaultChain,
		})
	}
	// The offer a reader takes as the default is the one this side named, so it has to
	// be in what was offered: a default naming a row the table does not carry would
	// leave every stream on whichever chain a reader picked instead.
	assert.Assert(slices.ContainsFunc(out, func(c Chain) bool { return c.Default }),
		"an offered chain is the one rendered with by default", DefaultChain)
	return out
}
