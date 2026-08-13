package form

// Which controls the current settings leave usable, and what greys the ones they do not
// (docs/field-availability.md).
//
// Every verdict reads a table - capabilities, transport, gpupath, publish - and restates nothing
// those tables carry.
// What this file adds is the statement: a code and the identifiers it is about, never a sentence
// (statements.go), so the wording stays the surface's and what is true stays here.
//
// Two contracts govern every rule below.
// A disabled control says why, which form.go asserts on each field it renders.
// Where two facts block one field, the reason names the one the user can act on: B-frames under
// software x264 name the families that take a count, not the mode.
//
// An unresolved fact withholds nothing.
// The tables are compiled in, so the only facts that can be missing are the machine's - a capture
// backend with no publisher names no engine, an unprobed machine states no verdict - and both leave
// the control live.

import (
	"slices"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/receive"
	"bjoernblessin.de/screenshare/internal/rules"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
	"bjoernblessin.de/screenshare/internal/wire"
)

// fieldState decides one control's visibility and availability.
//
// An unknown key is a caller inventing a control: a plain enabled state for it would put a widget on
// screen that no rule here governs and no repair moves.
// The key is the row's own, which for a repeated control is the template rather than one entry's,
// since the statement is about the control.
func fieldState(d Deps, s settings.Settings, key string, entry int) state {
	return fieldStateOf(availabilityOf(d, s), key, entry)
}

// fieldStateOf answers the same question against an evaluation the caller already holds.
//
// A resolve asks it of every control and of every option of every control, and availabilityOf walks
// the whole rule registry, so the evaluation is made once where the draft it describes is fixed and
// handed down (form.go, repair.go).
func fieldStateOf(av availability, key string, entry int) state {
	rule, ok := availabilityRules[key]
	assert.Assert(ok, "an availability question names a field the form declares", key)

	st := rule(av.forEntry(entry))

	assert.Assert(st.enabled || st.reason != nil, "a disabled control says why", key)
	assert.Assert(st.enabled || st.note == nil, "a note rides on a control that is still editable", key)
	return st
}

// optionState decides one entry of a select or radio, within the field named by key.
//
// A field with no option-level rule leaves every entry enabled: whole-control greying is
// fieldState's, and repeating it per entry would grey a dropdown twice over.
func optionState(d Deps, s settings.Settings, key, value string, entry int) (enabled bool, reason *screensharev1.Text) {
	return optionStateOf(availabilityOf(d, s), key, value, entry)
}

// optionStateOf answers the same question against an evaluation the caller already holds, for the
// reason fieldStateOf takes one.
func optionStateOf(av availability, key, value string, entry int) (enabled bool, reason *screensharev1.Text) {
	_, declared := availabilityRules[key]
	assert.Assert(declared, "an option question names a field the form declares", key)

	rule, ok := availabilityOptionRules[key]
	if !ok {
		return true, nil
	}
	reason = rule(av.forEntry(entry), value)
	return reason == nil, reason
}

