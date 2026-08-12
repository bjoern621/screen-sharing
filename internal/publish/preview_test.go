package publish

import (
	"net"
	"strconv"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/settings"
)

// previewCodecs is one codec per bitstream format the app publishes, so the tests below
// cover the whole of what the preview leg has to carry rather than the one format that
// happens to be the default. RTSP is the transport because it is the one that carries
// every format, which is the same reason the preview leg is RTP.
var previewCodecs = map[string]string{
	"h264": "libx264",
	"hevc": "libx265",
	"av1":  "libsvtav1",
	"vp9":  "libvpx-vp9",
	"vp8":  "libvpx",
}

// previewStream is settings that publish the codec over the one transport that carries
// every format, through the engine the capture backend runs on.
//
// The rate-control mode is asked of the capability table rather than named, for the
// reason every consumer of that table asks it: the defaults are a lossless encode at
// 150 Mbit/s and four of the five rows gap that one way or another, while what these
// tests are about is the shape of the command rather than which qualities a family can
// code. A mode written here would be a sixth statement of the table's own answer.
func previewStream(t *testing.T, capture, codec string) settings.Settings {
	t.Helper()

	engine, err := EngineFor(capture)
	if err != nil {
		t.Fatal(err)
	}

	s := baseStream()
	s.Publish.Capture, s.Publish.Transport = capture, "rtsp"
	s.Publish.Codec, s.Publish.Chroma = codec, "yuv420p"
	s.Publish.BitrateM, s.Publish.ColorRange = 20, "tv"
	for _, mode := range capabilities.Modes {
		s.Publish.Mode = mode
		if capabilities.Validate(engine, codec, s.Publish.CapabilityOptions(), s.Publish.Cq, s.Publish.BitrateM) == nil {
			return s
		}
	}
	t.Fatalf("%s codes nothing on the %s engine, so no command can be rendered for it", codec, engine)
	return s
}

// Every format the app publishes has a local preview leg, and the two halves of that leg
// agree. A payloader writing one payload format beside caps naming another is a preview
// that decodes nothing, and nothing in either half says so on its own.
func TestEveryPublishableFormatHasAPreviewLegWithBothHalves(t *testing.T) {
	for _, format := range capabilities.Formats() {
		codec, ok := previewCodecs[format]
		if !ok {
			t.Fatalf("format %s has no codec in this test's table, so its preview leg is untested", format)
		}
		if got, carried := PreviewCarried(codec); !carried {
			t.Fatalf("%s produces %s, which carries no local preview", codec, got)
		}

		carriage := previewCarriages[format]
		source, err := PreviewSource(codec, 5000)
		if err != nil {
			t.Fatalf("%s: %v", codec, err)
		}
		if !strings.Contains(source, "encoding-name="+carriage.encoding) {
			t.Errorf("%s: the receiving caps name no encoding the payloader writes: %s", format, source)
		}
		if !strings.Contains(source, "payload="+strconv.Itoa(previewPayloadType)) {
			t.Errorf("%s: the receiving caps pin no payload type: %s", format, source)
		}

		tap, err := gstPreviewTap(codec, PreviewLeg{Port: 5000})
		if err != nil {
			t.Fatalf("%s: %v", codec, err)
		}
		if joined := strings.Join(tap, " "); !strings.Contains(joined, carriage.payloader[0]) ||
			!strings.Contains(joined, "pt="+strconv.Itoa(previewPayloadType)) {
			t.Errorf("%s: the branch does not payload what the caps expect: %s", format, joined)
		}
	}
}

