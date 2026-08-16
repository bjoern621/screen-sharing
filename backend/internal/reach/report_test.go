package reach

import (
	"strings"
	"testing"
	"time"
)

// A row is read at a glance, so the mark is what tells the three verdicts apart, and each carries
// the words behind it.
func TestAReportMarksEveryVerdictAndSaysWhy(t *testing.T) {
	var out strings.Builder
	err := Report(&out, []Result{
		{Leg: "rtsp", Address: "rtsps://relay:8322", Verdict: Reachable, Detail: "RTSP/1.0 200 OK", Took: 41 * time.Millisecond},
		{Leg: "srt", Address: "srt://relay:8890", Verdict: Unreachable, Detail: "i/o timeout", Took: 5 * time.Second},
		{Leg: legAPI, Verdict: Unaddressed, Unaddressed: ReasonLoopbackOnly},
	})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}

	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("%d lines, want one per leg", len(lines))
	}
	for i, want := range []string{"✓", "✗", "–"} {
		if !strings.HasPrefix(lines[i], want) {
			t.Errorf("line %d is %q, want it marked %q", i, lines[i], want)
		}
	}
	if !strings.Contains(lines[0], "RTSP/1.0 200 OK") || !strings.Contains(lines[0], "41ms") {
		t.Errorf("the reachable row is %q, want the answer and the wait", lines[0])
	}
	if !strings.Contains(lines[1], "i/o timeout") {
		t.Errorf("the unreachable row is %q, want the dial's own words", lines[1])
	}
	if !strings.Contains(lines[2], reasons[ReasonLoopbackOnly]) {
		t.Errorf("the unaddressed row is %q, want why nothing was dialled", lines[2])
	}
}

// The exit status is the answer for a caller that reads no table: a leg that did not answer is a
// failure, and a leg nothing dialled is not.
func TestOnlyALegThatWasDialledCanFail(t *testing.T) {
	if Failed([]Result{{Leg: legAPI, Verdict: Unaddressed, Unaddressed: ReasonLoopbackOnly}}) {
		t.Error("a leg addressed nowhere reads as a failure")
	}
	if !Failed([]Result{{Leg: "srt", Verdict: Unreachable}}) {
		t.Error("a leg that did not answer reads as a success")
	}
	if Failed([]Result{{Leg: "srt", Verdict: Reachable}}) {
		t.Error("a leg that answered reads as a failure")
	}
}
