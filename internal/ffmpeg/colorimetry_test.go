package ffmpeg

import (
	"context"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

// colourOptions are the output options that carry a colour value, and colourFilters
// the filters that state one on the frames. A publish command says what colour its
// stream is in both places at once, so a test that runs one and not the other asserts
// half a description.
var (
	colourOptions = []string{"-color_range", "-colorspace", "-color_primaries", "-color_trc"}
	colourFilters = []string{"setparams", "scale", "zscale"}
)

// elementaryMuxers is the output format one round trip writes each codec format with:
// the bitstream and its framing, and nothing that could describe a colour. H.26x needs
// no framing at all, since Annex B start codes carry it. An AV1 OBU states its own
// size, so the low-overhead stream needs none either. VP9 and VP8 frames do not, so
// they travel in IVF, whose 32-byte header holds a fourcc, the picture size, the frame
// rate and the frame count: no colour field of any kind, and byte-identical between a
// full-range and a limited-range encode.
//
// A container would answer in the encoder's place: stream-copying an untagged AV1
// stream into MP4 while stating a colour on the copy makes the same bytes probe as
// full-range BT.709.
var elementaryMuxers = map[string]string{
	"h264": "h264",
	"hevc": "hevc",
	"av1":  "obu",
	"vp9":  "ivf",
	"vp8":  "ivf",
}

// bitstreamColour names the colour entries each format has a field for, as ffprobe
// names them. AV1 codes the full description in its sequence header. VP9 codes one
// three-bit colour-space enum and a range bit, so it carries the matrix and the range
// and has nowhere to put primaries or transfer. VP8 codes a single colour-space bit
// with one defined value and no range at all (vp8NoFullRange).
//
// An entry a format has no field for reads back unsignalled whatever the encoder was
// told, which is what makes the list an assertion rather than an allowance: a viewer
// picks that component off the picture size, and the table has to gap the setting the
// stream cannot carry rather than offer it.
var bitstreamColour = map[string][]string{
	"h264": {"color_range", "color_space", "color_primaries", "color_transfer"},
	"hevc": {"color_range", "color_space", "color_primaries", "color_transfer"},
	"av1":  {"color_range", "color_space", "color_primaries", "color_transfer"},
	"vp9":  {"color_range", "color_space"},
	"vp8":  {},
}

// unsignalledColour is what ffprobe prints for a colour entry the bitstream leaves out.
const unsignalledColour = "unknown"

// A stream's colour reaches a viewer through the bitstream alone. RTP and MPEG-TS
// carry no colour description, so a component the encoder leaves unsignalled is one
// the viewer picks for itself, and it picks limited-range BT.709 off the picture
// size. That makes the colour-range setting an option nothing acts on: a full-range
// publish is then expanded as if it were limited, which crushes the blacks and clips
// the whites of the screen it captured.
//
// So this runs what the publish command says about colour through a real encode and
// reads the bitstream back with ffprobe, which is what a viewer's decoder reads too.
// The options and the filter are lifted from BuildPublishArgs rather than restated,
// so the test asserts the command a publish runs and not a copy of it.
func TestPublishedColorimetryIsSignalledInTheBitstream(t *testing.T) {
	exe, err := FindExe("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	probe, err := FindExe("ffprobe")
	if err != nil {
		t.Skip("ffprobe not installed")
	}

	for _, cap := range capabilities.Codecs {
		if !cap.Implemented {
			continue
		}
		if _, gap := cap.EngineGap("ffmpeg"); gap {
			continue
		}
		muxer, ok := elementaryMuxers[cap.Format]
		if !ok {
			t.Errorf("format %s is published on this engine and has no elementary muxer, so nothing measures the colour its streams arrive in", cap.Format)
			continue
		}
		device, surface, err := HwSurfaceDevice(cap.Name)
		if err != nil {
			t.Logf("%s: %v", cap.Name, err)
			continue
		}

		for _, colorRange := range []string{"pc", "tv"} {
			t.Run(cap.Name+"/"+colorRange, func(t *testing.T) {
				s := baseStream()
				// RTSP carries every format the codec table holds, so the transport check
				// inside BuildPublishArgs never decides which codecs this covers.
				s.Publish.Transport, s.Publish.RtspPublishProtocol = "rtsp", settings.Defaults().Publish.RtspPublishProtocol
				s.Publish.Codec, s.Publish.ColorRange = cap.Name, colorRange
				s.Publish.Chroma = yuvChroma(t, cap)
				s.Publish.Cq = cap.CqMaxOn(capabilities.EngineFfmpeg) / 2
				if limit := cap.BitrateLimitOn(capabilities.EngineFfmpeg); limit > 0 && s.Publish.BitrateM > limit {
					s.Publish.BitrateM = limit
				}

				// A colour range this engine has no way to state is declared as a gap and
				// refused before a command is built, so the refusal is what this asserts
				// for it. Encoding it anyway is the outcome the gap exists to prevent: a
				// stream watched in another range than the form shows.
				if gap, gapped := cap.OptionGap(capabilities.EngineFfmpeg, capabilities.OptionColorRange, colorRange); gapped {
					if _, err := BuildPublishArgs(s, nil); err == nil {
						t.Fatalf("colour range %s is gapped on this engine, so it must be refused rather than encoded: %s",
							colorRange, gap.Reason)
					}
					return
				}

				options, filters := publishColour(t, s)
				enc, err := encoderArgs(s, gopFor(s))
				if err != nil {
					t.Fatal(err)
				}
				if surface {
					upload, err := HwSurfaceFilters(s.Publish.Codec, s.Publish.Chroma)
					if err != nil {
						t.Fatal(err)
					}
					filters = append(filters, upload...)
				}

				stream := filepath.Join(t.TempDir(), "publish."+muxer)
				args := []string{"-hide_banner", "-loglevel", "error"}
				args = append(args, device...)
				args = append(args, "-f", "lavfi", "-i", "color=white:s=320x240:r=30:d=0.2")
				if len(filters) > 0 {
					args = append(args, "-vf", strings.Join(filters, ","))
				}
				args = append(args, enc...)
				args = append(args, options...)
				// A surface encode's layout is the upload filter's, and its encoder reads
				// no software pixel format at all, exactly as in BuildPublishArgs.
				if !surface {
					args = append(args, "-pix_fmt", s.Publish.Chroma)
				}
				args = append(args, "-f", muxer, stream)

				ctx, cancel := context.WithTimeout(context.Background(), encodeTimeout)
				defer cancel()
				if out, err := exec.CommandContext(ctx, exe, args...).CombinedOutput(); err != nil {
					// An absent hardware encoder is the machine's answer, not the
					// builder's, the same split TestEncoderArgsAgainstFfmpeg makes.
					if cap.Family == "software" {
						t.Fatalf("ffmpeg %s: %v\n%s", strings.Join(args, " "), err, out)
					}
					t.Skipf("%s does not run on this machine", cap.Name)
				}

				signalled := signalledColour(t, probe, stream)
				for _, want := range []struct{ entry, value string }{
					{"color_range", colorRange},
					{"color_space", colourDescription},
					{"color_primaries", colourDescription},
					{"color_transfer", colourDescription},
				} {
					got := signalled[want.entry]
					if !slices.Contains(bitstreamColour[cap.Format], want.entry) {
						// The format has no field for this component, so the viewer picks it
						// off the picture size. Asserting the unsignalled read keeps the
						// list a measurement: a format that starts carrying a component
						// belongs in bitstreamColour, and a range that stays unsignalled
						// belongs in a colour-range gap.
						if got != unsignalledColour {
							t.Errorf("%s signals %s=%s where the %s bitstream has no field for it: the format's entry in bitstreamColour is stale",
								cap.Name, want.entry, got, cap.Format)
						}
						continue
					}
					if got != want.value {
						t.Errorf("%s published at colour range %s signals %s=%s, want %s: a viewer picks that component itself",
							cap.Name, colorRange, want.entry, got, want.value)
					}
				}
			})
		}
	}
}

// The colour a stream claims has to be the colour it holds. Tagging the frames ahead
// of the conversion to the encoder's pixel format states the range as well as the
// description, and the conversion then writes limited range whatever -color_range
// says: full-range white would leave the capture chain at Y=235 under a bitstream
// claiming 255, which is worse than the unsignalled stream it replaced, since the
// viewer expands it a second time.
//
// White is the frame that shows it, at the top of the range where the two spellings
// are 20 codes apart.
func TestPublishedFullRangeStaysFullRangeThroughTheColourTag(t *testing.T) {
	exe, err := FindExe("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}

	for _, tc := range []struct {
		colorRange string
		white      byte
	}{
		{colorRange: "pc", white: 255},
		{colorRange: "tv", white: 235},
	} {
		t.Run(tc.colorRange, func(t *testing.T) {
			s := baseStream()
			s.Publish.Codec, s.Publish.Chroma, s.Publish.ColorRange = "libx264", "yuv420p", tc.colorRange
			options, filters := publishColour(t, s)

			stream := filepath.Join(t.TempDir(), "publish.h264")
			args := []string{"-hide_banner", "-loglevel", "error",
				"-f", "lavfi", "-i", "color=white:s=320x240:r=30:d=0.2"}
			if len(filters) > 0 {
				args = append(args, "-vf", strings.Join(filters, ","))
			}
			args = append(args, "-c:v", "libx264")
			args = append(args, options...)
			args = append(args, "-pix_fmt", s.Publish.Chroma, "-f", "h264", stream)

			ctx, cancel := context.WithTimeout(context.Background(), encodeTimeout)
			defer cancel()
			if out, err := exec.CommandContext(ctx, exe, args...).CombinedOutput(); err != nil {
				t.Fatalf("ffmpeg %s: %v\n%s", strings.Join(args, " "), err, out)
			}

			// The decoded frame is already in the encoder's own layout, so the first
			// byte of it is the stored luma and no conversion stands between.
			decode := exec.CommandContext(ctx, exe, "-hide_banner", "-loglevel", "error",
				"-i", stream, "-frames:v", "1", "-pix_fmt", s.Publish.Chroma, "-f", "rawvideo", "-")
			frame, err := decode.Output()
			if err != nil {
				t.Fatalf("decode %s: %v", stream, err)
			}
			if len(frame) == 0 {
				t.Fatalf("decoding %s yielded no frame", stream)
			}
			if frame[0] != tc.white {
				t.Errorf("white published at colour range %s is stored as Y=%d, want %d: the stream holds another range than it signals",
					tc.colorRange, frame[0], tc.white)
			}
		})
	}
}

// yuvChroma is the narrowest YUV format this engine reaches for the codec. Planar
// RGB is left out because it is full range by construction and carries no colour
// range at all, which is why BuildPublishArgs states none for it.
func yuvChroma(t *testing.T, cap capabilities.Codec) string {
	t.Helper()
	chromas := cap.EngineChromas("ffmpeg")
	for i := len(chromas) - 1; i >= 0; i-- {
		if chromas[i] != "gbrp" {
			return chromas[i]
		}
	}
	t.Fatalf("codec %s reaches no YUV format on this engine", cap.Name)
	return ""
}

// publishColour is everything the publish command says about colour: the output
// options with their values, and the filter links that tag the frames.
func publishColour(t *testing.T, s settings.Settings) (options, filters []string) {
	t.Helper()
	args, err := BuildPublishArgs(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range args {
		if slices.Contains(colourOptions, a) && i+1 < len(args) {
			options = append(options, a, args[i+1])
		}
		if a != "-vf" && a != "-filter_complex" || i+1 >= len(args) {
			continue
		}
		for _, link := range strings.Split(args[i+1], ",") {
			name, _, _ := strings.Cut(link, "=")
			if slices.Contains(colourFilters, name) {
				filters = append(filters, link)
			}
		}
	}
	return options, filters
}

// signalledColour is what the bitstream says about its colour, keyed as ffprobe names
// the entries. An entry the stream leaves unsignalled reads "unknown", which is what
// the viewer has to replace with a guess.
func signalledColour(t *testing.T, probe, stream string) map[string]string {
	t.Helper()
	out, err := exec.Command(probe, "-v", "error",
		"-show_entries", "stream=color_range,color_space,color_primaries,color_transfer",
		"-of", "default=noprint_wrappers=1", stream).Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", stream, err)
	}
	signalled := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		entry, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok {
			signalled[entry] = value
		}
	}
	return signalled
}
