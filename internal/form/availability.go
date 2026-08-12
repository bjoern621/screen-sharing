package form

// Availability: which controls the current settings leave usable, and what the screen
// says about the ones they do not.
//
// This is the Go half of what the Wails frontend's util/deps.ts decided in
// TypeScript. Every verdict here reads a table - capabilities for the codec facts,
// transport for the carriage, gpupath for the frame-memory pairs, publish for the
// capture backends and their engines - and none of them restates a rule those tables
// already carry. The one thing this file adds is the statement, because a limit the
// user cannot read is a limit they cannot act on.
//
// A statement is a code and the identifiers it is about, never a sentence
// (statements.go). What that fact reads as on screen is the surface's, written where
// the column width and the tone are visible; what is true is decided here and nowhere
// else. The two halves meet on identifiers - a capture backend's name, an engine's, a
// pixel format's - which is what lets the domain gain a codec without a shell being
// edited.
//
// Two contracts govern everything below. A disabled control always says why, which
// form.go asserts on every field it renders, so every greying path here has to produce
// a statement. And where two facts block the same field, the reason names the one the
// user can act on (docs/field-availability.md, "Three facts decide a rate-control
// field"): B-frames under software x264 state that only some encoder families take a
// B-frame count, not the mode fact that would be a lie there.
//
// An unresolved fact withholds nothing. deps.ts wrote that as a null check per table;
// here the only facts that can be missing are the machine's - a capture backend this
// app has no publisher for names no engine, and a probe that never ran states no
// verdict - because the tables themselves are compiled in. Both cases leave the
// control live rather than greying it under a fact nobody stated.

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
// The key names a row of the availability table, and an unknown one is a caller that
// invented a control: returning a plain enabled state for it would put a widget on
// screen that no rule here governs and no repair below moves, which is exactly the
// silent fork the contract exists to remove.
// The key is the row's own, which for a repeated control is the template rather than one
// entry's key: the statement is about the control, and writing it per index would be writing
// it once per entry a reader happens to have made. Which entry it is being asked about
// travels beside it, for the rows that answer differently per entry.
func fieldState(d Deps, s settings.Settings, key string, entry int) state {
	rule, ok := availabilityRules[key]
	assert.Assert(ok, "an availability question names a field the form declares", key)

	st := rule(availabilityOf(d, s).forEntry(entry))

	assert.Assert(st.enabled || st.reason != nil, "a disabled control says why", key)
	assert.Assert(st.enabled || st.note == nil, "a note rides on a control that is still editable", key)
	return st
}

// optionState decides one entry of a select or radio, within the field named by key.
//
// A field with no option-level rule leaves every entry enabled: the greying that
// applies to the whole control is fieldState's, and repeating it per entry would grey
// a dropdown twice over.
func optionState(d Deps, s settings.Settings, key, value string, entry int) (enabled bool, reason *screensharev1.Text) {
	_, declared := availabilityRules[key]
	assert.Assert(declared, "an option question names a field the form declares", key)

	rule, ok := availabilityOptionRules[key]
	if !ok {
		return true, nil
	}
	reason = rule(availabilityOf(d, s).forEntry(entry), value)
	return reason == nil, reason
}

