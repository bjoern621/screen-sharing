package decode

import (
	"encoding/gob"
	"net"
	"sync"

	"bjoernblessin.de/screenshare/internal/receive"
)

// What crosses between the backend and the decode host.
//
// gob and not protobuf: the two ends are one executable, built and shipped together, so the wire
// carries receive.Stats itself rather than a schema restating its fifty fields.
// A schema here would be a second declaration of every figure the receive package already owns,
// and a field added to one and not the other is the drift the contract exists to prevent.
// The public contract with the shell has the opposite problem, two languages and two release
// cadences, which is what its protobuf is for (docs/ipc-api.md).
//
// Three connection kinds, each dialled separately rather than multiplexed.
// A Unix socket is cheap enough that one per subscription costs less than a framing layer
// that interleaves them, and a dropped subscription then closes exactly what it owns.

// connKind is the first value on every connection, naming what the rest of it carries.
type connKind uint8

const (
	connControl connKind = iota + 1
	// connLifecycle carries what a decode does without being asked: its first frame, its end.
	connLifecycle
	// connFrames is one consumer's subscription, requests one way and events the other.
	connFrames
)

// op is what a control request asks for.
type op uint8

const (
	opOpen op = iota + 1
	opStop
	opSetAudio
	// opSnapshot reads every decode the host holds, which is the whole of what the backend reads.
	opSnapshot
)

// request is one control call.
// One shape for every op rather than one per call: gob writes no field that is zero, so the unused
// half of a request costs nothing on the wire.
type request struct {
	Op     op
	ID     ID
	Stream receive.Stream
	Open   receive.Open
	Volume float64
	Muted  bool
}

// response answers one control call.
// Err is an Umgebungsfehler the backend surfaces, and empty on success.
type response struct {
	Err string
	// ToneMap is what an open built with, which on a machine with no rung is not what was asked
	// for (receive.Receiver.ToneMap).
	ToneMap bool
	// States answers opSnapshot, and is nil on every other op.
	States map[ID]State
}

// lifecycleKind is which callback one lifecycle message stands for.
type lifecycleKind uint8

const (
	lifeLive lifecycleKind = iota + 1
	lifeEnd
)

type lifecycleMessage struct {
	ID   ID
	Kind lifecycleKind
	// Message is the reason, on lifeEnd alone.
	Message string
}

// frameOp is what a subscription's consumer asks for.
type frameOp uint8

const (
	// frameSubscribe is the first message on a frames connection and names the decode.
	frameSubscribe frameOp = iota + 1
	frameRelease
	frameRenderSize
)

type frameRequest struct {
	Op frameOp
	// ID is set on frameSubscribe alone, the connection being bound to one decode afterwards.
	ID            ID
	Generation    uint64
	Slot          int
	Serial        uint64
	Width, Height int
}

// frameEvent is one message of a subscription, with exactly one of the three set.
//
// Err ends the subscription and is the last message on the connection, carrying what the consumer
// is told: a pipeline that stopped, or a frame the host could not export.
type frameEvent struct {
	Pool  *receive.Pool
	Frame *receive.Frame
	Err   string
}

// conn is one gob connection, sends serialized.
//
// The lock covers writing alone.
// One goroutine reads a connection and any number may write to it, which is the shape all three
// kinds take: a control caller writes and waits, and a subscription's consumer releases slots while
// its own reader takes events.
type conn struct {
	c   net.Conn
	dec *gob.Decoder

	mu  sync.Mutex
	enc *gob.Encoder
}

func newConn(c net.Conn) *conn {
	return &conn{c: c, dec: gob.NewDecoder(c), enc: gob.NewEncoder(c)}
}

func (c *conn) send(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.enc.Encode(v)
}

// recv fills v from the next message, and answers io.EOF where the peer closed.
func (c *conn) recv(v any) error { return c.dec.Decode(v) }

func (c *conn) Close() error { return c.c.Close() }
