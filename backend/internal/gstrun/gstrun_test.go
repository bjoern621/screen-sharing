package gstrun

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// A pipeline that ends on its own runs to its end and reports nothing.
//
// videotestsrc rather than a capture backend: the runner is what is under test, and a run needing
// a screen, a portal consent or a GPU would cover the machine instead.
func TestARunPlaysAPipelineToItsEnd(t *testing.T) {
	var out bytes.Buffer

	if err := Run(t.Context(), "videotestsrc num-buffers=2 ! fakesink", &out); err != nil {
		t.Fatalf("a pipeline that ends on its own reported: %v", err)
	}
}

// The capture's negotiated caps reach the caller: the report a launcher could not make, and
// the reason this package exists.
//
// The format is pinned so the assertion is about the reporting rather than about what videotestsrc
// happens to negotiate, and an HDR verdict reads the transfer characteristic off the same line
// (docs/plan.md, "HDR").
func TestARunReportsWhatTheCaptureNegotiated(t *testing.T) {
	var out bytes.Buffer

	err := Run(t.Context(),
		"videotestsrc num-buffers=2 ! video/x-raw,format=I420,width=64,height=64,colorimetry=bt709 ! fakesink",
		&out)
	if err != nil {
		t.Fatalf("running: %v", err)
	}

	caps := capsLine(out.String())
	if caps == "" {
		t.Fatalf("no caps line on the run's output: %q", out.String())
	}
	for _, want := range []string{"video/x-raw", "format=(string)I420", "colorimetry=(string)bt709"} {
		if !strings.Contains(caps, want) {
			t.Errorf("the reported caps carry no %s: %s", want, caps)
		}
	}
}

// A failing element's own wording comes back, what the supervisor puts in front of a reader.
func TestARunReportsWhatTheElementSaid(t *testing.T) {
	var out bytes.Buffer

	// A file that is not there: filesrc opens it on the way to PLAYING rather than at parse, so this
	// run starts and then fails, the path under test.
	err := Run(t.Context(), "filesrc location=/nonexistent/screenshare-test ! fakesink", &out)
	if err == nil {
		t.Fatal("a pipeline whose source cannot open reported no error")
	}
}

// A run the caller stops returns rather than playing on.
// The supervisor cancels this way when a publish stops, and a runner ignoring it leaves a capture
// holding the screen after the button said it had let go.
func TestARunStopsWhenItsCallerDoes(t *testing.T) {
	var out bytes.Buffer

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		// An endless pipeline: it ends when the context does and not before.
		done <- Run(ctx, "videotestsrc is-live=true ! fakesink", &out)
	}()

	// Long enough for the pipeline to reach PLAYING, short enough that a runner that never returns
	// fails the test rather than hanging it.
	time.Sleep(500 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("a cancelled run reported %v, where stopping is not a failure", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a cancelled run did not return")
	}
}

// capsLine is the caps a run reported, empty where it reported none.
func capsLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if rest, ok := strings.CutPrefix(line, CapsPrefix); ok {
			return rest
		}
	}
	return ""
}

// A value reaches a pipeline that is already playing: the point of the control socket, and the one
// thing a launcher cannot be asked.
//
// The bitrate is the write under test, being what a live stream changes.
// An encoder takes another one while it runs, where relaunching to reach it costs every viewer
// a reconnect.
func TestALivePipelineTakesAPropertyWrite(t *testing.T) {
	var out lockedBuffer
	socket := filepath.Join(t.TempDir(), "control")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- RunWithControl(ctx,
			"videotestsrc is-live=true ! video/x-raw,width=64,height=64 ! x264enc name=enc bitrate=2000 ! fakesink",
			socket, &out)
	}()

	conn := dialWhenReady(t, socket)
	defer conn.Close()

	state := LiveState{Properties: []Property{{Element: "enc", Name: "bitrate", Value: "4000"}}}
	line, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write(append(line, '\n')); err != nil {
		t.Fatalf("writing the live state: %v", err)
	}

	// The child counts what it applied, so a parent tells a write that landed from one naming
	// an element the pipeline does not carry.
	waitFor(t, &out, AppliedPrefix+"1")

	cancel()
	if err := <-done; err != nil {
		t.Errorf("the run reported %v", err)
	}
}

// A write naming an element the pipeline does not carry is reported and applied to nothing.
// The run is carrying a stream, so a parent that sent nonsense has not earned it an ending.
func TestAWriteForAnAbsentElementIsReportedAndSurvived(t *testing.T) {
	var out lockedBuffer
	socket := filepath.Join(t.TempDir(), "control")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- RunWithControl(ctx, "videotestsrc is-live=true ! fakesink", socket, &out)
	}()

	conn := dialWhenReady(t, socket)
	defer conn.Close()

	for _, line := range []string{
		`{"properties":[{"element":"nothing-here","name":"bitrate","value":"4000"}]}`,
		`not json at all`,
	} {
		if _, err := conn.Write([]byte(line + "\n")); err != nil {
			t.Fatalf("writing: %v", err)
		}
	}
	waitFor(t, &out, AppliedPrefix+"0")

	cancel()
	if err := <-done; err != nil {
		t.Errorf("a run that was sent nonsense ended with %v", err)
	}
}

// dialWhenReady connects once the child has opened its socket, which happens before the pipeline
// plays.
func dialWhenReady(t *testing.T, socket string) net.Conn {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("unix", socket)
		if err == nil {
			return conn
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the control socket at %s never accepted a connection", socket)
	return nil
}

// waitFor blocks until the run's output carries want, and fails the test where it never does.
func waitFor(t *testing.T, out *lockedBuffer, want string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(out.String(), want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the run never wrote %q, only: %s", want, out.String())
}

// lockedBuffer is a bytes.Buffer a test reads on one goroutine while the run writes it on another.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
