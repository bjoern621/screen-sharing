package reach

import (
	"strings"
	"testing"
	"time"
)

// A row is read at a glance, so the mark tells the verdicts apart and each carries the words behind
// it.
func TestAReportMarksEveryVerdictAndSaysWhy(t *testing.T) {
	var out strings.Builder
	err := Report(&out, []Result{
		{Leg: "rtsp", Address: "rtsps://relay:8322", Verdict: Reachable, Detail: "RTSP/1.0 200 OK", Took: 41 * time.Millisecond},
		{Leg: "srt", Address: "srt://relay:8890", Verdict: Unreachable, Detail: "i/o timeout", Took: 5 * time.Second},
		{Leg: legGroups, Verdict: Unaddressed, Unused: ReasonNoRelay},
		{Leg: legDiscord, Address: "https://relay/discord", Verdict: Unused, Detail: "200 OK", Unused: ReasonDiscordOff, Took: time.Millisecond},
	})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}

	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("%d lines, want one per leg", len(lines))
	}
	for i, want := range []string{"✓", "✗", "–", "!"} {
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
	if !strings.Contains(lines[2], reasons[ReasonNoRelay]) {
		t.Errorf("the unaddressed row is %q, want why nothing was dialled", lines[2])
	}
	if !strings.Contains(lines[3], "200 OK") || !strings.Contains(lines[3], reasons[ReasonDiscordOff]) {
		t.Errorf("the unused row is %q, want the answer and why nothing here uses it", lines[3])
	}
}

// A verdict with no mark of its own prints a blank where a reader looks first.
func TestEveryVerdictPrintsUnderAMark(t *testing.T) {
	for _, verdict := range Verdicts {
		if marks[verdict] == "" {
			t.Errorf("%v prints under no mark", verdict)
		}
	}
}

// Exit status is the answer for a caller that reads no table: a leg that did not answer
// is a failure, and a leg nothing here dials or uses is not.
func TestOnlyALegThatDidNotAnswerFails(t *testing.T) {
	if Failed([]Result{{Leg: legGroups, Verdict: Unaddressed, Unused: ReasonNoRelay}}) {
		t.Error("a leg addressed nowhere reads as a failure")
	}
	if Failed([]Result{{Leg: legDiscord, Verdict: Unused, Detail: "200 OK", Unused: ReasonDiscordOff}}) {
		t.Error("a leg that answered and is unused reads as a failure")
	}
	if !Failed([]Result{{Leg: "srt", Verdict: Unreachable}}) {
		t.Error("a leg that did not answer reads as a success")
	}
	if Failed([]Result{{Leg: "srt", Verdict: Reachable}}) {
		t.Error("a leg that answered reads as a failure")
	}
}

// The version a listener named stands in its row, which is what a relay behind the app it serves
// is read off from a terminal.
func TestAReportCarriesTheVersionAListenerNamed(t *testing.T) {
	var out strings.Builder
	err := Report(&out, []Result{
		{Leg: legGroups, Address: "https://relay", Verdict: Reachable, Detail: "200 OK", Version: "0.6.1", Took: time.Millisecond},
		{Leg: "hls", Address: "https://relay", Verdict: Reachable, Detail: "401 Unauthorized", Took: time.Millisecond},
	})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}

	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if !strings.Contains(lines[0], "0.6.1") {
		t.Errorf("the row is %q, want the version the listener named", lines[0])
	}
	if strings.Contains(lines[1], "0.6.1") {
		t.Errorf("the row is %q, want no version on a listener that named none", lines[1])
	}
}
