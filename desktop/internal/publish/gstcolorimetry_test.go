package publish

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/settings"
)

// roundTripFrames is how many frames one round trip encodes. The colour
// description travels in the first parameter set, so a single frame would carry
// it; five keep an encoder that emits nothing until its lookahead fills from
// writing an empty file.
const roundTripFrames = 5

// roundTripWidth and roundTripHeight are the picture one round trip codes, and
// roundTripPicture below the caps fields stating it, which the caps on both ends of the
// conversion are completed with. HD, because the guess a viewer falls back to follows the
// picture size and every screen this app captures is HD or larger: a 320x240 round trip
// would be read as BT.601 and would measure a case no screen produces.
const (
	roundTripWidth  = 1280
	roundTripHeight = 720
)

var roundTripPicture = fmt.Sprintf(",width=%d,height=%d,framerate=30/1", roundTripWidth, roundTripHeight)

// gstUnsignalledColorimetry is what a viewer reads off an HD stream that carries no
// colour description: limited-range BT.709, the default it picks off the picture
// size. A stream that signals nothing is watched as this, whatever it holds, so it
// is the value such a publish is measured against.
const gstUnsignalledColorimetry = "bt709"

// gstFraming carries one format's elementary stream through a file: write is what the
// encoder's own link fragment hands the filesink, read what finds the frames in the
// file again. The file is what makes the round trip a measurement, since it severs caps
// negotiation: the colorimetry the decoder reports can then only have come from the
// bitstream.
//
// Framing is the whole of what the write side pins, never a colour description. Annex B
// start codes frame an H.26x stream and typefind reads them, so those two need nothing
// to be read back again. An AV1 OBU states its own size and a VP9 frame header is found
// by scanning, so both come out of a plain file once their parser is named. VP8 has no
// parser element to scan with, so its stream travels in IVF: 32 bytes of fourcc, picture
// size, frame rate and frame count, with no colour field of any kind and byte-identical
// between a full-range and a limited-range encode.
var gstFraming = map[string]struct{ write, read []string }{
	"h264": {write: []string{"video/x-h264,stream-format=byte-stream,alignment=au"}},
	"hevc": {write: []string{"video/x-h265,stream-format=byte-stream,alignment=au"}},
	"av1":  {write: []string{"video/x-av1,stream-format=obu-stream,alignment=obu"}, read: []string{"av1parse"}},
	"vp9":  {read: []string{"vp9parse"}},
	"vp8":  {write: []string{"avmux_ivf"}},
}

// gstColorimetryNames is the name GStreamer prints for a colorimetry it has one for.
// The publish side pins four enum values and a decoder reports the same colour space
// by name, so limited-range BT.709 goes in as 2:3:5:1 and comes back as bt709, which
// is what the grid's stats card shows. A pinned colorimetry with no name here is
// expected back verbatim, which is what full-range BT.709 does.
var gstColorimetryNames = map[string]string{
	gstRangeLimited + ":" + gstBt709: "bt709",
}

// gstSinkPadCaps matches the line gst-launch -v prints for the caps that reached the
// sink, and gstColorimetryField the one field of it this reads.
var (
	gstSinkPadCaps      = regexp.MustCompile(`GstFakeSink:[^.]*\.GstPad:sink: caps = (.*)`)
	gstColorimetryField = regexp.MustCompile(`colorimetry=\(string\)([^,\s]+)`)
)

