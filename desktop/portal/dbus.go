package portal

import (
	"fmt"
	"os"
	"strings"

	"github.com/godbus/dbus/v5"
)

// The D-Bus half of the ScreenCast conversation: every method is asynchronous, so
// each call registers a match for the Response signal of a Request path it makes
// predictable through a handle_token, then blocks for it.

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
