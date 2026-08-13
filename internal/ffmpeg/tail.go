package ffmpeg

import (
	"strings"
	"sync"

	"bjoernblessin.de/go-utils/util/assert"
)

// sanitizeTag maps tag to what a filename may hold.
// A watch tag carries the stream name, which is another party's text.
//
// The postcondition is the point of the function rather than a restatement of it: the tag reaches a
// path, so a separator surviving the mapping is a stream name choosing where the log is written.
func sanitizeTag(tag string) string {
	out := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, tag)

	assert.Assert(!strings.ContainsAny(out, `/\.`), "a sanitized tag names no path of its own", out)
	return out
}

// tailBuffer keeps the last max bytes written to it and drops what runs off the front, which is what
// carries the end of a child's stderr into an exit message without holding a run's whole output.
// The end and not the start, since a child states its failure on the way out.
type tailBuffer struct {
	// mu guards buf across the goroutine copying stderr and the one reporting the exit.
	mu  sync.Mutex
	buf []byte
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	assert.Assert(t.max > 0, "a tail keeps some number of bytes", t.max)

	t.mu.Lock()
	defer t.mu.Unlock()

	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}

	assert.Assert(len(t.buf) <= t.max, "a tail holds no more than it keeps", len(t.buf), t.max)
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}