// A stream is watched in the colour its bitstream states, and in the viewer's own
// where it states none: RTP and MPEG-TS carry no colour description, so what an
// encoder leaves out travels nowhere else. The colour range a publish is configured
// at therefore has to arrive at the decoder, and where it cannot the table gaps it
// rather than encoding a stream that says one thing and holds another. This measures
// both, one hop further out than the failure a partially named colorimetry produces
// inside the pipeline (TestCaptureCapsNameEveryColorimetryComponent).
//
// So this encodes through the engine's own encoder-input caps and encoder, hands a
// decoder the bitstream and its framing alone (gstFraming), and reads the colorimetry
// off the decoded caps: the same pad the grid's stats card reads its colorimetry from,
// since its render chain measures what decodebin produced.
//
// It runs each codec once per chain the pair table leaves it reachable through, not once.
// A stream's colour is the whole chain's answer and not the encoder's: the conversion is a
// different element on each path and the encoder often is too, so a device path measured
// through the system one would assert a claim about neither.
func TestPublishedColorimetryReachesTheDecoder(t *testing.T) {
	if _, err := exec.LookPath(GstExe); err != nil {
		t.Skipf("%s not installed", GstExe)
	}

	for codec := range gstCodecs {
		cap, ok := capabilities.Get(codec)
		if !ok {
			t.Errorf("%s has a GStreamer mapping but no capability row", codec)
			continue
		}
		framing, ok := gstFraming[cap.Format]
		if !ok {
			t.Errorf("format %s is published on this engine and has no round-trip framing, so nothing measures the colour its streams arrive in", cap.Format)
			continue
		}
		for _, chain := range gstRoundTripChains(t, cap.Family) {
			// The element is asked for per chain, because a family whose plugin ships one
			// per memory kind is encoded by a different one on each: asked for the other,
			// the skip below would read as a missing plugin where the run would have
			// launched an element this install does carry, or the reverse.
			elem, named := GstEncoderElementOn(codec, chain.memory)
			if !named {
				t.Errorf("%s has a GStreamer mapping and no encoder element to name, so the skip below would read as a missing plugin", codec)
				continue
			}
			if err := exec.Command("gst-inspect-1.0", "--exists", elem).Run(); err != nil {
				t.Logf("skipping %s in %s memory: %s plugin not installed", codec, chain.memory, elem)
				continue
			}

			for _, colorRange := range []string{"pc", "tv"} {
				t.Run(codec+"/"+chain.memory+"/"+colorRange, func(t *testing.T) {
					s := roundTripSettings(t, codec, colorRange, chain)
					// A colour range this engine has no way to state is declared as a gap
					// and refused before a pipeline is built, so the refusal is what this
					// asserts for it. Encoding it anyway is the outcome the gap exists to
					// prevent: a stream watched in another range than the form shows.
					if gap, gapped := cap.OptionGap(EngineGst, capabilities.OptionColorRange, colorRange); gapped {
						if _, err := gstTestCaps(s); err == nil {
							t.Fatalf("colour range %s is gapped on this engine, so it must be refused rather than encoded: %s",
								colorRange, gap.Reason)
						}
						return
					}
					pinned, err := gstColorimetry(s)
					if err != nil {
						t.Fatal(err)
					}
					stream := gstRoundTripEncode(t, s, roundTripPattern)

					decode := gstChain(
						[]string{"filesrc", "location=" + stream},
						framing.read,
						[]string{"decodebin"},
						[]string{"fakesink"})
					out, err := runGst(t, append([]string{"-v"}, decode...))
					if err != nil {
						t.Fatalf("decode: %v\n%s", err, out)
					}

					want := pinned
					if name, named := gstColorimetryNames[pinned]; named {
						want = name
					}
					got, signalled := decodedColorimetry(out)
					if !signalled {
						// The stream states nothing, so what it is watched as is the
						// viewer's own default. That is a match only where the publish
						// happens to be in it, which is the whole of what the colour-range
						// gaps are about.
						t.Logf("%s writes no colour description, so the stream is watched as the viewer's %s", elem, gstUnsignalledColorimetry)
						got = gstUnsignalledColorimetry
					}
					if got != want {
						t.Errorf("%s published at colour range %s in %s memory arrives as %s, want %s: the stream is watched in another colour than it was encoded in",
							codec, colorRange, chain.memory, got, want)
					}
				})
			}
		}
	}
}

// whitePatchLuma is what a white patch has to be stored as at each colour range: the two
// ends of the 8-bit range, and the studio ceiling 20 code values under it.
var whitePatchLuma = map[string]int{"pc": 255, "tv": 235}

// whitePatchTolerance is how far off a decoded patch may land. The two ranges are 20 code
// values apart and a lossy encode of a flat field can round one off it, so one value of
// slack cannot make either range read as the other.
const whitePatchTolerance = 1

