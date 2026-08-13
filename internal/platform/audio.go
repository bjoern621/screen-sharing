package platform

import (
	"slices"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/text"
)

// The second-track capture sources, and what each platform serves them with.
//
// Which sources a machine offers is the platform's answer, not the encoder's and not a form's:
// what the track is coded in is decided by the publish engine and the publish leg,
// and lives beside the audio codec table.
// That split is why the two are separate settings against separate tables,
// and why this list lives here rather than there (docs/ipc-api.md, Catalog.audio_sources).
//
// It was the frontend that knew "none" and "desktop", because someone typed them there,
// and moving two strings into Go did not make the domain own the question:
// a list that answers the same on every operating system is not a platform's answer,
// it is a constant wearing one's name.
// This is the table that does own it, in the shape publish/availability.go gives the capture
// backends and for the same reason - a source a machine cannot serve and a source no engine can
// open are the same question asked twice, and splitting them leaves one of them to be restated by
// whoever asks second.
//
// What each platform serves is read off the two publish engines rather than invented here.
// Both open desktop audio as the monitor of the default sink - ffmpeg through "-f pulse -i"
// (ffmpeg/args.go) and GStreamer through "pulsesrc device=" (publish/ gstpipeline.go),
// each passing AudioMonitorDevice below - and both refuse it anywhere a PulseAudio or PipeWire
// server does not run.
// So Linux is the one platform that serves it, and this table says so instead of offering every
// machine a source two of them have no code to open.
// A Windows WASAPI loopback or a macOS aggregate device would be another entry in Server and
// another platform in Platforms; neither engine has one, and a table that claimed otherwise would
// grey nothing and fail at launch.
//
// Nothing here touches the machine.
// A source is declared, not enumerated, so the lookup is a table read and a form may resolve on
// every keystroke without paying for one.
// The day a probe enumerates devices it is cached for the process lifetime and read back
// separately, the way control.Backend.Encoders and CachedEncoders divide the probing read from the
// resolving one; it does not belong inside this function, which is called from a resolve.
const (
	// AudioSourceNone is the value of a stream with no second track.
	AudioSourceNone = "none"
	// AudioSourceDesktop is everything the machine plays: the monitor of the default output,
	// served by PulseAudio or by PipeWire's Pulse server.
	AudioSourceDesktop = "desktop"
	// AudioSourceMic is whoever is talking: the default input, served by the same two.
	AudioSourceMic = "mic"
	// AudioSourceApplication is one running program's own output, which is what a stream carrying a
	// game and not the call about it needs.
	//
	// It is PipeWire-native: an application playing sound is a node, and recording it is
	// taking that node's output. Linux serves it for that reason, and the other two do not
	// - Windows needs WASAPI process loopback and macOS a ScreenCaptureKit or CoreAudio
	// tap, and neither is written - so the kind is declared and greyed there rather than
	// left off the list (docs/field-availability.md).
	AudioSourceApplication = "application"
	// AudioMonitorDevice is the handle a publish engine opens AudioSourceDesktop by:
	// the libpulse magic name for the monitor of the default sink.
	// PipeWire's Pulse server implements the same name, so one string reaches both servers.
	//
	// It is the machine-facing half of what Server says in prose, and it lives here for the reason the
	// whole table does: it was spelled once in ffmpeg/args.go and once in publish/gstpipeline.go,
	// and two spellings of one server's name are two things able to disagree about which device a
	// stream records.
	// The engines differ in how they pass it - "-f pulse -i" against "pulsesrc device=" - and not in
	// what they pass, so what is shared is exactly this constant and no more.
	//
	// A constant rather than a column of the table because there is one source served by one server,
	// and a per-platform handle would be structure standing in for an entry no row has.
	// A platform that served desktop audio through something else - a WASAPI loopback,
	// a macOS aggregate device - arrives as a Platforms entry with a Server sentence,
	// and that is the change that turns this into a column, made where the rest of the row already is.
	AudioMonitorDevice = "@DEFAULT_MONITOR@"
	// AudioInputDevice is the handle AudioSourceMic opens by: the libpulse magic name for the default
	// input.
	// The counterpart of the monitor name above, and it reaches both servers for the same reason.
	AudioInputDevice = "@DEFAULT_SOURCE@"
)

// AudioSourceDevice is the handle a publish engine opens one kind's own default by,
// and the empty string for a kind with no default to open.
//
// It is the machine-facing half of the table: an entry naming no device of its own takes the kind's
// default, which is what this answers, and an entry naming one takes that instead.
// Both engines read it, so the two differ in how they pass a device - "-f pulse -i" against
// "pulsesrc device=" - and never in which device that is.
func AudioSourceDevice(id string) string {
	switch id {
	case AudioSourceDesktop:
		return AudioMonitorDevice
	case AudioSourceMic:
		return AudioInputDevice
	default:
		return ""
	}
}

