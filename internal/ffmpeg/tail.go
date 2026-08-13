package ffmpeg

import (
	"strings"
	"sync"

	"bjoernblessin.de/go-utils/util/assert"
)

// sanitizeTag makes tag safe to use in a filename (watch tags carry the stream name,
// which is user-controlled).
//
// The postcondition is the whole point of the function rather than a restatement of it: the tag
// reaches a path, so a separator surviving the mapping would be a stream name choosing where the
// log is written.
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

// tailBuffer keeps the last max bytes written to it, used to surface the end of stderr in an exit
// message without holding the whole log in memory.
type tailBuffer struct {
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
