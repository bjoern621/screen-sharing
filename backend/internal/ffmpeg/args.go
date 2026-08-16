// Package ffmpeg builds the publish command line, locates the media executables and supervises the
// child processes.
// Watch commands live in the watch package.
//
// scripts/publish.ps1 is the reference for what the builder must produce: its values were tuned
// against a live relay.
// The destination and the muxer come from the transport registry, so the encoder arguments carry
// nothing about how bytes leave the machine.
package ffmpeg

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/cursor"
	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/gpu"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

// Tap is a second output the encoded video is copied to, never a second encode.
// ffmpeg builds one encoder per output, so two outputs written the ordinary way put two encoders on
// one capture.
// The tee muxer is the one shape that writes one encoder's packets to several muxers.
//
// Options are the slave's own, already in the tee muxer's key=value spelling, and are the caller's
// business: what to select, which muxer, and whatever that muxer needs.
// This package owns where the tee goes and which streams reach it.
type Tap struct {
	Options []string
	URL     string
}

// BuildPublishArgs returns the arguments, without the executable, that capture the selected monitor
// and push the encoded stream to the relay.
// The order is scripts/publish.ps1's: capture input, encoder, pixel format and colour range, GOP,
// then the transport's muxer and destination.
//
// taps is empty for a command with one output.
// Any tap makes the command a tee, which changes two things: automatic stream selection does not
// apply to a tee, so the streams are mapped by hand, and the transport's own output arguments are
// rendered as a slave of it beside every tap.
func BuildPublishArgs(s settings.Settings, taps []Tap) ([]string, error) {
	if _, ok := transport.Get(s.Publish.Transport); !ok {
		return nil, fmt.Errorf("unknown transport %q", s.Publish.Transport)
	}

	if err := capabilities.Validate(capabilities.EngineFfmpeg, s.Publish.Codec, s.Publish.CapabilityOptions(), s.Publish.Cq, s.Publish.BitrateM, s.Publish.Gop, gpu.Device()); err != nil {
		return nil, err
	}
	if err := transport.ValidatePublish(s.Publish.Transport, capabilities.EngineFfmpeg, s.Publish.Codec); err != nil {
		return nil, err
	}
	if err := capabilities.ValidateAudio(capabilities.EngineFfmpeg, s.Publish.AudioTrack()); err != nil {
		return nil, err
	}
	if err := transport.ValidatePublishAudio(s.Publish.Transport, capabilities.EngineFfmpeg, s.Publish.AudioTrack()); err != nil {
		return nil, err
	}
	if err := transport.ValidatePublishSettings(s); err != nil {
		return nil, err
	}
	// Every grabber takes the rate as an input option and gopFor derives the keyframe interval from it,
	// so a non-positive rate reaches the command as "-framerate 0" and "-g 0".
	// The GStreamer engine refuses the same value.
	if s.Publish.Fps <= 0 {
		return nil, fmt.Errorf("the ffmpeg publish engine needs a positive fps, got %d", s.Publish.Fps)
	}

	memory, err := frameMemory(s)
	if err != nil {
		return nil, err
	}

	src, err := captureArgs(s, memory)
	if err != nil {
		return nil, err
	}

	audioIn, err := audioInputArgs(s)
	if err != nil {
		return nil, err
	}

	enc, err := encoderArgs(s, gopFor(s))
	if err != nil {
		return nil, err
	}
	assert.Assert(len(src.args) > 0 || len(src.filters) > 0, "a validated stream yields a capture source", s.Publish.Capture)
	assert.Assert(len(enc) > 0, "a validated stream yields an encoder", s.Publish.Codec)

	size, scaled, err := s.Publish.OutputSize()
	if err != nil {
		return nil, err
	}

	// Frames never become software ones on the GPU path, so the map and the device-side conversion
	// replace the colour tag, the upload and the device option at once: the conversion states the
	// colour it wrote, and the encoder takes its device from the frames handed to it (gpu.go).
	var device []string
	onDevice := gpupath.OnDevice(memory)
	surface := onDevice
	if surface {
		// The device-side conversion carries the scale, since swscale cannot read frames already on the
		// device and no software stage is left to put one in.
		// A family whose device path leaves the conversion to the encoder has nothing to carry a size
		// either, and GpuFilters refuses that run rather than dropping the setting.
		gpu, err := GpuFilters(s.Publish.Codec, s.Publish.Chroma, s.Publish.ColorRange, size, scaled)
		if err != nil {
			return nil, err
		}
		src.filters = append(src.filters, gpu...)
	} else {
		// Scaled ahead of both the colour tag and the upload: the tag describes what the encoder is given
		// and the upload pins a surface layout, so a later scale would resize a picture both had already
		// been told the shape of.
		if scaled {
			src.filters = append(src.filters, scaleFilter(size))
		}

		// The colour description rides on the frames, so it is tagged while they are software ones, ahead
		// of the upload a surface encode ends its chain in.
		if colour := colourFilter(s.Publish.Chroma); colour != "" {
			src.filters = append(src.filters, colour)
		}

		// A VAAPI, QSV or Vulkan encoder reads GPU surfaces, so its device opens ahead of the input and the
		// grabber's chain ends in a conversion and an upload (hwsurface.go).
		var uploads bool
		device, uploads, err = HwSurfaceDevice(s.Publish.Codec)
		if err != nil {
			return nil, err
		}
		if uploads {
			upload, err := HwSurfaceFilters(s.Publish.Codec, s.Publish.Chroma)
			if err != nil {
				return nil, err
			}
			src.filters = append(src.filters, upload...)
		}
		surface = uploads
	}

	// The audio inputs follow the capture's, so the index each takes is a count of what came before them
	// rather than a constant: a filter source opens no -i at all, and a grabber that is one would
	// otherwise have its audio mapped off an input that is not there.
	inputs := inputCount(src.args)
	audioFilters, audioOut := audioMixFilters(s, inputs)

	// The chain and the maps are one decision wherever a tee or a mixed track exists: neither a filter
	// source nor a mix has an input to map, so each chain labels its output and the map names that
	// label.
	// With neither, ffmpeg's automatic stream selection picks the one video and the one audio stream.
	filters := src.filters
	var maps []string
	if len(taps) > 0 || len(audioFilters) > 0 {
		video := strconv.Itoa(inputs-1) + ":v"
		if src.filterFlag == "-filter_complex" {
			assert.Assert(len(filters) > 0, "a filter source yields a chain", s.Publish.Capture)
			assert.Assert(inputs == 0, "a filter source captures without an input", s.Publish.Capture, inputs)

			filters = append([]string{}, filters...)
			filters[len(filters)-1] += filterOutLabel
			video = filterOutLabel
		}
		maps = append(maps, "-map", video)
		if len(audioFilters) > 0 {
			maps = append(maps, "-map", audioOut)
		}
	}

	args := []string{"-hide_banner"}
	args = append(args, device...)
	args = append(args, src.args...)
	args = append(args, audioIn...)

	// The audio graph is always a complex one: it reads inputs by index and labels what it produces,
	// neither of which -af can do.
	// Beside a simple capture chain the two ride as separate options, each naming its own streams.
	// Beside a filter source they are one graph of two chains, since a second -filter_complex replaces
	// the first rather than adding to it.
	complexAudio := len(audioFilters) > 0
	if complexAudio && src.filterFlag == "-filter_complex" {
		filters = append(append([]string{}, strings.Join(filters, ",")), audioFilters...)
		args = append(args, "-filter_complex", strings.Join(filters, ";"))
	} else {
		if len(filters) > 0 {
			args = append(args, src.filterFlag, strings.Join(filters, ","))
		}
		if complexAudio {
			args = append(args, "-filter_complex", strings.Join(audioFilters, ";"))
		}
	}
	args = append(args, maps...)
	args = append(args, enc...)
	// The upload filter has pinned a surface encode's layout and the encoder's own pixel format is the
	// opaque hardware one, so -pix_fmt would ask ffmpeg to convert GPU surfaces it cannot read.
	if !surface {
		args = append(args, "-pix_fmt", s.Publish.Chroma)
	}
	// -color_range steers a conversion to a YUV format, and gbrp is full range by construction.
	// It is dropped on a device path whose family converts nothing as well: no swscale stage is left for
	// it to steer, the encoder converts the captured RGB at a range of its own and signals that one, and
	// a displayed command carries no option the run ignores (gpu.go, gpupath.ColourEncoder).
	if s.Publish.Chroma != "gbrp" && !(onDevice && !GpuStatesColour(s.Publish.Codec)) {
		args = append(args, "-color_range", s.Publish.ColorRange)
	}
	args = append(args, "-g", strconv.Itoa(gopFor(s)))
	if len(audioIn) > 0 {
		enc, err := audioEncodeArgs(s)
		if err != nil {
			return nil, err
		}
		args = append(args, enc...)
	}

	pub, ok := transport.PublishArgs(s)
	if !ok {
		return nil, fmt.Errorf("transport %q has no ffmpeg publish form", s.Publish.Transport)
	}
	if len(taps) == 0 {
		return append(args, pub...), nil
	}

	// The relay leg is rendered from the arguments it would otherwise be the whole output of, rather
	// than declared a second time per transport.
	// A transport states one output, option pairs and then the destination, and a tee slave is that
	// statement in the tee muxer's spelling.
	// A second declaration is what falls behind the first the next time a transport grows an option.
	relay, err := teeSlaveOf(pub)
	if err != nil {
		return nil, err
	}
	slaves := relay
	for _, tap := range taps {
		slaves += "|" + teeSlave(tap.Options, tap.URL)
	}
	return append(args, "-f", "tee", slaves), nil
}

