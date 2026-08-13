package platform

import (
	"slices"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/text"
)

// The second-track capture sources, and what each platform serves them with.
//
// Which sources exist is the platform's answer.
// What the track is coded in is the publish engine's and the publish leg's, and lives beside the
// audio codec table, which is why the two are separate settings against separate tables
// (docs/ipc-api.md, Catalog.audio_sources).
//
// What each platform serves is read off the two publish engines rather than invented here.
// Both open desktop audio as the monitor of the default sink, ffmpeg through "-f pulse -i"
// (ffmpeg/args.go) and GStreamer through "pulsesrc device=" (publish/gstpipeline.go),
// both passing AudioMonitorDevice, and both refuse it where no PulseAudio or PipeWire server runs.
// A Windows WASAPI loopback or a macOS aggregate device would be another platform on the row;
// neither engine has one, and a table claiming otherwise would grey nothing and fail at launch.
//
// Nothing here touches the machine.
// A source is declared rather than enumerated,
// so a lookup is a table read and a form may resolve on every keystroke.
// An enumeration is cached for the process lifetime and read back separately, the way
// control.Backend.Encoders and CachedEncoders divide the probing read from the resolving one.
const (
	// AudioSourceNone captures no second track.
	AudioSourceNone = "none"
	// AudioSourceDesktop is everything the machine plays: the monitor of the default output,
	// served by PulseAudio or by PipeWire's Pulse server.
	AudioSourceDesktop = "desktop"
	// AudioSourceMic is the default input, served by the same two.
	AudioSourceMic = "mic"
	// AudioSourceApplication is one running program's own output, for a stream carrying the game and
	// not the call about it.
	//
	// PipeWire-native: a program playing sound is a node, and recording it is taking that node's
	// output.
	// Windows needs WASAPI process loopback and macOS a ScreenCaptureKit or CoreAudio tap,
	// and neither is written, so the kind is declared and greyed there rather than left off the list
	// (docs/field-availability.md).
	AudioSourceApplication = "application"
	// AudioMonitorDevice is the handle AudioSourceDesktop opens by: the libpulse magic name for the
	// monitor of the default sink.
	// PipeWire's Pulse server implements the same name, so one string reaches both servers.
	//
	// One constant rather than one spelling per engine, because two spellings of one server's name are
	// two things able to disagree about which device a stream records.
	// The engines differ in how they pass it, "-f pulse -i" against "pulsesrc device=",
	// and not in what they pass.
	AudioMonitorDevice = "@DEFAULT_MONITOR@"
	// AudioInputDevice is the handle AudioSourceMic opens by: the libpulse magic name for the default
	// input.
	// It reaches both servers for the same reason the monitor name does.
	AudioInputDevice = "@DEFAULT_SOURCE@"
)

// AudioSourceDevice is the handle a publish engine opens one kind's own default by,
// empty for a kind with no default to open.
//
// An entry naming a device of its own takes that instead.
// Both engines read this, so they differ in how they pass a device and never in which device that
// is.
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

// AudioDevice is one thing inside a kind: a sound device, or a program whose output is recorded.
//
// Enumerated rather than declared, which is the difference from AudioSource.
// Which kinds exist is a fact about this app and its platforms.
// Which devices sit inside a kind is a fact about this machine at this moment,
// and no table can hold it.
type AudioDevice struct {
	// Kind is the source kind this device sits inside, as AudioSources spells it.
	Kind string `json:"kind"`
	// ID is the handle a publish engine opens it by, which the settings hold and the wire carries.
	// Empty is the kind's own default, which every served kind has and no enumeration reports.
	ID string `json:"id"`
	// Name is what the machine calls it, for a surface that would otherwise show a handle.
	// A description and never an identity: two devices may answer to one name, and the handle is what
	// separates them.
	Name string `json:"name"`
}

// AudioSource is one capture source as one platform answers for it: what the setting holds, whether
// a session of that platform serves it, and what serves it.
//
// A resolved row rather than the declaration behind it, which is audioSourceNeeds.
type AudioSource struct {
	// ID is the value settings.Settings.Audio holds, the string crossing the wire as
	// Catalog.audio_sources, and the key a form's face table names its label by.
	// One identifier on purpose: a face is matched against a value some domain table produced
	// (form/options.go), so a second one would be another pair of spellings able to disagree.
	ID string `json:"id"`
	// Available reports whether a session of the platform this row was resolved for serves the source.
	Available bool `json:"available"`
	// Reason states what that platform is missing, and is nil exactly where Available is true.
	// A statement rather than a sentence: the source and the operating system are the whole of what it
	// is about, so a surface holding both writes it at its own length
	// (api/proto/screenshare/v1/text.proto, docs/field-availability.md).
	Reason *screensharev1.Text `json:"reason"`
	// Server states what serves the source here, nil on a platform that serves it with nothing.
	// It is the note a form puts beside the entry, and it names the same two identifiers Reason does.
	Server *screensharev1.Text `json:"server"`
}

