package publish

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/colour"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

func TestBuildTestStreamArgs(t *testing.T) {
	// Transport srt on purpose: test streams must publish over RTSP anyway.
	s := settings.Settings{
		Relay: settings.Relay{
			Host:     "relay.example",
			RtspPort: 8554,
		},
		Publish: settings.Publish{
			Name:                "nixos",
			Transport:           "srt",
			RtspPublishProtocol: "tcp",
		},
	}
	args, err := BuildTestStreamArgs(s, "test-1", TestSurfaceOf(0))
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"videotestsrc",
		"is-live=true",
		"pattern=smpte",
		"x264enc",
		"protocols=tcp",
		"location=rtsp://relay.example:8554/test-1",
	} {
		if !slices.Contains(args, want) {
			t.Errorf("missing %q in %v", want, args)
		}
	}
}

// The surfaces are handed out round-robin so that simultaneous test streams are told apart on
// screen, and every row states the whole surface: a row that named a pattern and left the format or
// the colour to the source would publish a stream whose colour depends on the frame size.
func TestTestSurfacesCycleAndStateTheirSurface(t *testing.T) {
	if TestSurfaceOf(0) != TestSurfaceOf(len(testSurfaces)) {
		t.Error("the surfaces must wrap around")
	}

	seen := map[string]bool{}
	for i := range len(testSurfaces) {
		surface := TestSurfaceOf(i)
		if seen[surface.Pattern] {
			t.Errorf("pattern %q handed out twice within one cycle", surface.Pattern)
		}
		seen[surface.Pattern] = true

		if surface.Format == "" || surface.Colorimetry == "" {
			t.Errorf("surface %d draws %q into %q in %q, and states no whole surface",
				i, surface.Pattern, surface.Format, surface.Colorimetry)
		}
	}
}

// The set carries one HDR stream, and it is inside the set this process brings up with itself
// rather than behind a count nobody asks for.
// A viewer's HDR path is not exercised by a grid of standard-range streams,
// and that path is the one with no other way to be reached on a machine whose screens are all
// standard range.
//
// The HDR row is also the ten-bit one, which is the rule the publish path enforces on a real
// capture: an HDR surface cannot ride in eight bits.
func TestTheSetCarriesAnHdrStreamItBringsUpWithItself(t *testing.T) {
	const bootSet = 3

	hdr := 0
	for i := range bootSet {
		surface := TestSurfaceOf(i)
		if !colour.IsHDR(colour.TransferOfColorimetry(surface.Colorimetry)) {
			continue
		}
		hdr++
		if surface.Format != testChroma10 {
			t.Errorf("the HDR surface draws into %q, which carries eight bits per component", surface.Format)
		}
		if surface.Label == "" {
			t.Error("the HDR surface reaches the roster under a name that says nothing about it")
		}
	}
	if hdr != 1 {
		t.Errorf("the set of %d brings up %d HDR streams, want exactly one", bootSet, hdr)
	}
}

// The set carries one sounding stream, and it too is inside the set this process brings up with
// itself: the per-stream volume, the level meter beside it and two streams playing at once are
// reached by a stream that has a track and by nothing else.
//
// One row and not all of them, so a silent tile stays there to compare against.
// Its codec is coded by this engine and carried by RTSP, which is the leg every test stream
// publishes over, so the row is a stream the relay ingests rather than a refusal at launch.
func TestTheSetCarriesASoundingStreamItBringsUpWithItself(t *testing.T) {
	const bootSet = 3

	sounding := 0
	for i := range bootSet {
		surface := TestSurfaceOf(i)
		if surface.Audio == "" {
			continue
		}
		sounding++
		if surface.Label == "" {
			t.Error("the sounding surface reaches the roster under a name that says nothing about it")
		}
		if err := capabilities.ValidateAudio(EngineGst, surface.Audio); err != nil {
			t.Errorf("the sounding surface is coded by no GStreamer element: %v", err)
		}
		if err := transport.ValidatePublishAudio("rtsp", EngineGst, surface.Audio); err != nil {
			t.Errorf("the sounding surface publishes over a leg that does not carry it: %v", err)
		}
	}
	if sounding != 1 {
		t.Errorf("the set of %d brings up %d sounding streams, want exactly one", bootSet, sounding)
	}
	if len(testSurfaces) == sounding {
		t.Error("every surface sounds, so there is no silent tile to compare one against")
	}
}

