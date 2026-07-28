package roster

import (
	"io"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// Status is what the window has open: the names of the streams with a tile, sorted.
// It is the whole set rather than what changed, so the app replaces what it held and neither side merges.
//
// It travels as a KindWatchSet line; the consuming half is watch.GridStatus in desktop/watch/grid.go.
type Status struct {
	Watching []string `json:"watching"`
}

// Report delivers one status to the app.
// It is a report and not a question: what the window watches is decided here, and nothing waits for an answer.
type Report func(Status)

// Reporter writes statuses to w, one JSON line each.
// A failed write is reported and dropped, because the next change carries the whole set again.
func Reporter(w io.Writer) Report {
	assert.IsNotNil(w, "statuses need a writer")

	return func(s Status) {
		// An empty set marshals as [] rather than null,
		// so the app reads "nothing is watched" as a set and not as a missing field.
		if s.Watching == nil {
			s.Watching = []string{}
		}
		if err := writeLine(w, setLine{Kind: KindWatchSet, Status: s}); err != nil {
			logger.Warnf("watch set not reported: %v", err)
		}
	}
}

// DiscardReport drops every status, for a run with no app behind it:
// the demo run has nobody to tell what it is watching.
func DiscardReport(Status) {}
