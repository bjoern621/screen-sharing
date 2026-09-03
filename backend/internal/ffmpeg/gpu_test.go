package ffmpeg

import (
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/settings"
)

// gpuStream is a draft publishing capture into codec over the device path.
func gpuStream(capture, codec string) settings.Settings {
	s := baseStream()
	s.Publish.Capture = capture
	s.Publish.UseCodec(codec)
	s.Publish.Chroma, s.Publish.Mode, s.Publish.ColorRange = "yuv420p", "cbr", "pc"
	s.Publish.CaptureMemory = gpupath.MemoryGpu
	return s
}

// A pair the table declares for this engine and the builder cannot build is a frame memory the form
// offers and the publish refuses.
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

// A family that converts nothing takes no steer: there is no swscale stage for -color_range to aim,
// and -pix_fmt would ask ffmpeg to convert a GPU surface it cannot read.
// The tag stays, the primaries and the transfer being the capture's whatever the encoder
// does with the matrix and the range.
//
// What the stream ends up carrying is the row's own claim (gpupath.Signalled),
// so a row that starts honouring the settings fails here and belongs at ColourExact.
func TestTheEncoderColourPathDropsTheOptionsItsEncoderIgnores(t *testing.T) {
	p, ok := gpupath.For(capabilities.EngineFfmpeg, "ddagrab", capabilities.FamilyNvenc)
	if !ok {
		t.Skip("no ffmpeg ddagrab/nvenc row to cover")
	}
	if p.Colour != gpupath.ColourEncoder {
		t.Skipf("the row is %s, so it does not cover the encoder-colour path", p.Colour)
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

// A demand the pair cannot meet is refused rather than quietly downloaded:
// the two commands differ by a full round trip per frame at capture resolution,
// the fact the setting exists to name.
func TestTheGpuDemandIsRefusedForAPairWithoutAPath(t *testing.T) {
	s := gpuStream("x11grab", "libx264")
	s.Publish.Chroma = "yuv444p"
	if _, err := BuildPublishArgs(s, nil); err == nil {
		t.Fatal("x11grab into a software encoder has no GPU path and must be refused")
	}
}

// The downloading path is where the strategy is read, so it is where a name no row carries
// has to be refused: drmMapFor answering the same on its own says
// nothing about the command reaching it.
// A capture that fell through to the driver's guess would run the setting as its own opposite.
func TestADownloadingCaptureRefusesAStrategyNoRowCarries(t *testing.T) {
	s := gpuStream("kmsgrab", "h264_vaapi")
	s.Publish.CaptureMemory = gpupath.MemorySystem
	s.Publish.DrmMap = "vaapi-with-a-typo"

	_, err := BuildPublishArgs(s, nil)
	if err == nil {
		t.Fatal("a DRM download strategy no row carries built a command")
	}
	// A machine with no DRM node refuses first and for its own reason, which is not what this covers.
	if !strings.Contains(err.Error(), "vaapi-with-a-typo") {
		t.Skipf("this machine refuses kmsgrab before the strategy is read: %v", err)
	}
}

// firstCodecOfFamily returns an implemented codec of family,
// false where the capability table carries none.
func firstCodecOfFamily(family string) (string, bool) {
	for _, c := range capabilities.Codecs {
		if c.Family == family && c.Implemented {
			return c.Name, true
		}
	}
	return "", false
}

// The frames never come back to system memory, so the one filter on the path is the only thing
// that can resize them, and it takes the size beside the layout and the colour.
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

// A filter told to produce the size it was given is a size the command claims to have chosen.
func TestAnUnscaledDeviceConversionStatesNoSize(t *testing.T) {
	filters, err := GpuFilters("hevc_vaapi", "yuv420p", "pc", settings.Size{}, false)
	if err != nil {
		t.Fatalf("GpuFilters = %v, want a device chain", err)
	}

	if chain := strings.Join(filters, ","); strings.Contains(chain, "w=") || strings.Contains(chain, "h=") {
		t.Errorf("the device chain %q carries a size for a run that scales nothing", chain)
	}
}

// A family whose device path carries no conversion has nothing on it that resizes,
// the encoder reading the captured surfaces directly.
// The refusal names the size that was asked for, rather than the run publishing at the capture's
// size under a setting that says otherwise.
func TestAPathWithNoConversionRefusesAScaledRun(t *testing.T) {
	_, err := GpuFilters("hevc_nvenc", "yuv420p", "pc", settings.Size{Width: 1280, Height: 720}, true)
	if err == nil {
		t.Fatal("a scaled run was accepted on a device path with no filter on it")
	}
	if !strings.Contains(err.Error(), "1280x720") {
		t.Errorf("the refusal reads %q, which does not name the size that was asked for", err)
	}
}