// availabilityRules is one row per field the form declares, each deciding that control's state from
// the draft and the machine.
//
// A table rather than a switch (docs/development-principles.md): a control added to keys.go and left
// out here fails the lookup above instead of rendering as a plain enabled widget, and a row that
// decides nothing says so rather than falling through a default nobody reads.
var availabilityRules = map[string]func(availability) state{
	// The connection group.
	// A listener port is a knob of one protocol and hides with it, the hidden treatment's own case: a
	// user on SRT has no reason to read what the RTMP listener's port means.
	KeyName:          func(availability) state { return availabilityLive() },
	KeyRelayHost:     func(availability) state { return availabilityLive() },
	KeyRelayTls:      func(availability) state { return availabilityLive() },
	KeyGroupKey:      func(availability) state { return availabilityLive() },
	KeySrtPassphrase: func(availability) state { return availabilityLive() },
	KeyAPIPort:       func(availability) state { return availabilityLive() },
	KeySrtPort:       func(av availability) state { return availabilityShownFor(av.s.Publish.Transport == availabilitySrt) },
	KeyRtspPort:      func(av availability) state { return availabilityShownFor(av.s.Publish.Transport == availabilityRtsp) },
	KeyWebrtcPort: func(av availability) state {
		return availabilityShownFor(av.s.Publish.Transport == availabilityWebrtc)
	},
	KeyRtmpPort: func(av availability) state { return availabilityShownFor(av.s.Publish.Transport == availabilityRtmp) },
	// HLS follows the watch leg: the relay serves it and ingests nothing over it, so the publish
	// dropdown never names it.
	KeyHlsPort: func(av availability) state {
		return availabilityShownFor(av.s.Viewer.PlayerWatchTransport == availabilityHls)
	},

	// Greyed per entry rather than per control: a greyed entry and its reason name the thing to change.
	KeyTransport: func(availability) state { return availabilityLive() },
	KeyCodec:     func(availability) state { return availabilityLive() },
	KeyMode:      func(availability) state { return availabilityLive() },
	KeyCapture:   func(availability) state { return availabilityLive() },

	KeyFps: func(availability) state { return availabilityLive() },
	// The pointer greys per entry and never as a control: every backend serves at least one mode.
	KeyCursor: func(availability) state { return availabilityLive() },

	// The pixel format carries a note about the viewer's machine (decodeNote) and greys for one fact
	// only: on the device path that converts nothing the encoder picks the layout, so the control
	// would otherwise show a value the stream does not carry.
	KeyChroma: func(av availability) state {
		if reason := av.encoderColourReason(); reason != nil {
			return availabilityDisabled(reason)
		}
		return availabilityNoted(av.decodeNote())
	},
	// Two facts block the colour range, and the encoder-colour path outranks the format's own: under
	// it neither colour field reaches the stream, so naming RGB would send the user changing a control
	// that changes nothing.
	KeyColorRange: func(av availability) state {
		if reason := av.encoderColourReason(); reason != nil {
			return availabilityDisabled(reason)
		}
		if slices.Contains(availabilityFullRangeChromas, av.s.Publish.Chroma) {
			return availabilityDisabled(say(rgbIsFullRange))
		}
		return availabilityLive()
	},

	// The rate-control knobs.
	// Three facts decide each, weighed by the knob helper in the order docs/field-availability.md
	// states.
	KeyCq: func(av availability) state {
		return av.knob(KeyCq, av.mode().usesCq, say(cqOnlyInConstantQuality))
	},
	KeyBitrateM: func(av availability) state {
		return av.knob(KeyBitrateM, av.mode().usesBitrate, say(bitrateNotInMode, argMode(av.s.Publish.Mode)))
	},
	KeyMaxrateM: func(av availability) state {
		st := av.knob(KeyMaxrateM, av.mode().usesMaxrate, say(maxrateOnlyInConstrained))
		if st.enabled {
			st.note = av.vaapiCeilingNote()
		}
		return st
	},
	KeyVbvMs: func(av availability) state {
		return av.knob(KeyVbvMs, av.mode().usesVbv, say(vbvOnlyInBoundedModes))
	},
	// The keyframe interval is no rate-control concept, so no mode withholds it.
	// Only an encoder element with no property for it does.
	KeyGop: func(av availability) state { return av.knob(KeyGop, true, nil) },
	// Two independent facts block B-frames, so the reason names the one that applies rather than
	// always blaming the mode.
	// Which families take a count is the family table's, so the statement lists them from it.
	KeyBframes: func(av availability) state {
		usesBframes := av.mode().usesBframes
		reason := say(bframesOffInMode, argMode(av.s.Publish.Mode))
		if usesBframes {
			reason = say(bframesOnlyOnFamilies,
				argFamilies(availabilityFamiliesWith(func(f availabilityFamily) bool { return f.takesBframes })))
		}
		return av.knob(KeyBframes, usesBframes && av.family().takesBframes, reason)
	},
	// The effort step follows the codec's own row rather than its family's, because the ladder does:
	// the steps are the encoder's identifiers, so two codecs of one family can offer different ones.
	// The entries this control lists and the step a build spends come off that same row, which keeps
	// the greying and the encode in step.
	KeyEffort: func(av availability) state {
		ladder := av.codec.Effort
		pinned := ladder.PinsIn(av.s.Publish.Mode)
		reason := say(codecTakesNoEffortLadder, argCodec(av.s.Publish.Codec))
		// A mode that pins the step carries the one it pins to, so the statement names the declared value
		// rather than a restated one.
		if pinned {
			step, _ := ladder.StepFor(av.s.Publish.Mode)
			reason = say(effortPinnedByMode, argMode(av.s.Publish.Mode), argEffort(step))
		}
		return av.knob(KeyEffort, len(ladder.Steps) > 0 && !pinned, reason)
	},
	// The tune reads the same row, being the other half of one decision: how hard the encoder works,
	// and what it works towards.
	// Either ladder can be declared without the other - the Vulkan rows tune and take no effort step,
	// the libvpx ones take a step and tune for nothing - so each control asks about its own.
	KeyTune: func(av availability) state {
		ladder := av.codec.Tune
		pinned := ladder.PinsIn(av.s.Publish.Mode)
		reason := say(codecTakesNoTuneLadder, argCodec(av.s.Publish.Codec))
		if pinned {
			step, _ := ladder.StepFor(av.s.Publish.Mode)
			reason = say(tunePinnedByMode, argMode(av.s.Publish.Mode), argTune(step))
		}
		return av.knob(KeyTune, len(ladder.Steps) > 0 && !pinned, reason)
	},

	// The audio codec is read only where the stream has a track to code, so with no source the control
	// is inert rather than wrong.
	// Greyed rather than hidden: the codec is a general concept, so the statement says why it does not
	// apply here.
	KeyAudioCodec: func(av availability) state {
		if len(av.s.Publish.Recorded()) == 0 {
			return availabilityDisabled(say(audioCodecNeedsSource))
		}
		return availabilityLive()
	},

	// One entry of the source list.
	// The kind is always editable, because it is what an entry is: setting it to none is how an entry
	// is taken off, and greying it would leave a source nobody can remove.
	KeyAudioSource: func(availability) state { return availabilityLive() },

	// What is inside the kind, which only some kinds have more than one of.
	// A kind with one device leaves nothing to choose, so the control greys naming the kind rather
	// than offering a list of one.
	KeyAudioSourceDevice: func(av availability) state {
		if !av.source.Records() {
			return availabilityDisabled(say(audioDeviceNeedsSource))
		}
		if len(audioDevicesOf(av.deps, av.source.Source)) < 2 {
			return availabilityDisabled(say(audioSourceHasOneDevice, argAudio(av.source.Source)))
		}
		return availabilityLive()
	},

	// The level and the silence mean nothing until the entry names a source.
	// Both reach a pipeline that is already running, so nothing else greys them.
	KeyAudioSourceGain: func(av availability) state {
		if !av.source.Records() {
			return availabilityDisabled(say(audioDeviceNeedsSource))
		}
		return availabilityLive()
	},
	KeyAudioSourceMute: func(av availability) state {
		if !av.source.Records() {
			return availabilityDisabled(say(audioDeviceNeedsSource))
		}
		return availabilityLive()
	},

	// The DRM download strategy is the hidden treatment's own example: a knob of the kmsgrab scanout
	// path and of nothing else, whose help would teach a user on any other backend nothing.
	// Under kmsgrab it greys rather than hiding a second time, which would make it appear and vanish
	// while the user changes codecs.
	KeyDrmMap: func(av availability) state {
		if av.s.Publish.Capture != availabilityKmsgrab {
			return availabilityHidden()
		}
		if av.staysOnDevice() {
			return availabilityDisabled(say(drmMapUnusedOnDevice))
		}
		return availabilityLive()
	},
	// ddagrab selects an output by index and the X backends crop the X screen to the monitor's
	// geometry.
	// A backend that takes no index names itself, so the surface can state what that one captures
	// instead.
	KeyMonitor: func(av availability) state {
		if slices.Contains(availabilityMonitorless, av.s.Publish.Capture) {
			return availabilityDisabled(say(captureTakesNoMonitor, argCapture(av.s.Publish.Capture)))
		}
		return availabilityLive()
	},
	// The frame memory never greys as a whole: auto and the system copy are values every pair
	// satisfies, so no combination leaves a dead control.
	KeyCaptureMemory: func(av availability) state { return availabilityNoted(av.frameMemoryNote()) },

	// The per-protocol knobs, hidden with their protocol like the listener ports: each names a
	// mechanism of one leg of one transport.
	KeySrtPublishLatencyMs: func(av availability) state {
		return availabilityShownFor(av.s.Publish.Transport == availabilitySrt)
	},
	KeyRtspPublishProtocol: func(av availability) state {
		return availabilityShownFor(av.s.Publish.Transport == availabilityRtsp)
	},
	// The SRT window and the RTP lower transport belong to the link rather than to one reader, so they
	// follow either receiver (watchesOver).
	KeySrtWatchLatencyMs: func(av availability) state {
		return availabilityShownFor(av.watchesOver(availabilitySrt))
	},
	KeyRtspWatchProtocol: func(av availability) state {
		return availabilityShownFor(av.watchesOver(availabilityRtsp))
	},
	// The jitter buffer is the tile receiver's alone: a player buffers by reorder queue rather than by
	// time.
	KeyRtspWatchLatencyMs: func(av availability) state {
		return availabilityShownFor(av.s.Viewer.TileWatchTransport == availabilityRtsp)
	},

	KeyUplinkMbps:           func(availability) state { return availabilityLive() },
	KeyPlayerWatchTransport: func(availability) state { return availabilityLive() },
	KeyTileWatchTransport:   func(availability) state { return availabilityLive() },

	// The render chain is live whatever leg is chosen: what a decode converts its frames with is a
	// property of this machine's GStreamer rather than of the protocol they arrived over.
	KeyRenderChain: func(availability) state { return availabilityLive() },

	// What refuses an output resolution is one entry rather than the control, so a frame path that
	// resizes nothing leaves the source size standing (outputResolutionReason).
	KeyOutputResolution: func(availability) state { return availabilityLive() },
}

