package ffmpeg

import (
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/settings"
)

// gpuStream returns settings publishing the pair over the direct path.
func gpuStream(capture, codec string) settings.Settings {
	s := baseStream()
	s.Publish.Capture, s.Publish.Codec = capture, codec
	s.Publish.Chroma, s.Publish.Mode, s.Publish.ColorRange = "yuv420p", "cbr", "pc"
	s.Publish.CaptureMemory = gpupath.MemoryGpu
	return s
}

// Every pair the table declares for this engine has to build a command, or the form offers a frame
// memory the builder then refuses.
func TestEveryFfmpegGpuPathBuildsAChain(t *testing.T) {
	for _, p := range gpupath.Paths {
		if p.Engine != capabilities.EngineFfmpeg {
			continue
		}
		codec, ok := firstCodecOfFamily(p.Family)
		if !ok {
			t.Errorf("%s/%s: the family has no implemented codec to publish it with", p.Capture, p.Family)
			continue
		}
		if _, err := GpuFilters(codec, "yuv420p", "pc", settings.Size{}, false); err != nil {
			t.Errorf("%s/%s: %v", p.Capture, p.Family, err)
		}
	}
}

// A path whose family converts nothing states no colour on the way in, and the two options that
// would have steered a conversion are dropped with it: there is no swscale stage for -color_range
// to aim, and -pix_fmt would ask ffmpeg to convert a GPU surface it cannot read.
// What the command still carries is the tag, since the primaries and the transfer are the capture's
// whatever the encoder does with the matrix and the range.
//
// The values the stream ends up with are the row's own claim (gpupath.Signalled),
// so a row that starts honouring the settings fails here and gets promoted to ColourExact.
func TestTheEncoderColourPathDropsTheOptionsItsEncoderIgnores(t *testing.T) {
	p, ok := gpupath.For(capabilities.EngineFfmpeg, "ddagrab", capabilities.FamilyNvenc)
	if !ok {
		t.Skip("no ffmpeg ddagrab/nvenc row to cover")
	}
	if p.Colour != gpupath.ColourEncoder {
		t.Skipf("the row is %s, so it no longer covers the encoder-colour path", p.Colour)
	}

	s := gpuStream("ddagrab", "h264_nvenc")
	s.Publish.CaptureMemory = gpupath.MemoryGpuEncoderColor
	args, err := BuildPublishArgs(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(args, " ")
	for _, forbidden := range []string{"hwdownload", "hwupload", "-pix_fmt", "-color_range"} {
		if strings.Contains(line, forbidden) {
			t.Errorf("the encoder-colour path must not carry %s, which its encoder ignores: %s", forbidden, line)
		}
	}
	if !strings.Contains(line, "setparams") {
		t.Errorf("the primaries and the transfer are still this side's to state: %s", line)
	}
	if GpuStatesColour("h264_nvenc") {
		t.Error("the nvenc family states no device-side colour, so GpuStatesColour must say so")
	}
}

// The whole point of the path is that no frame crosses the bus, so a chain that still downloads or
// uploads one has kept the round trip while claiming to have dropped it.
// The device option goes with them: the map derives the encoder's device from the captured frames,
// and naming a second one would open a GPU the frames are not on.
func TestTheGpuPathNeitherDownloadsNorUploads(t *testing.T) {
	args, err := BuildPublishArgs(gpuStream("kmsgrab", "h264_vaapi"), nil)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(args, " ")
	for _, forbidden := range []string{"hwdownload", "hwupload", "-vaapi_device", "-pix_fmt"} {
		if strings.Contains(line, forbidden) {
			t.Errorf("the GPU path must not carry %s: %s", forbidden, line)
		}
	}
	if !strings.Contains(line, "hwmap=derive_device=vaapi") {
		t.Errorf("the GPU path must map the scanout buffer onto the encoder's device: %s", line)
	}
}

// The conversion is the only place the colour description can be stated on this path,
// because there is no software stage left for a setparams to tag.
// All four components have to reach it: a range named beside three unknown ones is dropped with
// them, and the stream then signals nothing and is watched in the viewer's own default.
func TestTheGpuPathStatesEveryColourComponentOnTheConversion(t *testing.T) {
	for _, colorRange := range []string{"pc", "tv"} {
		s := gpuStream("kmsgrab", "h264_vaapi")
		s.Publish.ColorRange = colorRange
		args, err := BuildPublishArgs(s, nil)
		if err != nil {
			t.Fatal(err)
		}
		line := strings.Join(args, " ")
		for _, want := range []string{
			"scale_vaapi=format=nv12",
			"out_color_matrix=" + colourDescription,
			"out_color_primaries=" + colourDescription,
			"out_color_transfer=" + colourDescription,
			"out_range=" + colorRange,
		} {
			if !strings.Contains(line, want) {
				t.Errorf("colour range %s: the conversion lacks %q: %s", colorRange, want, line)
			}
		}
		// setparams tags software frames ahead of a conversion that honours -color_range.
		// Here the conversion states the colour itself, and tagging as well would put a second answer on
		// frames the filter already described.
		if strings.Contains(line, "setparams") {
			t.Errorf("the GPU path states its colour on the conversion, not on a tag: %s", line)
		}
	}
}

// The system-memory path is what every pair without a row runs, and it has to keep working exactly
// as it did: the download, the tag, the conversion and the upload.
func TestTheSystemPathStillMakesTheRoundTrip(t *testing.T) {
	s := gpuStream("kmsgrab", "h264_vaapi")
	s.Publish.CaptureMemory = gpupath.MemorySystem
	args, err := BuildPublishArgs(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(args, " ")
	for _, want := range []string{"hwdownload", "format=bgr0", "setparams", "format=nv12", "hwupload", "-vaapi_device"} {
		if !strings.Contains(line, want) {
			t.Errorf("the system-memory path lacks %q: %s", want, line)
		}
	}
}

// Auto is the setting a stored stream carries, so the pair table alone has to decide which of the
// two commands it builds.
// A pair with a row that still downloaded would make the default the slow path on every machine.
func TestAutoBuildsTheGpuChainForAPairWithAPath(t *testing.T) {
	s := gpuStream("kmsgrab", "h264_vaapi")
	s.Publish.CaptureMemory = gpupath.MemoryAuto
	args, err := BuildPublishArgs(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if line := strings.Join(args, " "); strings.Contains(line, "hwdownload") {
		t.Errorf("auto must take the direct path where the pair has one: %s", line)
	}

	// And the copy where it has none: x11grab hands over system memory whatever the encoder can read,
	// so the same codec downloads there.
	s.Publish.Capture = "x11grab"
	args, err = BuildPublishArgs(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if line := strings.Join(args, " "); !strings.Contains(line, "hwupload") {
		t.Errorf("auto must copy where the pair has no direct path: %s", line)
	}
}

// A demand the pair cannot meet is refused rather than quietly downloaded.
// The two commands differ by a full round trip per frame at capture resolution,
// which is the difference the setting exists to name.
func TestTheGpuDemandIsRefusedForAPairWithoutAPath(t *testing.T) {
	s := gpuStream("x11grab", "libx264")
	s.Publish.Chroma = "yuv444p"
	if _, err := BuildPublishArgs(s, nil); err == nil {
		t.Fatal("x11grab into a software encoder has no GPU path and must be refused")
	}
}

// The DRM download strategy names the device a tiled scanout buffer is mapped through so hwdownload
// can read it.
// A run that downloads nothing chooses no such device, so the strategy must not reach the command,
// and a value the table does not carry must not fail a run that never reads it.
func TestTheGpuPathReadsNoDrmDownloadStrategy(t *testing.T) {
	s := gpuStream("kmsgrab", "h264_vaapi")
	s.Publish.DrmMap = "vulkan"
	args, err := BuildPublishArgs(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if line := strings.Join(args, " "); strings.Contains(line, "derive_device=vulkan") {
		t.Errorf("the GPU path maps onto the encoder's device, not the download strategy's: %s", line)
	}
}

// firstCodecOfFamily returns an implemented codec of the family, and false when the capability
// table carries none.
func firstCodecOfFamily(family string) (string, bool) {
	for _, c := range capabilities.Codecs {
		if c.Family == family && c.Implemented {
			return c.Name, true
		}
	}
	return "", false
}

// A scaled run on the device path is the conversion filter's own business:
// the frames never come back to system memory, so the one filter on the path is the only thing that
// can resize them, and it takes the size beside the layout and the colour.
func TestTheDeviceConversionCarriesTheOutputSize(t *testing.T) {
	filters, err := GpuFilters("hevc_vaapi", "yuv420p", "pc", settings.Size{Width: 1280, Height: 720}, true)
	if err != nil {
		t.Fatalf("GpuFilters = %v, want a scaled device chain", err)
	}

	chain := strings.Join(filters, ",")
	for _, want := range []string{"w=1280", "h=720"} {
		if !strings.Contains(chain, want) {
			t.Errorf("the device chain %q does not carry %s", chain, want)
		}
	}
}

// An unscaled run states no size at all rather than restating the capture's:
// a filter told to produce the size it was given is a size the command claims to have chosen.
func TestAnUnscaledDeviceConversionStatesNoSize(t *testing.T) {
	filters, err := GpuFilters("hevc_vaapi", "yuv420p", "pc", settings.Size{}, false)
	if err != nil {
		t.Fatalf("GpuFilters = %v, want a device chain", err)
	}

	if chain := strings.Join(filters, ","); strings.Contains(chain, "w=") || strings.Contains(chain, "h=") {
		t.Errorf("the device chain %q carries a size for a run that scales nothing", chain)
	}
}

// A family whose device path carries no conversion has nothing on it that resizes:
// the encoder reads the captured surfaces directly.
// The run is refused with the way across named, rather than published at the capture's size under a
// setting that says otherwise.
func TestAPathWithNoConversionRefusesAScaledRun(t *testing.T) {
	_, err := GpuFilters("hevc_nvenc", "yuv420p", "pc", settings.Size{Width: 1280, Height: 720}, true)
	if err == nil {
		t.Fatal("a scaled run was accepted on a device path with no filter on it")
	}
	if !strings.Contains(err.Error(), "1280x720") {
		t.Errorf("the refusal reads %q, which does not name the size that was asked for", err)
	}
}
