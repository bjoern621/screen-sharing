// Package roster holds who a group's members are, and closes what anybody else holds.
//
// Membership cannot be enforced by withholding a token. The relay reads one at the handshake and
// not again, so a session outlives its token and a client that is kicked opens another with the
// same one (internal/relay, sessions.go). Removal is therefore two things at once: whatever
// distributes keys stops issuing tokens to the member who left, and this closes what they already
// hold.
//
// This is the one place in the service that keeps anything, and it keeps the fact rather than a copy
// of it: nothing else knows which members a group has, because the answer comes from whatever serves
// the voice channel and is pushed here. What is never kept is the relay's side, which is read
// through on every run.
//
// Enforcement is stated, never stepped. A caller says who the members are now, and a run brings the
// relay to that. A second run over an unchanged roster closes nothing, which is what lets it run on
// every membership change and on every read the relay reports.
//
// A group nobody stated a roster for is not enforced at all. Membership this service was never told
// is not the same as a group with no members, and treating the two alike would close every
// connection on the relay the moment one group started enforcing.
package roster

import (
	"strings"
	"sync"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/relay"
)

// Relay is the connection list and the kick, as enforcement needs them.
//
// An interface rather than the client: the client is addressed by host and port, which is one
// deployment's answer, and this way a test states a relay's connections instead of serving them.
type Relay interface {
	Sessions() ([]relay.Session, []relay.Unread)
	Kick(segment, id string) error
}

// Registry is every group this service enforces.
// Safe for concurrent use: mu guards held, and the relay is reached outside it.
type Registry struct {
	mu sync.Mutex
	// held is each enforced group's members, keyed by the group's path prefix, which is what a
	// connection's path is matched against.
	held  map[string]members
	relay Relay
}

// members is one group's roster: the names as they were stated, and the ids those derive.
//
// Both, because the two answer different questions. The relay knows a member by the id its token's
// subject carries, and a person reading a roster knows them by the name.
type members struct {
	names []string
	// byID maps a member id to the name it was derived from.
	byID map[string]string
}

// Connection is one connection as an answer names it.
//
// What a caller can act on, and nothing else: which stream of the group, over which leg, in which
// direction, and whose it is where that is known.
// Not the relay's own session id and not the address it came from. A caller here holds a group key,
// which is membership, and neither figure is something membership answers: one is an operator's
// handle on the relay and the other is where another member is sitting.
// The stream index draws the same line, answering what a viewer may open and never who else is
// reading (groupsvc.Stream).
type Connection struct {
	// Stream is the name inside the group, which is what the index answers under "name".
	Stream    string `json:"stream"`
	Transport string `json:"transport,omitempty"`
	// State is "publish" or "read", and empty on a leg whose connections are only ever readers.
	State string `json:"state,omitempty"`
	// Member is the name whose id this connection's subject matches, and empty where no member of
	// this group derives it.
	Member string `json:"member,omitempty"`
}

// Result is what one run did.
//
// Kept and Kicked are counted from the same sweep, so a run reports the whole of what it saw rather
// than the half it acted on.
type Result struct {
	Prefix string `json:"prefix"`
	// Enforced is false for a group nobody stated a roster for, where a run is a no-op rather than a
	// removal of everybody.
	Enforced bool         `json:"enforced"`
	Members  []string     `json:"members"`
	Kicked   []Connection `json:"kicked"`
	Failed   []Failed     `json:"failed,omitempty"`
	Kept     int          `json:"kept"`
	// Unread names the relay's lists that would not answer. A run that could not read every list may
	// have missed the connection that mattered, and this is what says so.
	Unread []relay.Unread `json:"unread,omitempty"`
}

// Failed is a connection the relay would not close, and its own words about why.
// Reported rather than counted as a removal: the member is still watching.
type Failed struct {
	Connection
	Reason string `json:"reason"`
}

// View is a group as a reader is shown it: the roster, and who holds each live connection.
type View struct {
	Prefix   string         `json:"prefix"`
	Enforced bool           `json:"enforced"`
	Members  []string       `json:"members"`
	Sessions []Connection   `json:"sessions"`
	Unread   []relay.Unread `json:"unread,omitempty"`
}