// availabilityOptionRules is the option half of the same table: one row per field whose entries grey
// individually, each answering why this combination rules a value out.
// A field with no row here greys no entry.
var availabilityOptionRules = map[string]func(availability, string) *screensharev1.Text{
	KeyCapture:           availability.captureReason,
	KeyTransport:         availability.transportReason,
	KeyCodec:             availability.codecReason,
	KeyChroma:            availability.chromaReason,
	KeyMode:              availability.modeReason,
	KeyColorRange:        availability.colorRangeReason,
	KeyAudioSource:       availability.audioReason,
	KeyAudioSourceDevice: availability.audioDeviceReason,
	KeyAudioCodec:        availability.audioCodecReason,
	KeyCaptureMemory:     availability.frameMemoryReason,
	KeyCursor:            availability.cursorReason,
	KeyOutputResolution:  availability.outputResolutionReason,
	// The two watch legs read the same carriage table under different receivers: an external player
	// opens a URL through libavformat and a tile decodes through a GStreamer pipeline, so the two
	// reach different protocol sets.
	KeyPlayerWatchTransport: func(av availability, value string) *screensharev1.Text {
		return av.watchLegReason(capabilities.EngineFfmpeg, value)
	},
	KeyTileWatchTransport: func(av availability, value string) *screensharev1.Text {
		return av.watchLegReason(capabilities.EngineGst, value)
	},
	KeyRenderChain: availability.renderChainReason,
}

// The four ways a state can read, one constructor each, so a rule states which treatment it applies
// rather than assembling a struct that could silently be a fifth.

// availabilityLive is a control this combination blocks in no way.
func availabilityLive() state {
	return state{visible: true, enabled: true}
}

// availabilityNoted is a control that stays editable and means something its label does not describe
// here.
// A nil note yields a plain live control, which keeps a caller from branching on whether it has one.
func availabilityNoted(note *screensharev1.Text) state {
	return state{visible: true, enabled: true, note: note}
}

// availabilityDisabled is a control the combination blocks, with the statement shown in place of the
// value.
func availabilityDisabled(reason *screensharev1.Text) state {
	assert.IsNotNil(reason, "a disabled control says why")
	return state{visible: true, enabled: false, reason: reason}
}

// availabilityHidden is a backend implementation knob outside the one selection it belongs to.
// Enabled because nothing draws it: the disabled-says-why contract would otherwise demand a
// statement for a widget never on screen.
func availabilityHidden() state {
	return state{visible: false, enabled: true}
}

// availabilityShownFor is the hidden treatment as a condition, for a knob that appears with the one
// protocol it belongs to.
func availabilityShownFor(shown bool) state {
	if !shown {
		return availabilityHidden()
	}
	return availabilityLive()
}

// availability is what a rule reads beyond the fixed tables: the machine, the draft and the facts
// derived from them.
//
// They are derived here rather than inside each rule because each is a lookup with a failure mode of
// its own, and a rule repeating one would be a second place the same "not resolved" case is decided.
type availability struct {
	deps Deps
	s    settings.Settings

	// verdicts is what the rule system says about this draft.
	// Every fact the domain tables state arrives through it rather than being looked up per site, so a
	// control's greying and the publish's own refusal are two readings of one evaluation
	// (internal/rules).
	verdicts rules.Verdicts

	// engine is the publish engine the selected capture backend runs.
	// Empty exactly where the settings name a backend this app has no publisher for, which a
	// hand-edited settings file can do.
	// Every rule keyed by engine then withholds nothing, a fact nobody stated being no limit.
	engine string

	// codec is the capability row of the selected codec, knownCodec whether the table has one.
	// A codec outside the table has no family, format or gaps, so the rules reading them state nothing
	// rather than guessing a half.
	codec      capabilities.Codec
	knownCodec bool

	// path is the frame-memory row of the selected capture backend and the codec's family on that
	// engine, onDevicePath whether the pair has one.
	// A pair rather than a property of either end: the portal capture shares memory with a VAAPI
	// encoder and not with an x264 one.
	path         gpupath.Path
	onDevicePath bool

	// entry is which entry of the audio source list the question is about, noEntry for a control
	// belonging to no list.
	// It rides here rather than as a second argument to every rule, since a rule that does not repeat
	// has no use for it and a table with two signatures would be two tables.
	entry int
	// source is that entry, which for the row past the end of the list is the default: no kind, unity
	// gain, unmuted.
	// That is what the row a reader grows the list by holds, so a rule reading it needs no branch for
	// the row not stored yet.
	source settings.AudioSource
}

func (av availability) forEntry(entry int) availability {
	av.entry = entry
	av.source = audioEntry(av.s, entry)
	return av
}

func availabilityOf(d Deps, s settings.Settings) availability {
	av := availability{deps: d, s: s, verdicts: verdictsOf(d, s)}
	if engine, err := publish.EngineFor(s.Publish.Capture); err == nil {
		av.engine = engine
	}
	av.codec, av.knownCodec = capabilities.Get(s.Publish.Codec)
	if av.engine != "" && av.knownCodec {
		av.path, av.onDevicePath = gpupath.For(av.engine, s.Publish.Capture, av.codec.Family)
	}
	return av
}

// mode is what the selected rate-control concept needs from the encoder.
// A mode the table does not name needs nothing, which greys every knob with its own statement rather
// than offering knobs no builder would read.
func (av availability) mode() availabilityMode {
	return availabilityModes[av.s.Publish.Mode]
}

// family is what the selected codec's encoder family takes.
// A codec outside the table has no family, and the zero row takes neither field, so both grey naming
// who does take them.
func (av availability) family() availabilityFamily {
	if !av.knownCodec {
		return availabilityFamily{}
	}
	return availabilityFamilies[av.codec.Family]
}