// availabilityRules is the availability table: one row per field the form declares,
// each deciding that control's state from the draft and the machine.
//
// It is a table rather than a switch for the reason every other fixed fact in this
// repository is (docs/development-principles.md): a control added to keys.go and left
// out here fails the lookup above instead of quietly rendering as a plain enabled
// widget, and the rows that decide nothing say so in one word rather than by falling
// through a default nobody reads.
var availabilityRules = map[string]func(availability) state{
	// The connection group. The stream's name and the relay's address apply whatever
	// else is selected; each relay listener port is a knob of one protocol and is
	// hidden while that protocol is not the leg being configured, which is the hidden
	// treatment's own case (docs/field-availability.md, "The rule"): a user on SRT has
	// no reason to read what the RTMP listener's port means.
	KeyName:      func(availability) state { return availabilityLive() },
	KeyRelayHost: func(availability) state { return availabilityLive() },
	KeyAPIPort:   func(availability) state { return availabilityLive() },
	KeySrtPort:   func(av availability) state { return availabilityShownFor(av.s.Publish.Transport == availabilitySrt) },
	KeyRtspPort:  func(av availability) state { return availabilityShownFor(av.s.Publish.Transport == availabilityRtsp) },
	KeyWebrtcPort: func(av availability) state {
		return availabilityShownFor(av.s.Publish.Transport == availabilityWebrtc)
	},
	KeyRtmpPort: func(av availability) state { return availabilityShownFor(av.s.Publish.Transport == availabilityRtmp) },
	// HLS follows the watch leg rather than the publish one: the relay serves it and
	// ingests nothing over it, so nothing is ever published this way and the publish
	// dropdown never names it.
	KeyHlsPort: func(av availability) state {
		return availabilityShownFor(av.s.Viewer.PlayerWatchTransport == availabilityHls)
	},

	// The dimensions whose greying is per entry rather than per control. The dropdown
	// keeps every value and greys the ones this combination rules out, because the
	// greyed entry plus its reason is what tells the user what to change.
	KeyTransport: func(availability) state { return availabilityLive() },
	KeyCodec:     func(availability) state { return availabilityLive() },
	KeyMode:      func(availability) state { return availabilityLive() },
	KeyCapture:   func(availability) state { return availabilityLive() },

	KeyFps: func(availability) state { return availabilityLive() },
	// The pointer is greyed per entry and never as a control: every backend serves at
	// least one mode, so there is no combination where the question does not apply.
	KeyCursor: func(availability) state { return availabilityLive() },

	// The pixel format is the one control that carries a note about somebody else's
	// machine: what the choice costs a viewer to decode. It is a note and never a
	// greying, since every format has a software decoder, so a format no GPU takes is
	// a viewer spending cores rather than a limit here - a trade the publisher is
	// entitled to make once it is stated (docs/field-availability.md).
	//
	// It greys for one reason only: on the device path that converts nothing, the
	// encoder picks the layout itself, so the control would otherwise show a value the
	// stream does not carry.
	KeyChroma: func(av availability) state {
		if reason := av.encoderColourReason(); reason != nil {
			return availabilityDisabled(reason)
		}
		return availabilityNoted(av.decodeNote())
	},
	// The colour range is blocked by two facts, and the encoder-colour path outranks
	// the format's own: under it neither field reaches the stream, so naming RGB would
	// send the user changing a control that changes nothing here.
	KeyColorRange: func(av availability) state {
		if reason := av.encoderColourReason(); reason != nil {
			return availabilityDisabled(reason)
		}
		if slices.Contains(availabilityFullRangeChromas, av.s.Publish.Chroma) {
			return availabilityDisabled(say(rgbIsFullRange))
		}
		return availabilityLive()
	},

	// The rate-control knobs. Three facts decide each one and the knob helper weighs
	// them in the order docs/field-availability.md states.
	KeyCq: func(av availability) state {
		return av.knob(KeyCq, av.mode().usesCq, say(cqOnlyInConstantQuality))
	},
	KeyBitrateM: func(av availability) state {
		return av.knob(KeyBitrateM, av.mode().usesBitrate, say(bitrateNotInMode, argMode(av.s.Publish.Mode)))
	},
	KeyMaxrateM: func(av availability) state {
		st := av.knob(KeyMaxrateM, av.mode().usesMaxrate, say(maxrateOnlyInConstrained))
		// The va elements express a VBR target as a percentage of the ceiling and take
		// 50% at the lowest, so a ceiling more than twice the target has no form there
		// and the GStreamer builder refuses it. The knob is forwarded, so the field
		// stays live and carries the bound instead of being greyed.
		if st.enabled {
			st.note = av.vaapiCeilingNote()
		}
		return st
	},
	KeyVbvMs: func(av availability) state {
		return av.knob(KeyVbvMs, av.mode().usesVbv, say(vbvOnlyInBoundedModes))
	},
	// The keyframe interval is not a rate-control concept, so no mode withholds it;
	// only an encoder element with no property for it does.
	KeyGop: func(av availability) state { return av.knob(KeyGop, true, nil) },
	// B-frames are blocked by two independent facts, so the reason names the one that
	// applies instead of always blaming the mode. Which families take a count is the
	// family table's, so the statement lists them from it rather than naming one by hand.
	KeyBframes: func(av availability) state {
		usesBframes := av.mode().usesBframes
		reason := say(bframesOffInMode, argMode(av.s.Publish.Mode))
		if usesBframes {
			reason = say(bframesOnlyOnFamilies,
				argFamilies(availabilityFamiliesWith(func(f availabilityFamily) bool { return f.takesBframes })))
		}
		return av.knob(KeyBframes, usesBframes && av.family().takesBframes, reason)
	},
	// The effort step follows the codec's own row rather than its family's, because the
	// ladder does: the steps are the encoder's identifiers, so two codecs of one family
	// can offer different ones and a family that declares none offers nothing to spend.
	// Both the entries this control lists and the step a build spends come off that same
	// row, which is what keeps the greying and the encode in step.
	KeyEffort: func(av availability) state {
		ladder := av.codec.Effort
		pinned := ladder.PinsIn(av.s.Publish.Mode)
		reason := say(codecTakesNoEffortLadder, argCodec(av.s.Publish.Codec))
		// A mode that pins the step carries the one it pins to, so the statement names
		// the declared value instead of restating one.
		if pinned {
			step, _ := ladder.StepFor(av.s.Publish.Mode)
			reason = say(effortPinnedByMode, argMode(av.s.Publish.Mode), argEffort(step))
		}
		return av.knob(KeyEffort, len(ladder.Steps) > 0 && !pinned, reason)
	},
	// The tune is read the same way and from the same row, since it is the other half of
	// one decision: how hard the encoder works, and what it works towards. A codec can
	// declare either ladder without the other - the Vulkan rows tune and take no effort
	// step, the libvpx ones take a step and tune for nothing - so each control asks about
	// its own ladder rather than about the pair.
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

	// The audio codec is read only where the stream has a track to code, so with no
	// source the control is inert rather than wrong. It is greyed with the reason
	// rather than hidden: the codec is a general concept, and the greyed field plus
	// its statement say why it does not apply here.
	KeyAudioCodec: func(av availability) state {
		if len(av.s.Publish.Recorded()) == 0 {
			return availabilityDisabled(say(audioCodecNeedsSource))
		}
		return availabilityLive()
	},

	// One entry of the source list. The kind is always editable, because it is what an
	// entry is: setting it to none is how an entry is taken off, and greying it would leave
	// a reader with a source they cannot remove.
	KeyAudioSource: func(availability) state { return availabilityLive() },

	// What is inside the kind, which only some kinds have more than one of. A kind with
	// one device is a control with nothing to choose, so it greys and names the kind rather
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

	// The level and the silence, which mean nothing until the entry names a source. Both
	// reach a pipeline that is already running, so neither is greyed for anything else.
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

	// The DRM download strategy is the hidden treatment's own example: a knob of the
	// kmsgrab scanout path and of nothing else, whose help would teach a user on any
	// other backend nothing. Under kmsgrab it stays rendered and greys where the run
	// downloads nothing, rather than vanishing a second time while the user changes
	// codecs (docs/field-availability.md, "Three facts decide the frame memory").
	KeyDrmMap: func(av availability) state {
		if av.s.Publish.Capture != availabilityKmsgrab {
			return availabilityHidden()
		}
		if av.staysOnDevice() {
			return availabilityDisabled(say(drmMapUnusedOnDevice))
		}
		return availabilityLive()
	},
	// ddagrab selects an output by index and the X backends crop the X screen to the
	// monitor's geometry. The backends that take no monitor index each say so, naming
	// themselves, so the surface can state what that one captures instead.
	KeyMonitor: func(av availability) state {
		if slices.Contains(availabilityMonitorless, av.s.Publish.Capture) {
			return availabilityDisabled(say(captureTakesNoMonitor, argCapture(av.s.Publish.Capture)))
		}
		return availabilityLive()
	},
	// The frame memory is never greyed as a whole: auto and the system copy are values
	// every pair satisfies, so no combination leaves a dead control. What it carries is
	// which import the direct path uses, and on the path that converts nothing, what
	// that costs.
	KeyCaptureMemory: func(av availability) state { return availabilityNoted(av.frameMemoryNote()) },

	// The per-protocol knobs, hidden with their protocol for the same reason the
	// listener ports are. Each names a mechanism of one leg of one transport.
	KeySrtPublishLatencyMs: func(av availability) state {
		return availabilityShownFor(av.s.Publish.Transport == availabilitySrt)
	},
	KeyRtspPublishProtocol: func(av availability) state {
		return availabilityShownFor(av.s.Publish.Transport == availabilityRtsp)
	},
	// The watch-leg knobs follow the receivers that read them, and a knob two receivers
	// read follows either of them: the SRT window and the RTP lower transport are the
	// link's rather than one reader's, so a viewer configuring the tile leg is configuring
	// the same link a player would open.
	KeySrtWatchLatencyMs: func(av availability) state {
		return availabilityShownFor(av.watchesOver(availabilitySrt))
	},
	KeyRtspWatchProtocol: func(av availability) state {
		return availabilityShownFor(av.watchesOver(availabilityRtsp))
	},
	// The jitter buffer is the tile receiver's alone: a player buffers by reorder queue
	// rather than by time, so it follows the tile leg and not either.
	KeyRtspWatchLatencyMs: func(av availability) state {
		return availabilityShownFor(av.s.Viewer.TileWatchTransport == availabilityRtsp)
	},

	KeyUplinkMbps:           func(availability) state { return availabilityLive() },
	KeyPlayerWatchTransport: func(availability) state { return availabilityLive() },
	KeyTileWatchTransport:   func(availability) state { return availabilityLive() },

	// The render chain is live whatever leg is chosen: what a decode converts its frames
	// with is a property of this machine's GStreamer rather than of the protocol they
	// arrived over. A chain this machine cannot run is greyed per entry with the element
	// that is missing named (availabilityOptionRules below).
	KeyRenderChain: func(availability) state { return availabilityLive() },

	// The output resolution is live. What can refuse a value is one entry rather than
	// the control: a frame path with nothing on it that resizes greys the scaled
	// entries and leaves the source size, so the reader is told which pair to change
	// instead of finding a dead field (availabilityOptionRules below).
	KeyOutputResolution: func(availability) state { return availabilityLive() },
}

