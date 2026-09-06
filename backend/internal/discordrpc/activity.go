package discordrpc

import (
	"time"

	"bjoernblessin.de/go-utils/util/assert"
)

// Activity is what Discord draws under this application's name on a member's profile.
//
// The zero value is a profile with nothing on it, which ClearActivity states.
type Activity struct {
	// Details is the first line Discord draws, State the second.
	Details string
	State   string
	// Readers and Members are the party, which Discord draws as "1 of 4".
	// Both zero leaves the party off, a party of nobody being a figure about nothing.
	Readers int
	Members int
	// Start dates the timer Discord counts up from, zero for an activity with none.
	Start time.Time
}

// activityWatching is the type Discord draws as "Watching".
//
// Type 1 carries the purple streaming badge and Discord grants it to a Twitch or YouTube address
// alone, so it is unreachable from an activity stated over this socket (docs/discord-mode.md).
const activityWatching = 3

// activityMessage is the activity in the shape SET_ACTIVITY takes.
type activityMessage struct {
	Type       int                `json:"type"`
	Details    string             `json:"details,omitempty"`
	State      string             `json:"state,omitempty"`
	Party      *partyMessage      `json:"party,omitempty"`
	Timestamps *timestampsMessage `json:"timestamps,omitempty"`
}

// partyMessage is the audience. Size is the pair Discord draws, current first.
type partyMessage struct {
	Size [2]int `json:"size"`
}

// timestampsMessage dates the timer. Seconds since the epoch, which is what this socket takes,
// where Discord's gateway carries the same field in milliseconds.
type timestampsMessage struct {
	Start int64 `json:"start,omitempty"`
}

// setActivity is the one command this speaks, and setActivityArgs what it carries.
// A nil activity clears the profile.
// The process id is what Discord ties the activity to, so an app that exits takes its own off.
type setActivity struct {
	Cmd   string          `json:"cmd"`
	Nonce string          `json:"nonce"`
	Args  setActivityArgs `json:"args"`
}

type setActivityArgs struct {
	Pid      int              `json:"pid"`
	Activity *activityMessage `json:"activity"`
}

// message is the activity as the wire carries it.
//
// The comparison deciding whether a send is needed runs on this rather than on the struct above:
// what the connection states is the JSON, and a nonce differing per command would compare unequal
// every pass (client.go).
func (a Activity) message() activityMessage {
	assert.Assert(a.Readers >= 0 && a.Members >= 0, "a party counts nobody twice over", a.Readers, a.Members)

	msg := activityMessage{
		Type:    activityWatching,
		Details: a.Details,
		State:   a.State,
	}
	if a.Members > 0 {
		msg.Party = &partyMessage{Size: [2]int{a.Readers, a.Members}}
	}
	if !a.Start.IsZero() {
		msg.Timestamps = &timestampsMessage{Start: a.Start.Unix()}
	}
	return msg
}
