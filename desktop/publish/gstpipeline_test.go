package publish

import (
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/settings"
)

// Every pixel format the capability table leaves reachable on this engine has to
// name a raw format the capture chain can pin, or a portal publish fails after the
// UI offered the combination.
//
// The reverse is not a rule: the format map keys off the encoder family, so a chroma
// one element rejects can still name a layout the family's other elements take. What
// keeps that combination out of a pipeline is the gap, checked below.
func TestGstChromaFormatCoversTheEngineChromas(t *testing.T) {
	for _, c := range capabilities.Codecs {
		if !c.Implemented {
			continue
		}
		for _, chroma := range c.EngineChromas("gstreamer") {
			if _, err := gstChromaFormat(c.Name, chroma); err != nil {
				t.Errorf("%s: %v", c.Name, err)
			}
		}
	}
}

// A chroma this engine's encoder element cannot take is refused before a pipeline is
// built, rather than converted to the nearest format the element does negotiate. The
// default settings are exactly that case: they carry planar RGB, which the ffmpeg
// engine codes and no GStreamer element here does.
func TestBuildPipelineRejectsAGappedChroma(t *testing.T) {
	s := settings.Defaults()
	s.Capture = "portal"
	s.Transport = "srt"
	if s.Chroma != "gbrp" {
		t.Skipf("the default chroma is %s, no longer the gapped format this covers", s.Chroma)
	}

	_, err := buildPipeline(s, "3", "42", "")
	if err == nil {
		t.Fatal("a chroma gapped on this engine must not build a pipeline")
	}
	// The message is what the user sees when a settings file skips the form's repair,
	// so it names the format and the way to reach it.
	if !strings.Contains(err.Error(), "gbrp") || !strings.Contains(err.Error(), "ffmpeg") {
		t.Errorf("the rejection must name the format and the engine that codes it: %v", err)
	}
}