// availabilityOptionRules is the option half of the same table: one row per field
// whose entries are greyed individually, each answering why this combination rules a
// value out. A field with no row here greys no entry.
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
	// The two watch legs read the same carriage table under different receivers: an
	// external player opens a URL through libavformat, and a tile decodes through a
	// GStreamer pipeline, so the two reach different protocol sets.
	KeyPlayerWatchTransport: func(av availability, value string) *screensharev1.Text {
		return av.watchLegReason(capabilities.EngineFfmpeg, value)
	},
	KeyTileWatchTransport: func(av availability, value string) *screensharev1.Text {
		return av.watchLegReason(capabilities.EngineGst, value)
	},
	KeyRenderChain: availability.renderChainReason,
}

// The four ways a state can read, one constructor each, so a rule above states which
// of the four treatments it is applying rather than assembling a struct that could
// silently be a fifth.

// availabilityLive is a control this combination blocks in no way.
func availabilityLive() state {
	return state{visible: true, enabled: true}
}

// availabilityNoted is a control that stays editable and means something its label
// does not describe here. No note is a plain live control, which is what keeps a
// caller from branching on whether it has one.
func availabilityNoted(note *screensharev1.Text) state {
	return state{visible: true, enabled: true, note: note}
}

// availabilityDisabled is a control the combination blocks, greyed with the statement
// shown in its place.
func availabilityDisabled(reason *screensharev1.Text) state {
	assert.IsNotNil(reason, "a disabled control says why")
	return state{visible: true, enabled: false, reason: reason}
}

