package settings

import (
	"bjoernblessin.de/screenshare/internal/platform"
)

// The list a stream's second track is mixed from.
//
// A list rather than one source, a screen share normally carrying several at once: what the machine
// is playing, and whoever is talking over it.
// They mix into one track, which is carriage rather than preference.
// RTMP carries one audio track and the relay re-serves every ingest on all of its listeners, so a
// two-track stream would be unplayable on the narrowest leg while the form said it published
// (docs/domain-model.md).

// GainUnity is the gain at which a source contributes what it produces, in the percent the setting
// counts in.
//
// Percent rather than decibels, that being what a control writes and what a mixer takes: the
// element's volume property is a linear multiplier, and a decibel figure would be converted at
// every consumer, each conversion free to round differently.
const GainUnity = 100

// GainMax is the loudest a source may be set to.
// Above unity is amplification, worth having for a quiet microphone and worth bounding: an
// unbounded multiplier clips every other source out of the mix.
const GainMax = 200

// AudioSource is one thing the second track is mixed from: which kind, which device or application
// inside that kind, how loud, and whether it is silenced.
type AudioSource struct {
	// Source is the kind, a row of platform.AudioSources, which declares the values and which platform
	// serves each.
	Source string `json:"source"`
	// Device is which device or application inside the kind, as that kind's enumeration names it,
	// empty for the kind's own default.
	//
	// Two fields because a kind is declared and what is inside it is enumerated: whether a machine
	// serves desktop audio at all is a table's answer, and which microphone is plugged in is not
	// something a table can hold.
	Device string `json:"device"`
	// Gain is what this source contributes, in percent, GainUnity being unity.
	Gain int `json:"gain"`
	// Mute silences the source without taking it off the list, so turning one off and back on loses
	// neither the entry, nor its device, nor its gain.
	Mute bool `json:"mute"`
}

// Records reports whether this entry produces a branch, which every entry naming a kind other than
// the absent one does.
//
// The absent kind is a real value of the source control and no hole: it is what the row a form
// draws past the end of the list holds, and what an entry a reader turns off becomes.
// Both are entries the settings carry and neither is a source.
func (a AudioSource) Records() bool {
	return a.Source != "" && a.Source != platform.AudioSourceNone
}

// Volume is the multiplier a mixer applies to this source: what the gain says, zero where it is
// muted.
//
// Mute wins over the gain without replacing it, which is what lets a source be silenced and brought
// back at the level it had.
// One function, both engines asking the same question of one entry: a second reading of "muted
// means zero" is one that can disagree about whether a muted source at 150 percent is loud.
//
// A gain outside the settable range is no assert.
// The value arrives from a file the user owns, so it is repaired where settings are repaired.
func (a AudioSource) Volume() float64 {
	if a.Mute {
		return 0
	}
	return float64(a.Gain) / GainUnity
}

// DefaultAudioSource is what a fresh entry holds: no kind, the kind's own device, unity gain,
// unmuted.
//
// It is what the row past the end of the list holds, so picking a kind on that row is the whole of
// adding a source.
// That is the only way the list grows, and it takes no effect on the contract: the row is an
// ordinary settings write through an ordinary control.
func DefaultAudioSource() AudioSource {
	return AudioSource{Source: platform.AudioSourceNone, Gain: GainUnity}
}

// Recording is a list recording one kind at unity gain, unmuted, from that kind's own device.
//
// A stream naming one source is this, which is what a settings file written before the list becomes
// and what a caller holding one kind means.
// Building the entry per site would be the gain and the mute spelled per site, free to differ.
//
// The kind is not asserted: the absent kind and the empty string are both legal, a file written
// before the option naming neither, and an entry naming no source is the one Records reports false
// for rather than a broken contract.
func Recording(kind string) []AudioSource {
	return []AudioSource{{Source: kind, Gain: GainUnity}}
}
