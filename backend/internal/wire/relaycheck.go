package wire

import (
	"google.golang.org/protobuf/proto"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/reach"
	"bjoernblessin.de/screenshare/internal/text"
)

// relayLegVerdicts is one row per verdict a check states.
//
// A table rather than a switch: a verdict added to internal/reach and left out here fails
// the lookup below.
// A default arm would cross as the zero the contract keeps for "not set", drawing as a row a shell
// cannot mark.
var relayLegVerdicts = map[reach.Verdict]screensharev1.RelayLegVerdict{
	reach.Reachable:   screensharev1.RelayLegVerdict_RELAY_LEG_VERDICT_REACHABLE,
	reach.Unreachable: screensharev1.RelayLegVerdict_RELAY_LEG_VERDICT_UNREACHABLE,
	reach.Unaddressed: screensharev1.RelayLegVerdict_RELAY_LEG_VERDICT_UNADDRESSED,
}

// relayLegReasons is why a leg went undialled, as the statement a shell writes the sentence for.
// ReasonNone is absent: a dialled leg, whose row carries what came back instead.
var relayLegReasons = map[reach.Reason]screensharev1.TextCode{
	reach.ReasonNoRelay: screensharev1.TextCode_TEXT_CODE_RELAY_LEG_NO_RELAY,
}

// RelayLegs carries a check across, one message per leg, in the order the check answered.
func RelayLegs(results []reach.Result) []*screensharev1.RelayLeg {
	out := make([]*screensharev1.RelayLeg, 0, len(results))
	for _, r := range results {
		assert.Assert(r.Leg != "", "a checked leg names itself")

		verdict, ok := relayLegVerdicts[r.Verdict]
		assert.Assert(ok, "every verdict a check states crosses as one of the contract's", r.Leg, r.Verdict)

		leg := &screensharev1.RelayLeg{
			Leg:     r.Leg,
			Address: r.Address,
			Verdict: verdict,
			// The listener's own words, or the dial's, raw:
			// another machine's string is data rather than this app's vocabulary
			// (api/proto/screenshare/v1/text.proto).
			Detail:      r.Detail,
			Unaddressed: relayLegUnaddressed(r.Unaddressed),
		}
		// Absent rather than nought where nothing was dialled:
		// a wait of zero milliseconds is a figure, and no wait at all is not one.
		if r.Took > 0 {
			leg.WaitedMs = proto.Int64(r.Took.Milliseconds())
		}

		unaddressed := leg.GetVerdict() == screensharev1.RelayLegVerdict_RELAY_LEG_VERDICT_UNADDRESSED
		assert.Assert(unaddressed == (leg.GetUnaddressed() != nil),
			"a leg says why it went undialled exactly where it did", leg.GetLeg(), leg.GetVerdict())
		assert.Assert(unaddressed == (leg.GetAddress() == ""),
			"a leg names where it was dialled exactly where it was", leg.GetLeg(), leg.GetVerdict())

		out = append(out, leg)
	}
	return out
}

// relayLegUnaddressed is the statement behind an undialled leg, and nil for a leg that was dialled.
func relayLegUnaddressed(reason reach.Reason) *screensharev1.Text {
	if reason == reach.ReasonNone {
		return nil
	}

	code, ok := relayLegReasons[reason]
	assert.Assert(ok, "every reason a leg goes undialled for crosses as a statement", reason)
	return text.Of(code)
}
