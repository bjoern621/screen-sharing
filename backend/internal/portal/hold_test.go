package portal

import "testing"

// A session with no client behind it: Close finds nothing to release,
// which is what lets a hold be exercised without a compositor.
func heldSessions(t *testing.T) (*Hold, *int) {
	t.Helper()

	opened := 0
	hold := &Hold{open: func(Options) (*Session, error) {
		opened++
		return &Session{NodeID: uint32(opened)}, nil
	}}
	return hold, &opened
}

// The whole point of the hold: a relaunch reuses the consent the first launch took,
// so a compositor that persists none pops its picker once per stream rather than once per child.
func TestASecondCaptureReusesTheHeldSession(t *testing.T) {
	hold, opened := heldSessions(t)
	opts := Options{Types: SourceMonitor | SourceWindow, Cursor: CursorEmbedded}

	first, err := hold.Session(opts)
	if err != nil {
		t.Fatalf("first Session: %v", err)
	}
	second, err := hold.Session(opts)
	if err != nil {
		t.Fatalf("second Session: %v", err)
	}

	if first != second {
		t.Errorf("the second capture opened a session of its own: %v against %v", second, first)
	}
	if *opened != 1 {
		t.Errorf("the compositor was asked %d times for one stream", *opened)
	}
}

// The restore token moves under a held session:
// the store is rewritten after every capture,
// and a consent already granted is not reopened over what the next one would start from.
func TestAMovedRestoreTokenReusesTheHeldSession(t *testing.T) {
	hold, opened := heldSessions(t)

	if _, err := hold.Session(Options{Types: SourceMonitor, RestoreToken: "consent-1"}); err != nil {
		t.Fatalf("first Session: %v", err)
	}
	if _, err := hold.Session(Options{Types: SourceMonitor, RestoreToken: "consent-2"}); err != nil {
		t.Fatalf("second Session: %v", err)
	}

	if *opened != 1 {
		t.Errorf("a moved restore token reopened the session, %d opens", *opened)
	}
}

// The defaults Open fills are not a different source,
// so a caller that spells one out and one that leaves it off share a session.
func TestSpelledOutDefaultsReuseTheHeldSession(t *testing.T) {
	hold, opened := heldSessions(t)

	if _, err := hold.Session(Options{}); err != nil {
		t.Fatalf("first Session: %v", err)
	}
	if _, err := hold.Session(Options{Types: SourceMonitor, Cursor: CursorEmbedded}); err != nil {
		t.Fatalf("second Session: %v", err)
	}

	if *opened != 1 {
		t.Errorf("a spelled-out default reopened the session, %d opens", *opened)
	}
}

// The cursor mode is fixed at SelectSources, so a stream that wants another one is another consent.
func TestAnotherCursorModeOpensAnotherSession(t *testing.T) {
	hold, opened := heldSessions(t)

	if _, err := hold.Session(Options{Types: SourceMonitor, Cursor: CursorEmbedded}); err != nil {
		t.Fatalf("first Session: %v", err)
	}
	if _, err := hold.Session(Options{Types: SourceMonitor, Cursor: CursorHidden}); err != nil {
		t.Fatalf("second Session: %v", err)
	}

	if *opened != 2 {
		t.Errorf("a new cursor mode was served off the old consent, %d opens", *opened)
	}
}

// Release is what ends the stream's hold, and the capture after it takes a consent of its own.
func TestACaptureAfterAReleaseOpensAgain(t *testing.T) {
	hold, opened := heldSessions(t)
	opts := Options{Types: SourceMonitor}

	if _, err := hold.Session(opts); err != nil {
		t.Fatalf("first Session: %v", err)
	}
	hold.Release()
	hold.Release()
	if _, err := hold.Session(opts); err != nil {
		t.Fatalf("Session after release: %v", err)
	}

	if *opened != 2 {
		t.Errorf("a released hold served the session it had dropped, %d opens", *opened)
	}
}