// filterOutLabel names the output of a filter source's chain, so a map has something to name where
// there is no input.
// Only a command that tees carries it, the label existing for the map and the map for the tee.
const filterOutLabel = "[out]"

// inputCount reports how many inputs a capture backend's arguments open.
// It counts -i rather than reading a field, because a field is a second copy of the fact and the one
// free to disagree with the arguments built.
func inputCount(args []string) int {
	n := 0
	for _, arg := range args {
		if arg == "-i" {
			n++
		}
	}
	assert.Assert(n <= 1, "a capture backend opens at most one input", n)
	return n
}

// teeSlaveOf renders one output's arguments as a slave of the tee muxer.
// The input is the shape every transport yields: option and value pairs, then the destination.
// "-f mpegts srt://..." becomes "[f=mpegts]srt://...", and an option a protocol adds for itself
// travels with it.
//
// What the tee spelling cannot hold is refused rather than escaped.
// A colon separates the options inside the bracket and a bar separates the slaves, so neither may
// appear in an option.
// The URL is everything after the bracket and may hold colons, which every destination here does.
func teeSlaveOf(args []string) (string, error) {
	assert.Assert(len(args) > 0, "an output yields arguments")

	url := args[len(args)-1]
	pairs := args[:len(args)-1]
	if len(pairs)%2 != 0 {
		return "", fmt.Errorf("output arguments %q are not option pairs and a destination", strings.Join(args, " "))
	}

	options := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		name, value := strings.TrimPrefix(pairs[i], "-"), pairs[i+1]
		if name == pairs[i] {
			return "", fmt.Errorf("output argument %q is not an option, so it cannot be teed", pairs[i])
		}
		if strings.ContainsAny(name+value, ":|][") {
			return "", fmt.Errorf("output option %s=%s cannot be written as a tee slave", name, value)
		}
		options = append(options, name+"="+value)
	}
	if strings.Contains(url, "|") {
		return "", fmt.Errorf("destination %q cannot be written as a tee slave", url)
	}
	return teeSlave(options, url), nil
}

