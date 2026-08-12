package audiodev

import (
	"context"
	"testing"

	"bjoernblessin.de/screenshare/internal/platform"
)

// The enumeration is the one part of the audio list that touches the machine, so what it is
// held to is that it answers a real one: a list built from nothing would offer every kind its
// default and nothing else, which is what the app did before this existed.

// Every device carries the kind it is inside and the handle a publish opens it by. One
// without either is an entry a control can offer and a pipeline cannot open.
func TestEveryEnumeratedDeviceIsOpenable(t *testing.T) {
	devices := Cached(context.Background())
	if len(devices) == 0 {
		t.Skip("no PipeWire daemon on this machine, so there is nothing inside any kind")
	}

	kinds := map[string]int{}
	for _, d := range devices {
		if d.ID == "" {
			t.Errorf("a %s device carries no handle to open it by", d.Kind)
		}
		if d.Name == "" {
			t.Errorf("%s carries no name for a surface to show", d.ID)
		}
		if !platform.KnownAudioSource(d.Kind) {
			t.Errorf("%s is inside %q, which is no declared kind", d.ID, d.Kind)
		}
		kinds[d.Kind]++
	}

	// A machine playing sound has a sink, and the desktop kind is that sink's monitor. It is
	// the one kind every machine with a daemon has something in.
	if kinds[platform.AudioSourceDesktop] == 0 {
		t.Errorf("the daemon reports no sink to record, and it enumerated %v", kinds)
	}
}

// The desktop kind is a sink's monitor and the microphone kind is not, so a monitor listed
// under both would offer one device twice under two names.
func TestAMonitorIsInOneKind(t *testing.T) {
	for _, d := range Cached(context.Background()) {
		if d.Kind == platform.AudioSourceMic && len(d.ID) > 8 && d.ID[len(d.ID)-8:] == monitorSuffix {
			t.Errorf("%s is offered as a microphone and it is a sink's monitor", d.ID)
		}
	}
}

// Reading twice reads once. The enumeration is a subprocess and a form resolves on every
// keystroke, which is the whole reason it is cached rather than taken on demand.
func TestTheEnumerationIsTakenOnce(t *testing.T) {
	first := Cached(context.Background())
	second := Cached(context.Background())

	if len(first) != len(second) {
		t.Fatalf("two reads answered %d and %d devices", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("device %d differs between reads: %+v and %+v", i, first[i], second[i])
		}
	}
}