// The greying rules.
// Each returns the statement shown in place of the value it withholds, nil where the value reaches
// the encoder.

// captureReason states why this machine cannot run a capture backend.
//
// The verdict and the statement are both publish's, read through rather than restated: the catalog
// shows the same list and has to give the same answer.
//
// A privilege the backend needs is deliberately not a greying (publish.Grant).
// The process either holds it or the capture dies at launch and nothing can tell which in advance,
// so the entry stays selectable and what it needs granted rides on the option as a note.
func (av availability) captureReason(capture string) *screensharev1.Text {
	available, reason := publish.Available(capture, av.deps.Platform)
	if available {
		return nil
	}
	assert.IsNotNil(reason, "an unavailable capture backend says why", capture)
	return reason
}

// transportReason states why the selected capture backend's engine cannot carry a publish transport.
// The map from backend to carriable transports is publish's, so a transport known to some backend
// and absent from this one is one whose sink this engine has no serialization for.
func (av availability) transportReason(name string) *screensharev1.Text {
	allowed, err := publish.TransportsFor(av.s.Publish.Capture)
	if err != nil {
		return nil
	}
	assert.Assert(av.engine != "", "a capture backend the app runs names a publish engine", av.s.Publish.Capture)
	if slices.Contains(allowed, name) {
		return nil
	}
	return say(engineHasNoPublishSink,
		argCapture(av.s.Publish.Capture), argEngine(av.engine), argTransport(name))
}

// codecReason states why this combination cannot encode with a codec.
//
// Four facts withhold one, weighed by what the user can act on.
// A probe that could not run the encoder outranks every table fact, since "no NVIDIA encoder on this
// machine" is the message that stops a search for the right transport.
// Below it sit the engine's own gap, the roadmap, and last the publish leg's carriage, which is the
// one a different transport fixes.
//
// A leg this engine cannot serialize at all states nothing about any codec: that refusal belongs to
// the transport control, and greying every codec under it would name the wrong one.
func (av availability) codecReason(codec string) *screensharev1.Text {
	if av.engine == "" {
		return nil
	}
	// An engine whose own tooling is missing runs nothing, the encoders no probe is spent on included,
	// so one statement covers every codec.
	if _, ok := av.deps.Encoders.Unprobed[av.engine]; ok {
		return say(engineToolingMissing, argEngine(av.engine))
	}
	c, ok := capabilities.Get(codec)
	if !ok {
		return nil
	}
	if reason := av.probeReason(c); reason != nil {
		return reason
	}
	// The rule binds on the engine alone and names the codec it takes, so one evaluation answers for
	// every entry of the dropdown rather than for the selected one alone.
	if reasons := av.verdicts.ValueReasons(KeyCodec, codec); len(reasons) > 0 {
		return reasons[0]
	}
	if !c.Implemented {
		return say(codecNotImplemented)
	}
	if !transport.CanPublish(av.s.Publish.Transport, av.engine) {
		return nil
	}
	if transport.CanPublishFormat(av.s.Publish.Transport, av.engine, c.Format) {
		return nil
	}
	return av.carryBlockReason(c)
}

// probeReason states why this machine's probe could not run a codec on the selected engine, nil
// where it ran or was never asked.
//
// A failed probe means different things per engine and family, and naming the wrong one sends the
// user after the wrong fix.
// On the ffmpeg engine a hardware codec needs a card the machine may not have and a software one
// needs its library compiled into the build.
// On the GStreamer engine the element is missing from the registry instead, its plugin not installed
// or, for the hardware families, finding no device to register it for.
// Which of the two a verdict is follows the family and not the engine.
//
// An engine with no verdicts at all is an unprobed machine rather than one with nothing usable, so
// it greys nothing.
func (av availability) probeReason(c capabilities.Codec) *screensharev1.Text {
	probed, ok := av.deps.Encoders.Usable[av.engine]
	if !ok {
		return nil
	}
	if ran, tested := probed[c.Name]; !tested || ran {
		return nil
	}
	family, ok := availabilityFamilies[c.Family]
	if !ok {
		// Which half is missing is the family's fact, and this row names a family no table here carries.
		// The probe's verdict still holds, so the codec greys under what is known rather than under a
		// guessed half.
		return say(probeFailed, argEngine(av.engine), argCodec(c.Name))
	}
	if family.needsDevice {
		return say(probeNoDevice, argEngine(av.engine), argFamily(c.Family))
	}
	return say(probeNoBuild, argEngine(av.engine), argCodec(c.Name))
}

// carryBlockReason states that the publish leg has no mapping for a codec's bitstream, naming the
// engine that lacks it and where the combination would have worked.
//
// The engine belongs in the statement because the same protocol carries a format on one engine and
// not the other: ffmpeg's WHIP muxer publishes H.264 alone where the GStreamer one payloads VP8 and
// VP9 with it.
// A reason naming the protocol alone would send a user hunting for another transport where another
// capture backend is the fix, so both ways out are named where the tables hold them, and each is an
// argument the surface may or may not receive.
func (av availability) carryBlockReason(c capabilities.Codec) *screensharev1.Text {
	other := capabilities.EngineGst
	if av.engine == capabilities.EngineGst {
		other = capabilities.EngineFfmpeg
	}
	otherEngine := ""
	if transport.CanPublishFormat(av.s.Publish.Transport, other, c.Format) {
		otherEngine = other
	}
	return say(transportCarriesNoCodec,
		argTransport(av.s.Publish.Transport),
		argCodec(c.Name),
		argEngine(av.engine),
		argTransports(transport.PublishNamesFor(av.engine, c.Format)),
		argOtherEngine(otherEngine))
}

// chromaReason states why the selected codec cannot be handed a pixel format.
//
// Two facts block one and the capability table carries both: a format the codec's encoder codes on
// no engine is absent from its Chromas, and one only the other engine's encoder takes carries a gap
// naming that engine.
// The gap's code passes through, since it already says which library or element is the limit.
func (av availability) chromaReason(chroma string) *screensharev1.Text {
	if !av.knownCodec {
		return nil
	}
	if !slices.Contains(av.codec.Chromas, chroma) {
		return availabilityChromaBlock(chroma, av.codec.Name)
	}
	if av.engine == "" {
		return nil
	}
	if reasons := av.verdicts.ValueReasons(KeyChroma, chroma); len(reasons) > 0 {
		return reasons[0]
	}
	return nil
}

