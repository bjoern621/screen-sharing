package app

import (
	"testing"

	"bjoernblessin.de/screenshare/internal/events"
	"bjoernblessin.de/screenshare/internal/settings"
)

// What a test can hold the preview to, now that the pipeline itself runs in the decode host
// (internal/decode).
// Bringing one up takes a host child, and a host child is this executable spawned again, which under
// a test binary is that binary running its own tests.
// So what is asserted here is the part that answers before anything is opened: the guard against
// a second launch, and a stop that names a state already holding.

// previewApp needs no relay, no child and no control socket.
// The decode client is left nil, which every path below returns before reaching.
func previewApp() *App {
	return &App{events: events.New(), settings: settings.Defaults()}
}

// previewSettings names a codec with a local carriage; without one there is no preview to bring up.
func previewSettings() settings.Settings {
	s := settings.Defaults()
	s.Publish.UseCodec("libx264")
	return s
}

// A launch brings the preview up, and two launches can race: a retry against a manual start.
// Without the guard the second binds a second port for a child that is told about one.
func TestBringingTheLocalPreviewUpTwiceChangesNothing(t *testing.T) {
	a := previewApp()
	s := previewSettings()

	a.procMu.Lock()
	defer a.procMu.Unlock()

	// The state a second launch meets, written rather than opened:
	// what the guard answers from is the field, not the pipeline behind it.
	running := &previewRun{port: 5004}
	a.preview = running

	second := a.startPreviewLocked(s)
	if second.Port != running.port {
		t.Errorf("a second start moved the port from %d to %d", running.port, second.Port)
	}
	if a.preview != running {
		t.Error("a second start replaced the pipeline the first one built")
	}
}

// A stop on a preview that is not running names a state that already holds,
// the contract StopReceive keeps.
func TestStoppingALocalPreviewThatIsNotRunning(t *testing.T) {
	a := previewApp()

	a.procMu.Lock()
	defer a.procMu.Unlock()

	a.stopPreviewLocked()
	if a.preview != nil {
		t.Error("stopping when nothing ran left a preview behind")
	}
	a.stopPreviewLocked()
}