// AudioDevice is one thing inside a kind: a sound device, or an application whose own output is
// being recorded.
//
// It is enumerated rather than declared, which is the whole difference from AudioSource above.
// Which kinds exist is a fact about this app and its platforms, the same on every machine of one
// operating system; which devices are inside a kind is a fact about this machine at this moment,
// and no table can hold it.
type AudioDevice struct {
	// Kind is the source kind this device is inside, as AudioSources names them.
	Kind string `json:"kind"`
	// ID is the handle a publish engine opens it by, which is what the settings hold and what crosses
	// the wire.
	// Empty is the kind's own default, which every served kind has and no enumeration has to report.
	ID string `json:"id"`
	// Name is what the machine calls it, for a surface that would otherwise show a handle.
	// It is a description and never an identity: two devices may answer to one name,
	// and the handle is what separates them.
	Name string `json:"name"`
}

// AudioSource is one second-track capture source as one platform answers for it:
// what the setting holds, whether a session of that platform serves it, and what serves it.
//
// It is a resolved row rather than the declaration behind it, because every consumer asks the same
// two questions of one platform and none of them asks about a machine it is not describing.
// The declaration is audioSourceNeeds below.
type AudioSource struct {
	// ID is the value settings.Settings.Audio holds, the string that crosses the wire as
	// Catalog.audio_sources, and the key a form's face table names its label by.
	// It is the label key as well as the value on purpose: a face is matched against a value some
	// domain table produced (form/options.go), so a second identifier would be one more pair of
	// spellings able to disagree.
	ID string `json:"id"`
	// Available reports whether a session of the platform this row was resolved for serves the source.
	Available bool `json:"available"`
	// Reason states why that platform serves no such source, and is nil exactly where Available is
	// true.
	// It is a statement rather than a sentence: the source and the operating system are the whole of
	// what the reason is about, so a surface holding both writes what that machine is missing at its
	// own length (api/proto/screenshare/v1/text.proto, docs/field-availability.md).
	Reason *screensharev1.Text `json:"reason"`
	// Server states what serves the source here, and is nil on a platform that serves it with nothing.
	// It is the note a form puts beside the entry, and it names the same two identifiers Reason does,
	// because what serves a source and what is missing where nothing does are one fact read from
	// either side.
	Server *screensharev1.Text `json:"server"`
}

// audioSourceNeed is one row of the declaration: a source, the platforms serving it,
// what serves it on each, and why the others do not.
type audioSourceNeed struct {
	// id is the source's settings value.
	id string
	// platforms are the operating systems whose sessions serve this source, as Info spells them.
	//
	// A row naming none is served nowhere, which is a kind this app declares and has no code to open
	// on any machine.
	// The absent source names every platform rather than none, which is what keeps the two statements
	// apart: "asks nothing of the machine" and "nothing here serves it" would otherwise be written the
	// same way.
	platforms []string
}

// audioSourceNeeds is the table, in the order a form presents it, with the absent source first:
// a list of capture sources whose first entry is "capture nothing" is the order a reader meets the
// choice in, and it is the value a stream defaults to.
//
// A slice and not a map because that order is part of the answer, and a map has none.
var audioSourceNeeds = []audioSourceNeed{
	{id: AudioSourceNone, platforms: audioPlatforms},
	{id: AudioSourceDesktop, platforms: []string{"linux"}},
	{id: AudioSourceMic, platforms: []string{"linux"}},
	// One application's own output is a PipeWire node, which is why Linux serves it and the other two
	// do not: PulseAudio has no way to record one program's stream, Windows needs WASAPI process
	// loopback and macOS a ScreenCaptureKit or CoreAudio tap, and neither is written.
	// Which engine can open it is a second question, answered where the backends are
	// (publish.AudioAvailable).
	{id: AudioSourceApplication, platforms: []string{"linux"}},
}

// The table describes one closed set of sources, and every row of it has to be answerable for on
// every platform this app knows.
// A source served nowhere, a platform that serves it with nothing, or a platform that does not
// serve it and says nothing about why are all bugs in this file rather than conditions to survive,
// so they fail at load.
func init() {
	assert.Assert(len(audioSourceNeeds) > 0, "the audio source table offers something")
	assert.Assert(audioSourceNeeds[0].id == AudioSourceNone,
		"the absent source leads the audio source table", audioSourceNeeds[0].id)

	seen := map[string]bool{}
	for _, n := range audioSourceNeeds {
		assert.Assert(n.id != "", "an audio source has a value the settings can hold")
		assert.Assert(!seen[n.id], "an audio source is declared once", n.id)
		seen[n.id] = true

		for _, p := range n.platforms {
			assert.Assert(slices.Contains(audioPlatforms, p),
				"a platform serving an audio source is one this table answers for", n.id, p)
		}
	}
}

// audioPlatforms are the operating systems this table answers for.
//
// It is the same reasoning publish.gatedOperatingSystems is built on and the opposite conclusion,
// because the two tables gate on opposite things.
// A capture backend names the platform it runs on, so an operating system no backend names is one
// nothing here has anything true to say about and every backend stays offered on it.
// A source names the platform that serves it, so an operating system outside this set would take
// every source away with no sentence to show for it - which is why the set is stated rather than
// derived, and why a source is available on an unknown platform unless the table says otherwise.
var audioPlatforms = []string{"windows", "linux", "darwin"}