// teeSlave writes one slave: "[key=value:key=value]url".
func teeSlave(options []string, url string) string {
	assert.Assert(url != "", "a tee slave names its destination", strings.Join(options, ":"))

	return "[" + strings.Join(options, ":") + "]" + url
}

// scaleAlgorithm is what swscale resamples with.
// Lanczos rather than the bicubic default: what is scaled is a desktop of text and hairlines, and a
// softer kernel costs exactly the edges a reader is reading.
// Stated rather than left to the default, so a displayed command names the resampler that ran.
const scaleAlgorithm = "lanczos"

// scaleFilter resamples software frames to the size the encoder is fed.
// The aspect ratio is not preserved: the sizes come from a list derived from the capture's own
// dimensions, so one that does not match the source's ratio was generated deliberately.
func scaleFilter(size settings.Size) string {
	return fmt.Sprintf("scale=%d:%d:flags=%s", size.Width, size.Height, scaleAlgorithm)
}

// colourDescription is the colour space this engine encodes against, spelled as setparams and
// ffprobe name it.
// BT.709 covers every HD and larger picture, which is every screen this app captures, and the
// GStreamer engine pins the same one as gstBt709, so a stream's colour does not follow from the
// engine that published it.
const colourDescription = "bt709"

// colourFilter tags captured frames with the colour space they are encoded against, empty for a
// chroma with none to state.
//
// The tag is what puts the description in the bitstream, and the bitstream is the only place a
// viewer reads it from, since RTP and MPEG-TS carry no colour of their own.
// A component left unsignalled is one the viewer picks off the picture size, and it picks
// limited-range BT.709: an unsignalled full-range stream is then expanded as limited, crushing the
// blacks and clipping the whites.
//
// The output options reach part of the description only.
// -colorspace lands in the bitstream where -color_primaries and -color_trc do not, and a partial
// description is no description, a GStreamer viewer reporting no colorimetry at all for one.
//
// The range is deliberately absent from the tag.
// Left unspecified on the frames, the conversion to the encoder's pixel format takes its target range
// from -color_range.
// Stated here as well, that conversion writes limited range whatever -color_range says, and
// full-range white reaches the encoder as Y=235 under a bitstream claiming 255.
//
// Planar RGB is skipped for the reason it carries no -color_range: no matrix, and full range by
// construction.
//
// The chroma is the parameter rather than the settings because the GPU path needs the same tag from
// a caller holding no settings struct, and on a pair whose device path leaves the conversion to the
// encoder this tag is the whole of what the command still states about the colour (gpu.go).
func colourFilter(chroma string) string {
	if chroma == "gbrp" {
		return ""
	}
	return fmt.Sprintf("setparams=colorspace=%s:color_primaries=%s:color_trc=%s",
		colourDescription, colourDescription, colourDescription)
}

