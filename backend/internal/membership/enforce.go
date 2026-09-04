package membership

import (
	"slices"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/relay"
)

// Relay is the connection list and the kick, as enforcement needs them.
//
// An interface rather than the client:
// the client is addressed by host and port, which is one deployment's answer,
// and a test states a relay's connections instead of serving them.
type Relay interface {
	Sessions() ([]relay.Session, []relay.Unread)
	Kick(segment, id string) error
}

// Connection is one connection as an answer names it.
//
// What a caller can act on, and nothing else:
// which stream of the group, over which leg, in which direction, and whose it is where that is known.
// Not the relay's own session id and not the address it came from.
// A caller here holds a group key, which is what lets somebody join,
// and neither figure is something a member is owed:
// one is an operator's handle on the relay and the other is where another member is sitting.
// The stream index draws the same line,
// answering what a viewer may open and never who else is reading (groupsvc.Stream).
type Connection struct {
	// Stream is the name inside the group, which is what the index answers under "name".
	Stream    string `json:"stream"`
	Transport string `json:"transport,omitempty"`
	// State is "publish" or "read", and empty on a leg whose connections are only ever readers.
	State string `json:"state,omitempty"`
	// Member is the display name of the live member whose id this connection's subject matches,
	// and empty where no live member of this group derives it.
	Member string `json:"member,omitempty"`
}

// Result is what one run did.
//
// Kept and Kicked are counted from the same sweep,
// so a run reports the whole of what it saw rather than the half it acted on.
type Result struct {
	Prefix string `json:"prefix"`
	// Enforced is false for a group with no live member,
	// where a run is a no-op rather than a removal of everybody.
	Enforced bool `json:"enforced"`
	// Members is what the live members call themselves, for a person reading a run.
	Members []string     `json:"members"`
	Kicked  []Connection `json:"kicked"`
	Failed  []Failed     `json:"failed,omitempty"`
	Kept    int          `json:"kept"`
	// Unread names the relay's lists that would not answer.
	// A run that could not read every list may have missed the connection that mattered,
	// and this is what says so.
	Unread []relay.Unread `json:"unread,omitempty"`
}

// Failed is a connection the relay would not close, and its own words about why.
// Reported rather than counted as a removal: the member is still watching.
type Failed struct {
	Connection
	Reason string `json:"reason"`
}

// Reconcile closes every connection under the prefix that no live member of that group holds.
//
// Idempotent by construction: it acts on what the relay reports against the leases as they stand,
// so a second run over an unchanged pair finds nothing to do.
// A refused kick is carried in the Result rather than retried here,
// a caller that reports a removal which did not happen being worse than one that says so.
func (r *Registry) Reconcile(prefix string) Result {
	assert.Assert(prefix != "", "a run names the group it enforces")

	result, _ := r.sweep(prefix)
	return result
}

// sweep is Reconcile beside the connections it left open,
// which is where an answer reads who is publishing from.
func (r *Registry) sweep(prefix string) (Result, []relay.Session) {
	now := r.now()
	r.mu.Lock()
	live := map[string]string{}
	for id, held := range r.held[prefix] {
		if held.expires.After(now) {
			live[id] = held.displayName
		}
	}
	r.mu.Unlock()

	if len(live) == 0 {
		return Result{Prefix: prefix, Members: []string{}, Kicked: []Connection{}}, nil
	}

	sessions, unread := r.connections(prefix)
	result := Result{
		Prefix:   prefix,
		Enforced: true,
		Members:  names(live),
		Kicked:   []Connection{},
		Unread:   unread,
	}
	kept := []relay.Session{}

	for _, session := range sessions {
		if !strings.HasPrefix(session.Path, prefix) {
			continue
		}
		// The leases are read again per connection rather than off the snapshot above:
		// reading the relay and closing a connection are both round trips,
		// and a member that states presence across one holds a lease by the moment its kick would land.
		name, member := r.member(prefix, session.User)
		if member {
			result.Kept++
			kept = append(kept, session)
			continue
		}

		assert.Assert(session.Segment != "", "a connection to close names the list it is on", session.ID)
		closing := connectionOf(session, prefix, name)
		if err := r.relay.Kick(session.Segment, session.ID); err != nil {
			r.refused.add(session.Transport)
			result.Failed = append(result.Failed, Failed{Connection: closing, Reason: err.Error()})
			continue
		}
		r.kicked.add(session.Transport)
		r.gone(prefix, session.ID)
		result.Kicked = append(result.Kicked, closing)
	}

	for _, closed := range result.Kicked {
		assert.Assert(closed.Member == "", "a closed connection was held by no live member", closed.Stream)
	}
	return result, kept
}

// connectionOf names one of the relay's connections the way an answer carries it.
func connectionOf(session relay.Session, prefix, member string) Connection {
	return Connection{
		Stream:    strings.TrimPrefix(session.Path, prefix),
		Transport: session.Transport,
		State:     session.State,
		Member:    member,
	}
}

// names is what a group's live members call themselves, in one order so two runs read alike.
func names(live map[string]string) []string {
	out := make([]string, 0, len(live))
	for _, name := range live {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}
