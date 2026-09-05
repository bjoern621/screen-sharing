package receive

import (
	"fmt"
	"slices"
	"strings"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// Colour is the degree to which a render chain describes the colour it produces.
//
// The axis the chains differ on.
// Each one that converts anything ends in RGBA at the sink,
// and what separates them is whether the caps saying so also describe what the conversion did.
type Colour string

const (
	// ColourStated takes in the colour the caps carry and puts out the colour the caps state.
	ColourStated Colour = "stated"
	// ColourDriverDecides converts through an API the caps do not describe,
	// leaving the pinned colorimetry a label rather than a guarantee.
	ColourDriverDecides Colour = "driver"
	// ColourUnstated converts nothing, leaving the frames to be interpreted by whoever draws them.
	ColourUnstated Colour = "unstated"
)

// DefaultChain is what a stream renders through where nothing chose a chain.
//
// The download is what it saves.
// Every decoded frame comes into system memory on the CPU chain to be converted and scaled there,
// which at 1440p144 in 4:4:4 costs the cores gigabytes a second per tile.
// A device chain converts on the GPU and hands the sink a texture the frame channel exports without
// ever reading it (share.go).
//
// Per platform, and the frame channel is why.
// A decoded frame arrives at the window as a shared handle and not as bytes,
// which only a chain leaving its frames in the memory this platform's handle names can produce.
// On Windows that is Direct3D 11 alone: the compositor imports a DXGI shared texture,
// and GStreamer's OpenGL there is WGL, whose textures the shell's ANGLE device cannot open.
// Elsewhere it is the GL chain,
// itself the only one that both leaves frames on the GPU and states the colour it produces.
//
// Measurement rather than device residency earns GL that place.
// Against the CPU chain, flat dark, flat bright and gradient content render bit-identical,
// and a saturated colour-bar frame parts by at most one code value per channel,
// averaging under half of one: shader float rounding against videoconvert's fixed point,
// with none of the shadow-heavy error a transfer-function mismatch leaves.
// Identical dark content is the whole of the evidence,
// washed-out shadows being what the pinned sRGB caps prevent.
//
// The Windows default states nothing of the kind,
// a trade the platform imposes rather than one taken here:
// GstD3D11Converter may hand the conversion to ID3D11VideoProcessor,
// whose configuration the caps do not describe.
// Stated colour on Windows means picking the CPU chain and paying the download,
// a choice the form offers and a fact the receive state reports.
//
// Fallback covers a machine that cannot run it (resolve), so this names the chain to prefer
// and not one every install has.
const DefaultChain = defaultChain

// unconvertedChain converts nothing.
// Named here so resolve knows the one chain it must not fall back to.
const unconvertedChain = "raw"

// chain is one render chain: the elements completing a stream's source fragment, and what they say
// about where the frames live and what colour they carry.
type chain struct {
	name string
	// label and tip are the wording of record:
	// the chain's name, what it does, and what it says about its colour.
	// Neither crosses the control API, the words on screen being the shell's
	// (api/proto/screenshare/v1/text.proto).
	// They sit here because the reasoning behind each row does,
	// and the shell's copy is written off them.
	label string
	tip   string
	// needs are the element factories this chain is built from past the ones every chain has.
	// A GStreamer registering none of them cannot run it, and resolve leaves it out.
	needs []string
	// elements are the fragments following the source, joined into the launch line with " ! ".
	elements []string
	// fitCaps is the template SetRenderSize writes into the chain's fit capsfilter, width then
	// height.
	//
	// The chain's memory feature rides in it.
	// Caps carrying none pin the frames into system memory,
	// so a device chain written with a bare video/x-raw here downloads every frame the moment a tile
	// reports the size it draws.
	// Empty where a chain names no fit capsfilter, which then renders at the size the source sends.
	fitCaps string
	// device is the memory feature the chain asks its converter for,
	// "" for one working in system memory.
	// Where the chain converts, and not necessarily what the sink takes:
	// a chain may convert on the GPU and hand the sink system memory afterwards.
	device string
	colour Colour
}

// The memory features the device chains work in.
const (
	glMemory    = "memory:GLMemory"
	d3d11Memory = "memory:D3D11Memory"
	d3d12Memory = "memory:D3D12Memory"
)

// chains are the render chains on offer, and the one place a launch line is written.
//
// All of them decode through decodebin,
// which picks depayloader, demuxer and decoder off the stream's caps backed by gst-libav,
// leaving a tile decoding whatever a native ffplay or mpv window decodes,
// HEVC 4:4:4 and RGB included.
// All of them end in the appsink the frame channel reads,
// behind a queue bounding the decode thread against the sink's.
//
// In every chain the element straight after decodebin takes video and no audio,
// which holds parse-launch's delayed linking off the audio pads: their caps never match,
// so the video pad alone joins the branch and an audio pad is given a branch of its own when it
// turns up (audio.go).
//
// Every converting chain pins square pixels at its format caps.
// The pool announces raster width and height alone and the window derives the picture's shape
// from them, while a scaler left free satisfies a size bound by bending the pixel aspect ratio
// and an encoder's SAR survives the decode:
// either way the raster's shape stops being the picture's, and the window draws it stretched.
// The pin makes the chain's own scaler resample instead, so the raster is the shape on screen.
//
// No CUDA chain exists.
// The nvcodec plugin registers cudaupload and cudadownload and stops there:
// cudaconvert and cudaconvertscale want nvrtc, so CUDA memory moves and does not convert,
// and moving frames without converting them is the unconverted chain plus a copy.
var chains = []chain{
	{
		name:  "cpu",
		label: "System memory (exact colour)",
		// Scale before convert is the cheaper order on the CPU.
		// The frame scales in the format the decoder produced,
		// 1.5 bytes a pixel for the 4:2:0 most streams arrive in,
		// and the conversion to 4 bytes a pixel then runs over the tile's pixel count rather than
		// the source's.
		//
		// The RGBA/sRGB filter is not optional.
		// Without it the sink takes whatever the decoder produced and the window drawing the frames
		// interprets them itself, mapping an unknown transfer function to BT.709 and lifting every
		// shadow of sRGB-encoded screen content.
		// Pinned to RGBA, videoconvert applies matrix and range alone (gamma-mode defaults to none)
		// and tags the result sRGB, the reading ffplay makes.
		// 4:4:4 and RGB streams keep full chroma, nothing on this path subsampling.
		tip:   "Scales and converts on the CPU, and states the colour it produces: videoconvert applies matrix and range only and tags the result sRGB. Every frame the decoder made on the GPU is downloaded first.",
		needs: []string{"videoscale", "videoconvert"},
		elements: []string{
			"videoscale n-threads=0",
			"capsfilter name=" + fitName + " caps=video/x-raw",
			"videoconvert n-threads=0",
			"video/x-raw,format=RGBA,colorimetry=sRGB,pixel-aspect-ratio=1/1",
		},
		fitCaps: "video/x-raw,width=[1,%d],height=[1,%d]",
		colour:  ColourStated,
	},
	{
		name:  "gl",
		label: "GPU, OpenGL (exact colour)",
		// Scale and convert in one pass over the frame on the GPU:
		// the CPU chain's scale-then-convert order buys nothing where the conversion is free.
		// glcolorscale only scales, so glcolorconvert makes the format ahead of it,
		// the two running in one GL context on one texture.
		//
		// glcolorconvert strips colorimetry in transform_caps, which is how it declares that it
		// converts it: matrix and range come from the input and no transfer function is applied,
		// as under videoconvert with gamma-mode=none.
		// The sRGB the filter pins is a statement about the conversion.
		//
		// Bounding nothing is the row's shape and not an omission.
		// A bound pays where a CPU conversion costs its output pixels,
		// converting at tile size rather than source size being most of what the cpu row saves.
		// On the GPU the conversion is cheap enough that a whole frame beats a renegotiation.
		// Writing the bound mid-stream is also what this chain cannot survive:
		// the reconfigure travels past glcolorscale to the decoder, which has no answer for it,
		// and the pipeline dies with not-negotiated.
		// The window scales the texture at draw time regardless, so the tile shows the same picture.
		tip:   "Uploads to the GPU and converts and scales there, handing the sink a GL texture, so no frame crosses the bus. States the colour it produces: the GL converter derives matrix and range from the input and applies no transfer function. Converts whole frames rather than tile-sized ones, which on the GPU is the cheaper of the two.",
		needs: []string{"glupload", "glcolorconvert", "glcolorscale"},
		elements: []string{
			"glupload",
			"glcolorconvert",
			"glcolorscale",
			"video/x-raw(" + glMemory + "),format=RGBA,colorimetry=sRGB,pixel-aspect-ratio=1/1",
		},
		device: glMemory,
		colour: ColourStated,
	},
	{
		name:  "d3d11",
		label: "GPU, Direct3D 11 (driver decides colour)",
		// The row ends on the device and downloads nothing.
		// appsink takes any memory, and a DXGI shared texture is exported from a Direct3D 11 resource
		// and from nothing else (docs/viewer-architecture.md, "The frame channel"),
		// so a download here would pull every frame into system memory for the exporter to push
		// straight back onto the same GPU.
		//
		// GstD3D11Converter may hand the conversion to ID3D11VideoProcessor,
		// configured through an API the caps do not describe.
		// What is pinned behind it is a label on the frames rather than a guarantee about how they
		// were made.
		tip:   "Uploads to the GPU and scales and converts with Direct3D 11, handing the sink a texture the window imports without a copy. The driver may convert through its video processor, so the colour it produces is labelled rather than guaranteed.",
		needs: []string{"d3d11upload", "d3d11convert"},
		elements: []string{
			"d3d11upload",
			"d3d11convert",
			"capsfilter name=" + fitName + " caps=video/x-raw(" + d3d11Memory + ")",
			"video/x-raw(" + d3d11Memory + "),format=RGBA,colorimetry=sRGB,pixel-aspect-ratio=1/1",
		},
		fitCaps: "video/x-raw(" + d3d11Memory + "),width=[1,%d],height=[1,%d]",
		device:  d3d11Memory,
		colour:  ColourDriverDecides,
	},
	{
		name:  "d3d12",
		label: "GPU, Direct3D 12 (driver decides colour)",
		// The D3D11 row's shape and the D3D11 row's reservation, on the later API:
		// it meets a decoder already producing D3D12 memory without the frames passing through D3D11
		// on the way.
		tip:   "Uploads to the GPU, scales and converts with Direct3D 12, then downloads to system memory. The driver may convert through its video processor, so the colour it produces is labelled rather than guaranteed.",
		needs: []string{"d3d12upload", "d3d12convert", "d3d12download"},
		elements: []string{
			"d3d12upload",
			"d3d12convert",
			"capsfilter name=" + fitName + " caps=video/x-raw(" + d3d12Memory + ")",
			"video/x-raw(" + d3d12Memory + "),format=RGBA,colorimetry=sRGB,pixel-aspect-ratio=1/1",
			"d3d12download",
		},
		fitCaps: "video/x-raw(" + d3d12Memory + "),width=[1,%d],height=[1,%d]",
		device:  d3d12Memory,
		colour:  ColourDriverDecides,
	},
	{
		name:  unconvertedChain,
		label: "No conversion (colour left to the window)",
		// Between decoder and sink nothing converts,
		// so nothing states a colour and the window drawing the frames reads them as it likes:
		// an unknown transfer function maps to BT.709,
		// which linearizes sRGB-encoded screen content with the wrong curve and lifts every shadow.
		//
		// Its one element converts nothing.
		// Any video in any memory is accepted, which passes the frames through untouched,
		// and audio is refused, which holds parse-launch's delayed linking off the audio pad:
		// with the queue first after decodebin, a stream whose audio pad appeared first would put
		// sound into the video branch and leave the video pad nowhere to go.
		//
		// Whatever size the tile draws, the render is at the size the source sends:
		// nothing in here can scale.
		tip: "Hands the decoded frames over untouched, in whatever format and memory the decoder produced. Nothing states a colour, so the window interprets the frames itself and maps an unknown transfer function to BT.709, which washes out dark content. It also renders at the source's size, since nothing in it scales.",
		elements: []string{
			"capsfilter caps=video/x-raw(ANY)",
		},
		colour: ColourUnstated,
	},
}

func init() {
	// The default serves a viewer told nothing about what to render with, so converting is its job:
	// stating no colour at all leaves the window mapping an unknown transfer function to BT.709,
	// a washed-out picture nobody asked for.
	// Whether it states an exact colour on top of that is the platform's answer and not the table's,
	// for the reason DefaultChain gives.
	//
	// Held against the table rather than against a resolution:
	// the chains this machine runs are the machine's business,
	// where the table's own claim is checkable anywhere.
	assert.Assert(chainNamed(DefaultChain).colour != ColourUnstated,
		"the default render chain converts what it renders", DefaultChain)
	// The frame channel is written against the default too,
	// so its frames have to end up somewhere a handle can name them.
	// Converting in system memory would cost every tile the download this table exists to avoid.
	assert.Assert(chainNamed(DefaultChain).device != "",
		"the default render chain keeps its frames on the device", DefaultChain)
}

// chainNamed is the table's row of that name.
// A name the table does not hold is one this package wrote itself and got wrong.
func chainNamed(name string) chain {
	for _, c := range chains {
		if c.name == name {
			return c
		}
	}
	assert.Never("a named render chain is one the table holds", name)
	return chain{}
}

// resolve takes the chain a name asks for, falling back where this machine cannot run it:
// to the default, then to the first chain that runs at all.
//
// While any other chain runs, the unconverted one is never fallen back to.
// It answers "show me the frames as they are", which nobody asks by accident.
func resolve(name string) chain {
	// What runs is the registry's answer, and the registry exists once the library is up.
	// Callers that resolve a chain before opening a pipeline are why this sits here and not in New.
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
	// Each converting chain wants an element this GStreamer does not register,
	// leaving the frames to reach the sink as they are:
	// a picture whose colour nobody states beats no picture.
	logger.Warnf("no render chain on this GStreamer converts anything, rendering the frames unconverted")
	return chainNamed(unconvertedChain)
}

// available is the named chain where the table holds it and this GStreamer runs it.
// Either refusal is logged, what plays afterwards not being what was asked for,
// and the receive state names what did.
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

// missing is the first factory the chain wants and this GStreamer does not register,
// "" for a chain that can run.
func (c chain) missing() string {
	return missingFactory(c.needs)
}

// missingFactory is the first of these factories this GStreamer does not register,
// "" where it registers all of them.
// One reading for both, a chain and a tone-map rung being offered on the same terms:
// what can be built is what the registry holds.
func missingFactory(needs []string) string {
	for _, need := range needs {
		if gst.ElementFactoryFind(need) == nil {
			return need
		}
	}
	return ""
}

// launch is the whole launch line: the stream's source fragment, the tone-map rung, the chain's
// elements, and the queue and sink every chain ends in.
//
// The rung arrives as an argument rather than being read here,
// and it is the resolved one rather than the requested one.
// A decode that asked for no tone mapping and a decode on a machine that cannot both hold the zero
// rung, which builds nothing, so what is written is what ran and never what was wanted.
//
// A raw stream hands over pictures instead of a bitstream, which takes the decoder out of the line.
// A screen read off this machine is that case: nothing encoded those frames,
// so there is nothing to autoplug a decoder for and the chain converts what the capture element
// produced.
//
// The rung sits between decoder and chain,
// where the frames still carry the range they were coded in.
// Past the chain's converter they are labelled sRGB,
// and a rolloff there would read its input off a label that has stopped describing the samples.
func (c chain) launch(st Stream, rung toneMapRung) string {
	parts := make([]string, 0, len(c.elements)+len(rung.elements)+4)
	parts = append(parts, st.Source)
	if !st.Raw {
		parts = append(parts, "decodebin name="+decodeName)
	}
	parts = append(parts, rung.elements...)
	parts = append(parts, c.elements...)
	parts = append(parts, renderQueue, renderSink)
	return strings.Join(parts, " ! ")
}

// fit is the caps bounding the chain to width x height,
// "" where it names no filter to write them into.
func (c chain) fit(width, height int) string {
	if c.fitCaps == "" {
		return ""
	}
	return fmt.Sprintf(c.fitCaps, width, height)
}

// Chain is one render chain as a form offers it:
// a name to ask it back by, and whether this machine runs it at all.
//
// Nothing above this package names GStreamer,
// so the elements a chain is built from stay here and only the reading of them crosses.
// Neither is what the chain is called or what it does:
// those are words, and words on screen belong to the shell.
type Chain struct {
	Name string
	// Available is whether this machine registers the elements the chain wants.
	// On a chain that cannot run, MissingElement names the first one it does not,
	// so an offer nobody can take says what is absent instead of only being refused.
	Available      bool
	MissingElement string
	// Default marks what a stream renders through where nothing chose a chain.
	//
	// Stated by the table rather than implied by position:
	// read off this slice's order, a default moves whenever the list is reordered for a picker's sake,
	// and the two facts part company with neither side changing.
	// One offer carries it.
	Default bool
}

// Chains is what this package offers a form, in the table's order,
// which ends with the unconverted chain.
func Chains() []Chain {
	// What runs is answered by the registry once the library is up,
	// and a form asks what it may offer before any stream is received.
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
	// The default a reader takes is the one named here, so it has to be among what was offered:
	// a default naming a row the table does not carry leaves every stream on whichever chain a reader
	// picked instead.
	assert.Assert(slices.ContainsFunc(out, func(c Chain) bool { return c.Default }),
		"an offered chain is the one rendered with by default", DefaultChain)
	return out
}
