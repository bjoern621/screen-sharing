package form

import (
	"fmt"
	"slices"
	"strconv"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/cursor"
	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/receive"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

// The option builders, one per control in fieldTable that offers entries: every select and radio,
// and the one number carrying a ladder beside its range.
//
// Every value comes from a domain table and never from a list typed here:
// publish.Captures for the capture backends, capabilities.Codecs for the encoders,
// capabilities.Modes for the rate controls, transport.PublishNames
// and transport.WatchNames for the legs, gpupath.Memories for the frame memory,
// ffmpeg.DrmMaps for the download strategies and Deps.Monitors for the screens.
// A list typed here is a second definition: the encoder accepts what the form never offered,
// or the form offers what the encoder refuses, and neither disagreement shows until it is a stream
// nobody can explain (docs/ipc-api.md).
//
// optionScaleHeights and optionFpsLadder are the exceptions, neither being a domain.
// What the encoder accepts is the range beside them, and their steps are the answers worth reaching
// in one move, so a step dropped from either loses a shortcut
// where a value dropped from a domain list loses a setting.
//
// A builder answers one question: which entries exist at all.
// It fills the value, and a note where the entry depends on something the value does not say.
// Enabled and Reason are left alone, since which entries the current combination rules
// out is availability's single answer, spent in resolveField,
// so the evaluation that greys an option is the one that repairs a draft onto another.
// An option a neighbouring combination allows is present and greyed;
// one no combination should ever show is absent (docs/field-availability.md).
//
// No builder fills a label or a description.
// What an entry is called and what the paragraph under it says are the surface's,
// keyed by the field and this value (api/proto/screenshare/v1/text.proto).

// optionEntry fills the three fields a builder owns and leaves Enabled and Reason to availability,
// so no builder has to remember which are which.
func optionEntry(value string, note *screensharev1.Text, recommended bool) *screensharev1.FieldOption {
	assert.Assert(value != "" || note == nil,
		"an entry with no value of its own carries no note about one")
	return &screensharev1.FieldOption{Value: value, Note: note, Recommended: recommended}
}

// optionPlainList is the whole of a builder over one closed domain list:
// every value the table declares, in the table's own order, with no note and no recommendation.
//
// key feeds the assertion alone.
// A closed list with nothing in it is a domain table that came back empty,
// which would render as a control offering no choice rather than as the missing table it is.
func optionPlainList(values []string, key string) []*screensharev1.FieldOption {
	assert.Assert(len(values) > 0, "a closed option list has values to offer", key)

	out := make([]*screensharev1.FieldOption, 0, len(values))
	for _, v := range values {
		out = append(out, optionEntry(v, nil, false))
	}
	return out
}

// optionEngineOf is the publish engine the draft's capture backend runs on,
// and the ffmpeg engine for a backend the registry does not carry.
//
// Every per-engine table below is asked with one, and a draft can name a backend that is not here:
// a settings file written on another platform, or one hand-edited.
// Answering rather than refusing keeps the option lists built, and it costs nothing:
// an entry the wrong engine would withhold is greyed by availability with the engine named,
// so the reader sees the capture backend is the thing to change.
func optionEngineOf(s settings.Settings) string {
	engine, err := publish.EngineFor(s.Publish.Capture)
	if err != nil {
		return capabilities.EngineFfmpeg
	}
	assert.Assert(slices.Contains(capabilities.Engines, engine),
		"a capture backend runs a publish engine the capability table names", s.Publish.Capture, engine)
	return engine
}

// optionCaptures offers every capture backend the publish registry carries,
// whatever platform this machine is.
//
// A backend this machine cannot run is kept and greyed with the platform named, not dropped,
// the greying being availability's off publish.Available.
// The list then reads the same on every machine, so a user following a screenshot
// or a support thread meets the entry the other machine offered.
//
// The note is the privilege the backend needs and no probe can establish (publish.Grant).
// The entry stays selectable: the process either holds the privilege or the capture dies at launch,
// and the note is what makes it honest about which of the two it is asking for.
// Which publish engine a backend runs is a column of its own catalog row rather than a note here.
func optionCaptures(_ Deps, _ settings.Settings) []*screensharev1.FieldOption {
	captures := publish.Captures()
	out := make([]*screensharev1.FieldOption, 0, len(captures))
	for _, capture := range captures {
		_, err := publish.EngineFor(capture)
		assert.IsNil(err, "a listed capture backend has a publisher", capture)
		out = append(out, optionEntry(capture, publish.Grant(capture), false))
	}
	return out
}

// optionCaptureMemories offers every value gpupath.Memories declares.
// Auto is marked as the one value every pair satisfies, which is a fact about the table rather
// than a preference: each of the others can be refused by some combination.
func optionCaptureMemories(_ Deps, _ settings.Settings) []*screensharev1.FieldOption {
	out := make([]*screensharev1.FieldOption, 0, len(gpupath.Memories))
	for _, memory := range gpupath.Memories {
		out = append(out, optionEntry(memory, nil, memory == gpupath.MemoryAuto))
	}
	return out
}

// optionCursors offers every pointer mode cursor.Modes declares,
// whatever the selected capture backend serves.
// One this backend cannot do is greyed with the backend's own reason,
// an entry that vanished having taken away the sentence naming what to change
// (docs/field-availability.md, "Where a greyed entry sits").
//
// Embedded is marked: it is what a screen share is expected to look like, and what every backend
// but the scanout one does.
func optionCursors(_ Deps, _ settings.Settings) []*screensharev1.FieldOption {
	out := make([]*screensharev1.FieldOption, 0, len(cursor.Modes))
	for _, mode := range cursor.Modes {
		out = append(out, optionEntry(mode, nil, mode == cursor.Embedded))
	}
	return out
}

// optionDrmMaps offers the download strategies ffmpeg.DrmMaps declares,
// that being the table which refuses a name no row carries.
func optionDrmMaps(_ Deps, _ settings.Settings) []*screensharev1.FieldOption {
	out := make([]*screensharev1.FieldOption, 0, len(ffmpeg.DrmMaps))
	for _, m := range ffmpeg.DrmMaps {
		out = append(out, optionEntry(m.Name, nil, false))
	}
	return out
}

// optionMonitorOf is the enumerated monitor at a capture index,
// and false where enumeration reported none.
// The index is the settings' own, so a stale one answers false rather than reaching past the slice.
func optionMonitorOf(d Deps, index int) (display.Monitor, bool) {
	for _, m := range d.Monitors {
		if m.Index == index {
			return m, true
		}
	}
	return display.Monitor{}, false
}

// optionMonitors offers one entry per enumerated monitor, valued by capture index.
//
// What each index shows, the geometry, the refresh rate and which one the platform calls primary,
// is the monitor's own catalog row and is not repeated here.
// The primary output is marked, emphasis being a verdict about this list rather than a property
// a reader derives from one row of it.
//
// The selected index is present even where enumeration does not carry it.
// A monitor unplugged since the settings were saved is what the user has to see in order to move
// off it, and a dropdown that dropped it would show another monitor
// above a capture that still runs on the missing one.
// That entry carries the note, nothing else on the wire
// describing an index no enumeration reported.
func optionMonitors(d Deps, s settings.Settings) []*screensharev1.FieldOption {
	out := make([]*screensharev1.FieldOption, 0, len(d.Monitors)+1)
	for _, m := range d.Monitors {
		out = append(out, optionEntry(strconv.Itoa(m.Index), nil, m.Primary))
	}
	if !slices.ContainsFunc(out, func(o *screensharev1.FieldOption) bool {
		return o.GetValue() == strconv.Itoa(s.Publish.Monitor)
	}) {
		out = append(out, optionEntry(strconv.Itoa(s.Publish.Monitor),
			say(monitorNotEnumerated, argMonitor(s.Publish.Monitor)), false))
	}
	return out
}

// optionScaleHeights is the ladder of standard heights an output resolution is offered at:
// the four sizes a screen is sold and a stream is described in, 4K down to 720p.
// A step ladder and not a value set: what reaches the settings is computed from the captured
// monitor at its own aspect ratio, and a step above the source's height is dropped rather
// than offered as an upscale.
var optionScaleHeights = []int{2160, 1440, 1080, 720}

// optionOutputResolutions is the sizes the captured picture can reach the encoder at: its own,
// and the standard heights below it at the same aspect ratio.
//
// Derived from the monitor rather than listed, so another screen produces another ladder
// with nothing here edited, and every scaled entry notes what it was derived from.
// A monitor enumeration reported no geometry for yields the unscaled entry alone,
// there being no source size to scale from and a ladder of absolute sizes being a claim
// about a screen this machine cannot measure.
//
// The unscaled entry is the empty value, what the settings hold for a capture that reaches
// the encoder at its own size.
// It is marked rather than noted, what that size is belonging to the monitor's catalog row.
func optionOutputResolutions(d Deps, s settings.Settings) []*screensharev1.FieldOption {
	out := []*screensharev1.FieldOption{optionEntry("", nil, true)}

	m, ok := optionMonitorOf(d, s.Publish.Monitor)
	if !ok || m.Width <= 0 || m.Height <= 0 {
		return out
	}
	for _, height := range optionScaleHeights {
		if height >= m.Height {
			continue
		}
		// Even width: every chroma subsampling encoded here needs one,
		// so an odd one is a scaler failure rather than a picture.
		width := (m.Width*height/m.Height + 1) / 2 * 2
		out = append(out, optionEntry(fmt.Sprintf("%dx%d", width, height),
			say(scaledFromSource, argWidth(m.Width), argHeight(m.Height)), false))
	}
	return out
}

// optionFpsLadder is the ladder the frame rate is offered on: the rates a film, a game
// and the panel refreshes this app has met run at.
//
// A ladder and not a domain, which is the whole of what separates this field from a select.
// The legal values are the fieldFpsBounds range entire; these are the answers worth reaching
// in one move, and a rate off the ladder is typed rather than refused
// (api/proto/screenshare/v1/form.proto, CONTROL_KIND_NUMBER_SELECT).
//
// Nothing here greys against a monitor.
// Capturing above a panel's refresh is legal and yields duplicate frames,
// which diagnostics.go says once as a warning; disabling the entry would say it twice
// and make a legal rate look refused.
var optionFpsLadder = []int{30, 60, 90, 120, 144, 165, 240}

// optionFpsPresets offers the ladder,
// plus the held rate wherever the settings carry one the ladder does not.
//
// The held rate is present for the reason a stale monitor index is (optionMonitors):
// a ladder that dropped it would leave the dropdown claiming a rate the stream is not captured at.
func optionFpsPresets(_ Deps, s settings.Settings) []*screensharev1.FieldOption {
	rates := optionFpsLadder
	if !slices.Contains(rates, s.Publish.Fps) {
		rates = append(append([]int{}, rates...), s.Publish.Fps)
		slices.Sort(rates)
	}

	out := make([]*screensharev1.FieldOption, 0, len(rates))
	for _, fps := range rates {
		out = append(out, optionEntry(strconv.Itoa(fps), nil, false))
	}
	assert.Assert(slices.ContainsFunc(out, func(o *screensharev1.FieldOption) bool {
		return o.GetValue() == strconv.Itoa(s.Publish.Fps)
	}), "the ladder offers the rate the settings hold", s.Publish.Fps)

	return out
}

// optionFormats offers every bitstream some row of the capability table produces,
// and optionEncoders every encoder some row runs on, both in table order.
//
// Both lists are the whole table's rather than the other control's,
// so neither dropdown loses an entry when the one beside it moves.
// A pair no row carries is greyed with what the tables say about it,
// which teaches where a shorter list would leave a user hunting for an entry that vanished
// (docs/field-availability.md).
// A row still on the roadmap contributes its two entries the same way,
// so hardware this app does not build for yet is visible where somebody goes looking for it.
//
// The entries are the identifiers and nothing more.
// What each reads as on screen is the surface's,
// off the catalog row that declares them (docs/ipc-api.md).
func optionFormats(_ Deps, _ settings.Settings) []*screensharev1.FieldOption {
	return optionPlainList(optionCodedFormats(), KeyFormat)
}

func optionEncoders(_ Deps, _ settings.Settings) []*screensharev1.FieldOption {
	return optionPlainList(capabilities.Encoders(), KeyEncoder)
}

// optionCodedFormats is every bitstream the capability table carries, in table order and once each.
//
// capabilities.Formats is the implemented half of the same list and answers a different question:
// what a relay path can be narrowed by is what this app produces,
// where a control offers what it can say something about.
func optionCodedFormats() []string {
	var out []string
	for _, c := range capabilities.Codecs {
		if !slices.Contains(out, c.Format) {
			out = append(out, c.Format)
		}
	}
	return out
}

// optionCodedChromas is every pixel format some codec in the capability table declares,
// which is the whole value space of the chroma setting.
// There is no list of pixel formats in Go: a chroma is a fact about a codec,
// so the union of the rows is the set
// and a row gaining a format puts it on the form with no edit here.
//
// The order is optionChromaOrder's, and a format no order names follows the ones that are named,
// so a codec table gaining a pixel format offers it rather than dropping it.
func optionCodedChromas() []string {
	var coded []string
	for _, c := range capabilities.Codecs {
		for _, chroma := range c.Chromas {
			if !slices.Contains(coded, chroma) {
				coded = append(coded, chroma)
			}
		}
	}

	out := make([]string, 0, len(coded))
	for _, chroma := range optionChromaOrder {
		if slices.Contains(coded, chroma) {
			out = append(out, chroma)
		}
	}
	for _, chroma := range coded {
		if !slices.Contains(optionChromaOrder, chroma) {
			out = append(out, chroma)
		}
	}
	assert.Assert(len(out) == len(coded), "one entry per pixel format some codec codes", len(out), len(coded))
	return out
}

// optionChromaOrder is the order the pixel formats are offered in: most colour detail kept first,
// down to the universally decodable one.
//
// Stated here because the capability rows declare chromas per codec
// and their union has no order to inherit, and because the order is itself the argument:
// a ladder from exact desktop pixels down to the cheapest format makes the trade legible
// where an arbitrary order makes it a list of names.
// It carries no other claim, what each format is and what it costs being the surface's to say.
var optionChromaOrder = []string{"gbrp", "yuv444p", "yuv422p", "yuv420p", "p010le"}

// optionChromas offers every pixel format the capability table codes.
// One the selected codec cannot encode stays and is greyed with the encoder or the element named,
// another codec reaching it.
func optionChromas(_ Deps, _ settings.Settings) []*screensharev1.FieldOption {
	return optionPlainList(optionCodedChromas(), KeyChroma)
}

// optionColorRanges offers the quantization ranges the capability table declares.
// No range is ever absent: a codec that cannot code one carries a gap,
// and a gap is a greying with a reason.
func optionColorRanges(_ Deps, _ settings.Settings) []*screensharev1.FieldOption {
	return optionPlainList(capabilities.ColorRanges, KeyColorRange)
}

// optionModes offers every rate-control mode the capability table's validator accepts,
// in that table's order: the modes that aim at a bitrate, then the one that aims at a quality,
// then the one that codes bit-exact.
func optionModes(_ Deps, _ settings.Settings) []*screensharev1.FieldOption {
	return optionPlainList(capabilities.Modes, KeyMode)
}

// optionEfforts offers the selected codec's own ladder, most effort first.
//
// The codec's and not one family's, because the step is the encoder's own identifier:
// x264 counts in names, SVT-AV1 in numbers to 13, NVENC from p1 to p7.
// A list fixed to one of them would offer every other encoder values it never heard of,
// which the repair would then walk the draft off on every codec change.
//
// The step this mode starts on is marked,
// that being the one the row declares rather than a preference.
// A codec whose row declares no ladder offers what the draft holds,
// so a greyed control still shows its value rather than an empty list.
func optionEfforts(_ Deps, s settings.Settings) []*screensharev1.FieldOption {
	return optionLadder(codecLadders(s).Effort, s.Publish.Effort, s.Publish.Mode)
}

// optionTunes offers the selected codec's tune ladder, read the way the effort one is:
// the steps are the encoder's own, and the step this mode starts on is marked.
//
// A ladder that leads with the untuned step says so as a value rather than by being short,
// so the control holds "tune for nothing" as a choice.
// The encoders with no such step are the ones always tuning for something.
func optionTunes(_ Deps, s settings.Settings) []*screensharev1.FieldOption {
	return optionLadder(codecLadders(s).Tune, s.Publish.Tune, s.Publish.Mode)
}

// codecLadders is the selected codec's row, and an empty row for a codec no table carries.
func codecLadders(s settings.Settings) capabilities.Codec {
	c, _ := capabilities.Get(s.Publish.Codec())
	return c
}

// optionLadder builds one ladder's entries: its steps, the step this mode starts on marked,
// and the held value kept where the ladder has no step for it.
//
// The held value stays for the reason a stale monitor index does (optionMonitors):
// a list that dropped it would leave the dropdown claiming a step the draft is not on.
func optionLadder(l capabilities.Ladder, held, mode string) []*screensharev1.FieldOption {
	start, _ := l.StepFor(mode)

	out := make([]*screensharev1.FieldOption, 0, len(l.Steps)+1)
	for _, step := range l.Steps {
		out = append(out, optionEntry(step, nil, step == start))
	}
	if held != "" && !l.Has(held) {
		out = append(out, optionEntry(held, nil, false))
	}
	return out
}

// optionAudioSources offers where the second track comes from:
// every source the platform table declares, each noting what serves it here.
//
// Every declared source is offered and not only the served ones,
// which is the division every builder here makes: which entries exist at all is the builder's
// answer, and which of them this machine rules out is availability's single one.
// A source no session here serves is present and greyed with what the machine is missing,
// the second track being a general concept rather than one platform's implementation knob
// (docs/field-availability.md, "The rule").
func optionAudioSources(d Deps, _ settings.Settings) []*screensharev1.FieldOption {
	sources := platform.AudioSources(d.Platform)
	assert.Assert(len(sources) > 0, "a platform offers somewhere for the second track to come from")

	out := make([]*screensharev1.FieldOption, 0, len(sources))
	for _, s := range sources {
		out = append(out, optionEntry(s.ID, s.Server, false))
	}
	return out
}

// optionAudioCodecs offers every row of the audio capability table, in table order,
// each noting the rate and bitrate its track is coded at.
//
// Those figures are the table's and not the user's, stated once
// so both engines build the same branch,
// and the note is what stops the form from implying a choice about them.
func optionAudioCodecs(_ Deps, _ settings.Settings) []*screensharev1.FieldOption {
	out := make([]*screensharev1.FieldOption, 0, len(capabilities.AudioCodecs))
	for _, a := range capabilities.AudioCodecs {
		out = append(out, optionEntry(a.Name,
			say(audioTrackCodedAt, argRateHz(a.Rate), argBitrateKbps(a.BitrateK)), false))
	}
	return out
}

// optionPublishTransports offers every transport some publish engine can serialize a stream
// through, which is the union of both engines' rosters rather than the running engine's alone.
//
// A transport this capture backend's engine has no sink for is one the neighbouring backend
// does have, WebRTC on the GStreamer engine being the case that matters, so the entry stays
// and is greyed with the engine named, which tells the user the capture backend is what to change.
// A protocol no engine ingests is in neither roster and so is absent entirely: HLS is watch-only,
// and greying it would state a reason no choice on this screen could lift.
func optionPublishTransports(_ Deps, _ settings.Settings) []*screensharev1.FieldOption {
	var names []string
	for _, engine := range capabilities.Engines {
		for _, name := range transport.PublishNames(engine) {
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	slices.Sort(names)
	return optionPlainList(names, KeyTransport)
}

// optionTileWatchTransports offers the legs a receive pipeline decodes from,
// which is the GStreamer roster.
// It reaches WHEP, which no player URL expresses.
//
// RTSP is marked, a hint about the protocols rather than about this machine.
// It is the one watch leg carrying every format this app publishes (transport.rtspCarriage),
// so a stream in any of them comes back on it,
// and the way through the relay measured on it is an order of magnitude shorter
// than SRT's or HLS's (docs/delay-measurement.md).
// What a draft holds is Field.value, and the mark says which leg to move to.
func optionTileWatchTransports(_ Deps, _ settings.Settings) []*screensharev1.FieldOption {
	names := transport.WatchNames(capabilities.EngineGst)
	assert.Assert(len(names) > 0, "a closed option list has values to offer", KeyTileWatchTransport)

	out := make([]*screensharev1.FieldOption, 0, len(names))
	for _, name := range names {
		out = append(out, optionEntry(name, nil, name == transport.RTSP{}.Name()))
	}
	return out
}

// optionRenderChains offers the chains a receive pipeline converts decoded frames with,
// in the receive package's own order: the default first, then the device chains,
// then the unconverted one.
//
// A chain this machine cannot run keeps its entry and is greyed,
// the entry plus its reason naming the element
// that is missing where a shorter list would say nothing.
// Which of them that is is availability's answer (availabilityOptionRules),
// for the reason every builder here leaves its verdict alone.
//
// The default is marked recommended, a hint about this machine rather than a value:
// what a draft holds is Field.value, and the mark says which chain a stream renders
// through when nothing chose one.
func optionRenderChains(_ Deps, _ settings.Settings) []*screensharev1.FieldOption {
	chains := receive.Chains()
	out := make([]*screensharev1.FieldOption, 0, len(chains))
	for _, c := range chains {
		out = append(out, optionEntry(c.Name, nil, c.Default))
	}
	return out
}

// optionRtspProtocols offers the RTP lower transports RTSP runs over, for either leg.
// One list for both, which transports carry RTP being a fact of the protocol rather
// than of a direction, so the transport package states it once too.
func optionRtspProtocols(_ Deps, _ settings.Settings) []*screensharev1.FieldOption {
	return optionPlainList(transport.RtspProtocols, KeyRtspPublishProtocol)
}

// optionMaxratePresets is the ladder the burst ceiling is offered on: the uncapped answer,
// the target itself, and twice the target.
//
// This ladder carries a value the range cannot: a burst ceiling is legal at zero,
// meaning bounded by nothing, and legal again from the target upward,
// with the band between the two walked up to the target on the next resolve (repairCeilings).
// A range runs from one end to the other, so the range carries the band
// and the entry carries the answer outside it (api/proto/screenshare/v1/form.proto,
// CONTROL_KIND_NUMBER_SELECT).
//
// The target and twice it are where the two ends of constrained variable bitrate sit:
// at the target the rate is held, and a ceiling above twice it is headroom the VAAPI note already
// warns about (availability.go, vaapiCeilingNote).
func optionMaxratePresets(d Deps, s settings.Settings) []*screensharev1.FieldOption {
	ceiling := int(fieldMaxrateBounds(d, s).GetMax())

	var rates []int
	// Zero is refused only where the encoder's constant-quality mode codes toward a rate,
	// which is the one case fieldMaxrateBounds lifts its floor for.
	if fieldMaxrateBounds(d, s).GetMin() != 1 || s.Publish.Mode != capabilities.ModeCrf {
		rates = append(rates, 0)
	}
	if s.Publish.Mode != capabilities.ModeCrf && s.Publish.BitrateM > 0 {
		rates = append(rates, min(s.Publish.BitrateM, ceiling), min(s.Publish.BitrateM*2, ceiling))
	}
	// The held ceiling stays on the ladder for the reason the held frame rate does:
	// a ladder that dropped it would leave the control
	// claiming a ceiling the stream is not bounded by.
	rates = append(rates, s.Publish.MaxrateM)

	slices.Sort(rates)
	rates = slices.Compact(rates)

	out := make([]*screensharev1.FieldOption, 0, len(rates))
	for _, rate := range rates {
		if rate < 0 {
			continue
		}
		out = append(out, optionEntry(strconv.Itoa(rate), nil, false))
	}
	assert.Assert(len(out) > 0, "the burst ceiling offers a ladder to reach", s.Publish.Mode)
	return out
}
