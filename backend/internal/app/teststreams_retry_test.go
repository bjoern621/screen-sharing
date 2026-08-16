package app

import (
	"strconv"
	"strings"
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/text"
	"bjoernblessin.de/screenshare/internal/wire"
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

// TestASlotWaitingOutARelaunchSaysSo: the count says how many publishers are up and nothing about
// which, so a slot that died is visible on the rows alone.
// A waiting slot is one the set still holds, and what it holds instead of a publisher is why the last
// one stopped.
func TestASlotWaitingOutARelaunchSaysSo(t *testing.T) {
	a := &App{testStreams: map[int]*testStream{
		0: {
			attempts: 2,
			cause:    text.Of(screensharev1.TextCode_TEXT_CODE_GROUP_MEMBERSHIP_LAPSED),
			message:  "the relay refused the publisher",
			logPath:  "/logs/teststream-test-1.log",
		},
	}}

	running, slots := a.TestStreamState()
	if running != 0 {
		t.Errorf("a set whose only slot is waiting reports %d publishers alive, want none", running)
	}
	if len(slots) != 1 {
		t.Fatalf("a set holding one slot reports %d rows, want one", len(slots))
	}

	slot := slots[0]
	if slot.Running {
		t.Error("a slot waiting out its relaunch reports a publisher filling it")
	}
	if slot.Name != testStreamName(0) {
		t.Errorf("slot 0 is listed as %q, want the stream it publishes to", slot.Name)
	}
	if slot.Attempt != 3 {
		t.Errorf("a slot behind two relaunches is on attempt %d, want the third", slot.Attempt)
	}
	if got := slot.Cause.GetCode(); got != screensharev1.TextCode_TEXT_CODE_GROUP_MEMBERSHIP_LAPSED {
		t.Errorf("a waiting slot states %v, want what emptied it", got)
	}
	if slot.Message != "the relay refused the publisher" || slot.LogPath != "/logs/teststream-test-1.log" {
		t.Errorf("a waiting slot carries %q and %q, want the last child's own words and its log",
			slot.Message, slot.LogPath)
	}

	// The contract asserts what a slot carries, so a row that broke it panics here rather than on the
	// shell that read it.
	wire.TestStreamState(running, slots...)
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
