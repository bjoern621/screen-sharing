package ffmpeg

import (
	"fmt"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/capabilities"
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
// The nvenc family has no entry, which is why no gpupath row pairs a grabber with it.
// scale_cuda is the only CUDA filter that converts a captured BGRA texture to the
// encoder's semi-planar layout, and it states no output matrix, primaries, transfer or
// range. A conversion that cannot say what it produced makes the stream's colour a
// property of the filter's internals, so the family stays on the system-memory path,
// where swscale converts by -color_range and setparams tags what it wrote.
var gpuConverts = map[string]gpuConvert{
	capabilities.FamilyVaapi: {device: "vaapi", scale: "scale_vaapi"},
	capabilities.FamilyQsv:   {device: "qsv", scale: "vpp_qsv"},
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
func GpuFilters(codec, chroma, colorRange string) ([]string, error) {
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

	format, ok := hwSurfaceFormats[chroma]
	if !ok {
		return nil, fmt.Errorf("chroma %q has no hardware surface layout", chroma)
	}
	scale := strings.Join([]string{
		convert.scale + "=format=" + format,
		"out_color_matrix=" + colourDescription,
		"out_color_primaries=" + colourDescription,
		"out_color_transfer=" + colourDescription,
		"out_range=" + colorRange,
	}, ":")
	return []string{"hwmap=derive_device=" + convert.device, scale}, nil
}
