// Package membership holds who is in a group, and closes what anybody else holds.
//
// Membership is a presence lease a member's own app states and refreshes.
// It exists while refreshed and lapses when refreshing stops,
// which makes a member's own machine the one thing that has to be running for them to be in a group.
// Nothing here is persisted: a restart forgets every lease,
// and every live app states its own again within one refresh interval.
//
// A member is known by the id its own secret derives under the group key (internal/group),
// so identity cannot be forged inside a group:
// an app claiming another member's display name still holds its own id and never their membership.
// The display name is a label claimed first-come, and never identity.
//
// Membership cannot be enforced by withholding a token.
// The relay reads one at the handshake and not again,
// so a session outlives its token and a closed client opens another with the same one
// (internal/relay, sessions.go).
// Removal is closing what a lapsed member already holds.
//
// This is the one place in the service that keeps anything, and it keeps two things:
// the leases, and one look at the relay's connections per group, held for SweepWindow.
//
// A run brings the relay to the leases as they stand,
// and a second run over unchanged leases closes nothing,
// which is what lets it run on every statement of presence and on every read the relay reports.
//
// A group with no live members is not enforced at all.
// Membership nobody stated is not the same as a group nobody is in,
// and enforcing the empty case would close the connections of an app that stated no presence.
package membership

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/relay"
)

// Lease is how long one statement of presence holds.
//
// Long enough to survive nine missed passes of the app's 2 s relay poll,
// so a network blip is not a member dropping out of their own group.
// Short enough that an app that stopped, crashed or lost its network
// stops being in the group inside a moment a person would call immediate.
const Lease = 20 * time.Second

// SweepWindow is how often one group's connections are read off the relay.
//
// Every statement of presence and every view of a group reaches them, over a publicly fronted route,
// and one read is a request per connection list with pages behind it.
// Unbounded, that is a whole listing of the relay per member per pass of the app's 2 s poll,
// and one more for anybody who asks the route about a group.
// A request arriving inside the window is answered off the look taken in it,
// so a kick acts on connections at most this old,
// which the next statement past the window and the reaper both correct.
const SweepWindow = time.Second

// ErrNameTaken is what a claim on a name another live member holds answers.
//
// A refusal rather than a rename: the name a member goes by is the one thing another member reads,
// and two of them are two people nobody can tell apart.
var ErrNameTaken = errors.New("that name is taken in this group")

// Registry is every group somebody has stated presence in.
// Safe for concurrent use: mu guards held and looks, and the relay is reached outside it.
type Registry struct {
	mu sync.Mutex
	// held is each group's leases, keyed by the group's path prefix and then by member id.
	// The prefix, because that is what a connection's path is matched against.
	held map[string]map[string]lease
	// looks is each group's last reading of the relay, keyed the same way.
	looks map[string]*look
	relay Relay
	// now is read rather than time.Now called directly,
	// so a test can let a lease lapse without waiting one out.
	now func() time.Time
	// What a scrape counts, guarded by nothing this holds:
	// a count never waits on a sweep (reading.go).
	counted
}

// lease is one member's presence: the name they claimed, and when it stops holding.
type lease struct {
	displayName string
	expires     time.Time
}

// look is what the relay answered one group about its connections, and when.
//
// A copy of a fact somebody else owns, which every other answer here reads through for,
// kept for SweepWindow:
// the alternative is a relay-wide listing per request on a route every member's poll reaches.
type look struct {
	// taking serialises one group's reads, so requests arriving together read the relay once
	// and share what it answered rather than asking it once each.
	taking   sync.Mutex
	at       time.Time
	sessions []relay.Session
	unread   []relay.Unread
}

// Member is one live member as an answer names them.
//
// What another member's app shows in a group card, and nothing else:
// who is here, what they are called, and whether they are sharing something.
// Never who is watching what, the same line the stream index draws (groupsvc.Stream).
type Member struct {
	MemberID    string `json:"memberId"`
	DisplayName string `json:"displayName"`
	// Publishing is read off the relay's connection list on every answer rather than stated by anybody,
	// so a publish that dropped stops showing without a second call.
	Publishing bool `json:"publishing"`
}