// audioSourceNeed is one row of the declaration: a source and the platforms serving it.
type audioSourceNeed struct {
	// id is the value the settings hold.
	id string
	// platforms are the operating systems serving this source, as Info spells them.
	//
	// A row naming none is a declared kind nothing opens anywhere.
	// The absent source names every platform rather than none, which keeps "asks nothing of the
	// machine" apart from "nothing here serves it".
	platforms []string
}

// audioSourceNeeds is the table, in the order a form presents it, the absent source first: that is
// the order a reader meets the choice in and the value a stream defaults to.
//
// A slice and not a map, because the order is part of the answer.
var audioSourceNeeds = []audioSourceNeed{
	{id: AudioSourceNone, platforms: audioPlatforms},
	{id: AudioSourceDesktop, platforms: []string{"linux"}},
	{id: AudioSourceMic, platforms: []string{"linux"}},
	// Which engine can open a program's own output is a second question, answered where the capture
	// backends are (publish.AudioAvailable).
	{id: AudioSourceApplication, platforms: []string{"linux"}},
}

// The table is one closed set of sources, and every row is answerable for on every known platform.
// A source served nowhere, or a platform refused one with nothing to say about why,
// is a bug in this file rather than a condition to survive, so it fails at load.
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
// Stated rather than derived from the rows, which is the opposite conclusion to
// publish.gatedOperatingSystems because the two tables gate on opposite things.
// A capture backend names the platform it runs on, so an unnamed operating system restricts no
// backend.
// A source names the platform that serves it, so an operating system outside this set would lose
// every source with no sentence to show for it, and is left every one instead.
var audioPlatforms = []string{"windows", "linux", "darwin"}

// AudioSources answers this platform's second-track capture sources: every declared source, in the
// order a form presents them, each carrying whether a session of this operating system serves it,
// why not where it does not, and what serves it where it does.
//
// Every entry is returned rather than only the served ones, because the two consumers want opposite
// halves of one answer.
// A form offers all of them and greys what this machine cannot serve, since a greyed entry with a
// reason teaches what a control quietly one item shorter does not
// (docs/field-availability.md, "The rule").
// A catalog reads the same rows and keeps the available ones.
//
// A pure table read: the same Info yields the same ordered list on every call, and nothing outside
// this file is touched, so it can sit on a resolve path.
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

// AudioSourceIDs is the same list as bare values, in the same order: what a stream's Audio setting
// may hold and what crosses the wire.
//
// It exists so an option builder and a catalog do not each walk the rows and eventually disagree
// about the order.
func AudioSourceIDs(info Info) []string {
	sources := AudioSources(info)
	out := make([]string, 0, len(sources))
	for _, s := range sources {
		out = append(out, s.ID)
	}
	return out
}

// AudioSourceAvailable reports whether a session of this platform serves the named source, and what
// it is missing where it does not.
//
// An unknown source answers true with no reason rather than asserting:
// a stored settings file naming a source this build never heard of is an environment fact,
// the repair walks it back onto a declared value,
// and greying it on the way would state a reason about a source nothing can describe.
func AudioSourceAvailable(id string, info Info) (bool, *screensharev1.Text) {
	for _, n := range audioSourceNeeds {
		if n.id == id {
			return audioAvailable(n, info)
		}
	}
	return true, nil
}

func audioAvailable(n audioSourceNeed, info Info) (bool, *screensharev1.Text) {
	if slices.Contains(n.platforms, info.OS) {
		return true, nil
	}
	if len(n.platforms) > 0 && !slices.Contains(audioPlatforms, info.OS) {
		// An operating system the table never named.
		// A source this app opens somewhere stays offered on it, rather than being taken away under a
		// statement written about somebody else's machine.
		// A row naming no platform is not covered: nothing opens it anywhere, so an unknown machine is
		// not one it might work on.
		return true, nil
	}
	return false, text.Of(screensharev1.TextCode_TEXT_CODE_AUDIO_SOURCE_UNSERVED,
		text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_AUDIO, n.id),
		text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_OS, info.OS))
}

// audioServer states what serves one declared row on one platform,
// nil where that platform serves it with nothing.
//
// The mechanism differs per operating system, a monitor source on one and a loopback device on
// another, so the statement names the source and the platform and leaves the surface to say which
// mechanism that is.
// A sentence written here would describe one machine to users of the other two.
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
// An enumeration sorts what it read into kinds,
// and a kind nothing declares is one no control offers and no publish can open.
func KnownAudioSource(id string) bool {
	for _, n := range audioSourceNeeds {
		if n.id == id {
			return true
		}
	}
	return false
}