// availabilityHidden is a backend implementation knob outside the one selection it
// belongs to. It stays enabled because nothing draws it: a hidden control carries no
// reason, and the disabled-says-why contract would otherwise demand a statement for a
// widget that is never on screen.
func availabilityHidden() state {
	return state{visible: false, enabled: true}
}

// availabilityShownFor is the hidden treatment stated as a condition, for the knobs
// that belong to one protocol and appear with it.
func availabilityShownFor(shown bool) state {
	if !shown {
		return availabilityHidden()
	}
	return availabilityLive()
}

// availability is what one resolve reads beyond the fixed tables: the machine, the
// draft, and the three facts every rule above derives from.
//
// The three are derived once here rather than per rule because each is a lookup with a
// failure mode of its own, and a rule that repeated one would be a second place the
// same "not resolved" case is decided.
type availability struct {
	deps Deps
	s    settings.Settings

	// verdicts is what the rule system said about this draft, evaluated once per resolve
	// and read through by whichever control asks. Every fact the domain tables state
	// arrives here rather than being looked up per site, so a control's greying and the
	// publish's own refusal are two readings of one evaluation (internal/rules).
	verdicts rules.Verdicts

	// engine is the publish engine the selected capture backend runs. It is empty
	// exactly when the settings name a backend this app has no publisher for, which a
	// hand-edited settings file can do; every rule keyed by engine then withholds
	// nothing, since a fact nobody stated is not a limit.
	engine string

	// codec is the capability row of the selected codec, and knownCodec whether the
	// table has one. A codec outside the table has no family, no format and no gaps,
	// so the rules that read them state nothing rather than guessing a half.
	codec      capabilities.Codec
	knownCodec bool

	// path is the frame-memory row of the selected capture backend and the codec's
	// family on that engine, and onDevicePath whether the pair has one. It is a pair
	// rather than a property of either end: the portal capture shares memory with a
	// VAAPI encoder and not with an x264 one.
	path         gpupath.Path
	onDevicePath bool

	// entry is which entry of the audio source list the question is about, and noEntry for
	// a control that belongs to no list. It rides here rather than as a second argument to
	// every rule, because a rule that does not repeat has no use for it and a table with
	// two signatures would be two tables.
	entry int
	// source is that entry, which for the row past the end of the list is the default
	// entry: no kind, unity gain, unmuted. It is what the row a reader grows the list by
	// holds, so a rule reading it needs no branch for the row that is not stored yet.
	source settings.AudioSource
}

// forEntry is this availability asked about one entry of the audio source list.
func (av availability) forEntry(entry int) availability {
	av.entry = entry
	av.source = audioEntry(av.s, entry)
	return av
}

// availabilityOf resolves the three derived facts for one draft.
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

// mode is what the selected rate-control concept needs from the encoder. A mode the
// table does not name needs nothing, which greys every knob with its own statement
// rather than offering knobs no builder would read.
func (av availability) mode() availabilityMode {
	return availabilityModes[av.s.Publish.Mode]
}

// family is what the selected codec's encoder family takes. A codec outside the table
// has no family, and the zero row takes neither of the two fields, so both grey with
// the statement naming who does take them.
func (av availability) family() availabilityFamily {
	if !av.knownCodec {
		return availabilityFamily{}
	}
	return availabilityFamilies[av.codec.Family]
}

// The greying rules. Each returns the statement shown in place of the value it
// withholds, and nil where the value reaches the encoder.

// captureReason states why this machine cannot run a capture backend.
//
// The verdict and the statement are both publish's, read through rather than restated:
// the catalog shows the same list and has to give the same answer, and a second copy of
// the gate here would be the drift docs/ipc-api.md exists to end.
//
// A privilege the backend needs is deliberately not a greying (publish.Grant). The
// process either holds it or the capture dies at launch and nothing can tell which in
// advance, so the entry stays selectable, where the choice is the user's and the failure
// is theirs to read; what it needs granted rides on the option as a note.
func (av availability) captureReason(capture string) *screensharev1.Text {
	available, reason := publish.Available(capture, av.deps.Platform)
	if available {
		return nil
	}
	assert.IsNotNil(reason, "an unavailable capture backend says why", capture)
	return reason
}