// gopFor returns the keyframe interval in frames.
// Zero in the settings is the form's automatic setting, a keyframe every two seconds.
// The encoder builder reads the same figure, since one encoder aligns its parameter-set repeat to
// the GOP and both halves of the command have to name one interval.
func gopFor(s settings.Settings) int {
	if s.Publish.Gop > 0 {
		return s.Publish.Gop
	}
	return s.Publish.Fps * 2
}

// captureSource is one screen grabber's contribution to the command: the input arguments, and the
// filter chain the frames reach the encoder through.
// The chain is held apart from the arguments because the encoder may extend it, a VAAPI encode
// appending a conversion and an upload, and a chain travels in one option.
type captureSource struct {
	// args opens the input: the options and the -i itself, empty for a filter source.
	args []string
	// filters holds one link per element, joined with commas when emitted.
	filters []string
	// filterFlag carries the chain: -vf, or -filter_complex for ddagrab, a source filter with no input
	// to attach a per-stream chain to.
	filterFlag string
}

// captureBackends is the input side of the command, one entry per screen grabber.
// Which operating system a grabber runs on is publish.captureNeeds' column, so nothing here reads a
// platform.
// fps arrives rendered, since every grabber takes it as a string.
//
// A backend errors where the settings name something it cannot capture: a monitor this machine does
// not have, a DRM download strategy no row carries.
// The alternative is a command that captures something else, which is the one outcome no form can
// show.
var captureBackends = map[string]func(s settings.Settings, fps, memory string) (captureSource, error){
	"ddagrab":      ddagrabArgs,
	"gdigrab":      gdigrabArgs,
	"x11grab":      x11grabArgs,
	"kmsgrab":      kmsgrabCaptureArgs,
	"avfoundation": avfoundationArgs,
}

// captureArgs returns the input arguments and filter chain for the configured backend, in the memory
// this run's frames reach the encoder in.
// A backend with no GPU path takes the resolved value all the same and ignores it, which keeps the
// pair table the one place the two shapes are chosen between.
func captureArgs(s settings.Settings, memory string) (captureSource, error) {
	build, ok := captureBackends[s.Publish.Capture]
	if !ok {
		return captureSource{}, fmt.Errorf("unknown capture backend %q", s.Publish.Capture)
	}
	src, err := build(s, strconv.Itoa(s.Publish.Fps), memory)
	if err != nil {
		return captureSource{}, err
	}
	if src.filterFlag == "" {
		src.filterFlag = "-vf"
	}
	return src, nil
}

// frameMemory resolves where this run's frames reach the encoder, against the pair table both
// publish engines read (gpupath.Paths).
//
// No device check follows.
// Every GPU path here maps the captured frames onto a device derived from the frames themselves, so
// the encoder runs on the GPU the capture came off whatever else the machine carries.
func frameMemory(s settings.Settings) (string, error) {
	c, ok := capabilities.Get(s.Publish.Codec)
	if !ok {
		return "", fmt.Errorf("unknown codec %q", s.Publish.Codec)
	}
	return gpupath.Resolve(capabilities.EngineFfmpeg, s.Publish.Capture, c.Family, s.Publish.CaptureMemory)
}

