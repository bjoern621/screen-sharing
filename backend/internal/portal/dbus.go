package portal

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"github.com/godbus/dbus/v5"

	"bjoernblessin.de/go-utils/util/assert"
)

// The D-Bus half of the ScreenCast conversation.
// Every method is asynchronous, so each call registers a match for the Response signal of a Request
// path it makes predictable through a handle_token, then blocks for it.

type options = map[string]dbus.Variant

type client struct {
	conn   *dbus.Conn
	sender string
}

// request calls a portal method taking the options map alone, CreateSession, and blocks for its
// Response.
func (c *client) request(method string, opts options) (options, error) {
	return c.await(method, func(reqToken string) *dbus.Call {
		opts["handle_token"] = dbus.MakeVariant(reqToken)
		return c.conn.Object(service, objectDir).Call(method, 0, opts)
	})
}

// requestOn calls a portal method taking a session handle and options, SelectSources,
// and blocks for its Response.
func (c *client) requestOn(method string, session dbus.ObjectPath, opts options) (options, error) {
	assert.Assert(session != "", "a session call names the session it is made on", method)

	return c.await(method, func(reqToken string) *dbus.Call {
		opts["handle_token"] = dbus.MakeVariant(reqToken)
		return c.conn.Object(service, objectDir).Call(method, 0, session, opts)
	})
}

// requestStart calls Start, which takes a session handle and a parent-window identifier.
// The identifier is empty, which asks the compositor to parent the picker itself.
func (c *client) requestStart(session dbus.ObjectPath) (options, error) {
	assert.Assert(session != "", "a start names the session it starts")

	return c.await(scIface+".Start", func(reqToken string) *dbus.Call {
		opts := options{"handle_token": dbus.MakeVariant(reqToken)}
		return c.conn.Object(service, objectDir).Call(scIface+".Start", 0, session, "", opts)
	})
}

// await installs the Response match for the Request path a token makes predictable, invokes the
// method, and answers the results map the Response carries.
// The match goes in before the invocation, so a Response arriving immediately is still caught.
// A non-zero response code is a cancelled picker or a portal-side failure, and leaves as an error.
func (c *client) await(method string, invoke func(reqToken string) *dbus.Call) (options, error) {
	assert.Assert(method != "", "an awaited call names the portal method it blocks for")
	assert.IsNotNil(invoke, "an awaited call has a method to invoke", method)
	assert.IsNotNil(c.conn, "an awaited call is made over a bus connection", method)

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

// openPipeWireRemote answers the session's PipeWire remote fd as an *os.File the caller owns.
func (c *client) openPipeWireRemote(session dbus.ObjectPath) (*os.File, error) {
	assert.Assert(session != "", "a remote is opened on a named session")

	var fd dbus.UnixFD
	err := c.conn.Object(service, objectDir).
		Call(scIface+".OpenPipeWireRemote", 0, session, options{}).Store(&fd)
	if err != nil {
		return nil, err
	}
	// os.NewFile answers nil for a negative descriptor, which is what a portal that reports its
	// failure in the reply rather than as a D-Bus error leaves.
	// The capture child inherits this file at a fixed position,
	// so a closed slot is refused before it travels that far.
	f := os.NewFile(uintptr(fd), "pipewire-remote")
	if f == nil {
		return nil, fmt.Errorf("invalid file descriptor %d", int(fd))
	}
	return f, nil
}

func (c *client) closeSession(session dbus.ObjectPath) {
	c.conn.Object(service, session).Call("org.freedesktop.portal.Session.Close", 0)
	c.conn.Close()
}

// firstNode is the PipeWire node id of the first stream in a Start result.
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

// senderToken is the caller's unique bus name reshaped for a Request object path: leading colon
// dropped, dots turned into underscores.
func senderToken(conn *dbus.Conn) string {
	assert.IsNotNil(conn, "a sender token names a connected bus")
	assert.Assert(len(conn.Names()) > 0, "a connected bus has a unique name")

	name := strings.TrimPrefix(conn.Names()[0], ":")
	return strings.ReplaceAll(name, ".", "_")
}

var tokenSeq atomic.Uint64

// newToken is a handle_token unique within the process, which is as far as uniqueness has to reach:
// the sender name already scopes a Request path to this connection.
// Two concurrent Opens draw from this counter, and a number handed out twice puts both of them on
// one Request object path.
func newToken() string {
	return fmt.Sprintf("screenshare%d", tokenSeq.Add(1))
}
