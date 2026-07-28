package publish

import (
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/settings"
	"bjoernblessin.de/screenshare/transport"
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

// The colour-range setting has to reach the frames, not just the caps. It only
// does so as part of a fully named colorimetry: with matrix, transfer and
// primaries left unknown, videoconvert ignores the range too and converts to
// limited range either way, which makes the setting a caps field nothing acts on.
func TestCaptureCapsNameEveryColorimetryComponent(t *testing.T) {
	for _, tc := range []struct {
		colorRange string
		want       string
	}{
		{colorRange: "pc", want: "colorimetry=1:" + gstBt709},
		{colorRange: "tv", want: "colorimetry=2:" + gstBt709},
	} {
		s := settings.Defaults()
		s.Chroma = "yuv444p"
		s.ColorRange = tc.colorRange
		caps, err := gstInputCaps(s)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(caps, tc.want) {
			t.Errorf("color range %q: encoder input caps %q lack %q", tc.colorRange, caps, tc.want)
		}
	}
}

// Every capture backend has to end in the capsfilter the encoder input is pinned
// by, whatever it does ahead of it, or the encoder negotiates its own format and
// the chroma and colour-range settings stop reaching the frames.
func TestEveryGstCaptureBackendEndsInTheEncoderInputCaps(t *testing.T) {
	s := settings.Defaults()
	s.Chroma = "yuv444p"
	inCaps, err := gstInputCaps(s)
	if err != nil {
		t.Fatal(err)
	}
	for name, p := range captureBackends {
		g, ok := p.(gstEngine)
		if !ok {
			continue
		}
		elements := g.capture.Describe(s, gstCaptureOptions{InCaps: inCaps})
		if len(elements) == 0 {
			t.Errorf("%s: capture backend describes no elements", name)
			continue
		}
		if !slices.Contains(elements, inCaps) {
			t.Errorf("%s: capture elements %v do not carry the encoder input caps %q", name, elements, inCaps)
		}
	}
}

// The rate probe belongs to a run, so a pipeline built without instrumentation
// carries none of it: the displayed command has to be the one the child runs.
// Asked for it, every backend has to place it, or the capture rate reads zero on
// that backend and the insights card reports the pacing target as if it were a
// measurement.
func TestEveryGstCaptureBackendPlacesTheRateProbeOnlyForARun(t *testing.T) {
	s := settings.Defaults()
	s.Chroma = "yuv444p"
	inCaps, err := gstInputCaps(s)
	if err != nil {
		t.Fatal(err)
	}
	for name, p := range captureBackends {
		g, ok := p.(gstEngine)
		if !ok {
			continue
		}
		plain := strings.Join(g.capture.Describe(s, gstCaptureOptions{InCaps: inCaps}), " ")
		if strings.Contains(plain, gstCaptureName) {
			t.Errorf("%s: a pipeline built without instrumentation carries the rate probe: %s", name, plain)
		}
		probed := strings.Join(g.capture.Describe(s, gstCaptureOptions{InCaps: inCaps, RateProbe: gstCaptureProbe}), " ")
		if !strings.Contains(probed, strings.Join(gstCaptureProbe, " ")) {
			t.Errorf("%s: capture elements drop the rate probe: %s", name, probed)
		}
	}
}

// The probe has to count new pictures, so nothing that repeats or paces a frame
// may sit in front of it. On the portal backend that is imagefreeze, whose whole
// job is to repeat the newest damage frame at the configured rate.
func TestPortalRateProbePrecedesTheFramePacer(t *testing.T) {
	s := settings.Defaults()
	s.Chroma = "yuv444p"
	inCaps, err := gstInputCaps(s)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(portalCapture{}.Describe(s, gstCaptureOptions{InCaps: inCaps, RateProbe: gstCaptureProbe}), " ")
	probe, pacer := strings.Index(line, gstCaptureName), strings.Index(line, "imagefreeze")
	if probe < 0 || pacer < 0 || probe > pacer {
		t.Errorf("the rate probe must precede imagefreeze: %s", line)
	}
}

// A chroma this engine's encoder element cannot take is refused rather than
// converted to the nearest format the element does negotiate. The default
// settings are exactly that case: they carry planar RGB, which the ffmpeg engine
// codes and no GStreamer element here does.
//
// The rejection has to come from the caps step, because that is the one the
// engine runs before it acquires a source: refused later, a gapped chroma would
// already have popped the compositor's screen picker.
func TestGstRejectsAGappedChromaBeforeAnythingIsAcquired(t *testing.T) {
	s := settings.Defaults()
	s.Capture = "portal"
	s.Transport = "srt"
	if s.Chroma != "gbrp" {
		t.Skipf("the default chroma is %s, no longer the gapped format this covers", s.Chroma)
	}

	_, err := gstInputCaps(s)
	if err == nil {
		t.Fatal("a chroma gapped on this engine must not yield encoder input caps")
	}
	// The message is what the user sees when a settings file skips the form's repair,
	// so it names the format and the way to reach it.
	if !strings.Contains(err.Error(), "gbrp") || !strings.Contains(err.Error(), "ffmpeg") {
		t.Errorf("the rejection must name the format and the engine that codes it: %v", err)
	}
	if _, err := buildPipeline(s, []string{"videotestsrc"}, ""); err == nil {
		t.Error("a chroma gapped on this engine must not build a pipeline either")
	}
}

// Every transport this engine carries has to terminate a pipeline with the audio
// branch attached, since a sink that is muxer and sink in one takes the second track
// on a request pad rather than through a muxer element. A branch that named a muxer
// this transport has none of would leave the audio pad unlinked at launch.
func TestEveryGstTransportTerminatesAPipelineWithAudio(t *testing.T) {
	for _, name := range transport.Names() {
		if !transport.CanGstPublish(name) {
			continue
		}
		s := settings.Defaults()
		s.Capture, s.Transport, s.Audio = "portal", name, "desktop"
		// libx264 over every transport: the transport's own format set decides
		// whether it may carry the codec, and this asserts the pipeline's shape.
		s.Codec, s.Chroma = "libx264", "yuv420p"
		if err := transport.ValidatePublish(name, s.Codec); err != nil {
			continue
		}
		pipeline, err := buildPipeline(s, []string{"videotestsrc"}, "")
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		joined := strings.Join(pipeline, " ")
		if !strings.Contains(joined, "name="+transport.GstMuxName) {
			t.Errorf("%s: no element carries the mux name the audio branch attaches to: %s", name, joined)
		}
		if !strings.HasSuffix(joined, transport.GstMuxName+".") {
			t.Errorf("%s: the audio branch must end at the mux name: %s", name, joined)
		}
	}
}

// A colour range with no mapping is refused rather than encoded as limited. The
// range travels in the bitstream and decides how every viewer expands the
// picture, so substituting one changes what the stream looks like with nothing
// said. The ffmpeg engine hands the same field to -color_range, which fails on a
// value it does not know, so refusing here is what keeps the two engines
// answering the same way.
func TestGstInputCapsRefusesAnUnmappedColorRange(t *testing.T) {
	s := settings.Defaults()
	s.Chroma = "yuv444p"
	for _, bad := range []string{"", "full", "limited", "PC"} {
		s.ColorRange = bad
		if _, err := gstInputCaps(s); err == nil {
			t.Errorf("colour range %q must be refused, not read as limited", bad)
		}
	}
	for _, good := range []string{"pc", "tv"} {
		s.ColorRange = good
		if _, err := gstInputCaps(s); err != nil {
			t.Errorf("colour range %q: %v", good, err)
		}
	}
}