func New(live Relay) *Registry {
	assert.IsNotNil(live, "a registry enforces against a relay")

	return &Registry{held: map[string]members{}, relay: live}
}

// Set states who a group's members are now and brings the relay to it.
//
// The whole roster and never a departure, so two callers racing cannot leave a member the last one
// did not name. "Nobody is in the channel" is an empty roster and closes everything, which is a
// different statement from a group this service was never told about.
//
// A name nobody gave is refused, and nothing is stored: a roster holding a member no id derives is
// one that cannot recognise them.
func (r *Registry) Set(key group.Key, names []string) (Result, error) {
	byID := map[string]string{}
	stated := []string{}
	for _, name := range names {
		id, err := key.MemberID(name)
		if err != nil {
			return Result{}, err
		}
		if _, twice := byID[id]; twice {
			continue
		}
		named := strings.TrimSpace(name)
		byID[id] = named
		stated = append(stated, named)
	}

	prefix := key.Prefix()
	r.mu.Lock()
	r.held[prefix] = members{names: stated, byID: byID}
	r.mu.Unlock()

	return r.Reconcile(prefix), nil
}

// Clear stops enforcing a group, and reports whether there was one to stop.
func (r *Registry) Clear(key group.Key) bool {
	prefix := key.Prefix()

	r.mu.Lock()
	defer r.mu.Unlock()

	_, held := r.held[prefix]
	delete(r.held, prefix)
	return held
}

// Reconcile closes every connection under the prefix that no member of that group holds.
//
// Idempotent by construction: it acts on what the relay reports now against the roster as it stands
// now, so a second run over an unchanged pair finds nothing to do.
// A refused kick is carried in the Result rather than retried here, a caller that reports a removal
// which did not happen being worse than one that says so.
func (r *Registry) Reconcile(prefix string) Result {
	assert.Assert(prefix != "", "a run names the group it enforces")

	r.mu.Lock()
	held, enforced := r.held[prefix]
	r.mu.Unlock()

	// Streams under the public prefix are watchable by anybody, so there is no membership to hold
	// them against. No key derives that prefix, which is what makes a roster there unreachable rather
	// than merely unset, and this states the same thing where a path arrives instead of a key.
	if !enforced || prefix == group.PublicPrefix {
		return Result{Prefix: prefix, Members: []string{}, Kicked: []Connection{}}
	}

	live, unread := r.relay.Sessions()
	result := Result{
		Prefix:   prefix,
		Enforced: true,
		Members:  held.names,
		Kicked:   []Connection{},
		Unread:   unread,
	}

	for _, session := range live {
		if !strings.HasPrefix(session.Path, prefix) {
			continue
		}
		if _, member := held.byID[session.User]; member {
			result.Kept++
			continue
		}

		assert.Assert(session.Segment != "", "a connection to close names the list it is on", session.ID)
		closing := held.connection(session, prefix)
		if err := r.relay.Kick(session.Segment, session.ID); err != nil {
			result.Failed = append(result.Failed, Failed{Connection: closing, Reason: err.Error()})
			continue
		}
		result.Kicked = append(result.Kicked, closing)
	}

	for _, closed := range result.Kicked {
		assert.Assert(closed.Member == "", "a closed connection was held by nobody on the roster", closed.Stream)
	}
	return result
}

// connection names one of the relay's connections the way an answer carries it.
func (m members) connection(session relay.Session, prefix string) Connection {
	return Connection{
		Stream:    strings.TrimPrefix(session.Path, prefix),
		Transport: session.Transport,
		State:     session.State,
		Member:    m.byID[session.User],
	}
}

// View is the group's roster beside the connections the relay is carrying for it.
// Read through on every call: what is live is the relay's answer and never this service's memory.
func (r *Registry) View(key group.Key) View {
	prefix := key.Prefix()

	r.mu.Lock()
	held, enforced := r.held[prefix]
	r.mu.Unlock()

	view := View{Prefix: prefix, Enforced: enforced, Members: []string{}, Sessions: []Connection{}}
	if enforced {
		view.Members = held.names
	}

	live, unread := r.relay.Sessions()
	view.Unread = unread
	for _, session := range live {
		if !strings.HasPrefix(session.Path, prefix) {
			continue
		}
		view.Sessions = append(view.Sessions, held.connection(session, prefix))
	}
	return view
}
