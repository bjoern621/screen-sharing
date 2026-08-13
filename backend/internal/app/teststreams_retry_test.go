package app

import (
	"strconv"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/publish"
)

// TestTheLadderIsWalkedOnceAndThenHeld: the synthetic set is always-on, so there is no attempt at
// which it stops being wanted.
// What the ladder buys is a relay that is down for an hour being asked at the last delay rather than
// as fast as a child dies.
func TestTheLadderIsWalkedOnceAndThenHeld(t *testing.T) {
	for attempts := range len(testStreamBackoff) {
		if wait := testStreamWait(attempts); wait != testStreamBackoff[attempts] {
			t.Errorf("wait after attempt %d = %s, want %s", attempts, wait, testStreamBackoff[attempts])
		}
	}

	last := testStreamBackoff[len(testStreamBackoff)-1]
	for _, attempts := range []int{len(testStreamBackoff), len(testStreamBackoff) + 100} {
		if wait := testStreamWait(attempts); wait != last {
			t.Errorf("wait after attempt %d = %s, want the ladder's last delay %s", attempts, wait, last)
		}
	}
}

// TestTheTestStreamBackoffGrows: the usual reason a synthetic publisher dies is the relay not being
// up yet, which takes seconds.
// A flat retry would spend the whole outage relaunching into the same refusal.
func TestTheTestStreamBackoffGrows(t *testing.T) {
	for i := 1; i < len(testStreamBackoff); i++ {
		if testStreamBackoff[i] <= testStreamBackoff[i-1] {
			t.Errorf("backoff[%d] = %s, want more than the %s before it", i, testStreamBackoff[i], testStreamBackoff[i-1])
		}
	}
}

// TestTheBootSetIsTheDefaultUnlessTheEnvironmentNamesACount: the roster is meant to carry streams on
// a machine publishing nothing, and the cost of that is an encoder per slot.
// The environment is where a run says it wants another number of them, or none.
func TestTheBootSetIsTheDefaultUnlessTheEnvironmentNamesACount(t *testing.T) {
	t.Setenv(EnvTestStreams, "")
	if count := testStreamsAtBootWanted(); count != testStreamsAtBoot {
		t.Errorf("boot count with nothing set = %d, want the default %d", count, testStreamsAtBoot)
	}

	t.Setenv(EnvTestStreams, "0")
	if count := testStreamsAtBootWanted(); count != 0 {
		t.Errorf("boot count at 0 = %d, want the set off", count)
	}

	t.Setenv(EnvTestStreams, "1")
	if count := testStreamsAtBootWanted(); count != 1 {
		t.Errorf("boot count at 1 = %d, want the count that was asked for", count)
	}
}

// TestACountOutsideTheBoundTakesTheDefault: this is a development knob, so a typo in it leaves the
// app running the set it would have run anyway rather than refusing to start or saturating the
// machine with encoders it did not mean to ask for.
func TestACountOutsideTheBoundTakesTheDefault(t *testing.T) {
	for _, set := range []string{"three", "-1", strconv.Itoa(maxTestStreams + 1)} {
		t.Setenv(EnvTestStreams, set)
		if count := testStreamsAtBootWanted(); count != testStreamsAtBoot {
			t.Errorf("boot count at %q = %d, want the default %d", set, count, testStreamsAtBoot)
		}
	}
}

// TestASlotNamesTheStreamItPublishes: the slot is the stream's identity, so a relaunch has to come
// back on the row the roster already shows rather than beside it.
//
// The slot number leads and the surface's own label follows it where it has one: two slots arriving
// at one name would be one row of the roster and two publishers pushing to it.
func TestASlotNamesTheStreamItPublishes(t *testing.T) {
	seen := map[string]bool{}
	for slot := range maxTestStreams {
		name := testStreamName(slot)

		if number := "test-" + strconv.Itoa(slot+1); !strings.HasPrefix(name, number) {
			t.Errorf("slot %d is named %q, which does not lead with %q", slot, name, number)
		}
		if label := publish.TestSurfaceOf(slot).Label; label != "" && !strings.Contains(name, label) {
			t.Errorf("slot %d draws a surface labelled %q and reaches the roster as %q", slot, label, name)
		}
		if seen[name] {
			t.Errorf("slot %d publishes under %q, which another slot already holds", slot, name)
		}
		seen[name] = true
	}
}