// transportReason states why the selected capture backend's engine cannot carry a
// publish transport. The map from backend to carriable transports is publish's, so a
// transport known to some backend and absent from this one is one whose sink this
// engine has no serialization for.
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
// Four facts withhold one and they are weighed in the order of what the user can act
// on. A probe that could not run the encoder outranks every table fact, since "no
// NVIDIA encoder on this machine" is the message that stops a search for the right
// transport. Below it sit the engine's own gap, the roadmap, and last the publish
// leg's carriage, which is the one a different transport fixes.
//
// A leg this engine cannot serialize at all states nothing about any codec. That is
// the transport's own refusal, already on the transport control, and greying every
// codec under it would name the wrong control.
func (av availability) codecReason(codec string) *screensharev1.Text {
	if av.engine == "" {
		return nil
	}
	// An engine whose own tooling is missing runs nothing, the encoders no probe is
	// spent on included, so its statement covers every codec rather than one at a time.
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
	// The entry is refused where no encoder for it exists on this engine. The rule binds
	// on the engine alone and names the codec it takes, which is what lets one evaluation
	// answer for every entry of the dropdown rather than only for the selected one.
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

// probeReason states why this machine's probe could not run a codec on the selected
// engine, and nil where it ran or was never asked.
//
// A failed probe means different things per engine and family, and naming the wrong one
// sends the user looking for the wrong fix. On the ffmpeg engine a hardware codec needs
// a card the machine may not have and a software one needs its library compiled into
// the build. On the GStreamer engine the element is missing from the registry instead,
// either because its plugin is not installed or, for the hardware families, because the
// plugin found no device to register it for. Which of the two a verdict is follows the
// family and not the engine, so the statement carries both and the surface says what
// that combination means.
//
// An engine with no verdicts at all is an unprobed machine and not one with nothing
// usable, so it greys nothing. That is where this parts from the frontend it replaces:
// a null probe was a distinguishable state in TypeScript, and a zero Availability is
// the same state here.
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
		// Which half is missing is the family's fact and this codec's row names a
		// family no table here carries. The probe's verdict still holds, so the codec
		// is greyed under what is known rather than under a guessed half.
		return say(probeFailed, argEngine(av.engine), argCodec(c.Name))
	}
	if family.needsDevice {
		return say(probeNoDevice, argEngine(av.engine), argFamily(c.Family))
	}
	return say(probeNoBuild, argEngine(av.engine), argCodec(c.Name))
}

// carryBlockReason states that the publish leg has no mapping for a codec's bitstream,
// naming the engine that lacks it and where the combination would have worked.
//
// The engine belongs in the statement because the same protocol carries a format on one
// engine and not the other: ffmpeg's WHIP muxer publishes H.264 alone where the
// GStreamer one payloads VP8 and VP9 with it. A reason naming the protocol by itself
// would send a user hunting for another transport where another capture backend is the
// fix, so both ways out are named where the tables hold them - and each is an argument
// the surface may or may not receive, since the tables declare one for some rows and
// neither for others.
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
// Two facts block one and the capability table carries both: a format the codec's
// encoder codes on no engine is absent from its Chromas, and one only the other
// engine's encoder takes carries a gap naming that engine. The gap's code is passed
// through, since it already says which library or element is the limit.
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
// The gaps come from the capability table, which keys them by publish engine where
// only one builder reaches the mode: libvpx codes lossless VP9 and the vp9enc element
// has no such property.
func (av availability) modeReason(mode string) *screensharev1.Text {
	if !av.knownCodec || av.engine == "" {
		return nil
	}
	if reasons := av.verdicts.ValueReasons(KeyMode, mode); len(reasons) > 0 {
		return reasons[0]
	}
	return nil
}

// colorRangeReason states why the stream would not carry a colour range. The capability
// table declares it per codec and engine: an encoder that signals no colour description,
// and a format with no colour range field, both leave a full-range publish watched as
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
// It reads the rules and nothing else, because the whole fact is theirs: what a backend
// does with the pointer is a per-backend table (internal/publish/cursor.go) written as
// rules rather than converted into them, and the one limit that is this app's rather
// than any backend's - that nothing carries a pointer position to a viewer yet - is a
// rule beside them. Both bind on the metadata mode, and both cross.
func (av availability) cursorReason(value string) *screensharev1.Text {
	if reasons := av.verdicts.ValueReasons(KeyCursor, value); len(reasons) > 0 {
		return reasons[0]
	}
	return nil
}

// audioReason states why a session of this platform serves no such capture source.
//
// The verdict and its statement are the platform table's, read through rather than
// restated: which sources a machine offers is that table's whole question, and a second
// copy of the answer here is what would let the form grey a source the catalog offered
// or offer one the publish then refuses to open (docs/domain-model.md, "The second-track
// capture sources").
func (av availability) audioReason(source string) *screensharev1.Text {
	_, reason := platform.AudioSourceAvailable(source, av.deps.Platform)
	return reason
}

// audioCodecReason states why the stream cannot be published with an audio codec.
//
// Two independent facts withhold one: the capture backend's publish engine has no
// encoder element for the codec, and the publish transport carries no track in that
// codec's bitstream format. The first is the audio table's own gap, the second the
// carriage table's, and the statement names whichever applies, since the fix differs -
// another capture backend against another codec on the same one.
func (av availability) audioCodecReason(name string) *screensharev1.Text {
	if av.engine == "" {
		return nil
	}
	a, ok := capabilities.GetAudio(name)
	if !ok {
		return nil
	}
	if _, codes := a.EncoderOn(av.engine); !codes {
		// A codec an engine cannot code normally states why as a rule. Where it does not,
		// the fact is still shown, without a cause invented for it.
		if reasons := av.verdicts.ValueReasons(KeyAudioCodec, a.Name); len(reasons) > 0 {
			return reasons[0]
		}
		return say(engineHasNoAudioEncoder, argEngine(av.engine), argAudioCodec(a.Name))
	}
	carriage, ok := transport.PublishCarriage(av.s.Publish.Transport, av.engine)
	if !ok || slices.Contains(carriage.Audio, a.Format) {
		return nil
	}
	// The codecs that would have worked: what this engine codes and this leg carries,
	// which is the list a refusal points at. It is empty on a leg that carries none,
	// and the statement then reads as the shorter fact rather than trailing off.
	return say(legCarriesNoAudioCodec,
		argTransport(av.s.Publish.Transport),
		argAudioCodec(a.Name),
		argEngine(av.engine),
		argAudioCodecs(capabilities.AudioNamesFor(av.engine, carriage.Audio)))
}