// Answer is what one statement, one release or one view says.
//
// The routes render their own fields out of it (internal/groupsvc):
// a claim answers who it is and how long its lease runs,
// a release answers whether there was one, and a view answers the group.
type Answer struct {
	MemberID    string
	DisplayName string
	// Lease is how long the lease just stated runs, and zero where nothing was stated.
	Lease time.Duration
	// Released reports whether there was a live lease to release.
	Released bool
	Members  []Member
	// Unread names the relay's lists that would not answer the look Publishing was read off.
	// A member on one of them carries Publishing false because nothing said otherwise,
	// which is not the same fact as a member sending nothing.
	Unread []relay.Unread
}

func New(live Relay) *Registry {
	assert.IsNotNil(live, "a registry enforces against a relay")

	r := &Registry{
		held:  map[string]map[string]lease{},
		looks: map[string]*look{},
		relay: live,
		now:   time.Now,
	}
	assert.IsNotNil(r.now, "a registry reads the clock its leases are measured on")
	return r
}

// State claims this member's presence in the group and refreshes it, both being the same call.
//
// Idempotent by construction: it names the state it wants true,
// that this member is here under this name until the lease runs out,
// so a second call inside the lease changes nothing but the moment it lapses.
//
// A name another live member holds is ErrNameTaken and nothing is stored,
// a group holding two members under one name being one nobody can read.
// A member claiming the name it already holds is a refresh rather than a second claim.
//
// The group is reconciled before the answer,
// so a lease that lapsed since the last call has its connections closed by the call that noticed
// rather than at the reaper's next sweep.
func (r *Registry) State(groupKey group.Key, secret group.MemberSecret, displayName string) (Answer, error) {
	assert.Assert(len(groupKey) == group.KeyBytes, "presence is stated in a whole group", len(groupKey))
	assert.Assert(len(secret) == group.MemberSecretBytes, "a member states presence with a whole secret", len(secret))

	// The name arrives over HTTP from an app the user configured,
	// so an empty one is an Umgebungsfehler and leaves as an error.
	named := strings.TrimSpace(displayName)
	if named == "" {
		return Answer{}, errors.New("a member of a group states a display name to be known by in it")
	}

	id, prefix := groupKey.MemberID(secret), groupKey.Prefix()
	now := r.now()

	r.mu.Lock()
	for other, held := range r.held[prefix] {
		if other != id && held.displayName == named && held.expires.After(now) {
			r.mu.Unlock()
			return Answer{}, ErrNameTaken
		}
	}
	// Whether this is an arrival is read before the write, a refresh being the same call
	// and the two being what a reader of the churn has to tell apart.
	standing, holds := r.held[prefix][id]
	arriving := !holds || !standing.expires.After(now)
	if r.held[prefix] == nil {
		r.held[prefix] = map[string]lease{}
	}
	r.held[prefix][id] = lease{displayName: named, expires: now.Add(Lease)}
	r.mu.Unlock()

	if arriving {
		r.stated.Add(1)
	}

	run, kept := r.sweep(prefix)

	// Nothing is asserted over the answer.
	// A release crossing this call drops the lease between the write above
	// and the read the answer is built from,
	// this app's own leave meeting the poll it already runs,
	// so a member absent from its own group is two well-formed requests rather than a broken contract.
	return Answer{
		MemberID:    id,
		DisplayName: named,
		Lease:       Lease,
		Members:     r.members(prefix, kept),
		Unread:      run.Unread,
	}, nil
}

