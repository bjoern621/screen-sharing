package decode

import (
	"os"
	"time"

	"bjoernblessin.de/go-utils/util/logger"
)

// exitTimeout bounds the wait for the host to take its pipelines down.
//
// The host stops every decode before it exits, each bounded by the receive package's own stop
// timeout, and the pipelines stop together rather than one after another.
// A host past this is killed, which leaves its threads wherever they stand.
const exitTimeout = 10 * time.Second

// Close stops the host and removes its socket.
// Idempotent, and what the backend runs on its way out.
//
// The control connection closes first: that is what tells the host to take its pipelines down
// through GStreamer, which a kill would not.
// The wait is the point, a process killed mid-teardown leaving a decoder on the device.
func (c *Client) Close() {
	c.mu.Lock()
	proc, ctrl, dir := c.proc, c.ctrl, c.dir
	c.proc, c.ctrl, c.closed = nil, nil, true
	c.events = map[ID]Events{}
	c.mu.Unlock()

	if ctrl != nil {
		ctrl.Close()
	}
	if proc != nil {
		select {
		case <-proc.exited:
		case <-time.After(exitTimeout):
			logger.Warnf("the decode host did not stop within %s and is being killed", exitTimeout)
			proc.cmd.Process.Kill()
		}
	}
	if dir != "" {
		os.RemoveAll(dir)
	}
}
