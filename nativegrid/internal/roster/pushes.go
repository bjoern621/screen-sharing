package roster

import (
	"bufio"
	"io"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// pushBuffer sizes the line scanner: rosters are small, but a source fragment
// per stream grows with the roster, so the limit is generous rather than tuned.
const (
	pushBufferInitial = 64 * 1024
	pushBufferMax     = 4 * 1024 * 1024
)

// Pushes applies every push readable from r, one Config JSON per line, each the
// full set of live streams and the app state beside it. A malformed line is
// reported and skipped, so one bad push costs nothing beyond itself.
//
// apply runs on the reading goroutine, not on the UI loop; a caller that owns
// widgets hops it there itself. The call blocks until r ends, which is fine
// because the app kills this process when it is done with it.
func Pushes(r io.Reader, apply func(Config)) {
	assert.IsNotNil(r, "roster pushes need a reader")
	assert.IsNotNil(apply, "roster pushes need a sink")

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, pushBufferInitial), pushBufferMax)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		cfg, err := Parse(line)
		if err != nil {
			logger.Warnf("roster push ignored: %v", err)
			continue
		}
		logger.Debugf("roster push with %d streams", len(cfg.Streams))
		apply(cfg)
	}
	if err := sc.Err(); err != nil {
		logger.Warnf("roster pushes ended: %v", err)
	}
}