// Release drops this member's presence and closes what it held.
//
// Idempotent: a member holding no lease is already in the state this names,
// so it answers Released false and succeeds.
//
// What this member holds is closed by its own id, whether or not the group has anybody left:
// a group with no live member is not enforced,
// so the run that follows would leave the last member holding what it opened.
func (r *Registry) Release(groupKey group.Key, secret group.MemberSecret) (Answer, error) {
	assert.Assert(len(groupKey) == group.KeyBytes, "presence is released in a whole group", len(groupKey))
	assert.Assert(len(secret) == group.MemberSecretBytes, "a member releases presence with a whole secret", len(secret))

	id, prefix := groupKey.MemberID(secret), groupKey.Prefix()
	now := r.now()

	r.mu.Lock()
	held, holds := r.held[prefix][id]
	delete(r.held[prefix], id)
	if len(r.held[prefix]) == 0 {
		delete(r.held, prefix)
	}
	r.mu.Unlock()

	if holds && held.expires.After(now) {
		r.released.Add(1)
	}

	// Only where this registry knew the lease:
	// a member it never held one for is one whose connections no run of it ever accounted for,
	// and reaching the relay on a group key anybody can draw is a listing anybody can ask for.
	if holds {
		r.closeHeld(prefix, id)
	}
	r.Reconcile(prefix)

	// Nothing is asserted over the lease being gone.
	// A statement crossing this call claims it again between the delete above and any read after it,
	// which is the poll this app runs meeting its own leave, and both requests are well formed.
	return Answer{MemberID: id, Released: holds && held.expires.After(now)}, nil
}

// closeHeld closes every connection this member holds under the prefix.
//
// By member id rather than by a run, a run being what the empty group carve-out turns off.
// A refusal is left to the next run and to the reaper:
// the relay keeping a connection open is a condition to act on again
// rather than one to report from a release.
func (r *Registry) closeHeld(prefix, id string) {
	assert.Assert(prefix != "", "a member releases what it holds in a group")
	assert.Assert(id != "", "a member releases what it holds under its own id")

	sessions, _ := r.connections(prefix)
	for _, session := range sessions {
		if session.User != id || !strings.HasPrefix(session.Path, prefix) {
			continue
		}
		if r.relay.Kick(session.Segment, session.ID) != nil {
			r.refused.add(session.Transport)
			continue
		}
		r.kicked.add(session.Transport)
		r.gone(prefix, session.ID)
	}
}

// View is the group's live members, stating nothing.
// Who holds a lease is this registry's fact and who is publishing is the relay's,
// read off a look at most SweepWindow old.
func (r *Registry) View(groupKey group.Key) Answer {
	assert.Assert(len(groupKey) == group.KeyBytes, "a group is viewed by a whole group key", len(groupKey))

	prefix := groupKey.Prefix()
	// A group nobody stated presence in has nobody to read a stream for,
	// and it reaches the relay for nothing.
	// A group key is something anybody can draw,
	// so a look taken for one is a listing anybody can ask for.
	if !r.live(prefix) {
		return Answer{Members: []Member{}}
	}

	live, unread := r.connections(prefix)
	return Answer{Members: r.members(prefix, live), Unread: unread}
}

// connections is what the relay is carrying, as this group last saw it.
//
// Read again where that look is older than SweepWindow, and answered off it otherwise,
// so requests arriving together cost the relay one listing between them.
func (r *Registry) connections(prefix string) ([]relay.Session, []relay.Unread) {
	assert.Assert(prefix != "", "a look at the relay is taken for one group")

	r.mu.Lock()
	taken, holds := r.looks[prefix]
	if !holds {
		taken = &look{}
		r.looks[prefix] = taken
	}
	r.mu.Unlock()

	taken.taking.Lock()
	defer taken.taking.Unlock()

	if !taken.at.IsZero() && r.now().Sub(taken.at) < SweepWindow {
		return taken.sessions, taken.unread
	}
	sessions, unread := r.relay.Sessions()
	// Counted per look rather than per caller,
	// the looks inside one window being one read of the relay that every caller in it shares.
	for _, missed := range unread {
		r.unread.add(missed.Segment)
	}
	taken.at, taken.sessions, taken.unread = r.now(), sessions, unread
	return sessions, unread
}

// gone drops one connection from this group's look,
// a kick that landed being a connection the relay is not carrying.
//
// Rewritten rather than edited in place,
// a caller holding the slice this look answered with reading the connections it was given.
func (r *Registry) gone(prefix, id string) {
	r.mu.Lock()
	taken, holds := r.looks[prefix]
	r.mu.Unlock()
	if !holds {
		return
	}

	taken.taking.Lock()
	defer taken.taking.Unlock()

	carried := make([]relay.Session, 0, len(taken.sessions))
	for _, session := range taken.sessions {
		if session.ID != id {
			carried = append(carried, session)
		}
	}
	taken.sessions = carried
}