// The GStreamer child copies the encoded stream to the loopback port, off the same tee
// the meter hangs on. Two taps and the muxer are three branches of one tee, and a
// pipeline that grew a second tee would be a second copy of the encoded stream.
func TestTheGstPipelineTeesThePreviewOffTheEncodedStream(t *testing.T) {
	for format, codec := range previewCodecs {
		s := previewStream(t, "portal", codec)

		plain, err := buildPipeline(s, []string{"videotestsrc"}, "", PreviewLeg{})
		if err != nil {
			t.Fatalf("%s: %v", codec, err)
		}
		if line := strings.Join(plain, " "); strings.Contains(line, "udpsink") {
			t.Errorf("%s: a pipeline built without a preview port carries a preview branch: %s", format, line)
		}

		preview, err := buildPipeline(s, []string{"videotestsrc"}, "54321", PreviewLeg{Port: 45678})
		if err != nil {
			t.Fatalf("%s: %v", codec, err)
		}
		line := strings.Join(preview, " ")
		if strings.Count(line, "tee name=") != 1 {
			t.Errorf("%s: the preview and the meter do not share one tee: %s", format, line)
		}
		if !strings.Contains(line, "udpsink host="+previewHost+" port=45678") {
			t.Errorf("%s: no preview sink on the allocated loopback port: %s", format, line)
		}
		if !strings.Contains(line, "tcpclientsink host="+previewHost+" port=54321") {
			t.Errorf("%s: the preview branch displaced the meter's: %s", format, line)
		}
		// The branch leaks rather than backpressures, because a preview able to hold up
		// the encode path is a preview able to stall the stream it previews.
		payloader := strings.Index(line, previewCarriages[format].payloader[0])
		leaky := strings.LastIndex(line[:payloader], "leaky=downstream")
		if leaky < 0 {
			t.Errorf("%s: the preview branch can backpressure the encoder: %s", format, line)
		}
		// The trunk still reaches the muxer, and still through a queue: a tee's request
		// pad and a muxer's do not link to each other directly.
		last := strings.LastIndex(line, gstTeeName+". !")
		if mux := strings.Index(line, "name="+"mux"); last < 0 || mux < last || !strings.Contains(line[last:], "queue") {
			t.Errorf("%s: the muxer is no longer the last branch off the tee: %s", format, line)
		}
	}
}

// The rendered command carries no preview leg, and the reason is not tidiness: the port
// belongs to one launch, and whether two settings build one pipeline is decided by
// comparing exactly this string (SamePipeline).
func TestTheRenderedCommandCarriesNoPreviewLeg(t *testing.T) {
	for _, capture := range []string{"portal", "gdigrab"} {
		for _, codec := range previewCodecs {
			s := previewStream(t, capture, codec)
			engine, err := For(capture)
			if err != nil {
				t.Fatal(err)
			}
			line, err := engine.Command(s)
			if err != nil {
				// A capture backend this machine cannot render for is not what this test
				// is about; the shape of the command it would render is.
				continue
			}
			if strings.Contains(line, "udpsink") || strings.Contains(line, "rtp://"+previewHost) ||
				strings.Contains(line, "-f tee") {
				t.Errorf("%s/%s: the rendered command carries a run's own preview leg: %s", capture, codec, line)
			}
		}
	}
}

// The ffmpeg child writes the same encoded packets twice through the tee muxer: the
// relay's leg as the transport states it, and the preview's beside it. Two outputs
// written any other way are two encoders on one capture, which is the whole reason the
// tee muxer is the shape.
func TestTheFfmpegCommandTeesThePreviewBesideTheRelayLeg(t *testing.T) {
	for format, codec := range previewCodecs {
		s := previewStream(t, "gdigrab", codec)

		tap, ok := ffmpegPreviewTap(codec, PreviewLeg{Port: 45678})
		if !ok {
			t.Fatalf("%s: %s has no ffmpeg preview output", format, codec)
		}
		args, err := ffmpeg.BuildPublishArgs(s, &tap)
		if err != nil {
			t.Fatalf("%s: %v", codec, err)
		}

		line := strings.Join(args, " ")
		if strings.Count(line, "-f tee") != 1 {
			t.Errorf("%s: not exactly one tee: %s", format, line)
		}
		if !strings.Contains(line, "[f=rtsp:rtsp_transport=tcp") {
			t.Errorf("%s: the relay leg is not the transport's own arguments as a slave: %s", format, line)
		}
		if !strings.Contains(line, "rtp://"+previewHost+":45678") {
			t.Errorf("%s: no preview slave on the allocated loopback port: %s", format, line)
		}
		if !strings.Contains(line, "select=v") {
			t.Errorf("%s: the preview slave takes more than the video: %s", format, line)
		}
		if !strings.Contains(line, "onfail=ignore") {
			t.Errorf("%s: a preview slave that cannot open would end the stream: %s", format, line)
		}
		// The streams are mapped by hand, because automatic stream selection does not
		// apply to a tee and an unmapped tee writes no stream at all.
		if !strings.Contains(line, "-map 0:v") {
			t.Errorf("%s: the video is not mapped into the tee: %s", format, line)
		}
		// A draft payload format is refused by the RTP muxer unless compliance is
		// loosened, and it is loosened on the preview slave alone.
		draft := previewCarriages[format].draft
		if got := strings.Contains(line, "strict=experimental"); got != draft {
			t.Errorf("%s: strict=experimental is %v and the payload format's draft status is %v: %s", format, got, draft, line)
		}
	}
}

