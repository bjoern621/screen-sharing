package session

import (
	"fmt"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// retryBackoff is the wait before each reconnect attempt after a pipeline ended,
// and its length the attempt budget.
// It backs off because the usual reason a receive pipeline ends is the relay
// restarting or the publisher dropping out, which takes seconds rather than
// milliseconds.
// It ends because a stream nobody publishes any more must not have a window
// reconnecting to it forever.
var retryBackoff = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second}

// ended is what a player's OnEnd does to the model: reconnect while the budget
// holds, and land in Failed with what it cost once it does not.
//
// A dead receive pipeline recovers nothing by itself, so the recovery is a new
// player on the same fragment.
// The old one stays until that start, which is what keeps the last frame under a
// reconnecting tile.
func (s *Session) ended(i int, gen uint, message string) {
	e := s.at(i)
	if e.attempts >= len(s.retryDelays) {
		e.state = Failed
		e.message = fmt.Sprintf("%s (gave up after %d reconnect attempts)", message, e.attempts)
		logger.Warnf("%q stayed down over %d reconnects: %s", e.stream.Name, e.attempts, message)
		s.notify(Change{Kind: StateChanged, Index: i})
		return
	}

	assert.Assert(e.attempts >= 0 && e.attempts < len(s.retryDelays), "a reconnect attempt indexes the backoff", e.attempts, len(s.retryDelays))

	d := s.retryDelays[e.attempts]
	e.attempts++
	e.state = Reconnecting
	e.message = message
	s.clearStall(i)
	logger.Infof("%q ended (%s), reconnect %d of %d in %s", e.stream.Name, message, e.attempts, len(s.retryDelays), d)
	s.notify(Change{Kind: StateChanged, Index: i})
	s.scheduleRetry(i, gen, d)
}

// scheduleRetry re-opens stream i once d has passed.
// The generation it was scheduled in is carried along, so a retry that fires
// after an unwatch or after another start lands nowhere.
func (s *Session) scheduleRetry(i int, gen uint, d time.Duration) {
	t := s.after(d, func() {
		e := s.at(i)
		e.retry = nil
		if e.gen != gen {
			logger.Debugf("dropped a stale reconnect for %q", e.stream.Name)
			return
		}
		s.start(i)
	})
	if t != nil {
		s.at(i).retry = t
	}
}

// cancelRetry drops the pending reconnect of stream i, if it has one.
func (s *Session) cancelRetry(i int) {
	e := s.at(i)
	if e.retry == nil {
		return
	}
	e.retry.Stop()
	e.retry = nil
}

// after runs f on the UI loop once d has passed, and hands back the timer for a
// caller that has to cancel it.
//
// A zero delay runs f right here instead of arming a timer.
// That is the seam all of the model's own scheduling goes through, so a test
// sets the delays to zero and every deferred step lands inside the call that
// scheduled it, with no timer and no second thread in it.
func (s *Session) after(d time.Duration, f func()) *time.Timer {
	if d <= 0 {
		f()
		return nil
	}
	return time.AfterFunc(d, func() { s.dispatch(f) })
}
