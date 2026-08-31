package portal

import (
	"sync"

	"bjoernblessin.de/go-utils/util/assert"
)

// Hold keeps one ScreenCast session across the captures that read it.
//
// A session per capture is a picker per capture wherever the compositor persists no consent.
// The restore token is what would answer the picker instead, and only the compositor issues one:
// where Start answers an empty token, as xdg-desktop-portal-hyprland does,
// every SelectSources asks the user to point at a screen again.
// A publish that relaunches over a refused relay would ask once per attempt.
//
// The session outlives the child instead,
// so the picker is answered once per stream however many children carry it,
// and each child takes a remote of its own on the held session.
//
// Nothing here expires on its own: the holder decides when the stream is over and calls Release.
// A session held past the last child keeps the compositor sharing a screen nobody receives,
// and keeps whatever indicator it shows for that lit.
type Hold struct {
	mu sync.Mutex
	// source is what the held session was opened on, normalized, and the zero value where none is held.
	source  Options
	session *Session
	// Open where nil, replaced by a test.
	open func(Options) (*Session, error)
}

// Session answers the held session, opening one where nothing is held.
//
// Options naming a different source close the held session and open another, which pops the picker:
// the source kinds and the cursor mode are fixed at SelectSources and no method moves them.
// RestoreToken is no part of that comparison,
// being what opening a session starts from where a held one is a consent already granted.
func (h *Hold) Session(opts Options) (*Session, error) {
	opts = opts.normalized()

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.session != nil && h.source.sameSource(opts) {
		return h.session, nil
	}
	h.releaseLocked()

	open := h.open
	if open == nil {
		open = Open
	}
	session, err := open(opts)
	if err != nil {
		return nil, err
	}
	h.source, h.session = opts, session

	assert.IsNotNil(session, "an opened session is the session a hold takes")
	return session, nil
}

// Release closes the held session, so the next Session takes a consent of its own.
// Idempotent: a hold with nothing in it is already released.
func (h *Hold) Release() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.releaseLocked()
}

func (h *Hold) releaseLocked() {
	if h.session == nil {
		return
	}
	h.session.Close()
	h.session = nil
	h.source = Options{}
}

// sameSource reports whether two option sets ask the compositor for the same source.
// A default spelled out and one left off are one request, which the normalized precondition carries:
// comparing an unfilled set would reopen a session over a value Open would have supplied,
// and reopening is a picker.
func (o Options) sameSource(other Options) bool {
	assert.Assert(o == o.normalized() && other == other.normalized(),
		"a source comparison is made on normalized options", o, other)

	return o.Types == other.Types && o.Cursor == other.Cursor
}
