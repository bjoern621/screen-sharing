package app

import (
	"errors"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
)

// What a settings change costs the people watching is decided here, so these run the real decision:
// a change the running child takes must not reach the teardown, and one it does not take must.

// applierHandle is a running pipeline whose child takes values, as the GStreamer engine's handle
// does.
// It records what it was told and whether it was stopped, so a test tells an apply from a relaunch
// by what happened rather than by what was returned.
type applierHandle struct {
	applied []settings.Settings
	fail    error
	stopped bool
}

func (h *applierHandle) Running() bool { return !h.stopped }
func (h *applierHandle) Stop()         { h.stopped = true }

func (h *applierHandle) ApplyLive(s settings.Settings) error {
	if h.fail != nil {
		return h.fail
	}
	h.applied = append(h.applied, s)
	return nil
}

// liveStream is a running publish whose engine takes values while it plays.
func liveStream(t *testing.T) (*App, *applierHandle, settings.Settings) {
	t.Helper()
	s := settings.Defaults()
	s.Publish.Capture = "ximagesrc"
	s.Publish.UseCodec("libx264")
	s.Publish.Mode, s.Publish.Chroma = capabilities.ModeCbr, "yuv420p"
	s.Publish.Effort, s.Publish.Tune = settings.LadderSteps(s.Publish.Codec(), s.Publish.Mode)
	s.Publish.BitrateM = 20

	if fields := publish.LiveFields(s); len(fields) == 0 {
		t.Fatalf("these settings take nothing live, so they cover neither branch")
	}
	handle := &applierHandle{}
	a := &App{run: &publishRun{settings: s, handle: handle}}
	// The relaunch branch starts a real child, so a test that took it leaves an encoder running past
	// the run without this.
	t.Cleanup(func() { endPublish(a) })
	return a, handle, s
}

// endPublish ends whatever a test left running without announcing it: these apps carry no broker,
// and StopPublish's announcement asserts one.
func endPublish(a *App) {
	a.procMu.Lock()
	defer a.procMu.Unlock()
	if a.run != nil {
		a.run.handle.Stop()
		a.run = nil
	}
	a.stopPreviewLocked()
}

// A bitrate edit written to the playing child costs no viewer a reconnect, and the run keeps the
// handle it had.
func TestALiveChangeIsWrittenToTheRunningChild(t *testing.T) {
	a, handle, s := liveStream(t)

	next := s
	next.Publish.BitrateM = 35
	if err := a.restartPublish(next); err != nil {
		t.Fatalf("applying a live change: %v", err)
	}

	if handle.stopped {
		t.Error("a change the running pipeline takes stopped it anyway")
	}
	if len(handle.applied) != 1 || handle.applied[0].Publish.BitrateM != 35 {
		t.Fatalf("the child was told %+v, want one write carrying 35 Mbit/s", handle.applied)
	}
	// The run describes what is publishing, and what is publishing carries the new rate.
	// A run left on the old settings would report the form's value pending forever, since nothing is
	// going to restart onto it.
	if a.run.settings.Publish.BitrateM != 35 {
		t.Errorf("the run still describes %d Mbit/s after the write", a.run.settings.Publish.BitrateM)
	}
	if a.run.handle != handle {
		t.Error("the run was replaced, so the callbacks of the child that is still playing point at nothing")
	}
}

// The frame rate is the pipeline's own shape, so no property write carries it and the change
// relaunches whatever else moved with it.
func TestANonLiveChangeStopsTheChild(t *testing.T) {
	a, handle, s := liveStream(t)

	next := s
	next.Publish.BitrateM, next.Publish.Fps = 35, 30
	// The launch after the teardown runs a real child, so the assertion is on the teardown and not on
	// the return: what matters is that the pipeline carrying the stream is gone.
	_ = a.restartPublish(next)

	if !handle.stopped {
		t.Error("a change no property write carries left the old pipeline running")
	}
	if len(handle.applied) != 0 {
		t.Errorf("a change no property write carries was written to the child: %+v", handle.applied)
	}
}

// A write that failed is a child that cannot be told anything, so the change takes the relaunch.
// Reporting the apply as done would leave the stream on values nobody chose, with the form saying
// otherwise.
func TestAFailedWriteFallsBackToTheRelaunch(t *testing.T) {
	a, handle, s := liveStream(t)
	handle.fail = errors.New("the socket is gone")

	next := s
	next.Publish.BitrateM = 35
	_ = a.restartPublish(next)

	if !handle.stopped {
		t.Error("a write the child refused left the old pipeline running")
	}
}

// The ffmpeg engine's handle takes nothing and says so by not implementing the applier, so a change
// there relaunches even where the same change on the other engine is a write.
func TestAHandleThatTakesNothingRelaunches(t *testing.T) {
	s := settings.Defaults()
	s.Publish.Capture = "x11grab"
	s.Publish.UseCodec("libx264")
	s.Publish.Mode, s.Publish.Chroma = capabilities.ModeCbr, "yuv420p"
	s.Publish.Effort, s.Publish.Tune = settings.LadderSteps(s.Publish.Codec(), s.Publish.Mode)
	s.Publish.BitrateM = 20

	handle := &plainHandle{}
	a := &App{run: &publishRun{settings: s, handle: handle}}
	t.Cleanup(func() { endPublish(a) })

	next := s
	next.Publish.BitrateM = 35
	_ = a.restartPublish(next)

	if !handle.stopped {
		t.Error("a handle whose pipeline takes no values was asked to apply one")
	}
}

// plainHandle takes no values while it plays.
type plainHandle struct{ stopped bool }

func (h *plainHandle) Running() bool { return !h.stopped }
func (h *plainHandle) Stop()         { h.stopped = true }