// A silent row publishes what it always did.
// The audio branch is the sounding row's alone, and a row that names no codec builds none of it:
// the queue included, which is there for a second pad and for nothing else.
func TestASilentTestSurfacePublishesNoAudio(t *testing.T) {
	s := settings.Settings{
		Relay:   settings.Relay{Host: "relay.example", RtspPort: 8554},
		Publish: settings.Publish{Name: "nixos", Transport: "rtsp", RtspPublishProtocol: "tcp"},
	}

	silent := TestSurface{Pattern: "smpte", Format: testChroma8, Colorimetry: testSDR}
	args, err := BuildTestStreamArgs(s, "test-1", silent)
	if err != nil {
		t.Fatal(err)
	}

	joined := strings.Join(args, " ")
	for _, unwanted := range []string{"audiotestsrc", "queue", transport.GstMuxName + "."} {
		if strings.Contains(joined, unwanted) {
			t.Errorf("a silent test stream builds %q: %s", unwanted, joined)
		}
	}
}

// The sounding row's branch is the audio table's, element for element, rather than a second set of
// names beside it: a codec whose encoder or rate changes there changes here with it.
// The branch attaches to the sink's mux pad, which is what makes the track a second RTP stream of
// the session the picture travels in.
func TestASoundingTestSurfacePublishesTheAudioTablesElements(t *testing.T) {
	s := settings.Settings{
		Relay:   settings.Relay{Host: "relay.example", RtspPort: 8554},
		Publish: settings.Publish{Name: "nixos", Transport: "rtsp", RtspPublishProtocol: "tcp"},
	}

	surface := soundingSurface(t)
	a, ok := capabilities.GetAudio(surface.Audio)
	if !ok {
		t.Fatalf("the sounding surface names audio codec %q, which the table does not carry", surface.Audio)
	}
	enc, ok := a.EncoderOn(EngineGst)
	if !ok {
		t.Fatalf("audio codec %s has no GStreamer encoder", a.Name)
	}

	args, err := BuildTestStreamArgs(s, "test-3-audio", surface)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"audiotestsrc",
		fmt.Sprintf("audio/x-raw,rate=%d,channels=2", a.Rate),
		enc.Element,
		fmt.Sprintf("bitrate=%d", a.BitrateK*1000),
		enc.Parser,
	} {
		if !slices.Contains(args, want) {
			t.Errorf("missing %q in %v", want, args)
		}
	}
	if args[len(args)-1] != transport.GstMuxName+"." {
		t.Errorf("the audio branch attaches to %q rather than to the sink's mux pad", args[len(args)-1])
	}
	if !slices.Contains(args, "queue") {
		t.Errorf("a two-pad sink is fed without a queue, so one branch stalls the other: %v", args)
	}
}

// The whole point of the row, measured rather than assumed: a stream published from the sounding
// surface carries a track that decodes.
//
// It is the argv the app launches, with the relay's sink replaced by a file's muxer, and it is read
// back the way a viewer reads it - through a decoder, off the raw caps that reached the sink.
// Both sources are bounded, which is the only edit a run of a live pipeline needs to end on its own.
func TestTheSoundingTestStreamIsPublishedWithItsTrack(t *testing.T) {
	if _, err := exec.LookPath(GstExe); err != nil {
		t.Skipf("%s not installed", GstExe)
	}

	surface := soundingSurface(t)
	a, ok := capabilities.GetAudio(surface.Audio)
	if !ok {
		t.Fatalf("the sounding surface names audio codec %q, which the table does not carry", surface.Audio)
	}

	s := settings.Settings{
		Relay:   settings.Relay{Host: "relay.example", RtspPort: 8554},
		Publish: settings.Publish{Name: "nixos", Transport: "rtsp", RtspPublishProtocol: "tcp"},
	}
	args, err := BuildTestStreamArgs(s, "test-3-audio", surface)
	if err != nil {
		t.Fatal(err)
	}

	// The sink is cut by its own length and the branch behind it by the branch's,
	// because the transport states how many arguments the one is and the audio table the other:
	// what is left between them is the argv the app launches.
	sink, ok := transport.GstSink(s)
	if !ok {
		t.Fatal("the rtsp transport has no GStreamer sink, so there is nothing to cut")
	}
	audio := testAudioBranch(surface)
	if len(audio) == 0 {
		t.Fatal("the sounding surface builds no audio branch")
	}

	file := filepath.Join(t.TempDir(), "sounding.mkv")
	publish := slices.Concat(
		[]string{"videotestsrc", "num-buffers=30"},
		args[1:len(args)-len(sink)-len(audio)],
		[]string{"matroskamux", "name=" + transport.GstMuxName, "!", "filesink", "location=" + file},
		[]string{"audiotestsrc", "num-buffers=50"},
		audio[1:])
	if out, err := runGst(t, publish); err != nil {
		t.Fatalf("publishing the sounding test stream: %v\n%s", err, out)
	}

	out, err := runGst(t, []string{"-v", "filesrc", "location=" + file,
		"!", "matroskademux", "name=demux", "demux.audio_0",
		"!", "decodebin", "!", "audioconvert", "!", "fakesink"})
	if err != nil {
		t.Fatalf("decoding the sounding test stream: %v\n%s", err, out)
	}

	caps := gstSinkPadCaps.FindStringSubmatch(out)
	if caps == nil {
		t.Fatalf("the sounding test stream's track reached no sink:\n%s", out)
	}
	if !strings.Contains(caps[1], "audio/x-raw") {
		t.Errorf("the sounding test stream decodes as %q rather than as audio", caps[1])
	}
	if want := fmt.Sprintf("rate=(int)%d", a.Rate); !strings.Contains(caps[1], want) {
		t.Errorf("the sounding test stream decodes as %q, which is not the %s rate %q",
			caps[1], a.Name, want)
	}
}