// A filter source has no input to map, so its chain's output is labelled and the label
// is what the map names. Without it the tee writes nothing, and the failure is a stream
// that never starts rather than a preview that does not.
func TestAFilterSourceIsMappedIntoTheTeeByLabel(t *testing.T) {
	s := previewStream(t, "ddagrab", "libx264")
	tap, ok := ffmpegPreviewTap(s.Publish.Codec, PreviewLeg{Port: 45678})
	if !ok {
		t.Fatal("libx264 has no ffmpeg preview output")
	}

	args, err := ffmpeg.BuildPublishArgs(s, &tap)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(args, " ")
	if !strings.Contains(line, "-map [out]") {
		t.Errorf("a filter source is not mapped by the label its chain ends in: %s", line)
	}
	if !strings.Contains(line, "-filter_complex ") || !strings.Contains(line, "[out] -map") {
		t.Errorf("the filter chain does not end in the label the map names: %s", line)
	}
	if strings.Contains(line, "-map 0:v") {
		t.Errorf("a filter source has no input 0 to map: %s", line)
	}
}

// The port is the kernel's answer rather than a number this package picked, and two
// allocations in a row do not collide - which is the whole reason it is allocated at all
// rather than being a constant.
func TestPreviewPortsAreAllocatedAndBindable(t *testing.T) {
	first, err := AllocatePreviewPort()
	if err != nil {
		t.Fatal(err)
	}
	second, err := AllocatePreviewPort()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Errorf("two allocations landed on one port: %d", first)
	}
	for _, port := range []int{first, second} {
		if port <= 0 || port > 65535 {
			t.Errorf("port %d is not a port", port)
		}
		// The socket is handed back before the number is, so the receiving pipeline can
		// bind it. A port that stayed held would be one no udpsrc could take.
		conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(previewHost), Port: port})
		if err != nil {
			t.Errorf("port %d was not released by the allocation that reported it: %v", port, err)
			continue
		}
		conn.Close()
	}
}

// A codec whose format has no local carriage publishes without a preview rather than
// failing to publish. It is the branch the table's own completeness hides today, and it
// is the one that decides whether a format added later costs a stream.
func TestAFormatWithNoLocalCarriagePublishesWithoutAPreview(t *testing.T) {
	const format = "h264"
	codec := previewCodecs[format]

	carriage := previewCarriages[format]
	delete(previewCarriages, format)
	defer func() { previewCarriages[format] = carriage }()

	if _, carried := PreviewCarried(codec); carried {
		t.Fatal("a format taken out of the table still reports a preview leg")
	}
	if _, err := ffmpeg.BuildPublishArgs(previewStream(t, "gdigrab", codec), nil); err != nil {
		t.Fatalf("the ffmpeg command must still build: %v", err)
	}
	if _, ok := ffmpegPreviewTap(codec, PreviewLeg{Port: 45678}); ok {
		t.Error("a format with no carriage yielded an ffmpeg preview output")
	}
	if _, err := gstPreviewTap(codec, PreviewLeg{Port: 45678}); err == nil {
		t.Error("a format with no carriage yielded a GStreamer preview branch")
	}

	pipeline, err := buildPipeline(previewStream(t, "portal", codec), []string{"videotestsrc"}, "", PreviewLeg{Port: 45678})
	if err != nil {
		t.Fatalf("the GStreamer pipeline must still build: %v", err)
	}
	if line := strings.Join(pipeline, " "); strings.Contains(line, "udpsink") {
		t.Errorf("a format with no carriage still grew a preview branch: %s", line)
	}
}