// modeReason states why the selected codec cannot be driven in a rate-control mode.
// The capability table keys its gaps by publish engine where only one builder reaches the mode:
// libvpx codes lossless VP9 and the vp9enc element has no such property.
func (av availability) modeReason(mode string) *screensharev1.Text {
	if !av.knownCodec || av.engine == "" {
		return nil
	}
	if reasons := av.verdicts.ValueReasons(KeyMode, mode); len(reasons) > 0 {
		return reasons[0]
	}
	return nil
}

// colorRangeReason states why the stream would not carry a colour range.
// The capability table declares it per codec and engine: an encoder signalling no colour
// description, and a format with no colour range field, both leave a full-range publish watched as
// limited whatever the form said.
func (av availability) colorRangeReason(value string) *screensharev1.Text {
	if !av.knownCodec || av.engine == "" {
		return nil
	}
	if reasons := av.verdicts.ValueReasons(KeyColorRange, value); len(reasons) > 0 {
		return reasons[0]
	}
	return nil
}

// cursorReason states why the selected capture backend does not serve a pointer mode.
//
// It reads the rules and nothing else, the whole fact being theirs: what a backend does with the
// pointer is a per-backend table written as rules (internal/publish/cursor.go), and the one limit
// that is this app's rather than any backend's, that nothing carries a pointer position to a viewer,
// is a rule beside them.
// Both bind on the metadata mode, and both cross.
func (av availability) cursorReason(value string) *screensharev1.Text {
	if reasons := av.verdicts.ValueReasons(KeyCursor, value); len(reasons) > 0 {
		return reasons[0]
	}
	return nil
}

// audioReason states why a session of this platform serves no such capture source.
//
// The verdict and its statement are the platform table's, read through rather than restated: a
// second copy here is what would let the form grey a source the catalog offered, or offer one the
// publish then refuses to open (docs/domain-model.md).
func (av availability) audioReason(source string) *screensharev1.Text {
	_, reason := platform.AudioSourceAvailable(source, av.deps.Platform)
	return reason
}

// audioCodecReason states why the stream cannot be published with an audio codec.
//
// Two independent facts withhold one: the capture backend's publish engine has no encoder element
// for the codec, and the publish transport carries no track in that codec's bitstream format.
// The first is the audio table's own gap and the second the carriage table's, and the statement
// names whichever applies, since the fix differs - another capture backend against another codec on
// the same one.
func (av availability) audioCodecReason(name string) *screensharev1.Text {
	if av.engine == "" {
		return nil
	}
	a, ok := capabilities.GetAudio(name)
	if !ok {
		return nil
	}
	if _, codes := a.EncoderOn(av.engine); !codes {
		// A codec an engine cannot code normally states why as a rule.
		// Where none does, the fact is shown anyway, with no cause invented for it.
		if reasons := av.verdicts.ValueReasons(KeyAudioCodec, a.Name); len(reasons) > 0 {
			return reasons[0]
		}
		return say(engineHasNoAudioEncoder, argEngine(av.engine), argAudioCodec(a.Name))
	}
	carriage, ok := transport.PublishCarriage(av.s.Publish.Transport, av.engine)
	if !ok || slices.Contains(carriage.Audio, a.Format) {
		return nil
	}
	// The codecs that would have worked: what this engine codes and this leg carries.
	// Empty on a leg that carries none, and the statement then reads as the shorter fact rather than
	// trailing off.
	return say(legCarriesNoAudioCodec,
		argTransport(av.s.Publish.Transport),
		argAudioCodec(a.Name),
		argEngine(av.engine),
		argAudioCodecs(capabilities.AudioNamesFor(av.engine, carriage.Audio)))
}

// frameMemoryReason states why this capture backend and codec cannot publish through a frame memory.
//
// Both deciding facts come off the pair table: whether the pair has a device path at all, and what
// that path does to the colour (docs/field-availability.md).
// A greyed device value carries the cost it would have paid, so the value that accepts that cost
// stays live.
//
// Auto and the system copy are never greyed: auto answers with whichever path costs the user nothing
// and system memory is the path every pair has.
//
// Every statement names both ends, since neither decides on its own and switching either side is a
// way to reach the path.
func (av availability) frameMemoryReason(memory string) *screensharev1.Text {
	if av.engine == "" {
		return nil
	}
	if !av.onDevicePath {
		if memory != gpupath.MemoryGpu && memory != gpupath.MemoryGpuEncoderColor {
			return nil
		}
		return say(pairHasNoDeviceMemory,
			argCapture(av.s.Publish.Capture), argCodec(av.s.Publish.Codec), argEngine(av.engine))
	}
	if !av.path.Colour.TradesColour() {
		if memory != gpupath.MemoryGpuEncoderColor {
			return nil
		}
		return say(pairConvertsOnDevice,
			argCapture(av.s.Publish.Capture), argCodec(av.s.Publish.Codec), argMemory(gpupath.MemoryGpu))
	}
	if memory != gpupath.MemoryGpu {
		return nil
	}
	return say(pairTradesColour,
		argCapture(av.s.Publish.Capture),
		argCodec(av.s.Publish.Codec),
		argEngine(av.engine),
		argCost(wire.GpuPathCost(av.path)),
		argReach(availabilityExactColourReach(av.path.Family)))
}

// outputResolutionReason states why this pair cannot be asked for a scaled picture.
//
// One case has it, and it is about where the frames are rather than about the size.
// On a device path the only thing that can resize them is the filter already on that path, and a
// colour-trading row has none: the encoder reads the captured surfaces directly.
// The ffmpeg builder refuses such a run, so the entry greys with the same fact rather than being
// offered into a refusal.
//
// Having such a row is not enough, the run has to be taking it, which staysOnDevice answers.
// The statement offers the system copy as the way across, so the same fact on a run that already
// downloads every frame would name a fix already applied and grey a scale that path's CPU filter can
// perform.
//
// The source size is never greyed: it is what every pair does, and a control with every entry out
// teaches nothing.
func (av availability) outputResolutionReason(value string) *screensharev1.Text {
	if value == "" || !av.staysOnDevice() || !av.path.Colour.TradesColour() {
		return nil
	}
	return say(devicePathHasNoScaler,
		argCapture(av.s.Publish.Capture), argCodec(av.s.Publish.Codec), argMemory(gpupath.MemorySystem))
}

// watchLegReason states why a viewer on this engine cannot receive the stream over a transport.
// Two facts withhold one: the receiver has no form of that protocol at all, and the relay does not
// re-serve this bitstream format on that listener.
// An SRT viewer opened on a VP9 stream connects and receives nothing, MPEG-TS having no mapping for
// it, which is why the choice is answered per format.
//
// A format no implemented codec produces narrows nothing: hiding a choice on absent information
// would hide one that would have worked.
func (av availability) watchLegReason(engine, name string) *screensharev1.Text {
	if !transport.CanWatch(name, engine) {
		return say(noViewerReceivesOver,
			argEngine(engine), argTransport(name), argTransports(transport.WatchNames(engine)))
	}
	format := ""
	if av.knownCodec {
		format = av.codec.Format
	}
	if !capabilities.HasFormat(format) || transport.CanWatchFormat(name, engine, format) {
		return nil
	}
	return say(relayServesNoFormatOver,
		argFormat(format), argTransport(name), argTransports(transport.WatchNamesFor(engine, format)))
}