// member is what this subject calls itself where it holds a live lease in the group,
// and no name at all where it holds none.
func (r *Registry) member(prefix, id string) (string, bool) {
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()

	held, holds := r.held[prefix][id]
	if !holds || !held.expires.After(now) {
		return "", false
	}
	return held.displayName, true
}

// Swept reports whether a run would close a connection opened under this subject:
// the group states its members and this subject is none of them.
//
// The test a run makes of every connection it reads (enforce.go), asked of a subject alone.
// The token door asks it before it signs (internal/groupsvc),
// so a credential and a run read one set of leases,
// where two would let a token outlive its subject's presence by the whole token window.
//
// A group nobody states presence in sweeps nothing, so every subject stands in one.
func (r *Registry) Swept(groupKey group.Key, subject string) bool {
	assert.Assert(len(groupKey) == group.KeyBytes, "a group is asked about by a whole group key", len(groupKey))

	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()

	// One pass under one lock:
	// read apart, a lease lapsing between the two halves reads as a group that states members and holds this one.
	live := false
	for id, held := range r.held[groupKey.Prefix()] {
		if !held.expires.After(now) {
			continue
		}
		if id == subject {
			return false
		}
		live = true
	}
	return live
}

// live reports whether any member of this group holds a lease.
func (r *Registry) live(prefix string) bool {
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, held := range r.held[prefix] {
		if held.expires.After(now) {
			return true
		}
	}
	return false
}

// Reap drops every lease that stopped holding at this moment and reconciles the groups it changed.
//
// The one timer in the service.
// It closes what a member who stopped refreshing still holds where nobody else's call noticed first,
// which is a group whose remaining members are all idle.
func (r *Registry) Reap(now time.Time) []Result {
	assert.Assert(!now.IsZero(), "a sweep names the moment it measures leases against")

	r.mu.Lock()
	lapsed := []string{}
	gone := int64(0)
	for prefix, leases := range r.held {
		changed := false
		for id, held := range leases {
			if !held.expires.After(now) {
				delete(leases, id)
				changed = true
				gone++
			}
		}
		if len(leases) == 0 {
			delete(r.held, prefix)
		}
		if changed {
			lapsed = append(lapsed, prefix)
		}
	}
	// A group nobody holds a lease in keeps no look either,
	// a look being read against the leases and a group key being something anybody can draw one of.
	for prefix := range r.looks {
		if len(r.held[prefix]) == 0 {
			delete(r.looks, prefix)
		}
	}
	r.mu.Unlock()

	r.lapsed.Add(gone)

	// Sorted, so two sweeps of one relay read it in one order.
	slices.Sort(lapsed)
	swept := make([]Result, 0, len(lapsed))
	for _, prefix := range lapsed {
		swept = append(swept, r.Reconcile(prefix))
	}
	return swept
}

// members is the group's live members, with publishing read off the connections given.
func (r *Registry) members(prefix string, live []relay.Session) []Member {
	now := r.now()
	r.mu.Lock()
	out := make([]Member, 0, len(r.held[prefix]))
	for id, held := range r.held[prefix] {
		if held.expires.After(now) {
			out = append(out, Member{MemberID: id, DisplayName: held.displayName})
		}
	}
	r.mu.Unlock()

	for i := range out {
		out[i].Publishing = publishes(live, prefix, out[i].MemberID)
	}
	// By name, which is the order a reader of the group reads it in,
	// and by id where two members claimed one name in two groups this registry holds at once.
	slices.SortFunc(out, func(a, b Member) int {
		if a.DisplayName != b.DisplayName {
			return strings.Compare(a.DisplayName, b.DisplayName)
		}
		return strings.Compare(a.MemberID, b.MemberID)
	})
	return out
}

// publishState is the relay's own word for a connection pushing a stream, as it lists one
// (internal/relay, Session).
const publishState = "publish"

// publishes reports whether this member is pushing a stream of this group.
func publishes(live []relay.Session, prefix, id string) bool {
	assert.Assert(id != "", "a member is asked about by the id its secret derives")

	for _, session := range live {
		if session.User == id && session.State == publishState && strings.HasPrefix(session.Path, prefix) {
			return true
		}
	}
	return false
}
