//go:build hardware

// The GPU chain tests, which need a DRM card node under /dev/dri to answer at all.
// `task backend:test` sets the tag, and CI leaves it off (.github/workflows/check.yml).

package ffmpeg

import (
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/gpupath"
)

// A chain that downloads or uploads has kept the round trip while claiming to have dropped it.
// -vaapi_device goes with them: hwmap derives the encoder's device from the captured frames,
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

// The conversion is the only place this path can state the colour description,
// no software stage being left for a setparams to tag.
// All four components have to reach it: a range named beside three unknown ones is dropped
// with them, and the stream then signals nothing and is watched in the viewer's own default.
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
		// This conversion states the colour itself, so a tag as well would put a second answer on frames
		// the filter already described.
		if strings.Contains(line, "setparams") {
			t.Errorf("the GPU path states its colour on the conversion, not on a tag: %s", line)
		}
	}
}

// Every pair without a row runs this one: the download, the tag, the conversion and the upload.
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

// Auto is what a stored stream carries,
// so the pair table alone decides which of the two commands it builds.
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

	// x11grab hands over system memory whatever the encoder can read,
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

// The DRM download strategy names the device a tiled scanout
// buffer is mapped through so hwdownload can read it.
// A run that downloads nothing chooses no such device, so the strategy must not reach the command.
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