// renderChainReason states why this machine cannot render through a chain, nil for one it can.
//
// The one availability rule that asks the machine rather than a table: whether a chain runs is
// whether this GStreamer registers the element factories it is built from, which no compiled-in list
// can answer.
// The receive package asks, and this turns its answer into the chain and the first element it needs
// and does not have.
//
// A name no chain carries is not greyed.
// It is a settings file naming a chain this build dropped, which the repair moves off on the same
// resolve.
// Refusing it here would grey an entry no list offers.
func (av availability) renderChainReason(name string) *screensharev1.Text {
	for _, c := range receive.Chains() {
		if c.Name != name || c.Available {
			continue
		}
		return say(renderChainElementMissing, argValue(name), argElement(c.MissingElement))
	}
	return nil
}

// The derived facts the field rules read.

// watchesOver reports whether anything on this machine receives over the named transport: the tile,
// set to one leg, or a player, openable on any leg this machine has a receiver for.
//
// The two receivers are asked different questions, which is the difference between what each of
// their settings decides.
// A tile receives over the leg TileWatchTransport names and over no other.
// A player is opened per press on whichever leg the reader picked, so PlayerWatchTransport is the
// leg a surface offers first and decides nothing about which players run.
// Asking it here hid knobs that were in force: a player opened over RTSP reads RtspWatchProtocol
// whatever that setting says, and with both legs on SRT the control holding it was on no screen.
//
// Hidden and greyed are both answers about a knob that does nothing here.
// One that does something is shown (docs/field-availability.md).
func (av availability) watchesOver(name string) bool {
	return av.s.Viewer.TileWatchTransport == name || transport.CanWatch(name, capabilities.EngineFfmpeg)
}

// knob weighs the three facts that decide a rate-control control: the mode's concept uses it, the
// codec's encoder has it, and the capture backend's engine forwards the value.
// uses carries the first two, already weighed by the caller, reason being their statement.
// The engine's own rule is read here.
//
// An engine that forwards a knob the mode marks unused leaves a note rather than a greying, so the
// field states what the value does there instead of feeding the encoder a number the form never
// showed.
// An engine that drops the knob in every mode outranks the mode's own reason: no rate control brings
// the control back, so naming the mode would send the user hunting for a switch that changes
// nothing.
func (av availability) knob(key string, uses bool, reason *screensharev1.Text) state {
	rule, ruled := av.engineRule(key)
	switch {
	case ruled && rule.forwards:
		return availabilityNoted(say(rule.reason))
	case ruled && len(rule.modes) == 0:
		return availabilityDisabled(say(rule.reason))
	case !uses:
		return availabilityDisabled(reason)
	case ruled:
		return availabilityDisabled(say(rule.reason))
	}
	return availabilityLive()
}

// engineRule finds the rule governing a knob for this engine, codec and mode, false where the
// builder treats the knob exactly as the mode table says.
// Earlier rows win, so a mode-specific reason precedes a codec-wide one.
func (av availability) engineRule(key string) (availabilityEngineRule, bool) {
	for _, r := range availabilityEngineRules {
		if r.knob != key || (r.engine != "" && r.engine != av.engine) {
			continue
		}
		if len(r.modes) > 0 && !slices.Contains(r.modes, av.s.Publish.Mode) {
			continue
		}
		// A rule naming neither codecs nor families covers every codec the engine builds.
		// One naming either covers the union.
		if len(r.codecs) == 0 && len(r.families) == 0 {
			return r, true
		}
		if slices.Contains(r.codecs, av.s.Publish.Codec) {
			return r, true
		}
		if av.knownCodec && slices.Contains(r.families, av.codec.Family) {
			return r, true
		}
	}
	return availabilityEngineRule{}, false
}

// vaapiCeilingNote states the bound the GStreamer VAAPI elements place on a VBR burst ceiling, nil
// where the settings sit inside it.
// Those elements express the target as a percentage of the ceiling and take 50% at the lowest, so a
// ceiling above twice the target has no form there and the GStreamer builder refuses it.
// A note rather than a greying, since the knob is forwarded: the field stays live and carries the
// bound.
func (av availability) vaapiCeilingNote() *screensharev1.Text {
	if av.engine != capabilities.EngineGst || av.s.Publish.Mode != capabilities.ModeVbr {
		return nil
	}
	if !av.knownCodec || av.codec.Family != capabilities.FamilyVaapi {
		return nil
	}
	if av.s.Publish.BitrateM <= 0 || av.s.Publish.MaxrateM <= av.s.Publish.BitrateM*2 {
		return nil
	}
	return say(vaapiCeilingBound, argMaxrateMbps(av.s.Publish.BitrateM*2), argBitrateMbps(float64(av.s.Publish.BitrateM)))
}

// staysOnDevice reports whether this run hands the encoder the frames the capture produced, without
// a trip through system memory.
//
// It reads the row and the value together, the two facts gpupath.Resolve reads.
// Auto reads the colour verdict as well: it takes a device path only where that path costs nothing,
// so a pair whose only path leaves the conversion to the encoder downloads under auto.
func (av availability) staysOnDevice() bool {
	if !av.onDevicePath || av.s.Publish.CaptureMemory == gpupath.MemorySystem {
		return false
	}
	return !(av.s.Publish.CaptureMemory == gpupath.MemoryAuto && av.path.Colour.TradesColour())
}

// frameMemoryNote states which import carries the frames on the direct path, and on the path that
// converts nothing, what that costs.
// The field that chose the trade states the cost beside the import, rather than leaving it to the
// two colour fields it overrides.
func (av availability) frameMemoryNote() *screensharev1.Text {
	if !av.staysOnDevice() {
		return nil
	}
	return say(av.path.Import, argCost(wire.GpuPathCost(av.path)))
}

// encoderColourReason states why the colour fields do not reach this stream, nil where they do.
// On the path that converts nothing the encoder reads the captured surface on its own terms and
// signals what it chose, so both fields grey with the row's cost and Repair moves each onto what the
// encoder signals: a greyed field showing something other than what the run produces is the
// disagreement the form and the publish exist to prevent.
func (av availability) encoderColourReason() *screensharev1.Text {
	if !av.onDevicePath || av.s.Publish.CaptureMemory != gpupath.MemoryGpuEncoderColor {
		return nil
	}
	if !av.path.Colour.TradesColour() {
		return nil
	}
	return wire.GpuPathCost(av.path)
}

