package audiodev

import (
	"context"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/platform"
)

// The enumeration is the one part of the audio list touching the machine,
// so these hold it to answering a real one.
// A list built from nothing offers every kind its default and nothing else.

// Every device carries the kind it is inside and the handle a publish opens it by.
// One missing either is an entry a control offers and a pipeline cannot open.
func TestEveryEnumeratedDeviceIsOpenable(t *testing.T) {
	devices := Devices(context.Background())
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

	// A machine that plays sound has a sink, and the desktop kind is that sink's monitor,
	// so it is the one kind every machine running the daemon has something in.
	if kinds[platform.AudioSourceDesktop] == 0 {
		t.Errorf("the daemon reports no sink to record, and it enumerated %v", kinds)
	}
}

// The desktop kind records the monitor of a sink,
// so a handle without that suffix names the sink itself,
// which a publish opens for playback and never records.
func TestADesktopDeviceIsASinksMonitor(t *testing.T) {
	for _, d := range Devices(context.Background()) {
		if d.Kind == platform.AudioSourceDesktop && !strings.HasSuffix(d.ID, monitorSuffix) {
			t.Errorf("%s is offered as desktop audio and is no sink's monitor", d.ID)
		}
	}
}

// A form resolves on every keystroke, so a read is a lock and a copy.
// Two of them with nothing between describe one machine, and in one order:
// a list reshuffled between two resolves would move an entry out from under the cursor.
func TestTwoReadsWithNothingBetweenAgree(t *testing.T) {
	first := Devices(context.Background())
	second := Devices(context.Background())

	if len(first) != len(second) {
		t.Fatalf("two reads answered %d and %d devices", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("device %d differs between reads: %+v and %+v", i, first[i], second[i])
		}
	}
}