// A device path claiming exact colour has to convert the frames and not only describe
// them. The signalled colorimetry says what a viewer will expand the picture as; this says
// what is in it, which is the half a label cannot earn on its own: an element that ignored
// the range and tagged it anyway signals bt709 full range over limited-range pixels, and
// every viewer then stretches a 235 white to 255 and clips everything the picture had.
//
// So this codes a white patch through the pair's own conversion and encoder, decodes it and
// reads the luma plane. The patch enters as RGB, which is what every capture backend here
// hands the pipeline: fed YUV, videotestsrc writes the studio values itself and the
// conversion under test becomes a passthrough that measures nothing.
func TestTheGstDevicePathStoresTheConfiguredRange(t *testing.T) {
	if _, err := exec.LookPath(GstExe); err != nil {
		t.Skipf("%s not installed", GstExe)
	}

	for _, p := range gpupath.Paths {
		if p.Engine != EngineGst || p.Colour != gpupath.ColourExact {
			continue
		}
		for _, c := range capabilities.Codecs {
			if c.Family != p.Family || !c.Implemented {
				continue
			}
			if _, ok := gstCodecs[c.Name]; !ok {
				continue
			}
			chain := roundTripChain{capture: p.Capture, demand: gpupath.MemoryGpu}
			memory, err := gpupath.Resolve(EngineGst, p.Capture, p.Family, chain.demand)
			if err != nil {
				t.Fatalf("%s/%s carries a row and refuses the device path: %v", p.Capture, p.Family, err)
			}
			chain.memory = memory
			elem, named := GstEncoderElementOn(c.Name, chain.memory)
			if !named {
				t.Errorf("%s has a GStreamer mapping and no encoder element to name", c.Name)
				continue
			}
			if err := exec.Command("gst-inspect-1.0", "--exists", elem).Run(); err != nil {
				t.Logf("skipping %s: %s plugin not installed", c.Name, elem)
				continue
			}

			for _, colorRange := range []string{"pc", "tv"} {
				want, measured := whitePatchLuma[colorRange]
				if !measured {
					t.Fatalf("colour range %s has no luma a white patch is stored as", colorRange)
				}
				t.Run(c.Name+"/"+chain.memory+"/"+colorRange, func(t *testing.T) {
					s := roundTripSettings(t, c.Name, colorRange, chain)
					// 8-bit 4:2:0, the layout every device element here negotiates and the
					// one whose luma plane is the file's first width*height bytes. What
					// this measures is the range, which is the same conversion at every
					// depth.
					s.Chroma = whitePatchChroma
					if _, gapped := c.OptionGap(EngineGst, capabilities.OptionColorRange, colorRange); gapped {
						t.Skipf("colour range %s is gapped on this engine, so no run stores it", colorRange)
					}
					if err := capabilities.Validate(EngineGst, s.Codec, s.CapabilityOptions(), s.Cq, s.BitrateM); err != nil {
						t.Skipf("%s at %s: %v", c.Name, whitePatchChroma, err)
					}

					stream := gstRoundTripEncode(t, s, whitePatchPattern)
					got := gstDecodedLuma(t, s, stream)
					if got < want-whitePatchTolerance || got > want+whitePatchTolerance {
						t.Errorf("%s stores white as luma %d at colour range %s in %s memory, want %d: the conversion signals the range without applying it",
							c.Name, got, colorRange, chain.memory, want)
					}
				})
			}
		}
	}
}

// whitePatchChroma is the layout the white patch is coded in, and whitePatchFormats the
// layouts its luma plane is read back in. Both start with the full plane, so the pixel is
// at the same offset in either, and offering both keeps the read a memory download: a
// videoconvert asked for one layout converts the range along with it, which is exactly the
// value under test.
const (
	whitePatchChroma  = "yuv420p"
	whitePatchFormats = "video/x-raw,format={ NV12, I420 }"
	// whitePatchPattern is videotestsrc's flat white field: every channel at its
	// maximum, which is the one input whose stored luma is the range and nothing else.
	whitePatchPattern = "white"
)

// roundTripPattern is what the colorimetry round trip codes: videotestsrc's colour bars,
// its own default. What the frames hold does not decide what the stream says it holds, so
// the pattern is named once here rather than chosen per codec.
const roundTripPattern = "smpte"

