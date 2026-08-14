package app

import (
	"errors"
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/events"
	"bjoernblessin.de/screenshare/internal/settings"
)

// A held screen source belongs to the stream rather than to the child that reads it, and what a
// compositor persisting no consent costs is a picker per release.
// These run the real decision on the real state, with the release counted instead of performed.

// countReleases replaces the release with a counter for the length of one test.
func countReleases(t *testing.T) *int {
	t.Helper()

	count := 0
	restore := releaseSources
	releaseSources = func() { count++ }
	t.Cleanup(func() { releaseSources = restore })
	return &count
}

// deadRun is a publish whose child has exited, on settings both engines can render.
func deadRun(t *testing.T, attempts int) (*App, *publishRun) {
	t.Helper()

	s := settings.Defaults()
	s.Publish.Capture = "ximagesrc"
	s.Publish.Codec, s.Publish.Mode, s.Publish.Chroma = "libx264", capabilities.ModeCbr, "yuv420p"
	s.Publish.Effort, s.Publish.Tune = settings.LadderSteps(s.Publish.Codec, s.Publish.Mode)

	run := &publishRun{settings: s, handle: &applierHandle{stopped: true}, startedAt: time.Now(), attempts: attempts}
	a := &App{events: events.New(), settings: s, run: run}
	t.Cleanup(func() {
		a.procMu.Lock()
		defer a.procMu.Unlock()
		a.cancelRetryLocked()
	})
	return a, run
}

// The relaunch is what the source is held for: a relay that refuses the stream must not cost a
// picker per attempt.
func TestARetryKeepsTheScreenSourceHeld(t *testing.T) {
	releases := countReleases(t)
	a, run := deadRun(t, 0)

	a.publishEnded(run, errors.New("the relay refused the publisher"), "", "")

	if a.retry == nil {
		t.Fatal("the exit scheduled no retry, so this covers neither branch")
	}
	if *releases != 0 {
		t.Errorf("the source was released %d times with a relaunch pending", *releases)
	}
}

// The last attempt ends the stream, and a source held past it leaves the compositor sharing a screen
// nobody receives.
func TestAnExhaustedBudgetReleasesTheScreenSource(t *testing.T) {
	releases := countReleases(t)
	a, run := deadRun(t, len(publishBackoff))

	a.publishEnded(run, errors.New("the relay refused the publisher"), "", "")

	if a.retry != nil {
		t.Fatal("a spent budget scheduled another attempt")
	}
	if *releases != 1 {
		t.Errorf("the source was released %d times after the stream ended, want once", *releases)
	}
}

// The user asked for no stream, so the consent goes back whether or not the pipeline was still
// running.
func TestAStopReleasesTheScreenSource(t *testing.T) {
	releases := countReleases(t)
	a, _ := deadRun(t, 0)
	a.run.handle = &applierHandle{}

	a.StopPublish()

	if *releases != 1 {
		t.Errorf("the source was released %d times on a stop, want once", *releases)
	}
}
