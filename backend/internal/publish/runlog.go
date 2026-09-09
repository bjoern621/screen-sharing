package publish

import (
	"bytes"
	"io"
	"sync"
)

// What a child says about itself, on its way to the run log and to the tail a failure is shown from.
//
// A child is handed placeholders and puts the values back before it plays
// (internal/transport, childsecrets.go), so it spells them out in its own words:
// a pipeline it echoes, an element error naming the property value it choked on.
// Both land in a file the app offers to open and in the sentence a reader is shown,
// so the run's secrets come out of the stream rather than out of the command line alone.

// redacting hides a run's secrets in everything written through it.
//
// Line by line, because a secret split across two writes is whole in neither:
// what holds no newline yet is kept until one arrives or Flush closes the stream.
type redacting struct {
	to     io.Writer
	redact func(string) string

	mu   sync.Mutex
	held []byte
}

// hiding writes through to, with every secret hidden by redact.
// A nil redact hides nothing, which is what a run carrying no secret takes.
func hiding(to io.Writer, redact func(string) string) *redacting {
	return &redacting{to: to, redact: redact}
}

// Write takes the whole of p and passes on every line it completes.
//
// The count answers len(p) whatever was passed on:
// a caller copying a stream reads a short write as a failure to write,
// where what is held here is written on the next line or by Flush.
func (r *redacting) Write(p []byte) (int, error) {
	if r.redact == nil {
		return r.to.Write(p)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.held = append(r.held, p...)
	cut := bytes.LastIndexByte(r.held, '\n')
	if cut < 0 {
		return len(p), nil
	}

	whole := r.held[:cut+1]
	r.held = append([]byte(nil), r.held[cut+1:]...)
	if _, err := r.to.Write([]byte(r.redact(string(whole)))); err != nil {
		return len(p), err
	}
	return len(p), nil
}

// Flush passes on the last line where the stream ended without one.
func (r *redacting) Flush() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.held) == 0 {
		return nil
	}
	held := r.held
	r.held = nil
	if r.redact != nil {
		held = []byte(r.redact(string(held)))
	}
	_, err := r.to.Write(held)
	return err
}