// ddagrabArgs captures on the GPU as a filter source.
// On the system-memory path hwdownload hands the frames back for any encoder to read, and
// format=bgra pins the layout swscale converts from.
// On a device path the texture stays where Desktop Duplication put it, and what follows is the
// family's: a map onto the encoder's device where the family converts there, nothing at all where
// the encoder reads the texture itself (gpu.go).
func ddagrabArgs(s settings.Settings, fps, memory string) (captureSource, error) {
	filters := []string{fmt.Sprintf("ddagrab=output_idx=%d:framerate=%s:draw_mouse=%s",
		s.Publish.Monitor, fps, drawMouse(s))}
	if !gpupath.OnDevice(memory) {
		filters = append(filters, "hwdownload", "format=bgra")
	}
	return captureSource{filterFlag: "-filter_complex", filters: filters}, nil
}

func gdigrabArgs(s settings.Settings, fps, _ string) (captureSource, error) {
	return captureSource{args: []string{
		"-f", "gdigrab", "-framerate", fps, "-draw_mouse", drawMouse(s), "-i", "desktop",
	}}, nil
}

// drawMouse renders the pointer setting as the "1" or "0" every ffmpeg capture device spells it
// with.
// The cursor modes collapse onto those two here: an ffmpeg capture either draws the pointer into the
// frames or leaves it out, and nothing on this path reports a position to send beside the picture.
// A mode a backend does not serve is refused before a command is built (internal/publish/cursor.go),
// so only a mode the device can run reaches here.
func drawMouse(s settings.Settings) string {
	if s.Publish.Cursor == cursor.Embedded {
		return "1"
	}
	return "0"
}

// x11grabArgs crops the X screen to the selected monitor's geometry.
// A monitor index no enumerated output carries is refused: it names a screen this machine does not
// have, and grabbing the whole desktop instead would publish something other than what the form
// shows selected.
// An entry with no geometry is display.List's placeholder for enumeration being unavailable, and the
// whole X screen is what it means.
//
// An unset DISPLAY is refused rather than guessed at: x11grab reads an X screen, and an environment
// naming none is no session to capture.
func x11grabArgs(s settings.Settings, fps, _ string) (captureSource, error) {
	disp := os.Getenv("DISPLAY")
	if disp == "" {
		return captureSource{}, fmt.Errorf("x11grab captures an X screen and DISPLAY names none")
	}
	args := []string{"-f", "x11grab", "-framerate", fps, "-draw_mouse", drawMouse(s)}
	m, ok := display.At(s.Publish.Monitor)
	if !ok {
		return captureSource{}, fmt.Errorf("monitor %d is not one of this machine's outputs", s.Publish.Monitor)
	}
	if m.Width <= 0 || m.Height <= 0 {
		return captureSource{args: append(args, "-i", disp)}, nil
	}
	return captureSource{args: append(args,
		"-video_size", fmt.Sprintf("%dx%d", m.Width, m.Height),
		"-i", fmt.Sprintf("%s+%d,%d", disp, m.OffsetX, m.OffsetY),
	)}, nil
}

// avfoundationScreenDevice is what AVFoundation lists a screen under, the monitor index appended:
// "Capture screen 0".
// ffmpeg takes the video and the audio device in one -i separated by a colon, so the audio half
// rides in this string rather than in an input of its own.
const avfoundationScreenDevice = "Capture screen "

// avfNoAudioDevice leaves the audio half of the device string unset.
// The two halves travel in one -i, so a device named there would become this input's second stream
// whatever the audio setting says.
// The second track opens its own inputs instead (audioInputArgs).
const avfNoAudioDevice = ":none"

// avfoundationArgs captures a macOS screen through AVFoundation.
// The monitor index goes into the device name unchecked, the way ddagrab's output_idx does: it
// indexes AVFoundation's own screen list, and display.List has no macOS enumerator to hold it
// against, only a placeholder that would refuse every index but zero.
//
// The device naming follows ffmpeg's documented avfoundation input rather than a reading off a
// machine, macOS being unavailable here.
func avfoundationArgs(s settings.Settings, fps, _ string) (captureSource, error) {
	return captureSource{args: []string{
		"-f", "avfoundation",
		"-capture_cursor", drawMouse(s),
		"-framerate", fps,
		"-i", avfoundationScreenDevice + strconv.Itoa(s.Publish.Monitor) + avfNoAudioDevice,
	}}, nil
}

