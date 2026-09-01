package gstrun

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/pointer"
)

// Reporting where the pointer is.
//
// One reading goes two ways.
// A line on standard output reaches this machine's own screens, at the rate the source answers.
// The stamp on each encoded frame reaches everybody else, at the frame rate, no leg over the relay
// carrying a channel beside the picture (internal/framestamp).
//
// A position leaves on standard output beside the caps, under a prefix of its own.
// One stream carrying three kinds of line keeps the child's contract to a pipe and three prefixes
// rather than a socket per report, and the parent's reader skips a line it does not recognise.

// PointerPrefix leads the line one position is reported on:
// the fraction of the way across the picture and down it,
// the moment in Unix nanoseconds,
// and whether the pointer is over the captured picture at all.
// A line: "screenshare-pointer 0.5 0.25 1700000000123456789 true".
const PointerPrefix = "screenshare-pointer "

// PointerFlag asks the runner for positions, as an argument after the subcommand (cmd/backend).
// A run given none reports none, which is every run whose cursor mode draws the pointer
// into the frames or leaves it out.
const PointerFlag = "--pointer"

// reportPointer writes one position per tick until the context ends, on its own goroutine.
//
// The rate is the source's and not the pipeline's, this being the leg that does not pay a frame
// for a position: it moves as fast as it is worth moving, and the moment on each line lets
// a viewer hold it back to the frame it belongs to.
// The stamp on each frame is the other leg and moves at the frame rate (stamp.go).
func reportPointer(ctx context.Context, hold *pointerHold, out io.Writer) {
	assert.IsNotNil(ctx, "a pointer report runs under a context")
	assert.IsNotNil(out, "a pointer report is written to a writer")

	ticker := time.NewTicker(pointer.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			at, answered := hold.spot()
			if !answered {
				continue
			}
			writePointer(out, at)
		}
	}
}

// writePointer renders one position onto the child's output.
// Beside ParsePointer so the two spellings are one, and separate from the loop so a test can state
// that they are.
func writePointer(out io.Writer, at pointer.Spot) {
	assert.IsNotNil(out, "a position is written to a writer")

	fmt.Fprintf(out, "%s%g %g %d %t\n", PointerPrefix, at.X, at.Y, at.At.UnixNano(), at.Visible)
}

// ParsePointer reads one reported position back, false for a line that is not one.
//
// A line that does not parse answers false rather than asserting.
// The reader is pointed at a child's whole standard output, where an unrelated line is the ordinary
// case rather than a broken contract.
func ParsePointer(line string) (pointer.Spot, bool) {
	rest, ok := strings.CutPrefix(line, PointerPrefix)
	if !ok {
		return pointer.Spot{}, false
	}
	fields := strings.Fields(rest)
	if len(fields) != 4 {
		return pointer.Spot{}, false
	}

	x, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return pointer.Spot{}, false
	}
	y, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return pointer.Spot{}, false
	}
	nanos, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil {
		return pointer.Spot{}, false
	}
	visible, err := strconv.ParseBool(fields[3])
	if err != nil {
		return pointer.Spot{}, false
	}
	return pointer.Spot{X: x, Y: y, At: time.Unix(0, nanos), Visible: visible}, true
}
