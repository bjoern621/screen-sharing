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
	// srt deliberately: a test stream leaves over RTSP whatever the settings name.
	s := settings.Settings{
		Relay: settings.Relay{
			Host:     "10.0.0.5",
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
		"location=rtsp://10.0.0.5:8554/test-1",
	} {
		if !slices.Contains(args, want) {
			t.Errorf("missing %q in %v", want, args)
		}
	}
}

// Round-robin is what keeps simultaneous test streams apart on screen, and a whole surface per row
// is what keeps the colour off the frame size: a row naming only a pattern would leave format and
// colour to the source's own defaults.
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

// One HDR stream, and inside the starting set rather than behind a count nobody asks for.
// Standard range throughout exercises no viewer's HDR path, and on a machine whose screens are all
// standard range there is nothing else that reaches it.
//
// Ten bits go with that row, the rule a real capture is held to as well: eight cannot carry an HDR
// surface.
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

// One sounding stream, and inside the starting set for the same reason: nothing but a stream with a
// track reaches the per-stream volume, the meter beside it, or two streams sounding at once.
//
// One row rather than every row, which leaves a silent tile to compare against.
// This engine codes its codec and RTSP carries it, RTSP being every test stream's leg, so the row
// is something the relay ingests instead of a refusal at launch.
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

// The branch belongs to the sounding row alone, so naming no codec builds none of it.
// The queue goes with it: a second pad is the only thing it is there for.
func TestASilentTestSurfacePublishesNoAudio(t *testing.T) {
	s := settings.Settings{
		Relay:   settings.Relay{Host: "10.0.0.5", RtspPort: 8554},
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

// Element for element, the branch is the audio table's rather than a second set of names beside it,
// so an encoder or a rate edited there is edited here.
// Attaching to the sink's mux pad is what makes the track a second RTP stream inside the session
// the picture travels in.
func TestASoundingTestSurfacePublishesTheAudioTablesElements(t *testing.T) {
	s := settings.Settings{
		Relay:   settings.Relay{Host: "10.0.0.5", RtspPort: 8554},
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

// The row's whole point, measured rather than assumed: a stream off the sounding surface carries a
// track that decodes.
//
// The argv is the app's, with a file's muxer standing in for the relay's sink, read back as a
// viewer reads it: through a decoder, off the raw caps that arrived at the sink.
// Bounding both sources is the one edit a live pipeline needs to end its own run.
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
		Relay:   settings.Relay{Host: "10.0.0.5", RtspPort: 8554},
		Publish: settings.Publish{Name: "nixos", Transport: "rtsp", RtspPublishProtocol: "tcp"},
	}
	args, err := BuildTestStreamArgs(s, "test-3-audio", surface)
	if err != nil {
		t.Fatal(err)
	}

	// Sink and branch are cut by their own lengths, the transport stating how many arguments the one
	// runs to and the audio table the other.
	// Between the two cuts is the argv the app launches.
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

// soundingSurface is the row a track is published from, and fails its caller where the table holds
// no such row: every audio case here is about that row, and its absence is the silence they exist
// to catch.
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

// The row's whole point, measured rather than assumed: a stream off the HDR surface arrives
// carrying HDR.
//
// The argv is the app's rather than a pipeline written for the case, read back as a viewer reads
// it: off the caps the decoder produces.
// Only the relay is left out, since bytes are all it re-serves, and what is asserted here is that
// the bytes leave the encoder in the colour the surface was drawn in.
func TestTheHdrTestStreamIsPublishedInHdr(t *testing.T) {
	if _, err := exec.LookPath(GstExe); err != nil {
		t.Skipf("%s not installed", GstExe)
	}

	surface := TestSurfaceOf(1)
	if !colour.IsHDR(colour.TransferOfColorimetry(surface.Colorimetry)) {
		t.Fatalf("surface 1 is drawn in %q, which is not the HDR row this case is about", surface.Colorimetry)
	}

	s := settings.Settings{
		Relay:   settings.Relay{Host: "10.0.0.5", RtspPort: 8554},
		Publish: settings.Publish{Name: "nixos", Transport: "rtsp", RtspPublishProtocol: "tcp"},
	}
	args, err := BuildTestStreamArgs(s, "test-2-hdr", surface)
	if err != nil {
		t.Fatal(err)
	}

	// A file stands in for the relay's sink and the run is bounded, leaving the argv the app launches
	// between the source and the handover.
	// Cutting the sink by its own length rather than searching for it works because the transport
	// states how many arguments it runs to.
	// Byte-stream, since a bare file gives a parser no framing of its own where the sink would have
	// carried it.
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

	// A viewer's reading: the transfer characteristic settles the verdict, and the verdict is what a
	// tile offers its choice on.
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