// decodeNote states what decoding this stream costs a viewer, nil for a codec the table does not
// carry.
//
// A note and never a block: every format has a software decoder, so the choice is between a viewer's
// GPU and a viewer's cores.
// Where some hardware decodes the pair the statement names those decode families, which ones they
// are being the whole point of the choice.
// Where none does, it names the software element and one family whose limit stands for the rest,
// since the others are out for reasons of their own and listing them states again what the first
// already shows.
func (av availability) decodeNote() *screensharev1.Text {
	if !av.knownCodec {
		return nil
	}
	var families []string
	for _, d := range capabilities.Decoders {
		if d.Format != av.codec.Format || !d.Hardware() || !slices.Contains(d.Chromas, av.s.Publish.Chroma) {
			continue
		}
		if !slices.Contains(families, d.Family) {
			families = append(families, d.Family)
		}
	}
	if len(families) > 0 {
		return say(decodesInHardware, argDecodeFamilies(families))
	}

	software, limited := "", ""
	for _, d := range capabilities.Decoders {
		if d.Format != av.codec.Format {
			continue
		}
		if !d.Hardware() && software == "" {
			software = d.Element
		}
		if d.Hardware() && limited == "" && !slices.Contains(d.Chromas, av.s.Publish.Chroma) {
			limited = d.Family
		}
	}
	return say(decodesOnCPU,
		argFormat(av.codec.Format), argChroma(av.s.Publish.Chroma), argDecoder(software), argDecodeFamily(limited))
}

// The tables the rules read, each stating a fact the Go domain packages do not carry: which capture
// backends take no monitor, what a family's encoders take, and how each engine's builder departs
// from the mode table.

// availabilityKmsgrab is the one capture backend a field is gated on by name: the DRM download
// strategy is a knob of its scanout path and of nothing else.
const availabilityKmsgrab = "kmsgrab"

// The transport names the per-protocol knobs are gated on, spelled as the transport registry keys
// them.
// A name the registry does not carry would gate a control on a protocol nothing publishes
// (TestEveryGatedTransportIsRegistered).
const (
	availabilitySrt    = "srt"
	availabilityRtsp   = "rtsp"
	availabilityWebrtc = "webrtc"
	availabilityRtmp   = "rtmp"
	availabilityHls    = "hls"
)

// availabilityMonitorless is the capture backends that take no monitor index.
// What each captures instead follows from the backend, so the statement names the backend and the
// surface says what that one grabs, the same sentence its entry in the capture dropdown already
// writes.
var availabilityMonitorless = []string{availabilityKmsgrab, "gdigrab", "portal"}

// availabilityFullRangeChromas are the pixel formats carrying no quantization range choice at all.
// RGB is full range by construction, leaving the colour range control nothing to decide under it.
var availabilityFullRangeChromas = []string{"gbrp"}

// availabilityMode is what one rate-control concept needs from the encoder, the first of the three
// facts that decide a rate-control field.
// It states which controls the mode uses, not what any encoder does with them.
type availabilityMode struct {
	// usesCq: crf alone targets a quantizer.
	usesCq bool
	// usesBitrate: cbr, vbr and abr target a bitrate, crf and lossless none.
	usesBitrate bool
	// usesMaxrate: vbr alone sets a burst ceiling over the target.
	usesMaxrate bool
	// usesVbv: cbr and vbr bound the rate with a buffer of tunable size.
	usesVbv bool
	// usesBframes: the lossy bitrate and quality modes gain from B-frames, where the family counts
	// them.
	usesBframes bool
}

// availabilityModes is what each rate-control mode needs, keyed as capabilities names the modes.
// A mode outside the table needs nothing, which answers a hand-edited settings file naming one no
// builder implements.
var availabilityModes = map[string]availabilityMode{
	capabilities.ModeCbr:      {usesBitrate: true, usesVbv: true},
	capabilities.ModeVbr:      {usesBitrate: true, usesMaxrate: true, usesVbv: true, usesBframes: true},
	capabilities.ModeAbr:      {usesBitrate: true, usesBframes: true},
	capabilities.ModeCrf:      {usesCq: true, usesBframes: true},
	capabilities.ModeLossless: {},
}

// availabilityFamily is what an encoder family's encoders take and where they come from, the second
// of the three facts a rate-control field is decided by.
type availabilityFamily struct {
	// takesBframes: a family with no property for the count greys the field whatever the rate-control
	// mode, and the builders pin it off rather than forwarding a value the encoder would ignore.
	// The effort step is no such field: it follows the codec's own ladder, one family's codecs being
	// free to declare different ones.
	takesBframes bool
	// needsDevice: whether the encoders come with a device rather than with a build.
	// An absent encoder is then the machine's answer (no such GPU, or no driver exposing that encode
	// entrypoint) where a software one's is the build's.
	needsDevice bool
}

// availabilityFamilies is one row per encoder family capabilities declares.
// Every family has a row (TestEveryEncoderFamilyStatesWhatItsEncodersTake): one missing here reaches
// no verdict of its own and would grey under the engine's name instead.
var availabilityFamilies = map[string]availabilityFamily{
	capabilities.FamilySoftware: {},
	capabilities.FamilyNvenc:    {takesBframes: true, needsDevice: true},
	capabilities.FamilyVaapi:    {needsDevice: true},
	capabilities.FamilyQsv:      {needsDevice: true},
	capabilities.FamilyAmf:      {needsDevice: true},
	capabilities.FamilyV4l2:     {needsDevice: true},
	capabilities.FamilyRkmpp:    {needsDevice: true},
	capabilities.FamilyVulkan:   {needsDevice: true},
}

// availabilityFamiliesWith lists the families whose row sets a flag, for a statement naming who takes
// a field rather than one family written in by hand.
// The list follows the table when a family gains the field.
//
// The order is capabilities.Families' own, so the statement reads the same on every call rather than
// in the map's iteration order of the moment.
func availabilityFamiliesWith(flag func(availabilityFamily) bool) []string {
	var out []string
	for _, name := range capabilities.Families {
		family, ok := availabilityFamilies[name]
		assert.Assert(ok, "every encoder family states what its encoders take", name)
		if flag(family) {
			out = append(out, name)
		}
	}
	return out
}

