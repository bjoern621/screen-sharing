package app

import (
	"strconv"
	"testing"
)

// TestTheLadderIsWalkedOnceAndThenHeld: the synthetic set is always-on, so there is no attempt at
// which it stops being wanted.
// What the ladder buys is that a relay which is down for an hour is asked once every thirty seconds
// rather than a hundred times in the first minute.
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

// TestTheBackoffGrows: the usual reason a synthetic publisher dies is the relay not being up yet,
// which takes seconds.
// A flat retry would spend the whole outage relaunching into the same refusal.
func TestTheTestStreamBackoffGrows(t *testing.T) {
	for i := 1; i < len(testStreamBackoff); i++ {
		if testStreamBackoff[i] <= testStreamBackoff[i-1] {
			t.Errorf("backoff[%d] = %s, want more than the %s before it", i, testStreamBackoff[i], testStreamBackoff[i-1])
		}
	}
}

// TestTheBootSetIsTheDefaultUnlessTheEnvironmentNamesACount: the roster is meant to carry streams
// on a machine publishing nothing, and the cost of that is three encoders.
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
// machine with nine encoders it did not mean to ask for.
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
func TestASlotNamesTheStreamItPublishes(t *testing.T) {
	if name := testStreamName(0); name != "test-1" {
		t.Errorf("slot 0 is named %q, want test-1", name)
	}
	if name := testStreamName(2); name != "test-3" {
		t.Errorf("slot 2 is named %q, want test-3", name)
	}
}
