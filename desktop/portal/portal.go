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
	"strings"

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

type options = map[string]dbus.Variant

type client struct {
	conn   *dbus.Conn
	sender string
}

// request calls a portal method whose first argument is only the options map
// (CreateSession) and blocks for its Response.
func (c *client) request(method string, opts options) (options, error) {
	return c.await(method, func(reqToken string) *dbus.Call {
		opts["handle_token"] = dbus.MakeVariant(reqToken)
		return c.conn.Object(service, objectDir).Call(method, 0, opts)
	})
}

// requestOn calls a portal method that takes a session handle plus options
// (SelectSources) and blocks for its Response.
func (c *client) requestOn(method string, session dbus.ObjectPath, opts options) (options, error) {
	return c.await(method, func(reqToken string) *dbus.Call {
		opts["handle_token"] = dbus.MakeVariant(reqToken)
		return c.conn.Object(service, objectDir).Call(method, 0, session, opts)
	})
}

// requestStart calls Start, which takes a session handle and a parent-window
// identifier (empty: the compositor parents the picker itself).
func (c *client) requestStart(session dbus.ObjectPath) (options, error) {
	return c.await(scIface+".Start", func(reqToken string) *dbus.Call {
		opts := options{"handle_token": dbus.MakeVariant(reqToken)}
		return c.conn.Object(service, objectDir).Call(scIface+".Start", 0, session, "", opts)
	})
}

// await installs the Response match for a predictable Request path, invokes the
// method, and returns the results map once the Response arrives. A non-zero
// response code (user cancelled, or an error) is reported as an error.
func (c *client) await(method string, invoke func(reqToken string) *dbus.Call) (options, error) {
	token := newToken()
	reqPath := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/" + c.sender + "/" + token)

	if err := c.conn.AddMatchSignal(
		dbus.WithMatchObjectPath(reqPath),
		dbus.WithMatchInterface("org.freedesktop.portal.Request"),
		dbus.WithMatchMember("Response"),
	); err != nil {
		return nil, err
	}
	signals := make(chan *dbus.Signal, 4)
	c.conn.Signal(signals)
	defer c.conn.RemoveSignal(signals)

	if call := invoke(token); call.Err != nil {
		return nil, call.Err
	}

	for sig := range signals {
		if sig.Path != reqPath {
			continue
		}
		var code uint32
		var results options
		if err := dbus.Store(sig.Body, &code, &results); err != nil {
			return nil, err
		}
		if code != 0 {
			return nil, fmt.Errorf("%s returned response code %d", method, code)
		}
		return results, nil
	}
	return nil, fmt.Errorf("%s: signal channel closed before Response", method)
}

// openPipeWireRemote returns the dup'd PipeWire remote fd for the session as an
// *os.File the caller owns.
func (c *client) openPipeWireRemote(session dbus.ObjectPath) (*os.File, error) {
	var fd dbus.UnixFD
	err := c.conn.Object(service, objectDir).
		Call(scIface+".OpenPipeWireRemote", 0, session, options{}).Store(&fd)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), "pipewire-remote"), nil
}

func (c *client) closeSession(session dbus.ObjectPath) {
	c.conn.Object(service, session).Call("org.freedesktop.portal.Session.Close", 0)
	c.conn.Close()
}

// firstNode extracts the PipeWire node id of the first stream in a Start result.
// The streams field is an array of (node_id, properties) structs.
func firstNode(results options) (uint32, error) {
	raw, ok := results["streams"]
	if !ok {
		return 0, fmt.Errorf("Start returned no streams")
	}
	var streams []struct {
		Node  uint32
		Props map[string]dbus.Variant
	}
	if err := dbus.Store([]interface{}{raw.Value()}, &streams); err != nil {
		return 0, fmt.Errorf("decode streams: %w", err)
	}
	if len(streams) == 0 {
		return 0, fmt.Errorf("Start returned an empty stream list")
	}
	return streams[0].Node, nil
}

// senderToken is the caller's unique bus name reshaped for a Request object
// path: the leading colon dropped and dots turned into underscores.
func senderToken(conn *dbus.Conn) string {
	name := strings.TrimPrefix(conn.Names()[0], ":")
	return strings.ReplaceAll(name, ".", "_")
}

var tokenSeq uint64

// newToken returns a per-connection-unique token for a handle_token option.
// Uniqueness within the process is enough: the sender name already scopes the
// Request path to this connection.
func newToken() string {
	tokenSeq++
	return fmt.Sprintf("screenshare%d", tokenSeq)
}
