package form

import (
	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/settings"
)

// What is inside one audio kind.
//
// A kind is declared and what is inside it is enumerated, which is what makes them two controls:
// whether a machine serves desktop audio at all is a table's answer, and which microphone is plugged
// in or which application is running is nothing a table can hold.
// platform.AudioSources is the declaration and this is the other half.
//
// The enumeration is the machine's answer and arrives in Deps, as the monitors and the encoder probe
// do.
// A resolve is a pure function of the draft and what the machine last answered, so it runs on every
// keystroke and pays for no probe of its own (internal/audiodev enumerates once and caches for the
// process).
// The reading of that answer is here, which an option list and a greying both ask for.

// audioDevicesOf is what one kind offers on this machine, in the order the enumeration reported,
// led by the kind's own default.
//
// The default is an entry rather than an absence: it is what an entry naming no device takes,
// and leading with it is what lets a reader move back to it after picking something else.
// Every served kind has that one entry, so a kind with nothing enumerated still offers something.
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

// audioDeviceKnown reports whether a selection is among what the kind enumerates.
//
// The kind's own default always is: it is what an entry naming no device takes, and no enumeration
// has to report it.
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

// optionAudioDevices offers what is inside one entry's kind: the kind's own default,
// every device the enumeration reported for it, then the selection itself where the enumeration
// reports no such device.
//
// A stranded selection stays for the reason a monitor index no enumeration reported does:
// an application not running is one that may be running when the stream starts, and dropping the
// entry would take a choice back without saying so.
// It carries the note naming it stranded, which is what separates a device that is here from one
// that was.
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

// optionAudioKinds offers where one entry records from.
// The entry decides nothing about the list: which kinds exist is the platform's answer and the same
// for every entry, so this ignores the entry and takes the declared table whole.
func optionAudioKinds(d Deps, s settings.Settings, _ settings.AudioSource) []*screensharev1.FieldOption {
	return optionAudioSources(d, s)
}
