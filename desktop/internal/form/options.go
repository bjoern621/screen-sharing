package form

import (
	"fmt"
	"slices"
	"strconv"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

// The option builders, one per select or radio in fieldTable.
//
// Every one of them takes its values from a domain table and never from a list
// written here: publish.Captures for the capture backends, capabilities.Codecs for the
// encoders, capabilities.Modes for the rate controls, transport.PublishNames and
// transport.WatchNames for the legs, gpupath.Memories for the frame memory,
// ffmpeg.DrmMaps for the download strategies and Deps.Monitors for the screens. A list
// typed here would be the second definition this whole package exists to remove: the
// encoder would accept what the form never offered, or the form would offer what the
// encoder refuses, and neither disagreement is visible until it is a stream nobody can
// explain (docs/ipc-api.md).
//
// A builder answers one question: which entries exist at all. It fills the value, and
// a note where the entry depends on something the value does not say. It does not fill
// Enabled and Reason - which entries the current combination rules out is availability's
// single answer, spent in resolveField, so the same evaluation that greys an option is
// the one that repairs a draft onto another. Two places deciding it is exactly how a
// greyed option and its replacement come to disagree. An option a neighbouring
// combination allows is present and greyed; an option no combination should ever show is
// absent (docs/field-availability.md).
//
// What a builder never fills is a label or a description. What an entry is called and
// what the paragraph under it says are the surface's, keyed by the field and this value
// (api/proto/screenshare/v1/text.proto). This file used to carry a face table per axis -
// a label and a paragraph for every capture backend, codec, pixel format, mode,
// transport and preset step - and that was three hundred lines of screen copy sitting in
// the package that decides what is legal. The two answers are now in the two places that
// own them: which values exist is here, and what they read as is where the layout is.

// optionEntry builds one entry. It exists so every builder fills the same three fields
// and leaves the same two alone, rather than each one remembering to.
func optionEntry(value string, note *screensharev1.Text, recommended bool) *screensharev1.FieldOption {
	assert.Assert(value != "" || note == nil,
		"an entry with no value of its own carries no note about one")
	return &screensharev1.FieldOption{Value: value, Note: note, Recommended: recommended}
}

// optionPlainList is the whole of a builder whose values are one closed domain list and
// whose entries carry no note and no recommendation: every value the table declares, in
// the table's own order.
//
// key names the field, and it is here for the assertion alone. A closed list with
// nothing in it is a domain table that came back empty, which would render as a control
// offering no choice at all rather than as the missing table it is.
func optionPlainList(values []string, key string) []*screensharev1.FieldOption {
	assert.Assert(len(values) > 0, "a closed option list has values to offer", key)

	out := make([]*screensharev1.FieldOption, 0, len(values))
	for _, v := range values {
		out = append(out, optionEntry(v, nil, false))
	}
	return out
}

// optionEngineOf is the publish engine the draft's capture backend runs on, and the
// ffmpeg engine for a backend the registry does not carry.
//
// Every per-engine table below has to be asked with one, and a draft can name a capture
// backend that does not exist here: a settings file written on another platform, or one
// hand-edited. Answering with an engine rather than refusing is what keeps the option
// lists built in that case, and it costs nothing, because an entry the wrong engine
// would withhold is greyed by availability with the engine named - which is the
// statement that tells the user the capture backend is the thing to change.
func optionEngineOf(s settings.Stream) string {
	engine, err := publish.EngineFor(s.Capture)
	if err != nil {
		return capabilities.EngineFfmpeg
	}
	assert.Assert(slices.Contains(capabilities.Engines, engine),
		"a capture backend runs a publish engine the capability table names", s.Capture, engine)
	return engine
}

// optionCaptures offers every capture backend the publish registry carries, whatever
// platform this machine is.
//
// A backend this machine cannot run is kept and greyed with the platform named rather
// than dropped, which is the rule for an option a neighbouring combination allows: the
// list is then the same on every machine, so a user following a screenshot or a support
// thread meets the entry the other machine offered instead of wondering where it went.
// The greying is availability's, off publish.Available; the list is this builder's.
//
// The note is the privilege the backend needs and no probe can establish (publish.Grant),
// carried by the two rows behind one and absent from the rest. The entry stays
// selectable: the process either holds the privilege or the capture dies at launch, and
// the note is what makes it honest about which of the two it is asking for. Which publish
// engine a backend runs is not a note here - it is a column of the backend's own catalog
// row, and stating it twice would be two answers to one question.
func optionCaptures(_ Deps, _ settings.Stream) []*screensharev1.FieldOption {
	captures := publish.Captures()
	out := make([]*screensharev1.FieldOption, 0, len(captures))
	for _, capture := range captures {
		_, err := publish.EngineFor(capture)
		assert.IsNil(err, "a listed capture backend has a publisher", capture)
		out = append(out, optionEntry(capture, publish.Grant(capture), false))
	}
	return out
}

// optionCaptureMemories offers the four memory settings. Auto is marked because it is
// the one value every pair satisfies, which is a fact about the table rather than a
// preference: the other three can each be refused by some combination.
func optionCaptureMemories(_ Deps, _ settings.Stream) []*screensharev1.FieldOption {
	out := make([]*screensharev1.FieldOption, 0, len(gpupath.Memories))
	for _, memory := range gpupath.Memories {
		out = append(out, optionEntry(memory, nil, memory == gpupath.MemoryAuto))
	}
	return out
}

// optionDrmMaps offers the download strategies the ffmpeg builder's own table declares,
// which is the table that refuses a name no row carries.
func optionDrmMaps(_ Deps, _ settings.Stream) []*screensharev1.FieldOption {
	out := make([]*screensharev1.FieldOption, 0, len(ffmpeg.DrmMaps))
	for _, m := range ffmpeg.DrmMaps {
		out = append(out, optionEntry(m.Name, nil, false))
	}
	return out
}

// optionMonitorOf is the enumerated monitor at an index, and false where enumeration
// reported none. The index is the settings' own, so a stale one answers false rather
// than reaching past the slice.
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
// What each index shows - the geometry, the refresh rate, which one the platform calls
// primary - is the monitor's own catalog row and is not repeated here. The primary
// output is marked, because which entry a surface may emphasise is a verdict about this
// list rather than a property a reader derives from one row of it.
//
// The selected index is always present, even where enumeration does not carry it. A
// stale selection - a monitor unplugged since the settings were saved - is what the user
// has to see in order to move off it, and a dropdown that silently dropped it would show
// some other monitor above a capture that still runs on the missing one. That entry
// carries the note saying so, since nothing else on the wire describes an index no
// enumeration reported.
func optionMonitors(d Deps, s settings.Stream) []*screensharev1.FieldOption {
	out := make([]*screensharev1.FieldOption, 0, len(d.Monitors)+1)
	for _, m := range d.Monitors {
		out = append(out, optionEntry(strconv.Itoa(m.Index), nil, m.Primary))
	}
	if !slices.ContainsFunc(out, func(o *screensharev1.FieldOption) bool {
		return o.GetValue() == strconv.Itoa(s.Monitor)
	}) {
		out = append(out, optionEntry(strconv.Itoa(s.Monitor),
			say(monitorNotEnumerated, argMonitor(s.Monitor)), false))
	}
	return out
}

// optionScaleHeights is the ladder of standard heights an output resolution is offered
// at. It is a step ladder and not a value set: what reaches the settings is computed from
// the captured monitor at its own aspect ratio, so a step above the source's height is
// dropped rather than offered as an upscale.
var optionScaleHeights = []int{2160, 1440, 1080, 900, 720, 540}

// optionOutputResolutions is the sizes the captured picture can be fed to the encoder at:
// its own, and the standard heights below it at the same aspect ratio.
//
// Derived from the monitor rather than listed, so selecting another screen produces
// another ladder with nothing here edited, and every scaled entry carries what it was
// derived from. A monitor enumeration reported no geometry for yields the unscaled entry
// alone: there is no source size to scale from, and a ladder of absolute sizes would be a
// claim about a screen this machine cannot measure.
//
// The unscaled entry is the empty value, which is what the settings hold for "the capture
// reaches the encoder at its own size". It is marked rather than noted: what that size is
// belongs to the monitor's catalog row, and the entry's own job is to say that nothing
// resizes.
func optionOutputResolutions(d Deps, s settings.Stream) []*screensharev1.FieldOption {
	out := []*screensharev1.FieldOption{optionEntry("", nil, true)}

	m, ok := optionMonitorOf(d, s.Monitor)
	if !ok || m.Width <= 0 || m.Height <= 0 {
		return out
	}
	for _, height := range optionScaleHeights {
		if height >= m.Height {
			continue
		}
		// Even widths only: every chroma subsampling this app encodes in needs one, so an
		// odd width is a scaler failure rather than a picture.
		width := (m.Width*height/m.Height + 1) / 2 * 2
		out = append(out, optionEntry(fmt.Sprintf("%dx%d", width, height),
			say(scaledFromSource, argWidth(m.Width), argHeight(m.Height)), false))
	}
	return out
}

// optionCodecs offers every row of the capability table, in table order, which is
// implemented backends first and then the families still on the roadmap.
//
// An unimplemented row is offered and greyed rather than dropped, so the roadmap is
// visible where a user goes looking for their hardware, and so is a codec this machine's
// probe could not run: both are things the user can act on, by waiting or by installing
// a build, and neither is helped by an entry that was never there.
//
// The entry is the encoder's own name and nothing more. What it produces and what
// produces it - the bitstream format and the encoder family - are the two columns of its
// catalog row, which is where a surface reads them to name the entry in whatever shape
// its dropdown takes.
func optionCodecs(_ Deps, _ settings.Stream) []*screensharev1.FieldOption {
	out := make([]*screensharev1.FieldOption, 0, len(capabilities.Codecs))
	for _, c := range capabilities.Codecs {
		out = append(out, optionEntry(c.Name, nil, false))
	}
	return out
}

// optionCodedChromas is every pixel format some codec in the capability table declares,
// which is the whole value space of the chroma setting. There is no list of pixel formats
// in Go: a chroma is a fact about a codec, so the union of the rows is the set, and a row
// gaining a format puts it on the form with no edit here.
//
// The order is the ladder optionChromaOrder declares, which is what makes the trade
// legible in a dropdown; a format no order names follows the ones that are named, so a
// codec table gaining a pixel format offers it rather than dropping it.
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

// optionChromaOrder is the order the pixel formats are offered in: most colour detail
// kept first, down to the universally decodable one.
//
// It is stated here because the capability rows declare chromas per codec and their
// union has no order to inherit, and because the order is itself an argument - a ladder
// from exact desktop pixels down to the cheapest format is what makes the trade legible,
// where an arbitrary order would make it a list of names. It carries no other claim: what
// each format is and what it costs are the surface's to say.
var optionChromaOrder = []string{"gbrp", "yuv444p", "yuv422p", "yuv420p", "p010le"}

// optionChromas offers every pixel format the capability table codes. One the selected
// codec cannot encode stays and is greyed by availability with the encoder or the element
// named, since another codec reaches it.
func optionChromas(_ Deps, _ settings.Stream) []*screensharev1.FieldOption {
	return optionPlainList(optionCodedChromas(), KeyChroma)
}

// optionColorRanges offers the quantization ranges the capability table declares.
// Neither is ever absent: a codec that cannot code one carries a gap, and a gap is a
// greying with a reason.
func optionColorRanges(_ Deps, _ settings.Stream) []*screensharev1.FieldOption {
	return optionPlainList(capabilities.ColorRanges, KeyColorRange)
}

// optionModes offers every rate-control mode the capability table's validator accepts,
// in that table's order: the three that aim at a bitrate, then the one that aims at a
// quality, then the one that codes bit-exact.
func optionModes(_ Deps, _ settings.Stream) []*screensharev1.FieldOption {
	return optionPlainList(capabilities.Modes, KeyMode)
}

// optionEncPresets offers the ladder steps the ffmpeg engine accepts, read off the
// ladder it validates against rather than restated here.
func optionEncPresets(_ Deps, _ settings.Stream) []*screensharev1.FieldOption {
	return optionPlainList(ffmpeg.NvencPresets, KeyEncPreset)
}

// optionAudioSources offers where the second track comes from: every source the platform
// table declares, each noting what serves it here.
//
// Every declared source is offered and not only the served ones, which is the same
// division optionPlainList makes everywhere else: which entries exist at all is this
// builder's answer, and which of them this machine rules out is availability's single
// one. A source no session here serves is therefore present and greyed with what the
// machine is missing, because the second track is a general concept rather than one
// platform's implementation knob (docs/field-availability.md, "The rule").
func optionAudioSources(d Deps, _ settings.Stream) []*screensharev1.FieldOption {
	sources := platform.AudioSources(d.Platform)
	assert.Assert(len(sources) > 0, "a platform offers somewhere for the second track to come from")

	out := make([]*screensharev1.FieldOption, 0, len(sources))
	for _, s := range sources {
		out = append(out, optionEntry(s.ID, s.Server, false))
	}
	return out
}

// optionAudioCodecs offers every row of the audio capability table, in table order, each
// noting the rate and bitrate its track is coded at.
//
// The two figures are the table's and not the user's: they are stated once so both
// engines build the same branch, and the note is what stops the form from implying a
// choice about them where there is none.
func optionAudioCodecs(_ Deps, _ settings.Stream) []*screensharev1.FieldOption {
	out := make([]*screensharev1.FieldOption, 0, len(capabilities.AudioCodecs))
	for _, a := range capabilities.AudioCodecs {
		out = append(out, optionEntry(a.Name,
			say(audioTrackCodedAt, argRateHz(a.Rate), argBitrateKbps(a.BitrateK)), false))
	}
	return out
}

// optionPublishTransports offers every transport some publish engine can serialize a
// stream through, which is the union of the two engines' rosters rather than the running
// engine's alone.
//
// The union is the point. A transport this capture backend's engine has no sink for is
// one the neighbouring backend does have - WebRTC on the GStreamer engine is the case
// that matters - so the entry stays and is greyed with the engine named, which tells the
// user the capture backend is what to change. A protocol no engine ingests at all is in
// neither roster and so is absent entirely: HLS is watch-only, and greying it would state
// a reason no choice on this screen could ever lift.
func optionPublishTransports(_ Deps, _ settings.Stream) []*screensharev1.FieldOption {
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

// optionWatchTransports offers the legs an external viewer can be pointed at: the
// transports a player opens by URL, which is the ffmpeg roster. WHEP is absent because it
// is an exchange rather than an address, so no URL expresses it and no greying could.
func optionWatchTransports(_ Deps, _ settings.Stream) []*screensharev1.FieldOption {
	return optionPlainList(transport.WatchNames(capabilities.EngineFfmpeg), KeyWatchTransport)
}

// optionRtspProtocols offers the RTP lower transports RTSP runs over, for either leg.
// One list for both, because which transports carry RTP is a fact of the protocol and
// not of a direction, which is why the transport package states it once too.
func optionRtspProtocols(_ Deps, _ settings.Stream) []*screensharev1.FieldOption {
	return optionPlainList(transport.RtspProtocols, KeyRtspPublishProtocol)
}
