package publish

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	"bjoernblessin.de/screenshare/internal/cursor"
	"bjoernblessin.de/screenshare/internal/gstrun"
	"bjoernblessin.de/screenshare/internal/settings"
)

// ControlFlag is how the runner is told where to listen, as the first argument after the subcommand
// (cmd/backend).
// A run given none takes no socket, which is what the rendered command shows and what the measuring
// runs use.
const ControlFlag = "--control="

// gstHandle is a running GStreamer publish: the supervised child, and the socket its pipeline takes
// values on.
//
// It is a Handle with one method more.
// An engine whose pipeline takes nothing while it runs returns a plain handle instead,
// which is how the ffmpeg engine says it is not live without a flag anywhere saying so
// (LiveApplier).
type gstHandle struct {
	Handle

	mu     sync.Mutex
	socket string
	conn   net.Conn
}

// ApplyLive hands the running pipeline the whole state these settings ask for.
//
// Whole rather than what changed, because the child converges to what it is told:
// sending the same state twice changes nothing the second time, and a state that never arrived
// cannot leave the pipeline on a value nobody chose.
// A settings object with nothing live in it sends nothing at all.
func (h *gstHandle) ApplyLive(s settings.Settings) error {
	state := gstLiveState(s)
	if len(state.Properties) == 0 {
		return nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.conn == nil {
		conn, err := net.Dial("unix", h.socket)
		if err != nil {
			return fmt.Errorf("reaching the pipeline: %w", err)
		}
		h.conn = conn
	}
	line, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if _, err := h.conn.Write(append(line, '\n')); err != nil {
		// A write that fails is a connection that died with the pipeline behind it.
		// The next apply dials again rather than reporting this one forever.
		h.conn.Close()
		h.conn = nil
		return fmt.Errorf("writing to the pipeline: %w", err)
	}
	return nil
}

// Stop closes the connection with the pipeline it was talking to.
func (h *gstHandle) Stop() {
	h.mu.Lock()
	if h.conn != nil {
		h.conn.Close()
		h.conn = nil
	}
	h.mu.Unlock()
	h.Handle.Stop()
}

// gstControlSocket is where one run's control socket lives.
//
// Under the user's runtime directory rather than beside the settings: it is a handle to a process,
// so it belongs with the sockets that die with their processes, and the name carries the tag the
// supervisor already gives each run.
func gstControlSocket(tag string) string {
	dir := os.TempDir()
	if runtime := os.Getenv("XDG_RUNTIME_DIR"); runtime != "" {
		dir = runtime
	}
	return filepath.Join(dir, fmt.Sprintf("screenshare-publish-%s-%d.sock", tag, os.Getpid()))
}

// gstLiveArgs are the arguments that give a run its control socket, and none for a run that takes
// no values: the encode probe and the test streams measure a pipeline rather than carry one,
// so nothing talks to them.
func gstLiveArgs(socket string) []string {
	if socket == "" {
		return nil
	}
	return []string{ControlFlag + socket}
}

// LiveApplier is a Handle whose pipeline takes values while it runs.
//
// An engine states it by implementing it.
// The ffmpeg engine does not: neither ffmpeg nor its command line takes a value back once it is
// running, so a caller asking a running ffmpeg publish to apply anything is asking for a relaunch,
// and the type says so at the call site rather than a table saying it somewhere else.
type LiveApplier interface {
	ApplyLive(s settings.Settings) error
}

// Live reports whether this handle's pipeline takes values while it runs, and returns the applier
// where it does.
func Live(h Handle) (LiveApplier, bool) {
	a, ok := h.(LiveApplier)
	return a, ok
}

// gstChildArgs are what this run asks its child to do beside playing the pipeline:
// the control socket, and whether to report where the pointer is.
//
// They lead the arguments so everything after them is the pipeline itself,
// which is what the child's own parsing relies on and what keeps a rendered command reading as a
// pipeline from the first word after the subcommand.
func gstChildArgs(s settings.Settings, socket string) []string {
	args := gstLiveArgs(socket)
	if s.Publish.Cursor == cursor.Metadata {
		args = append(args, gstrun.PointerFlag)
	}
	return args
}