// frameMemoryReason states why this capture backend and codec cannot publish through a
// frame memory.
//
// Two facts of the pair decide it, and both come off the pair table: whether the pair
// has a device path at all, and what that path does to the colour. With no row both
// device values are greyed under one statement, since neither has a way onto the device
// to demand. Where the row converts on the device, the value that pays for the path with
// the colour is greyed, having nothing to trade for it; where the row converts nothing,
// the value that demands the colour too is greyed and carries the cost it would have
// paid, leaving the value that accepts that cost as the live one.
//
// Auto and the system copy are never greyed. Auto answers with whichever path costs the
// user nothing and system memory is the path every pair has, so no combination leaves a
// dead control.
//
// Every statement names both ends, because neither decides on its own: the same portal
// capture shares memory with a VAAPI encoder and not with an x264 one, and switching
// either side is a way to reach the path.
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
// One case has it, and it is a fact about where the frames are rather than about the
// size. On a device path the frames never come back to system memory, so the only thing
// that can resize them is the filter already on that path - and a pair whose device path
// carries no conversion at all has none: the encoder reads the captured surfaces
// directly, which is exactly what makes it a colour-trading row. The ffmpeg builder
// refuses such a run, so the entry is greyed with the same fact rather than offered into
// a refusal.
//
// Having such a row is not enough: the run has to be taking it. The statement offers the
// system copy as the way across, so the same fact on a run that already downloads every
// frame would name a fix the user has already applied, and would grey a scale the CPU
// filter on that path can perform. staysOnDevice is what says which of the two a run is
// doing, and it is the same question gpupath.Resolve answers before either chain is
// built - auto included, which takes the round trip on exactly the rows this greys for.
//
// The source size is never greyed. It is what every pair does, and a control whose every
// entry is out is a control that teaches nothing.
func (av availability) outputResolutionReason(value string) *screensharev1.Text {
	if value == "" || !av.staysOnDevice() || !av.path.Colour.TradesColour() {
		return nil
	}
	return say(devicePathHasNoScaler,
		argCapture(av.s.Publish.Capture), argCodec(av.s.Publish.Codec), argMemory(gpupath.MemorySystem))
}

// watchLegReason states why a viewer on this engine cannot receive the stream over a
// transport. Two facts withhold one: the receiver has no form of that protocol at all,
// and the relay does not re-serve this bitstream format on that listener. An SRT viewer
// opened on a VP9 stream connects and receives nothing, since MPEG-TS has no mapping
// for it, which is why the choice is answered per format.
//
// A format no implemented codec produces narrows nothing, matching the roster the
// transport table answers with: hiding a choice on absent information would hide one
// that would have worked.
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

// renderChainReason says why this machine cannot render through a chain, and nil for one
// it can.
//
// It is the one availability rule that asks the machine rather than a table: whether a
// chain runs is whether this GStreamer registers the element factories it is built from,
// which no compiled-in list can answer. The receive package is what asks, and this turns
// its answer into the statement a surface shows - the chain and the first element it
// needs and does not have.
//
// A name no chain carries is not greyed. It is a settings file naming a chain this build
// dropped, which the repair moves off on the same resolve; refusing it here would grey an
// entry no list offers.
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

// watchesOver reports whether anything on this machine receives over the named
// transport: the tile, which is set to one leg, or a player, which can be opened on any
// leg this machine has one for.
//
// It exists because two of the watch knobs belong to the link rather than to a reader:
// the SRT retransmit window and the RTP lower transport are what the connection is
// negotiated with, and a viewer that decodes in a tile negotiates the same connection a
// player would open. Hiding them behind one of the two legs would leave a viewer that
// only uses the other unable to reach a knob its own leg reads.
//
// The two receivers are asked different questions, and that is the difference between
// what each of their settings decides. A tile receives over the leg TileWatchTransport
// names and over no other, so that setting is the whole answer for it. A player is opened
// per press, on whichever leg the reader picked from the ones this machine can open, so
// PlayerWatchTransport decides nothing about which players run - it is the leg a surface
// offers first. Asking it here therefore hid knobs that were in force: a player opened
// over RTSP reads RtspWatchProtocol whatever that setting says, and with both legs on SRT
// the control it reads was not on the screen at all.
//
// A hidden control is the wrong treatment for a value that still reaches a pipeline. The
// choice this page states is between hiding a knob and greying it with a reason, and both
// are answers about a knob that does nothing here; one that does something is shown
// (docs/field-availability.md).
func (av availability) watchesOver(name string) bool {
	return av.s.Viewer.TileWatchTransport == name || transport.CanWatch(name, capabilities.EngineFfmpeg)
}

// knob weighs the three facts that decide a rate-control control: the mode's concept
// uses it, the codec's encoder has it, and the capture backend's engine forwards the
// value. uses carries the first two, already weighed by the caller, with reason the
// statement for them; the engine's own rule is read here.
//
// An engine that forwards a knob the mode marks unused leaves a note instead of a
// greying, so the field states what the value does there rather than feeding the encoder
// a number the form never showed. An engine that drops the knob in every mode outranks
// the mode's own reason: no rate control brings the control back, so naming the mode
// would send the user hunting for a switch that changes nothing.
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

