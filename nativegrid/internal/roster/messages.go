package roster

import (
	"encoding/json"
	"io"
	"sync"

	"bjoernblessin.de/go-utils/util/assert"
)

// The kinds of message the window writes on its stdout, carried in every line as "type".
// One pipe holds both kinds, so the discriminator is what keeps a reader from taking one for the other,
// and what lets it skip a kind it does not know instead of refusing the line.
//
// The reading half is watch.ParseGridMessage in desktop/watch/grid.go.
// The two packages are the two halves of one contract and name each other,
// because no Go type crosses the module boundary.
const (
	KindWatchLeg = "watch-leg"
	KindWatchSet = "watch-set"
)

// legLine and setLine put the discriminator in front of one payload's fields.
// The payload is embedded rather than nested, so a line stays a flat object and keeps the payload's own field names.
type legLine struct {
	Kind string `json:"type"`
	Request
}

type setLine struct {
	Kind string `json:"type"`
	Status
}

// writeMu serializes the stdout lines.
// The two kinds come from different callers, and half a line interleaved with another is unrecoverable for the reader.
var writeMu sync.Mutex

// writeLine writes one message as a single JSON line.
// A message that does not marshal is a bug in this package's own types, not a condition to report.
func writeLine(w io.Writer, msg any) error {
	line, err := json.Marshal(msg)
	assert.IsNil(err, "a stdout message marshals")

	writeMu.Lock()
	defer writeMu.Unlock()
	_, err = w.Write(append(line, '\n'))
	return err
}
