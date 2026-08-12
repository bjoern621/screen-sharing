package form

import (
	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/settings"
)

// What is inside one audio kind.
//
// A kind is declared and what is inside it is enumerated, which is why the two are two
// controls: whether a machine serves desktop audio at all is a table's answer, and which
// microphone is plugged in or which application is running is not something a table can
// hold. The declaration is platform.AudioSources; this is the other half.
//
// The enumeration itself is the machine's answer and arrives in Deps, the way the monitors
// and the encoder probe do: a resolve is a pure function of the draft and what the machine
// last answered, so it may run on every keystroke and cannot pay for a probe. What is here
// is the reading of that answer, which is what an option list and a greying both ask.

// audioDevicesOf is what one kind offers on this machine, in the order the enumeration
// reported them, with the kind's own default leading.
//
// The default is an entry rather than an absence: it is what an entry naming no device
// takes, and offering it as the first entry is what lets a reader move back to it after
// picking something else. It is the one entry every served kind has, which is why a kind
// with nothing enumerated still offers something.
func audioDevicesOf(d Deps, kind string) []platform.AudioDevice {
	if kind == "" || kind == platform.AudioSourceNone {
		return nil
	}
	out := []platform.AudioDevice{{Kind: kind}}
	for _, device := range d.AudioDevices {
		if device.Kind == kind {
			out = append(out, device)
		}
	}
	return out
}

// audioDeviceKnown reports whether one selection is among what the kind enumerates now.
//
// The kind's own default always is, since it is what an entry naming no device takes and
// no enumeration has to report it.
func audioDeviceKnown(d Deps, kind, device string) bool {
	if device == "" {
		return true
	}
	for _, known := range audioDevicesOf(d, kind) {
		if known.ID == device {
			return true
		}
	}
	return false
}

// optionAudioDevices offers what is inside one entry's kind: the kind's own default, then
// every device the enumeration reported for it, then the selection itself where the
// enumeration no longer reports one.
//
// The stranded selection is kept for the reason a monitor index no enumeration reported is:
// an application that is not running now is one that may be running when the stream starts,
// and dropping the entry would lose a choice the user made without saying so. It carries the
// note that says which it is, so a reader sees the difference between a device that is here
// and one that was.
func optionAudioDevices(d Deps, _ settings.Settings, a settings.AudioSource) []*screensharev1.FieldOption {
	devices := audioDevicesOf(d, a.Source)
	out := make([]*screensharev1.FieldOption, 0, len(devices)+1)
	for _, device := range devices {
		out = append(out, optionEntry(device.ID, nil, false))
	}
	if !audioDeviceKnown(d, a.Source, a.Device) {
		out = append(out, optionEntry(a.Device,
			say(audioDeviceNotEnumerated, argAudio(a.Source), argDevice(a.Device)), false))
	}
	return out
}

// optionAudioKinds offers where one entry records from. The entry it is drawn for decides
// nothing about the list: which kinds exist is the platform's answer and the same for every
// entry, so this ignores the entry and takes the whole declared table.
func optionAudioKinds(d Deps, s settings.Settings, _ settings.AudioSource) []*screensharev1.FieldOption {
	return optionAudioSources(d, s)
}
