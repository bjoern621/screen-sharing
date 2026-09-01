package decode

import (
	"net"
	"sync"
	"sync/atomic"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// The child's half: the set of decodes, and the connections the backend reaches them over.
// What each connection carries is in hostcontrol.go and hostframes.go.

// hosted is one decode and what the host holds beside it.
//
// ended and endMessage are the pipeline having stopped by itself.
// The entry then stays in the set carrying the reason until the backend stops it: an entry dropped
// where it ended would take the reason with it, and the reason is what a tile shows.
type hosted struct {
	decoder    Decoder
	ended      bool
	endMessage string
}

// Host owns every decode in this process and answers the backend.
type Host struct {
	factory Factory

	mu       sync.Mutex
	decoders map[ID]*hosted

	lifecycle chan lifecycleMessage

	// conns are the connections being served, so a shutdown ends every consumer rather than
	// leaving one waiting on a host that has stopped.
	conns map[*conn]struct{}
	// shut is whether Close ran, so a connection accepted during one is closed rather than served.
	shut bool

	// done closes when a control connection ends, which is the backend having gone.
	done     chan struct{}
	doneOnce sync.Once
}

// NewHost builds a host that opens through factory.
func NewHost(factory Factory) *Host {
	assert.IsNotNil(factory, "a decode host opens through a factory")
	return &Host{
		factory:   factory,
		decoders:  map[ID]*hosted{},
		lifecycle: make(chan lifecycleMessage, lifecycleBuffer),
		conns:     map[*conn]struct{}{},
		done:      make(chan struct{}),
	}
}

// NewReceiveHost builds the host the child runs, opening real pipelines.
func NewReceiveHost() *Host { return NewHost(openReceiver) }

// Done closes when the backend's control connection ends.
func (h *Host) Done() <-chan struct{} { return h.done }

// Serve answers connections until the listener fails, which a close does.
func (h *Host) Serve(l net.Listener) error {
	assert.IsNotNil(l, "a decode host serves a listener")

	for {
		c, err := l.Accept()
		if err != nil {
			return err
		}
		go h.handle(c)
	}
}

// Close ends every connection being served.
//
// A consumer learns a host has stopped from its connection closing, which a process that aborted
// does for free and an orderly shutdown has to do itself.
// Idempotent.
func (h *Host) Close() {
	h.mu.Lock()
	held := h.conns
	h.conns = map[*conn]struct{}{}
	h.shut = true
	h.mu.Unlock()

	for c := range held {
		c.Close()
	}
}

// StopAll takes every decode down, and is what the child runs on its way out.
//
// Together rather than one after another.
// Each stop blocks until its pipeline reaches NULL, bounded by the receive package's own timeout,
// so a row would bound this shutdown at that timeout times the number of pipelines.
//
// Waiting at all is the point: the process exits next, and a pipeline still running then is torn
// down by the operating system with its threads wherever they stand, which on Windows leaves
// an unkillable process.
// The count says whether the exit about to happen is the clean one.
func (h *Host) StopAll() {
	h.mu.Lock()
	held := h.decoders
	h.decoders = map[ID]*hosted{}
	h.mu.Unlock()

	var stopping sync.WaitGroup
	var running atomic.Int32
	for id, entry := range held {
		if entry.ended {
			continue
		}
		stopping.Add(1)
		go func() {
			defer stopping.Done()
			if !entry.decoder.Stop() {
				running.Add(1)
				logger.Warnf("the decode of %s did not stop", id)
			}
		}()
	}
	stopping.Wait()

	if left := running.Load(); left > 0 {
		logger.Warnf("%d decode(s) were still running at shutdown; the streams they name are in the lines above", left)
	}
}

// handle reads the kind off a new connection and hands it to the loop that serves it.
func (h *Host) handle(c net.Conn) {
	conn := newConn(c)
	defer conn.Close()

	h.mu.Lock()
	shut := h.shut
	if !shut {
		h.conns[conn] = struct{}{}
	}
	h.mu.Unlock()

	if shut {
		return
	}
	defer func() {
		h.mu.Lock()
		delete(h.conns, conn)
		h.mu.Unlock()
	}()

	var kind connKind
	if err := conn.recv(&kind); err != nil {
		logger.Debugf("a decode connection named no kind: %v", err)
		return
	}

	switch kind {
	case connControl:
		h.serveControl(conn)
	case connLifecycle:
		h.serveLifecycle(conn)
	case connFrames:
		h.serveFrames(conn)
	default:
		logger.Warnf("a decode connection named kind %d, which this host does not serve", kind)
	}
}
