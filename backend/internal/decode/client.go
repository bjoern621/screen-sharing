package decode

import (
	"errors"
	"sync"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/receive"
)

// The backend's half: what it may ask of the host.
// Bringing the host up is spawn.go, and what it reports back is hostlife.go.

// Subcommand is the first argument of the child, and what main dispatches on.
const Subcommand = "decode-host"

// errNoHost is every call that names a decode while no host runs.
// A host that is not running holds no decode, so a stop and a read both answer through it rather
// than starting one.
var errNoHost = errors.New("no decode host is running")

// errShutDown is every call after Close, which the process exiting is the ordinary cause of.
var errShutDown = errors.New("the decode host is shut down")

// Client runs the decode host and addresses the decodes on it.
//
// The child is brought up on the first open and replaced on the next one after it exits, so a host
// that aborted costs the decodes that were running and nothing else.
// Every method is safe to call from any goroutine.
//
// A nil client answers errNoHost and stops nothing.
// That is what a caller holding none can be told without spawning:
// the child is this executable run again, which under a test binary is that binary running
// its own tests.
type Client struct {
	// spawnMu serializes bringing the host up, so two callers meeting an absent host start one
	// child rather than two.
	spawnMu sync.Mutex
	// callMu serializes the control connection: the protocol pairs an answer with the request
	// before it, so two calls in flight would each take the other's.
	callMu sync.Mutex

	mu sync.Mutex
	// dir holds the control socket and is made per spawn, so a host that exited leaves nothing
	// the next one has to clear.
	dir    string
	socket string
	proc   *process
	ctrl   *conn
	// events are the callbacks one decode was opened with.
	//
	// A registry of what to call rather than a copy of which decodes exist: the host owns that set
	// and the snapshot answers it.
	// An entry stays until the decode is stopped, so a host that exited can report every decode it
	// was running.
	events map[ID]Events
	closed bool
}

// NewClient prepares the host of this process, without starting it.
//
// Nothing is created and nothing is resolved here.
// The executable and the socket are settled when the child comes up, so a runtime directory
// that cannot be written and an executable that cannot be located are failures surfaced where every
// other spawn failure is, rather than ones the backend carries from its construction to the first
// decode.
func NewClient() *Client {
	return &Client{events: map[ID]Events{}}
}

// Open opens one decode on the host, and answers a handle addressing it.
//
// A decode already open is success and builds nothing, the state the call names already holding.
// The answer carries what it was built with, which on a machine with no tone-mapping rung is not
// what was asked for.
func (c *Client) Open(id ID, st receive.Stream, open receive.Open, ev Events) (*Handle, error) {
	assert.Assert(id.Kind != 0, "an opened decode names what kind it is")

	if c == nil {
		return nil, errNoHost
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errShutDown
	}
	// Recorded before the call: a host that aborts between the open and the answer has to report
	// this decode ending, and a registry written afterwards would have nothing to report.
	c.events[id] = ev
	c.mu.Unlock()

	if err := c.ensure(); err != nil {
		c.forget(id)
		return nil, err
	}

	res, err := c.call(request{Op: opOpen, ID: id, Stream: st, Open: open})
	if err != nil {
		c.forget(id)
		return nil, err
	}
	if res.Err != "" {
		c.forget(id)
		return nil, errors.New(res.Err)
	}
	return &Handle{client: c, id: id, toneMap: res.ToneMap}, nil
}

// Stop closes one decode, and succeeds where none is open.
func (c *Client) Stop(id ID) {
	c.forget(id)
	if _, err := c.call(request{Op: opStop, ID: id}); err != nil {
		// A host that is gone is one running no decode, so the state the stop names holds.
		logger.Debugf("stopping the decode of %s reached no host: %v", id, err)
	}
}

// SetAudio sets how loud one decode plays, and refuses one that is not open.
func (c *Client) SetAudio(id ID, volume float64, muted bool) error {
	res, err := c.call(request{Op: opSetAudio, ID: id, Volume: volume, Muted: muted})
	if err != nil {
		return err
	}
	if res.Err != "" {
		return errors.New(res.Err)
	}
	return nil
}

// Snapshot is every decode the host holds, read through on the call.
// A host that is not running holds none, which is what an empty answer says.
func (c *Client) Snapshot() map[ID]State {
	res, err := c.call(request{Op: opSnapshot})
	if err != nil {
		logger.Debugf("reading the decodes reached no host: %v", err)
		return map[ID]State{}
	}
	if res.States == nil {
		return map[ID]State{}
	}
	return res.States
}

// forget drops one decode's callbacks, so a host exiting afterwards reports nothing about it.
func (c *Client) forget(id ID) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.events, id)
	c.mu.Unlock()
}

// call sends one control request and waits for its answer.
//
// It starts no host.
// An open brings one up first, and every other call names a decode, which a host that is not
// running holds none of.
//
// A failure takes the connection down, so the next open spawns a host rather than writing into one
// that is gone.
func (c *Client) call(req request) (response, error) {
	if c == nil {
		return response{}, errNoHost
	}
	c.callMu.Lock()
	defer c.callMu.Unlock()

	c.mu.Lock()
	ctrl := c.ctrl
	c.mu.Unlock()

	if ctrl == nil {
		return response{}, errNoHost
	}

	if err := ctrl.send(req); err != nil {
		c.hostGone(err)
		return response{}, err
	}
	var res response
	if err := ctrl.recv(&res); err != nil {
		c.hostGone(err)
		return response{}, err
	}
	return res, nil
}