// AudioSources answers what this platform's second-track capture sources are:
// every source the table declares, in the order a form presents them, each carrying whether a
// session of this operating system serves it, why not where it does not, and what serves it where
// it does.
//
// Every entry is returned rather than only the served ones, because the two consumers want opposite
// halves of the same answer and both are it.
// A form offers all of them and greys the ones this machine cannot serve, since a general concept
// the current machine blocks is taught by a greyed entry and its reason rather than by a control
// that is quietly one item shorter (docs/field-availability.md, "The rule").
// A catalog naming what the machine offers reads the same rows and keeps the available ones.
// Neither restates the other's rule, and there is one place either could be changed.
//
// It is a pure table read: the same Info yields the same ordered list on every call,
// it touches nothing outside this file, and it is cheap enough to sit on a resolve path.
func AudioSources(info Info) []AudioSource {
	out := make([]AudioSource, 0, len(audioSourceNeeds))
	for _, n := range audioSourceNeeds {
		available, reason := audioAvailable(n, info)
		assert.Assert(available == (reason == nil),
			"an unserved audio source says what the platform is missing", n.id, info.OS)

		out = append(out, AudioSource{
			ID:        n.id,
			Available: available,
			Reason:    reason,
			Server:    audioServer(n, info),
		})
	}

	assert.Assert(len(out) == len(audioSourceNeeds),
		"a platform is answered for on every declared source", len(out), len(audioSourceNeeds))
	assert.Assert(out[0].ID == AudioSourceNone && out[0].Available,
		"the absent source is offered on every platform", info.OS)
	return out
}

// AudioSourceIDs is the same list as the bare values, in the same order: what a stream's Audio
// setting may hold and what crosses the wire.
//
// It exists so a caller that only needs the values does not walk the rows itself,
// which is what an option builder and a catalog both do and what two loops would eventually
// disagree about the order of.
func AudioSourceIDs(info Info) []string {
	sources := AudioSources(info)
	out := make([]string, 0, len(sources))
	for _, s := range sources {
		out = append(out, s.ID)
	}
	return out
}

// AudioSourceAvailable reports whether a session of this platform serves the named source,
// and the sentence saying what it is missing where it does not.
//
// An unknown source is a caller that made one up rather than a condition to survive:
// every name reaching here comes off this same table, off settings a repair walked onto it,
// or off a draft the form is about to move.
// It answers true with no reason instead of asserting, because a stored settings file naming a
// source this build has never heard of is an environment fact - the repair walks it back onto a
// declared value, and greying it on the way would state a reason about a source nothing can
// describe.
func AudioSourceAvailable(id string, info Info) (bool, *screensharev1.Text) {
	for _, n := range audioSourceNeeds {
		if n.id == id {
			return audioAvailable(n, info)
		}
	}
	return true, nil
}

// audioAvailable is the verdict for one declared row on one platform.
func audioAvailable(n audioSourceNeed, info Info) (bool, *screensharev1.Text) {
	if slices.Contains(n.platforms, info.OS) {
		return true, nil
	}
	if len(n.platforms) > 0 && !slices.Contains(audioPlatforms, info.OS) {
		// An operating system the table never named.
		// The declared platforms cover every machine this app knows, so reaching here means one outside
		// that set, and a source this app does open somewhere is left offered on it rather than taken
		// away under a statement written about somebody else's machine.
		// A row naming no platform at all is not covered by that: nothing opens it anywhere,
		// so an unknown machine is not a machine it might work on.
		return true, nil
	}
	return false, text.Of(screensharev1.TextCode_TEXT_CODE_AUDIO_SOURCE_UNSERVED,
		text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_AUDIO, n.id),
		text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_OS, info.OS))
}

// audioServer states what serves one declared row on one platform, and nil where that platform
// serves it with nothing.
//
// The mechanism differs per operating system - a monitor source on one, a loopback device on
// another - which is why the statement names both the source and the platform and lets the surface
// say which mechanism that is.
// Stating it here as a sentence would have described one machine to users of the other two.
func audioServer(n audioSourceNeed, info Info) *screensharev1.Text {
	if n.id == AudioSourceNone || !slices.Contains(n.platforms, info.OS) {
		return nil
	}
	return text.Of(screensharev1.TextCode_TEXT_CODE_AUDIO_SOURCE_SERVER,
		text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_AUDIO, n.id),
		text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_OS, info.OS))
}

// KnownAudioSource reports whether a value names one of the declared kinds.
//
// It is the same question Known asks of a pointer mode and exists for the same reason:
// an enumeration sorts what it read into kinds, and a kind nothing declares is one no control
// offers and no publish can open.
func KnownAudioSource(id string) bool {
	for _, n := range audioSourceNeeds {
		if n.id == id {
			return true
		}
	}
	return false
}
