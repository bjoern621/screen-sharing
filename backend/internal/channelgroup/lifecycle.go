package channelgroup

import (
	"time"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/voiceroster"
)

// Leave releases every member the leaver's account holds in the channel left,
// which closes their connections at the relay within the reconcile.
//
// The roster's leave callback, so the cut is seconds behind the channel.
// Idempotent: an account holding nothing there is already the state named.
// Where the event never fires, the lease lapses on its own and the next
// Presence pass releases the stale member, both standing behind this.
func (b *Broker) Leave(left voiceroster.Presence) {
	assert.Assert(left.UserID != "", "a leave names who left")

	b.mu.Lock()
	defer b.mu.Unlock()

	s, held := b.sessions[place{guildID: left.GuildID, channelID: left.ChannelID}]
	if !held {
		return
	}
	for linkSecret, m := range s.members {
		if m.userID == left.UserID {
			b.release(s, linkSecret, m)
		}
	}
}

// Sweep retires every channel empty past the window, releasing whatever is left in it.
//
// The broker's one timer-driven path, run by the process on a ticker.
// A channel occupied again before the window closes keeps its group,
// so everyone dropping and rejoining is a hiccup rather than a new session.
func (b *Broker) Sweep() {
	b.mu.Lock()
	defer b.mu.Unlock()
	assert.IsNotNil(b.occupancy, "a sweep counts occupants against a roster")

	now := b.now()
	for at, s := range b.sessions {
		if b.occupancy.Occupants(at.guildID, at.channelID) > 0 {
			s.emptySince = time.Time{}
			continue
		}
		if s.emptySince.IsZero() {
			s.emptySince = now
			continue
		}
		if now.Sub(s.emptySince) < RetireAfter {
			continue
		}
		// Members still here are installs whose release never landed;
		// their leases have long lapsed, so this is bookkeeping rather than enforcement.
		for linkSecret, m := range s.members {
			b.release(s, linkSecret, m)
		}
		delete(b.sessions, at)
	}
}

// Token trades a link secret for a relay access token under the channel's prefix.
//
// The presence is stated on the same pass, so the token's subject holds a live lease
// and passes the test groupd signs against.
// An account in no channel has no group to grant, which ErrNoChannel says.
func (b *Broker) Token(linkSecret string) (token, prefix string, err error) {
	link, ok := b.links.Resolve(linkSecret)
	if !ok {
		return "", "", ErrUnlinked
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	assert.IsNotNil(b.occupancy, "a trade is confirmed against a roster")

	where, in := b.occupancy.Where(link.UserID)
	if !in {
		return "", "", ErrNoChannel
	}

	s, m, _, err := b.ensureMember(linkSecret, link.UserID, where)
	if err != nil {
		return "", "", err
	}

	signed, err := b.groups.Token(b.base, s.encoded, m.secret.String())
	if err != nil {
		return "", "", err
	}

	assert.Assert(signed != "" && s.key.Prefix() != "", "a trade answers a token under a prefix")
	return signed, s.key.Prefix(), nil
}