// engineRule finds the rule governing a knob for this engine, codec and mode, and false
// where the builder treats the knob exactly as the mode table says. Earlier rows win, so
// a mode-specific reason precedes a codec-wide one.
func (av availability) engineRule(key string) (availabilityEngineRule, bool) {
	for _, r := range availabilityEngineRules {
		if r.knob != key || (r.engine != "" && r.engine != av.engine) {
			continue
		}
		if len(r.modes) > 0 && !slices.Contains(r.modes, av.s.Publish.Mode) {
			continue
		}
		// A rule naming neither codecs nor families covers every codec the engine
		// builds; one naming either covers the union of the two.
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

// vaapiCeilingNote states the bound the GStreamer VAAPI elements place on a VBR burst
// ceiling, nil where the settings sit inside it. It is a note rather than a greying
// because the knob is forwarded: the field stays live and carries the bound.
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
	return say(vaapiCeilingBound, argMaxrateMbps(av.s.Publish.BitrateM*2), argBitrateTarget(av.s.Publish.BitrateM))
}

// staysOnDevice reports whether this run hands the encoder the frames the capture
// produced, without a trip through system memory.
//
// It reads the row and the value together, the same two facts gpupath.Resolve reads.
// Auto is the value that reads the colour verdict as well: it takes a device path only
// where that path costs nothing, so a pair whose only path leaves the conversion to the
// encoder downloads under auto.
func (av availability) staysOnDevice() bool {
	if !av.onDevicePath || av.s.Publish.CaptureMemory == gpupath.MemorySystem {
		return false
	}
	return !(av.s.Publish.CaptureMemory == gpupath.MemoryAuto && av.path.Colour.TradesColour())
}

// frameMemoryNote states which import carries the frames on the direct path, and on the
// path that converts nothing, what that costs. A path that converts nothing carries its
// cost beside the import, so the field that chose the trade states it too instead of
// leaving it to the two colour fields it overrides.
func (av availability) frameMemoryNote() *screensharev1.Text {
	if !av.staysOnDevice() {
		return nil
	}
	return say(av.path.Import, argCost(wire.GpuPathCost(av.path)))
}

// encoderColourReason states why the colour fields do not reach this stream, nil where
// they do. On the path that converts nothing the encoder reads the captured surface on
// its own terms and signals what it chose, so both fields grey with the row's cost and
// Repair moves each onto what the encoder signals: a greyed field showing something
// other than what the run produces is the one disagreement the form and the publish
// exist to prevent.
func (av availability) encoderColourReason() *screensharev1.Text {
	if !av.onDevicePath || av.s.Publish.CaptureMemory != gpupath.MemoryGpuEncoderColor {
		return nil
	}
	if !av.path.Colour.TradesColour() {
		return nil
	}
	return wire.GpuPathCost(av.path)
}

// decodeNote states what decoding this stream costs a viewer, nil while the codec is
// one the table does not carry.
//
// It is a note and never a block: every format has a software decoder, so the choice is
// between a viewer's GPU and a viewer's cores. Where some hardware decodes the pair the
// statement names those decode families, since which ones they are is the whole point of
// the choice. Where none does, it names the software element and one family whose limit
// stands for the rest: the other families are out for reasons of their own, and listing
// four of them states four times what the first already shows.
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

// The tables the rules read. Every one of them states a fact the Go domain packages do
// not carry: which capture backends take no monitor, what a family's encoders take, and
// how each engine's builder departs from the mode table.

// availabilityKmsgrab is the one capture backend a field is gated on by name: the DRM
// download strategy is a knob of its scanout path and of nothing else.
const availabilityKmsgrab = "kmsgrab"

// The transport names the per-protocol knobs are gated on, spelled as the transport
// registry keys them. A name here the registry does not carry would gate a control on a
// protocol nothing publishes, which is what TestEveryGatedTransportIsRegistered holds.
const (
	availabilitySrt    = "srt"
	availabilityRtsp   = "rtsp"
	availabilityWebrtc = "webrtc"
	availabilityRtmp   = "rtmp"
	availabilityHls    = "hls"
)

// availabilityMonitorless is the capture backends that take no monitor index. What each
// of them captures instead follows from the backend itself, so the statement names the
// backend and the surface says what that one grabs - which is the same sentence its own
// entry in the capture dropdown already has to write.
var availabilityMonitorless = []string{availabilityKmsgrab, "gdigrab", "portal"}

// availabilityFullRangeChromas are the pixel formats that carry no quantization range
// choice at all. RGB is full range by construction, so there is nothing for the colour
// range control to decide under it.
var availabilityFullRangeChromas = []string{"gbrp"}

// availabilityMode is what one rate-control concept needs from the encoder. It is the
// first of the three facts that decide a rate-control field, and it says which controls
// the mode uses rather than what any encoder does with them.
type availabilityMode struct {
	// usesCq: crf targets a constant quantizer.
	usesCq bool
	// usesBitrate: cbr, vbr and abr target a bitrate; crf and lossless do not.
	usesBitrate bool
	// usesMaxrate: vbr sets a burst ceiling above the target.
	usesMaxrate bool
	// usesVbv: cbr and vbr bound the rate with a buffer whose size is tunable.
	usesVbv bool
	// usesBframes: B-frames help the lossy bitrate and quality modes, and only where
	// the family takes a count.
	usesBframes bool
}

// availabilityModes is what each rate-control mode needs, keyed as capabilities names
// the modes. A mode outside the table needs nothing, which is the answer for a
// hand-edited settings file naming one no builder implements.
var availabilityModes = map[string]availabilityMode{
	capabilities.ModeCbr:      {usesBitrate: true, usesVbv: true},
	capabilities.ModeVbr:      {usesBitrate: true, usesMaxrate: true, usesVbv: true, usesBframes: true},
	capabilities.ModeAbr:      {usesBitrate: true, usesBframes: true},
	capabilities.ModeCrf:      {usesCq: true, usesBframes: true},
	capabilities.ModeLossless: {},
}

// availabilityFamily is what an encoder family's encoders take and where they come
// from. It is the second of the three facts a rate-control field is decided by.
type availabilityFamily struct {
	// takesBframes: a field the family has no property for greys whatever the
	// rate-control mode, and the builders pin it off rather than forwarding a value the
	// encoder would ignore. The effort step is not such a field: it follows the codec's
	// own ladder, since one family's codecs can declare different ones.
	takesBframes bool
	// needsDevice: whether the encoders come with a device rather than with a build. An
	// absent encoder is then the machine's answer (no such GPU, or no driver exposing
	// that encode entrypoint) where a software one's is the build's.
	needsDevice bool
}

// availabilityFamilies is one row per encoder family capabilities declares. Every family
// has a row, which TestEveryEncoderFamilyStatesWhatItsEncodersTake holds: a family
// missing here reaches no verdict of its own and would be greyed under the engine's name
// instead.
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

// availabilityFamiliesWith lists the families whose row sets a flag, for a statement
// that names who takes a field instead of stating one family by hand. A control greyed
// for every other family says which ones own it, and the list follows the table when a
// family gains the field.
//
// The order is capabilities.Families' own, so the statement is the same on every call
// rather than the map's iteration order of the moment.
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

// availabilityEngineRule is one place where a publish engine's encoder builder departs
// from the mode table: a knob the mode uses that the engine ignores, or one the engine
// forwards in a mode the table marks unused. It is the third of the three facts.
//
// The mode table says which knobs a rate-control concept needs. Whether the value
// reaches the encoder also depends on which engine builds the command, because the two
// express the same modes through different properties: the GStreamer nvcodec elements
// take no effort step, x264enc cannot raise a ceiling above its bitrate, and vpxenc has
// no unbounded constant-quality mode.
//
// A knob an encoder library has no form of at all is a rule with no engine, since both
// builders hit the same wall. The rules mirror the two builders, encoderArgs in
// ffmpeg/encoders.go and gstEncoder in publish/gstencoders.go.
type availabilityEngineRule struct {
	// engine is empty for a rule that describes the library rather than a builder, and
	// then holds on both.
	engine string
	// knob is the field key the rule governs, so a rule and the control it greys are
	// the same identifier.
	knob string
	// codecs and families select what the rule covers, unioned. Both empty covers every
	// codec the engine builds.
	codecs   []string
	families []string
	// modes selects the rate-control modes the rule covers; empty covers every mode,
	// which is what makes such a rule outrank the mode's own reason.
	modes []string
	// forwards is true where the builder sends the value even though the mode table
	// marks the knob unused, and false where it ignores a value the mode would use.
	forwards bool
	// reason names why an ignored value never reaches the encoder, or what a forwarded
	// one does there. Both are shown to the user, as a code and not a sentence: what
	// each row says is a fact about one named element, so each carries its own.
	reason screensharev1.TextCode
}

// availabilityEngineRules is the departure table, earlier rows winning.
var availabilityEngineRules = []availabilityEngineRule{
	// No rule withholds the effort step on either engine. Every element that codes a
	// laddered codec takes its steps through a property of its own - speed-preset,
	// cpu-used, preset - and the nvcodec elements take the same p1-p7 steps ffmpeg does,
	// their own enum carrying them beside the presets it deprecates.
	//
	// An encoder that cannot bound the burst at all has no VBR mode, which the
	// capability table declares as a mode gap. No rule here withholds the ceiling field
	// for that case: the mode carrying it is gone, so a rule would grey a field under a
	// mode that cannot be selected.
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

// availabilityChromaBlock states that a codec cannot encode a pixel format on either
// engine. Planar RGB gets its own code: RGB reaches an encoder only through HEVC's
// Range Extensions or VP9's identity matrix, which is a coding-tool fact rather than a
// subsampling limit, and a reader told only "cannot encode gbrp" would go looking for a
// setting that would fix it.
func availabilityChromaBlock(chroma, codec string) *screensharev1.Text {
	if chroma == "gbrp" {
		return say(codecCodesNoRGB, argCodec(codec))
	}
	return say(codecCannotEncodeChroma, argCodec(codec), argChroma(chroma))
}

// availabilityExactColourReach states how an encoder family is reached on the device
// with the colour the form shows, nil where the pair table declares no such row.
//
// It is the way out the greyed direct value carries, and the one the dropdown cannot
// show by itself: the fix is another capture backend rather than another value of this
// field. The statement is assembled from the row, so it names whichever pair the table
// declares instead of a platform written in here.
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
// Nothing is refused today: what is inside a kind is what the machine answered, and a
// selection it no longer answers for is kept with a note rather than greyed - an application
// that is not running now is one that may be running when the stream starts (audio.go). The
// row exists because the control offers entries and every such control is answered for here,
// which is what makes a later refusal a line in this file rather than a new mechanism.
func (av availability) audioDeviceReason(string) *screensharev1.Text {
	return nil
}
