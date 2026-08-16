package membership

import (
	"errors"
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/relay"
)

// What a scrape reads is the registry as it stands, so the reading is taken the way an answer to a
// member is: leases from here, publishing off the relay.

func TestReadNamesEveryGroupHoldingALease(t *testing.T) {
	first, firstSecret := mustKey(t), mustSecret(t)
	second, secondSecret := mustKey(t), mustSecret(t)
	registry := New(&fakeRelay{})

	stated(t, registry, first, firstSecret, "Björn")
	stated(t, registry, second, secondSecret, "Alice")

	read := registry.Read()
	if len(read) != 2 {
		t.Fatalf("two groups hold a lease, and the reading names %d", len(read))
	}
	// Sorted, so two scrapes of one registry read alike.
	if read[0].Prefix > read[1].Prefix {
		t.Errorf("a reading names groups in one order, and this one reads %q then %q", read[0].Prefix, read[1].Prefix)
	}
	for _, group := range read {
		if len(group.Members) != 1 {
			t.Errorf("%s holds one member, and the reading names %+v", group.Prefix, group.Members)
		}
	}
}

func TestReadLeavesOutAGroupWhoseLeasesLapsed(t *testing.T) {
	groupKey, secret := mustKey(t), mustSecret(t)
	registry := New(&fakeRelay{})
	stated(t, registry, groupKey, secret, "Björn")

	at := registry.now().Add(Lease + time.Second)
	registry.now = func() time.Time { return at }

	if read := registry.Read(); len(read) != 0 {
		t.Errorf("a lapsed group is in no reading, and this one names %+v", read)
	}
}

func TestReadReportsWhoIsPublishing(t *testing.T) {
	groupKey, secret := mustKey(t), mustSecret(t)
	live := &fakeRelay{}
	registry := New(live)

	live.live = []relay.Session{
		{Segment: "srtconns", ID: "pushing", Path: groupKey.Prefix() + "desk",
			User: groupKey.MemberID(secret), State: "publish", Transport: "srt"},
	}
	stated(t, registry, groupKey, secret, "Björn")

	read := registry.Read()
	if len(read) != 1 || len(read[0].Members) != 1 {
		t.Fatalf("one member holds a lease, and the reading names %+v", read)
	}
	if !read[0].Members[0].Publishing {
		t.Errorf("a member holding a publish connection reads as publishing, and this one reads %+v", read[0].Members[0])
	}
}

// A refresh is not a member arriving. The app states presence on every pass of its poll, so counting
// each one leaves the churn a reader came for buried under the poll rate.
func TestANewLeaseIsCountedOnceAcrossRefreshes(t *testing.T) {
	groupKey, secret := mustKey(t), mustSecret(t)
	registry := New(&fakeRelay{})

	stated(t, registry, groupKey, secret, "Björn")
	past(registry)
	stated(t, registry, groupKey, secret, "Björn")

	if got := registry.Tallies().Stated; got != 1 {
		t.Errorf("one member arrived, and the tally counts %d", got)
	}
}

func TestALeaseStatedAgainAfterLapsingCountsAgain(t *testing.T) {
	groupKey, secret := mustKey(t), mustSecret(t)
	registry := New(&fakeRelay{})

	stated(t, registry, groupKey, secret, "Björn")
	at := registry.now().Add(Lease + time.Second)
	registry.now = func() time.Time { return at }
	stated(t, registry, groupKey, secret, "Björn")

	if got := registry.Tallies().Stated; got != 2 {
		t.Errorf("a member arrived twice, and the tally counts %d", got)
	}
}

func TestAReleaseIsCounted(t *testing.T) {
	groupKey, secret := mustKey(t), mustSecret(t)
	registry := New(&fakeRelay{})
	stated(t, registry, groupKey, secret, "Björn")

	if _, err := registry.Release(groupKey, secret); err != nil {
		t.Fatalf("releasing: %v", err)
	}

	if got := registry.Tallies().Released; got != 1 {
		t.Errorf("one member left, and the tally counts %d", got)
	}
}

// A release the registry held no lease for is already in the state it names, so it succeeds and is
// not a departure to count.
func TestAReleaseOfNothingIsNotCounted(t *testing.T) {
	groupKey, secret := mustKey(t), mustSecret(t)
	registry := New(&fakeRelay{})

	if _, err := registry.Release(groupKey, secret); err != nil {
		t.Fatalf("releasing: %v", err)
	}

	if got := registry.Tallies().Released; got != 0 {
		t.Errorf("nobody left, and the tally counts %d", got)
	}
}

func TestALapsedLeaseIsCounted(t *testing.T) {
	groupKey, secret := mustKey(t), mustSecret(t)
	registry := New(&fakeRelay{})
	stated(t, registry, groupKey, secret, "Björn")

	at := registry.now().Add(Lease + time.Second)
	registry.now = func() time.Time { return at }
	registry.Reap(at)

	if got := registry.Tallies().Lapsed; got != 1 {
		t.Errorf("one lease lapsed, and the tally counts %d", got)
	}
}

func TestAClosedConnectionIsCountedUnderItsTransport(t *testing.T) {
	groupKey, secret := mustKey(t), mustSecret(t)
	live := &fakeRelay{}
	registry := New(live)

	live.live = []relay.Session{
		{Segment: "srtconns", ID: "theirs", Path: groupKey.Prefix() + "desk",
			User: "whoever", State: "read", Transport: "srt"},
	}
	stated(t, registry, groupKey, secret, "Björn")

	if got := registry.Tallies().Kicked["srt"]; got != 1 {
		t.Errorf("one srt connection was closed, and the tally counts %d", got)
	}
	if got := registry.Tallies().Refused["srt"]; got != 0 {
		t.Errorf("the relay refused nothing, and the tally counts %d", got)
	}
}

// A refused kick is a member possibly still watching, so it is counted apart from what was closed
// rather than folded into it.
func TestARefusedKickIsCountedApart(t *testing.T) {
	groupKey, secret := mustKey(t), mustSecret(t)
	live := &fakeRelay{refuse: map[string]error{"theirs": errors.New("no")}}
	registry := New(live)

	live.live = []relay.Session{
		{Segment: "srtconns", ID: "theirs", Path: groupKey.Prefix() + "desk",
			User: "whoever", State: "read", Transport: "srt"},
	}
	stated(t, registry, groupKey, secret, "Björn")

	tallies := registry.Tallies()
	if tallies.Refused["srt"] != 1 {
		t.Errorf("one close was refused, and the tally counts %d", tallies.Refused["srt"])
	}
	if tallies.Kicked["srt"] != 0 {
		t.Errorf("nothing was closed, and the tally counts %d", tallies.Kicked["srt"])
	}
}

func TestAListThatWouldNotAnswerIsCountedUnderItsSegment(t *testing.T) {
	groupKey, secret := mustKey(t), mustSecret(t)
	live := &fakeRelay{listErr: errors.New("connection refused")}
	registry := New(live)

	stated(t, registry, groupKey, secret, "Björn")

	if got := registry.Tallies().Unread["srtconns"]; got == 0 {
		t.Errorf("a list would not answer, and the tally counts none")
	}
}