// soundingSurface is the row the set publishes a track from, and fails the case that asks for it
// when the table holds none: every audio case is about that row, and a table without one is the
// silence they exist to catch.
func soundingSurface(t *testing.T) TestSurface {
	t.Helper()

	for i := range len(testSurfaces) {
		if surface := TestSurfaceOf(i); surface.Audio != "" {
			return surface
		}
	}
	t.Fatal("no test surface carries audio")
	return TestSurface{}
}

// The whole point of the row, measured rather than assumed: a stream published from the HDR surface
// arrives carrying HDR.
//
// It is the argv the app launches, not a pipeline written for the test, and it is read back the way
// a viewer reads it - off the caps the decoder produces.
// The relay is the one thing left out: what it re-serves is bytes, and what this asserts is that
// the bytes leave the encoder with the colour the surface was drawn in.
func TestTheHdrTestStreamIsPublishedInHdr(t *testing.T) {
	if _, err := exec.LookPath(GstExe); err != nil {
		t.Skipf("%s not installed", GstExe)
	}

	surface := TestSurfaceOf(1)
	if !colour.IsHDR(colour.TransferOfColorimetry(surface.Colorimetry)) {
		t.Fatalf("surface 1 is drawn in %q, which is not the HDR row this case is about", surface.Colorimetry)
	}

	s := settings.Settings{
		Relay:   settings.Relay{Host: "relay.example", RtspPort: 8554},
		Publish: settings.Publish{Name: "nixos", Transport: "rtsp", RtspPublishProtocol: "tcp"},
	}
	args, err := BuildTestStreamArgs(s, "test-2-hdr", surface)
	if err != nil {
		t.Fatal(err)
	}

	// The relay's sink is replaced by a file and the run is bounded, so everything between the source
	// and the handover is the argv the app launches.
	// The sink is cut by its own length rather than searched for, because the transport is what states
	// how many arguments it is.
	// Byte-stream because that is what a parser reads back out of a bare file,
	// where the sink would have carried the framing itself.
	sink, ok := transport.GstSink(s)
	if !ok {
		t.Fatal("the rtsp transport has no GStreamer sink, so there is nothing to cut")
	}
	stream := filepath.Join(t.TempDir(), "hdr.h264")
	encode := slices.Concat(
		[]string{"videotestsrc", "num-buffers=30"},
		args[1:len(args)-len(sink)],
		[]string{"video/x-h264,stream-format=byte-stream,alignment=au", "!", "filesink", "location=" + stream})
	if out, err := runGst(t, encode); err != nil {
		t.Fatalf("encoding the HDR test stream: %v\n%s", err, out)
	}

	out, err := runGst(t, []string{"-v", "filesrc", "location=" + stream,
		"!", "h264parse", "!", "decodebin", "!", "fakesink"})
	if err != nil {
		t.Fatalf("decoding the HDR test stream: %v\n%s", err, out)
	}

	// Read as a viewer reads it: the transfer characteristic decides the verdict,
	// and the verdict is what a tile offers a choice on.
	decoded, stated := decodedColorimetry(out)
	if !stated {
		t.Fatalf("the HDR test stream decodes stating no colour at all:\n%s", out)
	}
	if transfer := colour.TransferOfColorimetry(decoded); !colour.IsHDR(transfer) {
		t.Errorf("the HDR test stream decodes as %q, transfer %q, which no viewer treats as HDR",
			decoded, transfer)
	}
	if !strings.Contains(out, "format=(string)"+surface.Format) {
		t.Errorf("the HDR test stream does not decode as the ten-bit %q it was drawn in:\n%s",
			surface.Format, out)
	}
}
