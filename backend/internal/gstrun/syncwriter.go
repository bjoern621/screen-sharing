package gstrun

import (
	"io"
	"sync"
)

// syncWriter serializes concurrent writes onto one writer.
//
// A run reports on its writer from goroutines that coordinate with nothing: the bus loop
// on the calling goroutine, the control server, the pointer reader and the delay reporter each
// write when they have something to say.
// The parent reads that stream a line at a time and matches each line against a prefix
// (publish/gststats.go), so two writes landing inside one another cost both lines rather than
// reordering them, and a caps line lost that way is the one that says whether the surface is HDR.
//
// Not left to the writer underneath either.
// os.Stdout is one write syscall per call and a pipe holds that atomic up to PIPE_BUF, which covers
// these lines by luck of their length rather than by anything stated here, and a bytes.Buffer, what
// the tests pass, has no such property.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}
