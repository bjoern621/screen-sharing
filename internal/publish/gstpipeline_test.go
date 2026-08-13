package publish

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

// A pixel format the capability table leaves reachable and the format map has no row for is a
// publish that fails after the UI offered the combination.
//
// Both memories, a family's device elements not being its system ones: a chroma only one of the two
// maps holds fails on the path it resolves to.
//
// The reverse is no rule: the map keys off the encoder family, so a chroma one element rejects can
// still name a layout the family's others take, and what keeps that out of a pipeline is the gap.
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

// The colour-range setting reaches the frames, and not only the caps, as part of a fully named
// colorimetry alone.
// With matrix, transfer and primaries left unknown, videoconvert ignores the range too and converts
// to limited either way.
func TestCaptureCapsNameEveryColorimetryComponent(t *testing.T) {
	for _, tc := range []struct {
		colorRange string
		want       string
	}{
		{colorRange: "pc", want: "colorimetry=1:" + gstBt709},
		{colorRange: "tv", want: "colorimetry=2:" + gstBt709},
	} {
		s := baseStream()
		s.Publish.Chroma = "yuv444p"
		s.Publish.ColorRange = tc.colorRange
		caps, err := gstTestCaps(s)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(caps, tc.want) {
			t.Errorf("color range %q: encoder input caps %q lack %q", tc.colorRange, caps, tc.want)
		}
	}
}

