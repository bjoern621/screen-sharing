// Package portal drives the xdg-desktop-portal ScreenCast interface over D-Bus.
//
// The portal is the compositor-agnostic Wayland capture backend: nothing here touches a raw
// framebuffer, the compositor draws its own picker, and a PipeWire node comes back on a dedicated
// remote fd.
// GNOME, KDE and wlroots compositors implement it.
//
// Every ScreenCast method is asynchronous: the call returns a Request object path and the result
// arrives later on that object's Response signal.
// The handle_token in each options map makes the Request path predictable, so the Response match is
// installed before the method is called and no signal races.
//
// The node the session yields is damage-driven: no frame while the screen is still,
// and the PipeWire clock stops with it.
// A consumer therefore paces and re-stamps the frames itself instead of following that clock
// (publish/gstcapture.go, imagefreeze).
//
// Every failure belongs to the compositor or the bus rather than to this code, so all of them are
// Umgebungsfehler and leave as errors: a portal that is not running, a dismissed picker, a revoked
// consent.
package portal

import (
	"fmt"
	"os"

	"github.com/godbus/dbus/v5"

	"bjoernblessin.de/go-utils/util/assert"
)

const (
	service   = "org.freedesktop.portal.Desktop"
	objectDir = "/org/freedesktop/portal/desktop"
	scIface   = "org.freedesktop.portal.ScreenCast"
)

// CursorMode selects how the pointer appears in the captured stream: hidden, embedded in the frame,
// or delivered as metadata beside it.
// The values are the portal's own bitmask.
type CursorMode uint32

const (
	CursorHidden   CursorMode = 1
	CursorEmbedded CursorMode = 2
	CursorMetadata CursorMode = 4
)

// SourceType selects what the picker offers.
// The values are the portal's own bitmask and combine.
type SourceType uint32

const (
	SourceMonitor SourceType = 1
	SourceWindow  SourceType = 2
	SourceVirtual SourceType = 4
)

// Options requests a capture shape.
// Types and Cursor default to a whole-monitor capture with the cursor drawn into the frame.
// A non-empty RestoreToken asks the compositor to skip the picker and reuse a prior consent.
type Options struct {
	Types        SourceType
	Cursor       CursorMode
	RestoreToken string
}

// normalized fills the defaults Open applies.
// Two option sets asking for one source therefore compare equal whether or not either spelled them,
// which is what a Hold decides reuse on.
func (o Options) normalized() Options {
	if o.Types == 0 {
		o.Types = SourceMonitor
	}
	if o.Cursor == 0 {
		o.Cursor = CursorEmbedded
	}
	return o
}

// Session is one granted ScreenCast consent: the node the compositor shares and the bus connection
// that keeps it alive.
// Closing it drops the node, so the next Open pops the picker unless a restore token survived.
//
// A session outlives the captures that read it, which is why the PipeWire remote is no field here.
// Restore is what the compositor issued for this consent, empty where it persisted none, and it is
// stored as it stands for the next SelectSources (settings.SavePortalToken): a revoked consent and
// an empty token replacing a spent one both send the next Open through the picker.
type Session struct {
	client  *client
	handle  dbus.ObjectPath
	NodeID  uint32
	Restore string
}

// Open runs CreateSession, SelectSources and Start, and answers the consent they granted.
// Start pops the compositor picker unless a valid RestoreToken is supplied.
// The frames are read over a remote the caller opens on the answer (Session.Remote).
//
// Every failure past CreateSession tears the session down before returning,
// so a refused Open leaves nothing open on the compositor's side.
func Open(opts Options) (*Session, error) {
	opts = opts.normalized()

	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("connect session bus: %w", err)
	}
	c := &client{conn: conn, sender: senderToken(conn)}

	created, err := c.request(scIface+".CreateSession", options{
		"session_handle_token": dbus.MakeVariant(newToken()),
	})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("CreateSession: %w", err)
	}
	handle, ok := created["session_handle"].Value().(string)
	if !ok {
		conn.Close()
		return nil, fmt.Errorf("CreateSession returned no session_handle")
	}
	session := dbus.ObjectPath(handle)

	selectOpts := options{
		"types":        dbus.MakeVariant(uint32(opts.Types)),
		"cursor_mode":  dbus.MakeVariant(uint32(opts.Cursor)),
		"multiple":     dbus.MakeVariant(false),
		"persist_mode": dbus.MakeVariant(uint32(2)), // persist until revoked, which issues a token
	}
	if opts.RestoreToken != "" {
		selectOpts["restore_token"] = dbus.MakeVariant(opts.RestoreToken)
	}
	if _, err := c.requestOn(scIface+".SelectSources", session, selectOpts); err != nil {
		c.closeSession(session)
		return nil, fmt.Errorf("SelectSources: %w", err)
	}

	started, err := c.requestStart(session)
	if err != nil {
		c.closeSession(session)
		return nil, fmt.Errorf("Start: %w", err)
	}
	node, err := firstNode(started)
	if err != nil {
		c.closeSession(session)
		return nil, err
	}

	restore, _ := started["restore_token"].Value().(string)
	open := &Session{
		client:  c,
		handle:  session,
		NodeID:  node,
		Restore: restore,
	}

	assert.Assert(open.handle != "", "an open session names the portal session it holds")
	return open, nil
}

// Remote opens a PipeWire connection on the session and answers the descriptor it lives on, which a
// child inherits and reads NodeID from.
// The caller owns the file and closes it when the child that inherited it is gone.
//
// A connection of its own per child rather than one shared between them: the descriptor carries the
// PipeWire protocol, an exited child already spoke it there, and a second one writing a hello into
// that conversation is not a client the daemon can serve.
func (s *Session) Remote() (*os.File, error) {
	assert.IsNotNil(s.client, "a remote is opened on a session that is still open")

	f, err := s.client.openPipeWireRemote(s.handle)
	if err != nil {
		return nil, fmt.Errorf("OpenPipeWireRemote: %w", err)
	}
	return f, nil
}

// Close ends the portal session and the bus connection carrying it.
// Idempotent: a second call finds both released and does nothing.
//
// The remotes opened on it are the caller's, and a child still reading one loses its frames here:
// the node goes with the session.
func (s *Session) Close() {
	if s.client == nil {
		return
	}
	s.client.closeSession(s.handle)
	s.client = nil
}
