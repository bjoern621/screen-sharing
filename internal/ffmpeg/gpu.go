package ffmpeg

import (
	"fmt"
	"strconv"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The GPU path's half of the publish command: a grabber that already holds its frames
// on the device hands them to the encoder without a download.
//
// It replaces three pieces of the system-memory command at once. The grabber's chain
// no longer ends in hwdownload, the colour tag is no longer a setparams on software
// frames, and no device is opened ahead of the input: the map derives the encoder's
// device from the captured frames, so both ends run on the GPU the capture came off
// and the command names neither.
//
// Which capture backends and encoder families pair up this way is gpupath.Paths. This
// file is the ffmpeg vocabulary for the rows that name this engine.

// gpuConvert is one encoder family's device-side conversion: the hardware device the
// captured frames are mapped onto, and the filter that converts them to the encoder's
// layout there.
type gpuConvert struct {
	// device is the hwmap derive_device, which is both the frames' new device and the
	// one the encoder takes from them.
	device string
	// scale converts the mapped frames to the encoder's layout and states the colour
	// it produced. A filter without those out_ options cannot be used here: the colour
	// description is what every viewer expands the picture by, and the GPU path has no
	// software stage left for a setparams to tag.
	scale string
}

// gpuConverts is the conversion per encoder family, keyed as capabilities.Codecs names
// the family, and this engine's half of the pairs gpupath.Paths declares. A family
// named in a row there and not here is a table half declared, which the builder
// asserts rather than mapping onto some other device.
//
// The nvenc entry names neither a device nor a filter, which is the whole of what this
// platform offers between a Windows grabber and that encoder. hwmap derives no CUDA and
// no Vulkan device from a Direct3D11 frame, answering ENOSYS, so scale_cuda and
// libplacebo are both unreachable however they state their colour; scale_d3d11 is
// reachable and cannot create an NV12 target from the captured BGRA. Nothing is left to
// stand between the two ends, so nvenc reads the texture on its own device and converts
// it itself, at a matrix and a range of its own choosing which it then signals. That is
// what makes its row a gpupath.ColourEncoder one, and why an empty entry here is a
// declared fact rather than a missing one.
var gpuConverts = map[string]gpuConvert{
	capabilities.FamilyVaapi: {device: "vaapi", scale: "scale_vaapi"},
	capabilities.FamilyQsv:   {device: "qsv", scale: "vpp_qsv"},
	capabilities.FamilyNvenc: {},
}

// GpuStatesColour reports whether this codec's family converts on the device and states
// the colour it produced, which is what decides whether the command's colour options
// reach anything on the GPU path.
//
// It answers false for a family with no entry as well as for one whose entry names no
// conversion: neither states a colour, and the caller's question is what the command
// should carry rather than whether the pair has a path at all (gpupath.Paths answers
// that).
func GpuStatesColour(codec string) bool {
	c, ok := capabilities.Get(codec)
	if !ok {
		return false
	}
	return gpuConverts[c.Family].scale != ""
}

// GpuFilters returns the filter chain mapping captured device frames onto the encoder's
// device and converting them there, for the codec, pixel format and colour range the
// settings name.
//
// The conversion states all four colour components, for the reason the GStreamer engine
// pins all four on its capsfilter: a range named beside three unknown components is
// dropped along with them, and the stream then signals nothing and is watched in the
// viewer's own default. out_range takes the settings value as it stands, since ffmpeg
// spells the two ranges pc and tv here exactly as the form does.
//
// A family whose entry names no conversion has no such chain, and the colour tag is the
// whole of what it contributes: the encoder converts the captured frames and signals the
// matrix and range it chose, so the primaries and the transfer are all this side still
// has to state. setparams accepts hardware frames, so the tag is placed the same way it
// is on the system path.
// The size is the device conversion's own business for the reason the layout and the
// colour are: the frames never come back to system memory, so the one filter on the path
// is the only thing that can resize them. A family whose entry names no conversion has no
// such filter, and a scaled run on it is refused here rather than published at the
// capture's size under a setting that says otherwise.
func GpuFilters(codec, chroma, colorRange string, size settings.Size, scaled bool) ([]string, error) {
	// Both are held against the codec's own table by capabilities.Validate before the
	// command is built, so an empty one is a caller that skipped it.
	assert.Assert(chroma != "", "a GPU-path encode names the pixel format it converts to")
	assert.Assert(colorRange != "", "a GPU-path encode names the colour range it converts to")

	c, ok := capabilities.Get(codec)
	if !ok {
		return nil, fmt.Errorf("unknown codec %q", codec)
	}
	convert, ok := gpuConverts[c.Family]
	// Only a pair gpupath.Paths carries reaches here, and every such row names a family
	// this table maps.
	assert.Assert(ok, "a GPU-path encode names a family whose device-side conversion is declared", c.Family)
	// The two halves of a conversion arrive together or not at all: a device with no
	// filter would map the frames and leave them in a layout the encoder cannot read,
	// and a filter with no device would convert on whichever device the frames are
	// already on rather than the encoder's.
	assert.Assert((convert.device == "") == (convert.scale == ""),
		"a device-side conversion names both the device it maps onto and the filter that converts there", c.Family)

	if convert.scale == "" {
		if scaled {
			return nil, fmt.Errorf(
				"codec %q reads the captured frames on its own device with no filter between, so nothing on that path can scale them to %s: publish at the capture's size, or pick the system-memory frame path",
				codec, size)
		}
		if colour := colourFilter(chroma); colour != "" {
			return []string{colour}, nil
		}
		return nil, nil
	}

	format, ok := hwSurfaceFormats[chroma]
	if !ok {
		return nil, fmt.Errorf("chroma %q has no hardware surface layout", chroma)
	}
	options := []string{convert.scale + "=format=" + format}
	// The size leads the colour options because it is what the filter is being asked to
	// produce; the four colour ones describe what it produced. Absent, the filter keeps
	// the size it was given, which is the unscaled run.
	if scaled {
		options = append(options,
			"w="+strconv.Itoa(size.Width),
			"h="+strconv.Itoa(size.Height))
	}
	options = append(options,
		"out_color_matrix="+colourDescription,
		"out_color_primaries="+colourDescription,
		"out_color_transfer="+colourDescription,
		"out_range="+colorRange)

	return []string{"hwmap=derive_device=" + convert.device, strings.Join(options, ":")}, nil
}