// A backend that does not end in the capsfilter, whatever it does ahead of it, lets the encoder
// negotiate its own format, and the chroma and colour-range settings stop reaching the frames.
func TestEveryGstCaptureBackendEndsInTheEncoderInputCaps(t *testing.T) {
	s := baseStream()
	s.Publish.Chroma = "yuv444p"
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

// The rate probe belongs to a run, so a pipeline built without instrumentation carries none of it:
// the displayed command has to be the one the child runs.
// A backend that drops it when asked reads a capture rate of zero, and the insights card reports
// the pacing target as if it were a measurement.
func TestEveryGstCaptureBackendPlacesTheRateProbeOnlyForARun(t *testing.T) {
	s := baseStream()
	s.Publish.Chroma = "yuv444p"
	opts, err := gstSourceOptions(s)
	if err != nil {
		t.Fatal(err)
	}
	// Matched as the whole element rather than by name: several source elements carry the probe's name
	// as a substring of their own ("capture-screen", "d3d11screencapturesrc").
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

// The probe counts new pictures, so nothing that repeats or paces a frame may sit ahead of it.
// On the portal backend that is imagefreeze, which repeats the newest damage frame at the
// configured rate.
func TestPortalRateProbePrecedesTheFramePacer(t *testing.T) {
	s := baseStream()
	s.Publish.Chroma = "yuv444p"
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

// A chroma this engine's encoder element cannot take is refused rather than converted to the
// nearest format the element does negotiate.
// Planar RGB on the software HEVC row is that case: x265enc negotiates YUV alone where the ffmpeg
// engine codes the format directly.
//
// The codec is named here rather than taken from the defaults, which carry planar RGB on
// hevc_nvenc, whose elements do take a GBR sink format.
// What this covers has to stay a gapped combination whatever the defaults move to.
//
// The rejection comes from the caps step, the one the engine runs before it acquires a source:
// refused later, a gapped chroma has already popped the compositor's screen picker.
func TestGstRejectsAGappedChromaBeforeAnythingIsAcquired(t *testing.T) {
	s := baseStream()
	s.Publish.Capture = "portal"
	s.Publish.Transport = "srt"
	s.Publish.Codec = "libx265"
	s.Publish.Chroma = "gbrp"
	cap, ok := capabilities.Get(s.Publish.Codec)
	if !ok {
		t.Fatalf("codec %s has no capability row", s.Publish.Codec)
	}
	if _, gapped := cap.OptionGap(EngineGst, capabilities.OptionChroma, s.Publish.Chroma); !gapped {
		t.Skipf("%s at %s is no longer gapped on this engine, so it no longer covers the refusal", s.Publish.Codec, s.Publish.Chroma)
	}

	_, err := gstTestCaps(s)
	if err == nil {
		t.Fatal("a chroma gapped on this engine must not yield encoder input caps")
	}
	// A settings file that skipped the form's repair surfaces this message, so it names the format and
	// the engine that codes it.
	if !strings.Contains(err.Error(), "gbrp") || !strings.Contains(err.Error(), "ffmpeg") {
		t.Errorf("the rejection must name the format and the engine that codes it: %v", err)
	}
	if _, err := buildPipeline(s, []string{"videotestsrc"}, "", PreviewLeg{}); err == nil {
		t.Error("a chroma gapped on this engine must not build a pipeline either")
	}
}

// A sink that is muxer and sink in one takes the second track on a request pad rather than through
// a muxer element, so a branch naming a muxer the transport has none of leaves the audio pad
// unlinked at launch.
func TestEveryGstTransportTerminatesAPipelineWithAudio(t *testing.T) {
	for _, name := range transport.Names() {
		if !transport.CanPublish(name, EngineGst) {
			continue
		}
		s := baseStream()
		s.Publish.Capture, s.Publish.Transport = "portal", name
		s.Publish.AudioSources = settings.Recording("desktop")
		// libx264 over every transport: the format set decides carriage, and this asserts the shape of
		// the pipeline.
		s.Publish.Codec, s.Publish.Chroma = "libx264", "yuv420p"
		if err := transport.ValidatePublish(name, EngineGst, s.Publish.Codec); err != nil {
			continue
		}
		pipeline, err := buildPipeline(s, []string{"videotestsrc"}, "", PreviewLeg{})
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

// The audio branch is built from the capability table: the element coding the selected codec on
// this engine, the parser framing it for the muxer pad, and the rate it codes at.
// Spelling any of them here instead would state one codec's answer for every codec, where the
// ffmpeg engine codes the same setting with an element of its own.
func TestGstAudioBranchNamesTheTableElements(t *testing.T) {
	for _, a := range capabilities.AudioCodecs {
		enc, ok := a.EncoderOn(EngineGst)
		if !ok {
			continue
		}
		s := baseStream()
		// RTSP carries every audio codec the table holds, so the transport decides none of this.
		// The backend is one this engine runs on a platform serving the monitor source: the branch is
		// refused per platform before any element is named, and the defaults carry a Windows grabber.
		s.Publish.Capture = "portal"
		s.Publish.Transport, s.Publish.AudioCodec = "rtsp", a.Name
		s.Publish.AudioSources = settings.Recording("desktop")
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
		// Ending at the muxer's request pad is what makes it a second track and not a pipeline of its
		// own.
		if !strings.HasSuffix(joined, transport.GstMuxName+".") {
			t.Errorf("%s: audio branch %q must end at the mux name", a.Name, joined)
		}
	}

	// A source that is off yields no branch whatever codec the settings carry, and one no backend
	// records is refused rather than left silent.
	for _, source := range []string{"none", ""} {
		s := baseStream()
		s.Publish.AudioSources = settings.Recording(source)
		branch, err := gstAudioBranch(s)
		if err != nil {
			t.Fatalf("audio source %q: %v", source, err)
		}
		if len(branch) > 0 {
			t.Errorf("audio source %q yields %v, want no branch", source, branch)
		}
	}
	s := baseStream()
	s.Publish.AudioSources = settings.Recording("microphone")
	if _, err := gstAudioBranch(s); err == nil {
		t.Error("an audio source no backend records must be refused")
	}
}

// The range travels in the bitstream and decides how every viewer expands the picture, so a value
// with no mapping is refused rather than encoded as limited.
// The ffmpeg engine hands the same field to -color_range, which fails on a value it does not know,
// and refusing here is what keeps the two engines answering alike.
func TestGstInputCapsRefusesAnUnmappedColorRange(t *testing.T) {
	s := baseStream()
	s.Publish.Chroma = "yuv444p"
	for _, bad := range []string{"", "full", "limited", "PC"} {
		s.Publish.ColorRange = bad
		if _, err := gstTestCaps(s); err == nil {
			t.Errorf("colour range %q must be refused, not read as limited", bad)
		}
	}
	for _, good := range []string{"pc", "tv"} {
		s.Publish.ColorRange = good
		if _, err := gstTestCaps(s); err != nil {
			t.Errorf("colour range %q: %v", good, err)
		}
	}
}

// gstGpuStream publishes the portal capture into a va encoder over the direct path.
func gstGpuStream() settings.Settings {
	s := baseStream()
	s.Publish.Capture, s.Publish.Codec = "portal", "h264_vaapi"
	// Limited range: the va elements signal no colour description, which the capability table
	// declares as a gap on full range for this engine.
	// The gap is about what the encoder writes into the bitstream, so the direct path inherits it.
	s.Publish.Chroma, s.Publish.Mode, s.Publish.ColorRange = "yuv420p", "cbr", "tv"
	s.Publish.Transport = "rtsp"
	s.Publish.CaptureMemory = gpupath.MemoryGpu
	return s
}

// gstD3d11Stream publishes the Windows Direct3D capture into an nvenc encoder over the direct path,
// the pair whose conversion keeps the colour on the device.
func gstD3d11Stream() settings.Settings {
	s := baseStream()
	s.Publish.Capture, s.Publish.Codec = "d3d11screencapturesrc", "h264_nvenc"
	s.Publish.Chroma, s.Publish.Mode, s.Publish.ColorRange = "yuv420p", "cbr", "pc"
	s.Publish.Transport = "rtsp"
	s.Publish.CaptureMemory = gpupath.MemoryGpu
	return s
}

// A pair the table declares and this engine gives no memory or no layout reaches an assertion
// instead of a pipeline once a run resolves onto the direct path.
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

// A family whose plugin ships one encoder element per memory kind names one for every codec it
// encodes.
// A missing entry is worse than a missing table: the run resolves onto the device and launches the
// element that refuses the memory, so the failure lands in negotiation with nothing naming the
// codec.
//
// An entry for another family's codec, or for one this engine has no mapping for, is an element no
// run can reach.
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

// A run on the device is encoded by the element negotiating the memory the conversion produced,
// and the same codec off system memory by the one reading system frames.
// The nvcodec plugin ships both and they are not interchangeable: the plain elements take CUDA and
// Direct3D 12 memory and refuse Direct3D 11.
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
			s.Publish.Codec, device[0])
	}
	want := gstGpuMemories[capabilities.FamilyNvenc].encoders[s.Publish.Codec]
	if device[0] != want {
		t.Errorf("the device path encodes %s with %s, want the family's device element %s", s.Publish.Codec, device[0], want)
	}
	// The properties belong to the base class both elements share, so the memory changes the element
	// name and nothing else about the encode.
	if strings.Join(device[1:], " ") != strings.Join(system[1:], " ") {
		t.Errorf("the two elements are configured differently:\ndevice: %s\nsystem: %s",
			strings.Join(device, " "), strings.Join(system, " "))
	}
	// What the registry is asked for is what a run launches, or the availability probe reports the
	// family present while the element is missing.
	elem, named := GstEncoderElementOn(s.Publish.Codec, gpupath.MemoryGpu)
	if !named || elem != device[0] {
		t.Errorf("GstEncoderElementOn names %q on the device path, want %s", elem, device[0])
	}
	if elem, named := GstEncoderElement(s.Publish.Codec); !named || elem != system[0] {
		t.Errorf("GstEncoderElement names %q, want the system-memory element %s", elem, system[0])
	}
}

// Plain video/x-raw is system memory, so the Direct3D chain carries the feature on every caps it
// pins, the rate one on the source included.
// Desktop Duplication offers a texture and nothing else, so a capsfilter naming no feature both
// fails negotiation and asks for the round trip the path exists to avoid.
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
	// videoconvert reads system memory, so its presence means the frames were downloaded after all.
	line := strings.Join((d3d11Capture{}).Describe(s, opts), " ")
	if strings.Contains(line, gstSystemConvert) {
		t.Errorf("the GPU path converts on the device, not with %s: %s", gstSystemConvert, line)
	}
	if !strings.Contains(line, gstGpuMemories[capabilities.FamilyNvenc].convert) {
		t.Errorf("the GPU path must convert with the family's post-processor: %s", line)
	}
}

// The same backend off system memory is the chain it was before the pair had a row: a CPU
// conversion, and caps naming no device memory.
func TestTheGstD3d11SystemPathPinsNoDeviceMemory(t *testing.T) {
	s := gstD3d11Stream()
	s.Publish.CaptureMemory = gpupath.MemorySystem
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

// The row is what the form reads to decide whether the direct path is offered, so every capture
// backend it names has to be one this app runs.
func TestEveryGpuPathNamesARunnableCapture(t *testing.T) {
	for _, p := range gpupath.Paths {
		if _, ok := captureBackends[p.Capture]; !ok {
			t.Errorf("%s/%s/%s names no capture backend this app runs", p.Engine, p.Capture, p.Family)
		}
	}
}

// Plain video/x-raw is system memory, so a capsfilter omitting the feature pins the frames back
// into the round trip.
// Every caps the chain pins downstream of the source carries it, the framerate one imagefreeze
// paces to included.
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
		// The source is pinned to what the compositor exports, not to the encoder's memory.
		// Every other raw caps in the chain is downstream of the conversion.
		if strings.Contains(caps, "memory:DMABuf") {
			continue
		}
		t.Errorf("caps %q pin system memory on the GPU path", caps)
	}
}