// availabilityEngineRule is one place where a publish engine's encoder builder departs from the mode
// table: a knob the mode uses that the engine ignores, or one the engine forwards in a mode the
// table marks unused.
// It is the third of the three facts.
//
// Whether a value the mode needs reaches the encoder depends on which engine builds the command, the
// two expressing the same modes through different properties: the GStreamer nvcodec and QSV elements
// carry no rate buffer, and the vpx elements read a constant-quality bitrate as a cap.
//
// A knob an encoder library has no form of at all is a rule with no engine, both builders hitting
// the same wall.
// The rules mirror encoderArgs in ffmpeg/encoders.go and gstEncoder in publish/gstencoders.go.
type availabilityEngineRule struct {
	// engine is empty for a rule describing the library rather than a builder, and then holds on both.
	engine string
	// knob is the field key the rule governs, so a rule and the control it greys are one identifier.
	knob string
	// codecs and families select what the rule covers, unioned.
	// Both empty covers every codec the engine builds.
	codecs   []string
	families []string
	// modes selects the rate-control modes the rule covers.
	// Empty covers every mode, which is what makes such a rule outrank the mode's own reason.
	modes []string
	// forwards is true where the builder sends the value though the mode table marks the knob unused,
	// false where it ignores a value the mode would use.
	forwards bool
	// reason names why an ignored value never reaches the encoder, or what a forwarded one does there.
	// A code and not a sentence, one per row: each states a fact about one named element.
	reason screensharev1.TextCode
}

// The table decides which knobs a builder never reads, so a malformed row is an Entwicklungsfehler
// and fails at load.
//
// A row naming no knob governs nothing and would sit in the table doing nothing visible.
// One carrying no reason would grey or annotate a control with no code behind the sentence.
func init() {
	for i, r := range availabilityEngineRules {
		assert.Assert(r.knob != "", "a departure names the control it governs", i)
		assert.Assert(r.reason != screensharev1.TextCode_TEXT_CODE_UNSPECIFIED,
			"a departure states the fact behind it", r.knob, i)
		assert.Assert(r.engine == "" || slices.Contains(capabilities.Engines, r.engine),
			"a departure that names an engine names a publish engine", r.engine, r.knob)
	}
}

// availabilityEngineRules is the departure table, earlier rows winning.
var availabilityEngineRules = []availabilityEngineRule{
	// Neither engine withholds an effort step, so no row does.
	// Every element that codes a laddered codec takes its steps through a property of its own,
	// speed-preset, cpu-used or preset, and the nvcodec elements take the same p1-p7 steps ffmpeg does.
	//
	// An encoder that cannot bound the burst at all has no VBR mode, declared as a mode gap in the
	// capability table, so no row withholds the ceiling field for that case: a rule would grey a field
	// under a mode that cannot be selected.
	{
		knob:   KeyVbvMs,
		codecs: []string{"librav1e"},
		reason: screensharev1.TextCode_TEXT_CODE_RAV1E_SIZES_NO_RATE_BUFFER,
	},
	{
		engine: capabilities.EngineGst,
		knob:   KeyGop,
		codecs: []string{"librav1e"},
		reason: screensharev1.TextCode_TEXT_CODE_GST_RAV1ENC_NO_KEYFRAME_INTERVAL,
	},
	{
		engine:   capabilities.EngineGst,
		knob:     KeyVbvMs,
		families: []string{capabilities.FamilyNvenc},
		reason:   screensharev1.TextCode_TEXT_CODE_GST_NVENC_NO_RATE_BUFFER,
	},
	{
		engine:   capabilities.EngineGst,
		knob:     KeyVbvMs,
		families: []string{capabilities.FamilyQsv},
		reason:   screensharev1.TextCode_TEXT_CODE_GST_QSV_NO_RATE_BUFFER,
	},
	{
		engine:   capabilities.EngineFfmpeg,
		knob:     KeyBitrateM,
		families: []string{capabilities.FamilyNvenc},
		modes:    []string{capabilities.ModeCrf},
		forwards: true,
		reason:   screensharev1.TextCode_TEXT_CODE_NVENC_CQ_BITRATE_CAPS_BURSTS,
	},
	{
		engine:   capabilities.EngineGst,
		knob:     KeyBitrateM,
		codecs:   []string{"libvpx-vp9", "libvpx"},
		modes:    []string{capabilities.ModeCrf},
		forwards: true,
		reason:   screensharev1.TextCode_TEXT_CODE_GST_VPX_CQ_BITRATE_IS_CAP,
	},
	{
		knob:     KeyBitrateM,
		families: []string{capabilities.FamilyVaapi, capabilities.FamilyAmf, capabilities.FamilyVulkan, capabilities.FamilyQsv},
		modes:    []string{capabilities.ModeAbr},
		forwards: true,
		reason:   screensharev1.TextCode_TEXT_CODE_FIXED_FUNCTION_ABR_DERIVES_CEILING,
	},
	{
		knob:     KeyBframes,
		families: []string{capabilities.FamilyAmf},
		reason:   screensharev1.TextCode_TEXT_CODE_AMF_CODES_NO_BFRAMES,
	},
}

// availabilityChromaBlock states that a codec cannot encode a pixel format on either engine.
// Planar RGB carries its own code: RGB reaches an encoder only through HEVC's Range Extensions or
// VP9's identity matrix, a coding-tool fact rather than a subsampling limit, and a reader told only
// "cannot encode gbrp" would go looking for a setting that would fix it.
func availabilityChromaBlock(chroma, codec string) *screensharev1.Text {
	if chroma == "gbrp" {
		return say(codecCodesNoRGB, argCodec(codec))
	}
	return say(codecCannotEncodeChroma, argCodec(codec), argChroma(chroma))
}

// availabilityExactColourReach states how an encoder family is reached on the device with the colour
// the form shows, nil where the pair table declares no such row.
//
// It is the way out the greyed direct value carries, and the one the dropdown cannot show by itself:
// the fix is another capture backend rather than another value of this field.
// Assembled from the row, so it names whichever pair the table declares rather than a platform
// written in here.
func availabilityExactColourReach(family string) *screensharev1.Text {
	for _, p := range gpupath.Paths {
		if p.Family != family || p.Colour.TradesColour() {
			continue
		}
		return say(exactColourReach, argCapture(p.Capture), argEngine(p.Engine), argImport(say(p.Import)))
	}
	return nil
}

// audioDeviceReason states why one entry cannot record from a device it is offered.
//
// It refuses nothing: what is inside a kind is what the machine answered, and a selection it stopped
// answering for is kept with a note rather than greyed, an application that is not running being one
// that may be running when the stream starts (audio.go).
// The row exists because every control that offers entries is answered for here, which makes a later
// refusal a line in this file rather than a new mechanism.
func (av availability) audioDeviceReason(string) *screensharev1.Text {
	return nil
}