// gstDecodedLuma decodes one stream and returns the luma of the centre pixel of its last
// frame. The centre because a patch is uniform and its border is where a scaler or a
// cropping decoder would show; the last frame because the first is the one an encoder codes
// alone, and a range applied to it and dropped afterwards is a failure this must not miss.
func gstDecodedLuma(t *testing.T, s settings.Stream, stream string) int {
	t.Helper()
	c, ok := capabilities.Get(s.Codec)
	if !ok {
		t.Fatalf("codec %s has no capability row", s.Codec)
	}
	framing, ok := gstFraming[c.Format]
	if !ok {
		t.Fatalf("format %s has no round-trip framing", c.Format)
	}

	raw := filepath.ToSlash(filepath.Join(t.TempDir(), "luma"))
	decode := gstChain(
		[]string{"filesrc", "location=" + stream},
		framing.read,
		[]string{"decodebin"},
		[]string{whitePatchFormats},
		[]string{"filesink", "location=" + raw})
	if out, err := runGst(t, append([]string{"-q"}, decode...)); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}

	frames, err := os.ReadFile(filepath.FromSlash(raw))
	if err != nil {
		t.Fatal(err)
	}
	// 4:2:0 stores the luma plane whole and the two chroma planes at a quarter each.
	frame := roundTripWidth * roundTripHeight * 3 / 2
	if len(frames) < frame {
		t.Fatalf("the decode produced %d bytes, less than one %dx%d frame", len(frames), roundTripWidth, roundTripHeight)
	}
	last := (len(frames)/frame - 1) * frame
	return int(frames[last+roundTripHeight/2*roundTripWidth+roundTripWidth/2])
}

// roundTripChain is one chain a codec is published through: the capture backend the pair
// table names, the memory setting demanded of it, and what that setting resolves to.
type roundTripChain struct {
	capture string
	demand  string
	memory  string
}

// gstRoundTripChains returns every chain this engine reaches a family's encoders through:
// system memory, which every pair has, plus one per device row the pair table declares.
//
// Only the exact-colour rows are taken. A row that lets the encoder convert states the
// colour it produces in Signalled and is measured against that instead of against the
// settings, which is the ffmpeg engine's round trip; this engine carries no such row.
func gstRoundTripChains(t *testing.T, family string) []roundTripChain {
	t.Helper()
	// No capture backend on the system chain: the memory setting alone decides that path,
	// and naming one would tie what this covers to which backend the defaults carry.
	chains := []roundTripChain{{demand: gpupath.MemorySystem, memory: gpupath.MemorySystem}}
	for _, p := range gpupath.Paths {
		if p.Engine != EngineGst || p.Family != family || p.Colour != gpupath.ColourExact {
			continue
		}
		memory, err := gpupath.Resolve(EngineGst, p.Capture, family, gpupath.MemoryGpu)
		if err != nil {
			t.Fatalf("%s/%s carries a row and refuses the device path: %v", p.Capture, family, err)
		}
		chains = append(chains, roundTripChain{capture: p.Capture, demand: gpupath.MemoryGpu, memory: memory})
	}
	return chains
}

// gstRoundTripEncode encodes generated frames of one videotestsrc pattern through the whole
// chain these settings publish through — the family's upload where its converter takes no
// system frames, the conversion, the encoder-input caps, the encoder and its link — and
// returns the file holding the elementary stream.
//
// The source is pinned to packed RGB (gstProbeSource), which is what every capture backend
// here produces and what leaves the conversion the one element deciding the layout and the
// colour.
func gstRoundTripEncode(t *testing.T, s settings.Stream, pattern string) string {
	t.Helper()
	inCaps, err := gstTestCaps(s)
	if err != nil {
		t.Fatal(err)
	}
	mem, err := gstMemory(s)
	if err != nil {
		t.Fatal(err)
	}
	encoder, link, err := gstEncoder(s, s.Fps*2, mem.memory)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := capabilities.Get(s.Codec)
	if !ok {
		t.Fatalf("codec %s has no capability row", s.Codec)
	}
	framing, ok := gstFraming[c.Format]
	if !ok {
		t.Fatalf("format %s has no round-trip framing", c.Format)
	}
	var upload []string
	if mem.upload != "" {
		upload = []string{mem.upload}
	}

	// Forward slashes, because this path is interpolated into a gst-launch line and that
	// parser reads a backslash as an escape: a Windows temp path reaches filesink with its
	// separators eaten, and the round trip writes into the working directory instead of the
	// temp one. GStreamer takes a drive-letter path in either separator.
	stream := filepath.ToSlash(filepath.Join(t.TempDir(), "roundtrip"))
	encode := gstChain(
		[]string{"videotestsrc", "num-buffers=" + strconv.Itoa(roundTripFrames), "pattern=" + pattern},
		[]string{"video/x-raw,format=" + gstProbeSource + roundTripPicture},
		upload,
		mem.convert,
		[]string{inCaps + roundTripPicture},
		encoder, link, framing.write,
		[]string{"filesink", "location=" + stream})
	if out, err := runGst(t, append([]string{"-q"}, encode...)); err != nil {
		t.Fatalf("encode: %v\n%s", err, out)
	}
	return stream
}

