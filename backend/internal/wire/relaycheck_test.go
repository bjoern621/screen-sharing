package wire

import (
	"testing"
	"time"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/reach"
)

// A verdict the contract has no value for would cross as the zero it keeps for "not set",
// which a shell draws as a row it cannot mark.
func TestEveryVerdictCrossesAsOneOfTheContractsOwn(t *testing.T) {
	for _, verdict := range reach.Verdicts {
		result := reach.Result{Leg: "srt", Address: "srt://relay:8890", Verdict: verdict}
		if verdict == reach.Unaddressed {
			result.Address, result.Unaddressed = "", reach.ReasonNoRelay
		}

		legs := RelayLegs([]reach.Result{result})
		if len(legs) != 1 {
			t.Fatalf("%v crossed as %d legs, want one", verdict, len(legs))
		}
		if legs[0].GetVerdict() == screensharev1.RelayLegVerdict_RELAY_LEG_VERDICT_UNSPECIFIED {
			t.Errorf("%v crosses as unspecified", verdict)
		}
	}
}

// A reason with no statement behind it draws as a blank where the row says why nothing was dialled,
// reading as a leg that failed for nothing.
func TestEveryReasonCrossesAsAStatement(t *testing.T) {
	for _, reason := range reach.Reasons {
		legs := RelayLegs([]reach.Result{{Leg: "groups", Verdict: reach.Unaddressed, Unaddressed: reason}})

		if code := legs[0].GetUnaddressed().GetCode(); code == screensharev1.TextCode_TEXT_CODE_UNSPECIFIED {
			t.Errorf("%v crosses with no statement", reason)
		}
	}
}

// What a listener answered is another machine's string,
// so it crosses raw and whole: a reader takes it to a bug report as it stands.
func TestALegCarriesTheListenersOwnWords(t *testing.T) {
	legs := RelayLegs([]reach.Result{{
		Leg:     "rtsp",
		Address: "rtsps://relay:8322",
		Verdict: reach.Reachable,
		Detail:  "RTSP/1.0 200 OK",
		Took:    41 * time.Millisecond,
	}})

	if got := legs[0].GetDetail(); got != "RTSP/1.0 200 OK" {
		t.Errorf("the leg carries %q, want the listener's own words", got)
	}
	if got := legs[0].GetWaitedMs(); got != 41 {
		t.Errorf("the leg waited %d ms, want 41", got)
	}
	if legs[0].GetUnaddressed() != nil {
		t.Error("a dialled leg says why it went undialled")
	}
}

// Nothing dialled is no wait at all, which is absent rather than the figure nought.
func TestAnUndialledLegReportsNoWait(t *testing.T) {
	legs := RelayLegs([]reach.Result{{Leg: "groups", Verdict: reach.Unaddressed, Unaddressed: reach.ReasonNoRelay}})

	if legs[0].WaitedMs != nil {
		t.Errorf("an undialled leg waited %d ms", legs[0].GetWaitedMs())
	}
}
