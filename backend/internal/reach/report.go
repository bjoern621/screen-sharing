package reach

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
)

// Subcommand runs a check from the command line: "backend check-relay".
//
// A question rather than a start: dials, prints and exits, without bringing up the app it shares
// an executable with.
const Subcommand = "check-relay"

// The mark a verdict prints under.
var marks = map[Verdict]string{
	Reachable:   "✓",
	Unreachable: "✗",
	Unaddressed: "–",
}

// Why a leg was dialled nowhere, in the words a reader gets.
//
// Words live here because the only reader is the terminal a check runs from.
// Rows crossing to a shell carry the code and the shell writes the sentence (docs/ipc-api.md).
var reasons = map[Reason]string{
	ReasonNoRelay:      "no relay is named in the settings",
	ReasonLoopbackOnly: "answers on the relay's own machine alone",
}

// Report writes a line per leg: mark, leg, where it was dialled, what came back, how long it
// waited.
//
// In Check's order, so the same relay prints the same way twice.
func Report(w io.Writer, results []Result) error {
	assert.IsNotNil(w, "a report names where it is written")

	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, r := range results {
		mark, ok := marks[r.Verdict]
		assert.Assert(ok, "every verdict prints under a mark of its own", r.Leg, r.Verdict)

		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", mark, r.Leg, r.Address, detailOf(r), waitOf(r))
	}
	return table.Flush()
}

// Failed reports whether a leg this deployment addresses did not answer, carried as an exit status
// so a script reads the answer without parsing the table.
// A leg addressed nowhere is no failure: nothing was asked of it.
func Failed(results []Result) bool {
	for _, r := range results {
		if r.Verdict == Unreachable {
			return true
		}
	}
	return false
}

// detailOf is what the row says: the listener's own words, or why it was never asked.
func detailOf(r Result) string {
	if r.Verdict != Unaddressed {
		return r.Detail
	}

	reason, ok := reasons[r.Unaddressed]
	assert.Assert(ok, "every reason a leg goes undialled for is spelled out", r.Leg, r.Unaddressed)
	return reason
}

// waitOf is how long the probe waited, and nothing where nothing was dialled.
// Rounded to the millisecond, a figure for comparing legs rather than measuring with.
func waitOf(r Result) string {
	if r.Took == 0 {
		return ""
	}
	return r.Took.Round(time.Millisecond).String()
}
