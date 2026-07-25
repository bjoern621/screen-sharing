// Package portal drives the xdg-desktop-portal ScreenCast interface over D-Bus.
//
// The portal is the compositor-agnostic Wayland capture path: the app never
// touches a raw framebuffer, it asks the portal, the compositor draws its own
// picker, and a PipeWire node is handed back on a dedicated remote fd. GNOME,
// KDE and wlroots compositors all implement it.
//
// Every ScreenCast method is asynchronous: the call returns a Request object
// path and the real result arrives later on that object's Response signal. The
// handle_token in each options map makes the Request path predictable, so the
// Response match is in place before the method is called and no signal races.
package portal

import (
	"fmt"
	"os"

	"github.com/godbus/dbus/v5"
)

const (
	service   = "org.freedesktop.portal.Desktop"
	objectDir = "/org/freedesktop/portal/desktop"
	scIface   = "org.freedesktop.portal.ScreenCast"
)

// CursorMode selects how the pointer appears in the captured stream. The values
// are the portal's bitmask: hidden, embedded in the frame, or delivered as
// separate metadata.
type CursorMode uint32

const (
	CursorHidden   CursorMode = 1
	CursorEmbedded CursorMode = 2
	CursorMetadata CursorMode = 4
)

// SourceType selects what the picker offers. The values are the portal's
// bitmask and may be combined.
type SourceType uint32

const (
	SourceMonitor SourceType = 1
	SourceWindow  SourceType = 2
	SourceVirtual SourceType = 4
)

// Options requests a capture shape. Types and Cursor default to a whole-monitor
// capture with the cursor drawn into the frame. RestoreToken, when non-empty,
// asks the compositor to skip the picker and reuse a prior consent.
type Options struct {
	Types        SourceType
	Cursor       CursorMode
	RestoreToken string
}

// Session is one open ScreenCast stream. Fd is the PipeWire remote the consumer
// reads NodeID from; it is passed to a child process as an inherited fd. Close
// releases the fd and tears the portal session down.
type Session struct {
	conn    *dbus.Conn
	handle  dbus.ObjectPath
	NodeID  uint32
	Fd      *os.File
	Restore string
}

// Open runs the CreateSession, SelectSources, Start and OpenPipeWireRemote
// sequence and returns the negotiated stream. Start pops the compositor picker
// unless a valid RestoreToken is supplied.
func Open(opts Options) (*Session, error) {
	if opts.Types == 0 {
		opts.Types = SourceMonitor
	}
	if opts.Cursor == 0 {
		opts.Cursor = CursorEmbedded
	}

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
		"persist_mode": dbus.MakeVariant(uint32(2)), // persist until revoked, so RestoreToken works
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

	fd, err := c.openPipeWireRemote(session)
	if err != nil {
		c.closeSession(session)
		return nil, fmt.Errorf("OpenPipeWireRemote: %w", err)
	}

	restore, _ := started["restore_token"].Value().(string)
	return &Session{
		conn:    conn,
		handle:  session,
		NodeID:  node,
		Fd:      fd,
		Restore: restore,
	}, nil
}

// Close releases the remote fd and closes both the portal session and the bus
// connection. It is safe to call once.
func (s *Session) Close() {
	if s.Fd != nil {
		s.Fd.Close()
		s.Fd = nil
	}
	if s.conn != nil {
		s.conn.Object(service, s.handle).Call("org.freedesktop.portal.Session.Close", 0)
		s.conn.Close()
		s.conn = nil
	}
}
