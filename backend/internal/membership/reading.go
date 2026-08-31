package membership

import (
	"slices"
	"sync"
	"sync/atomic"

	"bjoernblessin.de/go-utils/util/assert"
)

// Reading is one group as a scrape reads it: the prefix it is known by, and who holds a lease in it.
//
// The same pair of facts a member's own view answers with, taken for every group at once.
// A group nobody holds a lease in is absent rather than a reading with no members,
// the line this package draws between membership nobody stated and a group nobody is in.
type Reading struct {
	Prefix  string
	Members []Member
}

// Tallies is what has happened, as a scrape counts it.
//
// Every field only ever rises.
// Kicked and Refused are keyed by transport and Unread by the relay's own name for the list,
// so a reader can tell an SRT publisher being closed from an HLS viewer.
type Tallies struct {
	// Stated counts members arriving, never refreshing:
	// the app states presence on every pass of its poll,
	// and counting those buries the arrival under the poll rate.
	Stated   int64
	Released int64
	Lapsed   int64
	Kicked   map[string]int64
	Refused  map[string]int64
	Unread   map[string]int64
}

// Read is every group holding a live lease, oldest fact first:
// the leases are this registry's and publishing is the relay's,
// read off a look at most SweepWindow old.
//
// One look per group, which is what a scrape costs the relay.
// A scrape interval far above SweepWindow means the look is always taken fresh,
// so the figure is never staler than the window a member's own view already reads through.
func (r *Registry) Read() []Reading {
	r.mu.Lock()
	prefixes := make([]string, 0, len(r.held))
	for prefix := range r.held {
		prefixes = append(prefixes, prefix)
	}
	r.mu.Unlock()

	// Sorted, so two scrapes of one registry read alike and a series does not reorder between them.
	slices.Sort(prefixes)

	read := make([]Reading, 0, len(prefixes))
	for _, prefix := range prefixes {
		// The relay is reached outside the lock, which is why the prefixes are collected first.
		live, _ := r.connections(prefix)
		members := r.members(prefix, live)
		// A group whose last lease lapsed between the collection above and here holds nobody,
		// and an absent group is what says so.
		if len(members) == 0 {
			continue
		}
		read = append(read, Reading{Prefix: prefix, Members: members})
	}

	for _, group := range read {
		assert.Assert(group.Prefix != "", "a read group is named by its prefix")
		assert.Assert(len(group.Members) > 0, "a read group holds a member", group.Prefix)
	}
	return read
}

// Tallies is what this registry has counted since it was made.
func (r *Registry) Tallies() Tallies {
	return Tallies{
		Stated:   r.stated.Load(),
		Released: r.released.Load(),
		Lapsed:   r.lapsed.Load(),
		Kicked:   r.kicked.read(),
		Refused:  r.refused.read(),
		Unread:   r.unread.read(),
	}
}

// tally counts events under a key it does not know in advance.
//
// A map rather than a field per transport, the transports being a table another package owns
// (internal/relay, readerKinds) and a field per name here being a second copy of it.
type tally struct {
	mu sync.Mutex
	by map[string]int64
}

// add counts one event under key, which is the empty string where the relay named no transport.
func (t *tally) add(key string) {
	if key == "" {
		key = unnamed
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.by == nil {
		t.by = map[string]int64{}
	}
	t.by[key]++
}

// read copies the counts out, a caller rendering a scrape holding them while this goes on counting.
func (t *tally) read() map[string]int64 {
	t.mu.Lock()
	defer t.mu.Unlock()

	out := make(map[string]int64, len(t.by))
	for key, count := range t.by {
		out[key] = count
	}
	return out
}

// unnamed is the transport of a connection the relay described without one,
// a list this app's vocabulary has no name for rather than a connection with no leg.
const unnamed = "unnamed"

// counted is the set of counters a Registry carries,
// held apart from the leases so a count never waits on the lock a sweep holds.
type counted struct {
	stated   atomic.Int64
	released atomic.Int64
	lapsed   atomic.Int64
	kicked   tally
	refused  tally
	unread   tally
}
