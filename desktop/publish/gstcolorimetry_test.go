package publish

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/gpupath"
	"bjoernblessin.de/screenshare/settings"
)

// roundTripFrames is how many frames one round trip encodes. The colour
// description travels in the first parameter set, so a single frame would carry
// it; five keep an encoder that emits nothing until its lookahead fills from
// writing an empty file.
const roundTripFrames = 5

// roundTripPicture is what the encoder-input caps are completed with. The size is
// HD because the guess a viewer falls back to follows the picture size, and every
// screen this app captures is HD or larger: a 320x240 round trip would be read as
// BT.601 and would measure a case no screen produces.
const roundTripPicture = ",width=1280,height=720,framerate=30/1"

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
func TestPublishedColorimetryReachesTheDecoder(t *testing.T) {
	if _, err := exec.LookPath(gstExe); err != nil {
		t.Skipf("%s not installed", gstExe)
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
		elem, named := GstEncoderElement(codec)
		if !named {
			t.Errorf("%s has a GStreamer mapping and no encoder element to name, so the skip below would read as a missing plugin", codec)
			continue
		}
		if err := exec.Command("gst-inspect-1.0", "--exists", elem).Run(); err != nil {
			t.Logf("skipping %s: %s plugin not installed", codec, elem)
			continue
		}

		for _, colorRange := range []string{"pc", "tv"} {
			t.Run(codec+"/"+colorRange, func(t *testing.T) {
				s := roundTripSettings(t, codec, colorRange)
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
				inCaps, err := gstTestCaps(s)
				if err != nil {
					t.Fatal(err)
				}
				pinned, err := gstColorimetry(s)
				if err != nil {
					t.Fatal(err)
				}
				encoder, link, err := gstEncoder(s, s.Fps*2)
				if err != nil {
					t.Fatal(err)
				}

				stream := filepath.Join(t.TempDir(), "roundtrip")
				encode := gstChain(
					[]string{"videotestsrc", "num-buffers=" + strconv.Itoa(roundTripFrames)},
					[]string{"videoconvert"},
					[]string{inCaps + roundTripPicture},
					encoder, link, framing.write,
					[]string{"filesink", "location=" + stream})
				if out, err := runGst(t, append([]string{"-q"}, encode...)); err != nil {
					t.Fatalf("encode: %v\n%s", err, out)
				}

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
					t.Errorf("%s published at colour range %s arrives as %s, want %s: the stream is watched in another colour than it was encoded in",
						codec, colorRange, got, want)
				}
			})
		}
	}
}

// roundTripSettings are the settings one round trip publishes with: the codec under
// test at the narrowest chroma this engine reaches for it, in a rate-control mode its
// element implements.
//
// The transport is RTSP because it carries every format the codec table holds, so the
// transport check gstInputCaps runs never decides which codecs this covers.
func roundTripSettings(t *testing.T, codec, colorRange string) settings.Stream {
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
	// The encoder input caps feed a videotestsrc chain here, which produces system
	// memory whatever a screen capture would have produced. Pinning the memory keeps the
	// caps free of the device feature a GPU path would put on them, so what this covers
	// stays the colorimetry rather than which capture backend the defaults name.
	s.CaptureMemory = gpupath.MemorySystem
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
	out, err := exec.CommandContext(ctx, gstExe, args...).CombinedOutput()
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
