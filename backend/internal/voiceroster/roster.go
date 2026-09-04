// Package voiceroster is the one copy of who stands in which voice channel.
//
// The gateway feeds it and everything else reads it,
// so the bot's view of Discord exists in exactly one place.
// Apply names the state it wants true, a user standing here or nowhere,
// and applying a state already true changes nothing and fires nothing.
//
// The leave callback is the enforcement trigger:
// it fires once per channel actually left, on a disconnect and on a move,
// and carries the presence as it stood so the listener knows which channel to act on.
package voiceroster

import (
	"sync"

	"bjoernblessin.de/go-utils/util/assert"
)

// Presence is one user standing in one voice channel.
// A ChannelID of "" is the user in no channel, which Apply takes as a disconnect.
type Presence struct {
	UserID    string
	GuildID   string
	ChannelID string
	// DisplayName is the user's name as the channel shows it: nick, or global name behind it.
	DisplayName string
	// GuildName and ChannelName label the place for an answer a person reads.
	// Carried beside the ids rather than resolved per read,
	// the gateway holding them at the moment it feeds a presence.
	GuildName   string
	ChannelName string
}

// Roster holds every known presence. Safe for concurrent use.
type Roster struct {
	// onLeave fires after mu is released, so a listener may read this roster freely.
	onLeave func(left Presence)

	mu     sync.Mutex
	byUser map[string]Presence
}

// New is an empty roster.
// onLeave may be nil for a reader that enforces nothing.
func New(onLeave func(left Presence)) *Roster {
	return &Roster{
		onLeave: onLeave,
		byUser:  map[string]Presence{},
	}
}

// Apply states where one user is, replacing whatever was known about them.
//
// Idempotent: a presence already true is left as it is.
// A move fires the leave for the channel the user stood in,
// and a rename in place fires nothing, no channel having been left.
func (r *Roster) Apply(p Presence) {
	assert.Assert(p.UserID != "", "a presence names the user it places")
	assert.Assert(p.ChannelID == "" || p.GuildID != "",
		"a channel stands in a guild", p.ChannelID)

	r.mu.Lock()
	held, known := r.byUser[p.UserID]
	if known && held == p {
		r.mu.Unlock()
		return
	}
	if p.ChannelID == "" {
		delete(r.byUser, p.UserID)
	} else {
		r.byUser[p.UserID] = p
	}
	movedOut := known && held.ChannelID != "" && held.ChannelID != p.ChannelID
	r.mu.Unlock()

	if movedOut && r.onLeave != nil {
		r.onLeave(held)
	}
}

// DropGuild forgets every presence in one guild, leaving each of its channels.
//
// The gateway calls it where the bot loses a guild:
// without it those occupants stand in the roster forever,
// and their channels' sessions never count as empty.
func (r *Roster) DropGuild(guildID string) {
	assert.Assert(guildID != "", "a drop names the guild it forgets")

	r.mu.Lock()
	var left []Presence
	for userID, p := range r.byUser {
		if p.GuildID == guildID {
			left = append(left, p)
			delete(r.byUser, userID)
		}
	}
	r.mu.Unlock()

	if r.onLeave == nil {
		return
	}
	for _, p := range left {
		r.onLeave(p)
	}
}

// Where answers the presence this roster holds for one user, ok=false for a user in no channel.
func (r *Roster) Where(userID string) (Presence, bool) {
	assert.Assert(userID != "", "a lookup names the user it asks about")

	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.byUser[userID]
	return p, ok
}

// Occupants counts the users standing in one channel.
// Zero is a channel nobody stands in, a channel that does not exist reading the same.
func (r *Roster) Occupants(guildID, channelID string) int {
	assert.Assert(guildID != "" && channelID != "", "a count names the channel it counts")

	r.mu.Lock()
	defer r.mu.Unlock()

	n := 0
	for _, p := range r.byUser {
		if p.GuildID == guildID && p.ChannelID == channelID {
			n++
		}
	}
	return n
}
