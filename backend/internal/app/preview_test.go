package app

import (
	"net"
	"testing"

	"bjoernblessin.de/screenshare/internal/events"
	"bjoernblessin.de/screenshare/internal/settings"
)

// previewApp needs no relay, no child and no control socket: what is under test is the pipeline the
// publish brings up inside this process.
func previewApp() *App {
	return &App{events: events.New(), settings: settings.Defaults()}
}

// previewSettings names a codec with a local carriage; without one there is no preview to bring up.
func previewSettings() settings.Settings {
	s := settings.Defaults()
	s.Publish.Name = "preview-lifecycle-test"
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

	first := a.startPreviewLocked(s)
	if !first.Wanted() {
		t.Skip("no local preview came up on this machine, so there is no lifecycle to hold")
	}
	running := a.preview

	second := a.startPreviewLocked(s)
	if second != first {
		t.Errorf("a second start moved the port from %d to %d", first.Port, second.Port)
	}
	if a.preview != running {
		t.Error("a second start replaced the pipeline the first one built")
	}

	a.stopPreviewLocked()
	if a.preview != nil {
		t.Error("a stopped preview is still reported as running")
	}
	// A stop on a preview that is not running names a state that already holds, the contract
	// StopReceive keeps.
	a.stopPreviewLocked()
}

// A port left bound after a stop is one the next launch cannot be given.
func TestTheLocalPreviewHoldsThePortItReportsAndReleasesIt(t *testing.T) {
	a := previewApp()
	s := previewSettings()

	a.procMu.Lock()
	leg := a.startPreviewLocked(s)
	a.procMu.Unlock()

	if !leg.Wanted() {
		t.Skip("no local preview came up on this machine, so there is no port to hold")
	}

	snapshot := func() (port int, running bool) {
		a.procMu.Lock()
		defer a.procMu.Unlock()

		preview := a.previewSnapshotLocked()
		if preview == nil {
			return 0, false
		}
		return preview.Port, true
	}

	port, running := snapshot()
	if !running || port != leg.Port {
		t.Fatalf("the state reports port %d running=%v, and the child was told %d", port, running, leg.Port)
	}

	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: leg.Port}
	if conn, err := net.ListenUDP("udp", addr); err == nil {
		conn.Close()
		t.Errorf("port %d is free while the preview is supposed to be receiving on it", leg.Port)
	}

	a.procMu.Lock()
	a.stopPreviewLocked()
	a.procMu.Unlock()

	if _, running := snapshot(); running {
		t.Error("the state reports a preview after it was stopped")
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Errorf("port %d is still held after the preview stopped: %v", leg.Port, err)
		return
	}
	conn.Close()
}
