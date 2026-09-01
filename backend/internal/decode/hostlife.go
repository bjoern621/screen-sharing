package decode

import (
	"fmt"
	"os"

	"bjoernblessin.de/go-utils/util/logger"
)

// What the host reports back, and what the backend does when it stops answering.

// hostGone drops a control connection that failed, so the next open spawns a host.
func (c *Client) hostGone(err error) {
	c.mu.Lock()
	ctrl := c.ctrl
	c.ctrl = nil
	c.mu.Unlock()

	if ctrl != nil {
		ctrl.Close()
		logger.Warnf("the decode host stopped answering: %v", err)
	}
}

// hostExited reports every decode the host was running.
//
// A GPU reset aborts the host where it stands, so this is an ordinary way for a decode to end
// rather than an exceptional one, and each is reported the way a pipeline that stopped by itself
// is.
// The log is named because it holds what the host said on its way down, which is the one place
// the driver's own message lands.
func (c *Client) hostExited(dir, logPath string) {
	c.mu.Lock()
	ended := c.events
	c.events = map[ID]Events{}
	closed := c.closed
	c.ctrl, c.proc, c.socket = nil, nil, ""
	c.mu.Unlock()

	// A host that exited answers no dial, and the next spawn makes its own directory.
	os.RemoveAll(dir)

	if closed || len(ended) == 0 {
		return
	}
	logger.Warnf("the decode host exited with %d decode(s) running, its log is at %s", len(ended), logPath)
	for id, ev := range ended {
		logger.Debugf("the decode of %s ended with its host", id)
		if ev.OnEnd != nil {
			ev.OnEnd(fmt.Sprintf("the decoding process stopped, its log is at %s", logPath))
		}
	}
}

// readLifecycle delivers what decodes report unasked, until the host goes.
//
// A message about a decode this side has forgotten is dropped: a stop takes the callbacks away
// first, so an end announced afterwards would report a decode the caller closed.
func (c *Client) readLifecycle(conn *conn) {
	defer conn.Close()

	for {
		var msg lifecycleMessage
		if err := conn.recv(&msg); err != nil {
			return
		}

		c.mu.Lock()
		ev, present := c.events[msg.ID]
		c.mu.Unlock()

		if !present {
			continue
		}
		switch msg.Kind {
		case lifeLive:
			if ev.OnLive != nil {
				ev.OnLive()
			}
		case lifeEnd:
			if ev.OnEnd != nil {
				ev.OnEnd(msg.Message)
			}
		}
	}
}
