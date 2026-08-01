package publish

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/gpupath"
	"bjoernblessin.de/screenshare/settings"
	"bjoernblessin.de/screenshare/transport"
)

// Every pixel format the capability table leaves reachable on this engine has to
// name a raw format the capture chain can pin, or a portal publish fails after the
// UI offered the combination.
//
// Both memories, since which layouts an element negotiates is a fact about the element
// and a family's device elements are not its system ones: a chroma the form offers and
// only one of the two maps holds is a publish that fails on the path it resolves to.
//
// The reverse is not a rule: the format map keys off the encoder family, so a chroma
// one element rejects can still name a layout the family's other elements take. What
// keeps that combination out of a pipeline is the gap, checked below.
func TestGstChromaFormatCoversTheEngineChromas(t *testing.T) {
	for _, c := range capabilities.Codecs {
		if !c.Implemented {
			continue
		}
		memories := []string{gpupath.MemorySystem}
		if _, onDevice := gstGpuMemories[c.Family]; onDevice {
			memories = append(memories, gpupath.MemoryGpu)
		}
		for _, chroma := range c.EngineChromas("gstreamer") {
			for _, memory := range memories {
				if _, err := gstChromaFormat(c.Name, chroma, memory); err != nil {
					t.Errorf("%s in %s memory: %v", c.Name, memory, err)
				}
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
		caps, err := gstTestCaps(s)
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
	opts, err := gstSourceOptions(s)
	if err != nil {
		t.Fatal(err)
	}
	for name, p := range captureBackends {
		g, ok := p.(gstEngine)
		if !ok {
			continue
		}
		elements := g.capture.Describe(s, opts)
		if len(elements) == 0 {
			t.Errorf("%s: capture backend describes no elements", name)
			continue
		}
		if !slices.Contains(elements, opts.InCaps) {
			t.Errorf("%s: capture elements %v do not carry the encoder input caps %q", name, elements, opts.InCaps)
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
	opts, err := gstSourceOptions(s)
	if err != nil {
		t.Fatal(err)
	}
	// The probe is matched as the whole element rather than by its name alone,
	// which several source elements carry as a substring of their own ("capture-screen",
	// "d3d11screencapturesrc").
	probe := strings.Join(gstCaptureProbe, " ")
	for name, p := range captureBackends {
		g, ok := p.(gstEngine)
		if !ok {
			continue
		}
		plain := strings.Join(g.capture.Describe(s, opts), " ")
		if strings.Contains(plain, probe) {
			t.Errorf("%s: a pipeline built without instrumentation carries the rate probe: %s", name, plain)
		}
		probed := strings.Join(g.capture.Describe(s, gstProbed(opts)), " ")
		if !strings.Contains(probed, probe) {
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
	opts, err := gstSourceOptions(s)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(portalCapture{}.Describe(s, gstProbed(opts)), " ")
	probe, pacer := strings.Index(line, gstCaptureName), strings.Index(line, "imagefreeze")
	if probe < 0 || pacer < 0 || probe > pacer {
		t.Errorf("the rate probe must precede imagefreeze: %s", line)
	}
}

// A chroma this engine's encoder element cannot take is refused rather than
// converted to the nearest format the element does negotiate. Planar RGB on the
// software HEVC row is that case: x265enc negotiates YUV alone where the ffmpeg
// engine codes the format directly.
//
// It is not the default settings' case any more. Those carry planar RGB on
// hevc_nvenc, whose nvcodec elements do take a GBR sink format, so the codec is
// named here rather than taken from the defaults: the combination this covers has
// to stay a gapped one whatever the defaults move to.
//
// The rejection has to come from the caps step, because that is the one the
// engine runs before it acquires a source: refused later, a gapped chroma would
// already have popped the compositor's screen picker.
func TestGstRejectsAGappedChromaBeforeAnythingIsAcquired(t *testing.T) {
	s := settings.Defaults()
	s.Capture = "portal"
	s.Transport = "srt"
	s.Codec = "libx265"
	s.Chroma = "gbrp"
	cap, ok := capabilities.Get(s.Codec)
	if !ok {
		t.Fatalf("codec %s has no capability row", s.Codec)
	}
	if _, gapped := cap.OptionGap(EngineGst, capabilities.OptionChroma, s.Chroma); !gapped {
		t.Skipf("%s at %s is no longer gapped on this engine, so it no longer covers the refusal", s.Codec, s.Chroma)
	}

	_, err := gstTestCaps(s)
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
		if !transport.CanPublish(name, EngineGst) {
			continue
		}
		s := settings.Defaults()
		s.Capture, s.Transport, s.Audio = "portal", name, "desktop"
		// libx264 over every transport: the transport's own format set decides
		// whether it may carry the codec, and this asserts the pipeline's shape.
		s.Codec, s.Chroma = "libx264", "yuv420p"
		if err := transport.ValidatePublish(name, EngineGst, s.Codec); err != nil {
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

// The audio branch is built from the capability table: the element that codes the selected
// codec on this engine, the parser that frames it for the muxer pad, and the rate the
// encoder codes at.
// Spelling any of them here instead would state one codec's answer for every codec, where
// the ffmpeg engine codes the same setting with an element of its own.
func TestGstAudioBranchNamesTheTableElements(t *testing.T) {
	for _, a := range capabilities.AudioCodecs {
		enc, ok := a.EncoderOn(EngineGst)
		if !ok {
			continue
		}
		s := settings.Defaults()
		// RTSP carries every audio codec the table holds, so the transport never decides
		// which of them this covers.
		s.Transport, s.Audio, s.AudioCodec = "rtsp", "desktop", a.Name
		branch, err := gstAudioBranch(s)
		if err != nil {
			t.Fatalf("%s: %v", a.Name, err)
		}
		joined := strings.Join(branch, " ")
		for _, want := range []string{
			enc.Element,
			enc.Parser,
			fmt.Sprintf("rate=%d", a.Rate),
			fmt.Sprintf("bitrate=%d", a.BitrateK*1000),
		} {
			if !strings.Contains(joined, want) {
				t.Errorf("%s: audio branch %q lacks %q", a.Name, joined, want)
			}
		}
		// The branch ends at the muxer's request pad, which is what makes it a second track
		// rather than a pipeline of its own.
		if !strings.HasSuffix(joined, transport.GstMuxName+".") {
			t.Errorf("%s: audio branch %q must end at the mux name", a.Name, joined)
		}
	}

	// A source that is off yields no branch at all, whatever codec the settings carry,
	// and one no backend records is refused rather than left silent.
	for _, source := range []string{"none", ""} {
		s := settings.Defaults()
		s.Audio = source
		branch, err := gstAudioBranch(s)
		if err != nil {
			t.Fatalf("audio source %q: %v", source, err)
		}
		if len(branch) > 0 {
			t.Errorf("audio source %q yields %v, want no branch", source, branch)
		}
	}
	s := settings.Defaults()
	s.Audio = "microphone"
	if _, err := gstAudioBranch(s); err == nil {
		t.Error("an audio source no backend records must be refused")
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
		if _, err := gstTestCaps(s); err == nil {
			t.Errorf("colour range %q must be refused, not read as limited", bad)
		}
	}
	for _, good := range []string{"pc", "tv"} {
		s.ColorRange = good
		if _, err := gstTestCaps(s); err != nil {
			t.Errorf("colour range %q: %v", good, err)
		}
	}
}

// gstGpuStream returns settings publishing the portal capture into a va encoder over
// the direct path, the one pair this engine declares.
func gstGpuStream() settings.Stream {
	s := settings.Defaults()
	s.Capture, s.Codec = "portal", "h264_vaapi"
	// Limited range because the va elements signal no colour description, which the
	// capability table declares as a gap on full range for this engine. It is a fact
	// about what the encoder writes into the bitstream and is unaffected by where the
	// frames came from, so the direct path inherits it.
	s.Chroma, s.Mode, s.ColorRange = "yuv420p", "cbr", "tv"
	s.Transport = "rtsp"
	s.CaptureMemory = gpupath.MemoryGpu
	return s
}

// gstD3d11Stream returns settings publishing the Windows Direct3D capture into an nvenc
// encoder over the direct path, the pair whose conversion keeps the colour on the device.
func gstD3d11Stream() settings.Stream {
	s := settings.Defaults()
	s.Capture, s.Codec = "d3d11screencapturesrc", "h264_nvenc"
	s.Chroma, s.Mode, s.ColorRange = "yuv420p", "cbr", "pc"
	s.Transport = "rtsp"
	s.CaptureMemory = gpupath.MemoryGpu
	return s
}

// Every pair the table declares for this engine has to name the memory its surfaces
// carry and the layouts its elements negotiate there, or a run resolved onto the direct
// path reaches an assertion instead of a pipeline.
func TestEveryGstGpuPathNamesItsMemory(t *testing.T) {
	for _, p := range gpupath.Paths {
		if p.Engine != EngineGst {
			continue
		}
		gpu, ok := gstGpuMemories[p.Family]
		if !ok {
			t.Errorf("%s/%s: the family has a GPU path and no caps feature", p.Capture, p.Family)
			continue
		}
		if len(gpu.formats) == 0 {
			t.Errorf("%s/%s: the family has a GPU path and names no layout its elements negotiate on the device",
				p.Capture, p.Family)
		}
	}
}

// A family whose plugin ships one encoder element per memory kind has to name one for
// every codec it encodes. Missing an entry is worse than missing the table: the run
// resolves onto the device and launches the element that refuses the memory, so the
// failure lands in negotiation with nothing naming the codec.
//
// The reverse holds too, an entry for a codec of another family or one this engine has no
// mapping for being an element no run can ever reach.
func TestEveryGstDeviceEncoderCoversItsFamilysCodecs(t *testing.T) {
	for family, gpu := range gstGpuMemories {
		if len(gpu.encoders) == 0 {
			continue
		}
		for codec, elem := range gpu.encoders {
			c, ok := capabilities.Get(codec)
			if !ok {
				t.Errorf("%s names a device encoder for %s, which is no codec the table holds", family, codec)
				continue
			}
			if c.Family != family {
				t.Errorf("%s names a device encoder for %s, which belongs to the %s family", family, codec, c.Family)
			}
			if _, ok := gstCodecs[codec]; !ok {
				t.Errorf("%s names a device encoder for %s, which this engine has no mapping for", family, codec)
			}
			if elem == "" {
				t.Errorf("%s/%s names an empty device encoder element", family, codec)
			}
		}
		for codec := range gstCodecs {
			cap, ok := capabilities.Get(codec)
			if !ok || cap.Family != family {
				continue
			}
			if _, named := gpu.encoders[codec]; !named {
				plain, _ := GstEncoderElement(codec)
				t.Errorf("%s encodes %s and the family's device path names no element for it, so a run on the device would launch %s against %s",
					family, codec, plain, gpu.feature)
			}
		}
	}
}

// A run on the device is encoded by the element that negotiates the memory the conversion
// produced, and the same codec off system memory by the one that reads system frames. The
// nvcodec plugin ships both, and they are not interchangeable: the plain elements take
// CUDA and Direct3D 12 memory and refuse Direct3D 11.
func TestTheGstDevicePathNamesTheDeviceEncoderElement(t *testing.T) {
	s := gstD3d11Stream()
	device, _, err := gstEncoder(s, 60, gpupath.MemoryGpu)
	if err != nil {
		t.Fatal(err)
	}
	system, _, err := gstEncoder(s, 60, gpupath.MemorySystem)
	if err != nil {
		t.Fatal(err)
	}
	if device[0] == system[0] {
		t.Fatalf("%s names %s on both paths, so one of the two memories is encoded by an element that refuses it",
			s.Codec, device[0])
	}
	want := gstGpuMemories[capabilities.FamilyNvenc].encoders[s.Codec]
	if device[0] != want {
		t.Errorf("the device path encodes %s with %s, want the family's device element %s", s.Codec, device[0], want)
	}
	// The properties are the base class's and shared by both elements, so the memory
	// changes the element name and nothing else about the encode.
	if strings.Join(device[1:], " ") != strings.Join(system[1:], " ") {
		t.Errorf("the two elements are configured differently:\ndevice: %s\nsystem: %s",
			strings.Join(device, " "), strings.Join(system, " "))
	}
	// What the registry is asked for has to be what a run launches, or the availability
	// probe reports the family present while the element is missing.
	elem, named := GstEncoderElementOn(s.Codec, gpupath.MemoryGpu)
	if !named || elem != device[0] {
		t.Errorf("GstEncoderElementOn names %q on the device path, want %s", elem, device[0])
	}
	if elem, named := GstEncoderElement(s.Codec); !named || elem != system[0] {
		t.Errorf("GstEncoderElement names %q, want the system-memory element %s", elem, system[0])
	}
}

// Plain video/x-raw means system memory, so the Direct3D chain has to carry the feature on
// every caps it pins, the rate one on the source included: Desktop Duplication offers a
// texture and nothing else, so a capsfilter naming no feature both fails the negotiation
// and asks for the round trip the path exists to avoid.
func TestTheGstD3d11GpuPathCarriesTheMemoryFeatureOnEveryCaps(t *testing.T) {
	s := gstD3d11Stream()
	opts, err := gstSourceOptions(s)
	if err != nil {
		t.Fatal(err)
	}
	feature := gstGpuMemories[capabilities.FamilyNvenc].feature
	if !strings.Contains(opts.InCaps, feature) {
		t.Errorf("the encoder input caps %q lack the memory feature %q", opts.InCaps, feature)
	}
	for _, caps := range (d3d11Capture{}).Describe(s, gstProbed(opts)) {
		if !strings.HasPrefix(caps, "video/x-raw") || strings.Contains(caps, feature) {
			continue
		}
		t.Errorf("caps %q pin system memory on the GPU path", caps)
	}
	// The conversion has to run on the device as well. videoconvert reads system memory,
	// so its presence would mean the frames were downloaded after all.
	line := strings.Join((d3d11Capture{}).Describe(s, opts), " ")
	if strings.Contains(line, gstSystemConvert) {
		t.Errorf("the GPU path converts on the device, not with %s: %s", gstSystemConvert, line)
	}
	if !strings.Contains(line, gstGpuMemories[capabilities.FamilyNvenc].convert) {
		t.Errorf("the GPU path must convert with the family's post-processor: %s", line)
	}
}

// The same backend off system memory is the chain it was before the pair had a row: a CPU
// conversion and caps naming no device memory.
func TestTheGstD3d11SystemPathPinsNoDeviceMemory(t *testing.T) {
	s := gstD3d11Stream()
	s.CaptureMemory = gpupath.MemorySystem
	opts, err := gstSourceOptions(s)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join((d3d11Capture{}).Describe(s, opts), " ")
	if strings.Contains(line, "memory:") {
		t.Errorf("the system-memory path must pin no device memory: %s", line)
	}
	if !strings.Contains(line, gstSystemConvert) {
		t.Errorf("the system-memory path converts on the CPU: %s", line)
	}
}

// Every capture backend a row names has to be one this app runs, since the row is what
// the form reads to decide whether the direct path is offered at all.
func TestEveryGpuPathNamesARunnableCapture(t *testing.T) {
	for _, p := range gpupath.Paths {
		if _, ok := captureBackends[p.Capture]; !ok {
			t.Errorf("%s/%s/%s names no capture backend this app runs", p.Engine, p.Capture, p.Family)
		}
	}
}

// Plain video/x-raw means system memory, so a capsfilter that omits the feature pins
// the frames back into the round trip. Every caps the chain pins downstream of the
// source has to carry it, the framerate one imagefreeze paces to included.
func TestTheGstGpuPathCarriesTheMemoryFeatureOnEveryCaps(t *testing.T) {
	s := gstGpuStream()
	opts, err := gstSourceOptions(s)
	if err != nil {
		t.Fatal(err)
	}
	feature := gstGpuMemories[capabilities.FamilyVaapi].feature
	if !strings.Contains(opts.InCaps, feature) {
		t.Errorf("the encoder input caps %q lack the memory feature %q", opts.InCaps, feature)
	}
	for _, caps := range (portalCapture{}).Describe(s, opts) {
		if !strings.HasPrefix(caps, "video/x-raw") || strings.Contains(caps, feature) {
			continue
		}
		// The source is pinned to the memory the compositor exports, which is not the
		// encoder's; every other raw caps in the chain is downstream of the conversion.
		if strings.Contains(caps, "memory:DMABuf") {
			continue
		}
		t.Errorf("caps %q pin system memory on the GPU path", caps)
	}
}

// pipewiresrc negotiates the compositor's dmabuf export only when the caps ask for it,
// and settles on the copies PipeWire writes into shared memory when they do not. That
// copy is the round trip the path exists to avoid, so the source is pinned and a
// compositor exporting no dmabuf fails in negotiation rather than delivering it.
func TestThePortalGpuPathPinsTheSourceToDmabuf(t *testing.T) {
	s := gstGpuStream()
	opts, err := gstSourceOptions(s)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(portalCapture{}.Describe(s, opts), " ")
	if !strings.Contains(line, "video/x-raw(memory:DMABuf)") {
		t.Errorf("the portal source must be pinned to dmabuf on the GPU path: %s", line)
	}
	// The conversion has to run on the device as well. videoconvert reads system
	// memory, so its presence would mean the frames were mapped back after all.
	if strings.Contains(line, gstSystemConvert) {
		t.Errorf("the GPU path converts on the device, not with %s: %s", gstSystemConvert, line)
	}
	if !strings.Contains(line, gstGpuMemories[capabilities.FamilyVaapi].convert) {
		t.Errorf("the GPU path must convert with the family's post-processor: %s", line)
	}
}

// The system-memory path is what every pair without a row runs, and it has to stay the
// chain it was: a CPU conversion and caps naming no device memory.
func TestTheGstSystemPathPinsNoDeviceMemory(t *testing.T) {
	s := gstGpuStream()
	s.CaptureMemory = gpupath.MemorySystem
	opts, err := gstSourceOptions(s)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(portalCapture{}.Describe(s, opts), " ")
	if strings.Contains(line, "memory:") {
		t.Errorf("the system-memory path must pin no device memory: %s", line)
	}
	if !strings.Contains(line, gstSystemConvert) {
		t.Errorf("the system-memory path converts on the CPU: %s", line)
	}
}

// Auto is the setting a stored stream carries, so the pair table alone decides which
// chain it builds.
func TestGstAutoFollowsThePairTable(t *testing.T) {
	s := gstGpuStream()
	s.CaptureMemory = gpupath.MemoryAuto
	opts, err := gstSourceOptions(s)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Memory != gpupath.MemoryGpu {
		t.Errorf("auto must take the direct path where the pair has one, got %s", opts.Memory)
	}

	// The same capture into an encoder that reads system memory has no path, and auto
	// resolves to the copy rather than refusing.
	s.Codec, s.Chroma = "libx264", "yuv420p"
	opts, err = gstSourceOptions(s)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Memory != gpupath.MemorySystem {
		t.Errorf("auto must copy where the pair has no direct path, got %s", opts.Memory)
	}
}

// A demand the pair cannot meet is refused before anything is acquired, so a
// combination the form greys never pops the compositor's picker.
func TestGstRefusesTheGpuDemandForAPairWithoutAPath(t *testing.T) {
	s := gstGpuStream()
	s.Codec, s.Chroma = "libx264", "yuv420p"
	if _, err := gstSourceOptions(s); err == nil {
		t.Fatal("the portal into a software encoder has no GPU path and must be refused")
	}
}

// gstTestCaps returns the encoder input caps these settings publish through, resolved
// the way the engine resolves them. Tests that are about the caps alone read them from
// here rather than building a frame memory of their own, so a change in how the memory
// is resolved reaches them.
func gstTestCaps(s settings.Stream) (string, error) {
	opts, err := gstSourceOptions(s)
	return opts.InCaps, err
}

// gstProbed is the source options a run that reports progress carries.
func gstProbed(opts gstCaptureOptions) gstCaptureOptions {
	opts.RateProbe = gstCaptureProbe
	return opts
}
