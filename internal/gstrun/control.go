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
// A publish that changes a bitrate does not have to become another pipeline.
// Relaunching one costs every viewer a reconnect, so a knob the encoder will take while it runs
// should reach it while it runs, and that is a thing a launcher cannot do at all,
// which is why the pipeline moved into a process of this app's own.
//
// What crosses is a property write and never a settings field.
// The vocabulary stays in the parent, which knows that a bitrate is kbit on x264enc and bits per
// second on vp8enc; the child sets what it is told on the element it is named.
// A child that knew the settings would be a second place deciding what a field means.

// LiveState is the whole of what a pipeline should be holding, as element property writes.
//
// Whole rather than a delta: a child that converges to a complete state is one where a
// crash-restart and an apply are the same operation, and where a message that never arrived cannot
// leave the pipeline on a value nobody chose.
// Sending the same state twice changes nothing the second time.
type LiveState struct {
	Properties []Property `json:"properties"`
}

// Property is one write: which element, which property, and the value as text.
//
// Text because that is what GStreamer's own deserialization takes, so an integer,
// an enum nick and a fraction all cross the same way and the child needs no type table to read them
// with.
type Property struct {
	Element string `json:"element"`
	Name    string `json:"name"`
	Value   string `json:"value"`
}

// AppliedPrefix leads the line the child writes once it has converged to a state,
// so the parent can tell an applied write from one that never landed.
const AppliedPrefix = "screenshare-applied "

// serveControl accepts one connection at a time on the socket and applies what arrives.
//
// One at a time because there is one parent: the socket exists for the process that spawned this
// one, and a second connection would be a second opinion about what the pipeline should be holding.
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

// readControl applies every state that arrives on one connection.
//
// A malformed line and an element name nothing answers to are both reported and skipped rather than
// ending the run.
// The pipeline is carrying a stream, and a parent that sent nonsense has not made the picture
// wrong, so both are Umgebungsfehler from this process's side.
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
// A run nobody wants to talk to takes no socket, which is what the rendered command shows and what
// the measuring runs use.
func listenControl(path string) (net.Listener, error) {
	if path == "" {
		return nil, nil
	}
	// A stale socket file is one this process is entitled to remove: the path is handed out per run by
	// the parent, so anything already there belongs to a run that is gone.
	os.Remove(path)
	return net.Listen("unix", path)
}