// roundTripSettings are the settings one round trip publishes with: the codec under
// test at the narrowest chroma this engine reaches for it, in a rate-control mode its
// element implements, over one of the chains the pair table leaves it reachable through.
//
// The transport is RTSP because it carries every format the codec table holds, so the
// transport check gstInputCaps runs never decides which codecs this covers.
func roundTripSettings(t *testing.T, codec, colorRange string, chain roundTripChain) settings.Stream {
	t.Helper()
	c, ok := capabilities.Get(codec)
	if !ok {
		t.Fatalf("codec %s has a GStreamer mapping but no capability row", codec)
	}
	mode, ok := firstGstMode(codec)
	if !ok {
		t.Fatalf("codec %s has a GStreamer mapping and no rate-control mode on this engine", codec)
	}
	chromas := c.EngineChromas(EngineGst)
	if len(chromas) == 0 {
		t.Fatalf("codec %s has a GStreamer mapping and no chroma on this engine", codec)
	}

	s := settings.Defaults()
	s.Transport = "rtsp"
	s.Codec, s.Mode, s.ColorRange = codec, mode, colorRange
	s.Chroma = chromas[len(chromas)-1]
	// The chain is demanded rather than left to auto, so which path a run takes is this
	// test's choice and not the defaults' capture backend: the memory setting decides the
	// caps feature, the conversion and, on a family with one element per memory kind, the
	// encoder. The generated frames start in system memory either way, and the family's
	// upload is what puts them where a captured one already is.
	s.CaptureMemory = chain.demand
	if chain.capture != "" {
		s.Capture = chain.capture
	}
	// The quantizer target rides each encoder's own scale, and the default settings
	// carry one from another codec's.
	s.Cq = c.CqMaxOn(EngineGst) / 2
	// One encoder refuses a target above its ceiling outright, and the default settings
	// carry one above it.
	if limit := c.BitrateLimitOn(EngineGst); limit > 0 && s.BitrateM > limit {
		s.BitrateM = limit
	}
	return s
}

// gstChain joins pipeline fragments with the link operator and drops the empty ones. A
// codec whose encoder needs no parser and a format whose file needs no framing element
// each contribute nothing between their neighbours.
func gstChain(fragments ...[]string) []string {
	var chain []string
	for _, fragment := range fragments {
		if len(fragment) == 0 {
			continue
		}
		if len(chain) > 0 {
			chain = append(chain, "!")
		}
		chain = append(chain, fragment...)
	}
	return chain
}

// runGst runs one pipeline to its end and returns everything it printed. A pipeline
// that produces nothing is a failure like any other, so the timeout reports as one
// rather than holding the test open.
func runGst(t *testing.T, args []string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), encodeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, GstExe, args...).CombinedOutput()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return string(out), errors.New("the pipeline stalled: no end of stream")
	}
	return string(out), err
}

// decodedColorimetry reads the colorimetry off the caps that reached the sink, and
// reports false where the decoded frames carry no such field: a decoder that read no
// colour description states none rather than a default.
func decodedColorimetry(gstOutput string) (string, bool) {
	caps := gstSinkPadCaps.FindStringSubmatch(gstOutput)
	if caps == nil {
		return "", false
	}
	field := gstColorimetryField.FindStringSubmatch(caps[1])
	if field == nil {
		return "", false
	}
	return strings.TrimSpace(field[1]), true
}
