package gstrun

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/assert"
)

// The control socket is how a value reaches a pipeline that is already playing.
//
// Changing a bitrate does not have to become another pipeline.
// A relaunch costs every viewer a reconnect, so a knob the encoder takes while it runs reaches it
// while it runs.
// A launcher cannot do that at all, which is why the pipeline moved into a process of this app's
// own.
//
// What crosses is a property write and never a settings field.
// The vocabulary stays in the parent, which knows a bitrate is kbit on x264enc and bits per second
// on vp8enc, and the child writes what it is told on the element it is named.
// A child that knew the settings would be a second place deciding what a field means.

// LiveState is the whole of what a pipeline should be holding, as element property writes.
//
// Whole rather than a delta: a child converging to a complete state makes a crash-restart and an
// apply the same operation, and leaves no message whose loss pins the pipeline on a value nobody
// chose.
// The same state sent twice changes nothing the second time.
type LiveState struct {
	Properties []Property `json:"properties"`
}

// Property is one write: which element, which property, the value as text.
//
// Text is what GStreamer's own deserialization takes, so an integer, an enum nick and a fraction
// all cross alike and the child needs no type table to read them with.
type Property struct {
	Element string `json:"element"`
	Name    string `json:"name"`
	Value   string `json:"value"`
}

// AppliedPrefix leads the line the child writes once it has converged, carrying how many writes
// landed, so the parent can tell an applied write from one that named nothing.
const AppliedPrefix = "screenshare-applied "

// serveControl accepts one connection at a time and applies what arrives, on its own goroutine
// until the listener closes.
//
// One at a time because there is one parent: the socket exists for the process that spawned this
// one, and a second connection is a second opinion about what the pipeline should be holding.
func serveControl(listener net.Listener, pipeline gst.Pipeline, out io.Writer) {
	assert.IsNotNil(listener, "a control server serves a listening socket")
	assert.IsNotNil(out, "a control server reports what it applied to a writer")

	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		readControl(conn, pipeline, out)
		conn.Close()
	}
}

// readControl applies every state that arrives on one connection, one JSON line each.
//
// A malformed line and an element name nothing answers to are reported and skipped rather than
// ending the run.
// The pipeline is carrying a stream and a parent that sent nonsense has not made the picture wrong,
// so both are Umgebungsfehler from this process's side.
func readControl(r io.Reader, pipeline gst.Pipeline, out io.Writer) {
	assert.IsNotNil(r, "a control read reads from a connection")
	assert.IsNotNil(out, "a control read reports what it applied to a writer")

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		var state LiveState
		if err := json.Unmarshal(scanner.Bytes(), &state); err != nil {
			fmt.Fprintf(out, "control: %v\n", err)
			continue
		}
		applied := 0
		for _, p := range state.Properties {
			el := pipeline.GetByName(p.Element)
			if el == nil {
				fmt.Fprintf(out, "control: no element named %s\n", p.Element)
				continue
			}
			el.SetObjectProperty(p.Name, p.Value)
			applied++
		}
		assert.Assert(applied <= len(state.Properties),
			"a converged state applied no more writes than it carried", applied, len(state.Properties))
		fmt.Fprintf(out, "%s%d\n", AppliedPrefix, applied)
	}
}

// listenControl opens the control socket at path, and nothing at all for the empty path.
// A run nobody talks to takes no socket, which is what the rendered command shows and what the
// measuring runs use.
func listenControl(path string) (net.Listener, error) {
	if path == "" {
		return nil, nil
	}
	// A stale socket file is this process's to remove: the parent hands out a path per run,
	// so anything already there belongs to a run that is gone.
	os.Remove(path)
	return net.Listen("unix", path)
}
