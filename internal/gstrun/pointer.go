package gstrun

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"bjoernblessin.de/screenshare/internal/pointer"
)

// Reporting where the pointer is.
//
// It is the publish child's job and not the backend's, because on the session this exists
// for the position is not something any other process can ask for: a Wayland client cannot
// read the pointer outside its own surfaces, and what does know is the cursor metadata
// PipeWire carries beside each frame, which only the process holding the capture can read.
// The X11 reader is the same job on a session that will answer a question, and it is written
// first because it can be (internal/pointer).
//
// The position goes out on standard output beside the caps, under a prefix of its own. One
// stream carrying three kinds of line is what keeps the child's contract to a pipe and three
// prefixes rather than a socket per report, and the parent's reader already skips a line it
// does not recognise.
//
// The rate is the reader's and not the pipeline's, which is the whole reason the pointer is
// sent instead of drawn: a position costs no frame, so it moves as fast as it is worth
// moving, and the moment on each line is what lets a viewer hold it back to the frame it
// belongs to.

// PointerPrefix leads the line one position is reported on: x, y, the moment in Unix
// nanoseconds, and whether the pointer is over the captured surface at all.
const PointerPrefix = "screenshare-pointer "

// PointerFlag is how the runner is told to report positions, as an argument after the
// subcommand (cmd/backend). A run given none reports none, which is every run whose cursor
// mode draws the pointer into the frames or leaves it out.
const PointerFlag = "--pointer"

// reportPointer writes one position per tick until the context ends.
//
// A reader that will not answer stops the loop rather than writing nothing forever: on a
// session with no X server there is no position to have, and the capture table refuses the
// mode on the backends that read the screen through X anyway.
func reportPointer(ctx context.Context, out io.Writer) {
	reader, ok := pointer.NewX11()
	if !ok {
		return
	}
	defer reader.Close()

	ticker := time.NewTicker(pointer.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p, answered := reader.Read()
			if !answered {
				return
			}
			fmt.Fprintf(out, "%s%d %d %d %t\n",
				PointerPrefix, p.X, p.Y, p.At.UnixNano(), p.Visible)
		}
	}
}

// ParsePointer reads one reported position back, and false for a line that is not one.
//
// It lives beside the writer so the two spellings are one: the parent parses what this child
// writes, and a format written in two places is one that drifts the first time a field is
// added.
func ParsePointer(line string) (pointer.Position, bool) {
	rest, ok := strings.CutPrefix(line, PointerPrefix)
	if !ok {
		return pointer.Position{}, false
	}
	fields := strings.Fields(rest)
	if len(fields) != 4 {
		return pointer.Position{}, false
	}

	x, err := strconv.Atoi(fields[0])
	if err != nil {
		return pointer.Position{}, false
	}
	y, err := strconv.Atoi(fields[1])
	if err != nil {
		return pointer.Position{}, false
	}
	nanos, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil {
		return pointer.Position{}, false
	}
	visible, err := strconv.ParseBool(fields[3])
	if err != nil {
		return pointer.Position{}, false
	}
	return pointer.Position{X: x, Y: y, At: time.Unix(0, nanos), Visible: visible}, true
}
