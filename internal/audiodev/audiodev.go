// Package audiodev enumerates what is inside each audio capture kind on this machine.
//
// Which kinds exist is declared (platform.AudioSources) and the same on every machine of one
// operating system. What is inside a kind is not: which microphone is plugged in and which
// application is playing are facts about this machine at this moment, so they are read off it
// rather than listed anywhere.
//
// The read is separated from the resolve for the reason the encoder probe's is. A form
// resolves on every keystroke and a resolve is a pure function of the draft and what the
// machine last answered, so it cannot pay for a subprocess; the answer is taken once, cached
// for the process lifetime, and read back through Cached (docs/development-principles.md).
//
// PulseAudio is what answers, through pactl, which is what both publish engines record
// through as well: ffmpeg opens "-f pulse" and GStreamer a pulsesrc, and PipeWire's Pulse
// server implements the same interface, so one enumeration describes what either engine can
// open on either server. A machine with no such server enumerates nothing and every kind
// keeps its own default, which is the entry that needs no enumeration.
package audiodev

import (
	"context"
	"os/exec"
	"strings"
	"sync"

	"bjoernblessin.de/screenshare/internal/platform"
)

// pactlExe is the tool the enumeration reads, and pactlListArgs what it is asked for: the
// short form, which is one device per line and stable across versions where the long form is
// a paragraph per device written for a reader.
const pactlExe = "pactl"

var pactlListArgs = []string{"list", "short", "sources"}

// monitorSuffix is what PulseAudio names the monitor of a sink by. It is how a source that
// records what the machine plays is told from one that records what a device hears, which is
// the same division the kinds make.
const monitorSuffix = ".monitor"

// cached is the enumeration this process took, and once guards taking it. A sync.Once rather
// than a nil check because two surfaces may resolve at the same moment, and the answer is one
// machine's rather than one caller's.
var (
	once   sync.Once
	cached []platform.AudioDevice
)

// Cached is what this machine offers inside each kind, enumerated on the first call and
// answered from memory afterwards.
//
// Every caller gets the same slice, which callers read and do not write. A machine whose
// devices changed since the first call is answered with what was there then; the enumeration
// following PipeWire's own add and remove events is what replaces that, and it is worth its
// own mechanism rather than a shorter cache - the application just launched is the one worth
// selecting, and that is the case a cache gets wrong every time.
func Cached(ctx context.Context) []platform.AudioDevice {
	once.Do(func() { cached = enumerate(ctx) })
	return cached
}

// enumerate reads the sources the sound server offers and sorts them into kinds.
//
// A monitor of a sink is what the machine plays, so it is a device of the desktop kind;
// everything else is something the machine hears, which is the microphone kind. That is the
// same division the kinds themselves make, read off the one place that knows which is which.
//
// A server that is not running, a tool that is not installed and a line that does not parse
// all yield nothing rather than an error. The absence of an enumeration is not a failure of
// the app: every kind keeps its own default, which is what an entry naming no device takes,
// and that default is what every build before this enumeration recorded through.
func enumerate(ctx context.Context) []platform.AudioDevice {
	out, err := exec.CommandContext(ctx, pactlExe, pactlListArgs...).Output()
	if err != nil {
		return nil
	}

	var devices []platform.AudioDevice
	for _, line := range strings.Split(string(out), "\n") {
		// index, name, driver, sample spec, state - tab separated, and the name is the
		// handle both engines open the device by.
		fields := strings.Split(strings.TrimSpace(line), "\t")
		if len(fields) < 2 || fields[1] == "" {
			continue
		}
		name := fields[1]
		kind := platform.AudioSourceMic
		if strings.HasSuffix(name, monitorSuffix) {
			kind = platform.AudioSourceDesktop
		}
		devices = append(devices, platform.AudioDevice{Kind: kind, ID: name, Name: name})
	}
	return devices
}