// pipewiresrc negotiates the compositor's dmabuf export only when the caps ask for it, and settles
// on the copies PipeWire writes into shared memory when they do not.
// That copy is the round trip the path exists to avoid, so the source is pinned and a compositor
// exporting no dmabuf fails in negotiation rather than delivering it.
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
	// videoconvert reads system memory, so its presence means the frames were mapped back after all.
	if strings.Contains(line, gstSystemConvert) {
		t.Errorf("the GPU path converts on the device, not with %s: %s", gstSystemConvert, line)
	}
	if !strings.Contains(line, gstGpuMemories[capabilities.FamilyVaapi].convert) {
		t.Errorf("the GPU path must convert with the family's post-processor: %s", line)
	}
}

// The system-memory path is what every pair without a row runs: a CPU conversion, and caps naming
// no device memory.
func TestTheGstSystemPathPinsNoDeviceMemory(t *testing.T) {
	s := gstGpuStream()
	s.Publish.CaptureMemory = gpupath.MemorySystem
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

// Auto is what a stored stream carries, so the pair table alone decides which chain it builds.
func TestGstAutoFollowsThePairTable(t *testing.T) {
	s := gstGpuStream()
	s.Publish.CaptureMemory = gpupath.MemoryAuto
	opts, err := gstSourceOptions(s)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Memory != gpupath.MemoryGpu {
		t.Errorf("auto must take the direct path where the pair has one, got %s", opts.Memory)
	}

	// The same capture into an encoder reading system memory has no pair, and auto resolves to the
	// copy rather than refusing.
	s.Publish.Codec, s.Publish.Chroma = "libx264", "yuv420p"
	opts, err = gstSourceOptions(s)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Memory != gpupath.MemorySystem {
		t.Errorf("auto must copy where the pair has no direct path, got %s", opts.Memory)
	}
}

// A demand the pair cannot meet is refused before anything is acquired, so a combination the form
// greys never pops the picker.
func TestGstRefusesTheGpuDemandForAPairWithoutAPath(t *testing.T) {
	s := gstGpuStream()
	s.Publish.Codec, s.Publish.Chroma = "libx264", "yuv420p"
	if _, err := gstSourceOptions(s); err == nil {
		t.Fatal("the portal into a software encoder has no GPU path and must be refused")
	}
}

// gstTestCaps is the encoder input caps these settings publish through, resolved the way the engine
// resolves them.
// A test about the caps alone reads them here rather than building a frame memory of its own, so a
// change in how the memory is resolved reaches it.
func gstTestCaps(s settings.Settings) (string, error) {
	opts, err := gstSourceOptions(s)
	return opts.InCaps, err
}

// gstProbed is the source options a run reporting progress carries.
func gstProbed(opts gstCaptureOptions) gstCaptureOptions {
	opts.RateProbe = gstCaptureProbe
	return opts
}