// audioInputArgs returns one capture input per recorded source, in list order.
//
// Each opens by the handle its kind or its own device names: desktop audio as the monitor of the
// default sink, an entry naming a device as that device (platform.AudioSourceDevice).
// The names are stated once for both engines, and "-f pulse" against GStreamer's "pulsesrc device="
// is the whole of the difference.
//
// Whether the backend's platform serves a kind is settled above this builder, in
// publish.AudioAvailable: arguments are built from the settings alone, and which operating system a
// capture backend runs on is publish's column.
// A source no platform serves reaching here is a caller that skipped that check, the way an unknown
// codec is (audioEncodeArgs).
func audioInputArgs(s settings.Settings) ([]string, error) {
	var out []string
	for _, a := range s.Publish.Recorded() {
		device, err := audioDevice(a)
		if err != nil {
			return nil, err
		}
		out = append(out, "-f", "pulse", "-i", device)
	}
	return out, nil
}

// audioDevice is the handle one entry opens by: its own device where it names one, the kind's
// default where it does not.
// A kind with neither is refused rather than opened as something else: an entry naming an
// application the enumeration never answered for would record the desktop, which is the wrong room.
func audioDevice(a settings.AudioSource) (string, error) {
	if a.Device != "" {
		return a.Device, nil
	}
	if device := platform.AudioSourceDevice(a.Source); device != "" {
		return device, nil
	}
	return "", fmt.Errorf("audio source %q names no device to open", a.Source)
}

// audioMixFilters is the graph mixing every recorded source into one track, and the label its output
// carries.
// Empty for a stream with no second track.
//
// One track rather than several is carriage: RTMP carries one audio track and the relay re-serves
// every ingest on all of its listeners, so a second track would be unplayable on the narrowest leg
// while the form said it published (domain-model.md).
//
// amix normalizes by default, dividing every input by their number, so adding a second source would
// halve the first.
// The gains are the user's, so normalization is off and each input carries its own volume stage,
// a single source included: a gain applies to one source as it does to three.
//
// first is the input index the audio inputs start at, a count of what the capture contributed rather
// than a constant, since a filter source opens no -i.
func audioMixFilters(s settings.Settings, first int) ([]string, string) {
	recorded := s.Publish.Recorded()
	if len(recorded) == 0 {
		return nil, ""
	}

	filters := make([]string, 0, len(recorded)+1)
	mixed := ""
	for i, a := range recorded {
		label := fmt.Sprintf("[%s%d]", audioStageLabel, i)
		filters = append(filters, fmt.Sprintf("[%d:a]volume=%.3f%s", first+i, a.Volume(), label))
		mixed += label
	}
	if len(recorded) == 1 {
		// One source needs no mixer: its own stage takes the output label and the graph is that one gain.
		return []string{strings.TrimSuffix(filters[0], mixed) + audioOutLabel}, audioOutLabel
	}
	return append(filters, fmt.Sprintf("%samix=inputs=%d:normalize=0%s",
		mixed, len(recorded), audioOutLabel)), audioOutLabel
}

// Labels for the audio graph's stages and for its output.
// The map names the output one, the way it names a video filter source's (filterOutLabel).
const (
	audioStageLabel = "a"
	audioOutLabel   = "[aout]"
)

// audioEncodeArgs codes the captured audio as a stereo track in the configured codec.
// Element, sample rate and bitrate all come off capabilities.AudioCodecs, so both engines code one
// track from one declaration rather than from an element list each.
//
// The stream reaching here is validated, so an unknown codec or one this engine has no encoder for
// is a caller that skipped the validator rather than a user's value.
func audioEncodeArgs(s settings.Settings) ([]string, error) {
	a, ok := capabilities.GetAudio(s.Publish.AudioTrack())
	if !ok {
		return nil, fmt.Errorf("unknown audio codec %q", s.Publish.AudioTrack())
	}
	enc, ok := a.EncoderOn(capabilities.EngineFfmpeg)
	if !ok {
		return nil, fmt.Errorf("audio codec %s has no ffmpeg encoder", a.Name)
	}
	return []string{
		"-c:a", enc.Element,
		"-b:a", fmt.Sprintf("%dk", a.BitrateK),
		"-ar", strconv.Itoa(a.Rate),
		"-ac", "2",
	}, nil
}
