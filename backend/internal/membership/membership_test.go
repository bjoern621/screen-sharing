package membership

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/relay"
)

// Membership is a presence lease a member's own app holds,
// and enforcing it means closing what anybody holding none does.
// These hold the two halves that make it safe to run on every pass of a 2 s poll:
// it acts on the group it was given and no other,
// and a second run over unchanged leases does nothing.

// fakeRelay stands in for the relay's connection lists and its kicks.
type fakeRelay struct {
	live    []relay.Session
	unread  []relay.Unread
	kicked  []string
	refuse  map[string]error
	listErr error
	// reads counts the looks at the connection lists, which is what the sweep window bounds.
	reads int
}

// Sessions answers a snapshot, the way the client does:
// a kick closes a connection at the relay and never edits a listing a caller is already reading.
func (f *fakeRelay) Sessions() ([]relay.Session, []relay.Unread) {
	f.reads++
	if f.listErr != nil {
		return nil, []relay.Unread{{Segment: "srtconns", Reason: f.listErr.Error()}}
	}
	return slices.Clone(f.live), f.unread
}

func (f *fakeRelay) Kick(segment, id string) error {
	if err := f.refuse[id]; err != nil {
		return err
	}
	f.kicked = append(f.kicked, id)
	f.live = slices.DeleteFunc(f.live, func(s relay.Session) bool { return s.ID == id })
	return nil
}

func mustKey(t *testing.T) group.Key {
	t.Helper()
	groupKey, err := group.NewKey()
	if err != nil {
		t.Fatalf("drawing a group key: %v", err)
	}
	return groupKey
}

func mustSecret(t *testing.T) group.MemberSecret {
	t.Helper()
	secret, err := group.NewMemberSecret()
	if err != nil {
		t.Fatalf("drawing a member secret: %v", err)
	}
	return secret
}

// past moves the registry's clock beyond the sweep window,
// so the next call reads the relay again rather than answering off the look before it.
// Every step is far inside a lease, a test that means to lapse one saying so itself.
func past(r *Registry) {
	at := r.now().Add(SweepWindow + time.Second)
	r.now = func() time.Time { return at }
}

// stated claims a lease and fails the test where it was refused.
func stated(t *testing.T, r *Registry, groupKey group.Key, secret group.MemberSecret, name string) Answer {
	t.Helper()
	answered, err := r.State(groupKey, secret, name)
	if err != nil {
		t.Fatalf("stating %s: %v", name, err)
	}
	return answered
}

// Claim and refresh are one call, so what it answers is the whole of what the caller needs:
// the id it is known by, the name it holds, and how long the lease it just stated runs.
func TestAMemberStatesItsOwnPresence(t *testing.T) {
	groupKey, secret := mustKey(t), mustSecret(t)
	registry := New(&fakeRelay{})

	answered := stated(t, registry, groupKey, secret, "Björn")
	if answered.MemberID != groupKey.MemberID(secret) {
		t.Errorf("the answer names %s, where the secret derives %s", answered.MemberID, groupKey.MemberID(secret))
	}
	if answered.DisplayName != "Björn" {
		t.Errorf("the answer holds the name %q", answered.DisplayName)
	}
	if answered.Lease != Lease {
		t.Errorf("the lease runs %s, want %s", answered.Lease, Lease)
	}
	if len(answered.Members) != 1 || answered.Members[0].MemberID != answered.MemberID {
		t.Errorf("a member is missing from its own group: %+v", answered.Members)
	}
}

// A second statement over an unchanged secret and name is the first one again,
// which is what lets the poll that already runs be the heartbeat.
func TestRefreshingIsTheSameCallAsClaiming(t *testing.T) {
	groupKey, secret := mustKey(t), mustSecret(t)
	live := &fakeRelay{}
	registry := New(live)

	stated(t, registry, groupKey, secret, "Björn")
	answered := stated(t, registry, groupKey, secret, "Björn")

	if len(answered.Members) != 1 {
		t.Errorf("refreshing made the group %+v", answered.Members)
	}
	if len(live.kicked) != 0 {
		t.Errorf("refreshing closed %v", live.kicked)
	}
}

// Identity cannot be forged inside a group:
// a member claiming another member's name still holds their own id,
// so the name is refused rather than moved.
func TestANameHeldByAnotherMemberIsRefused(t *testing.T) {
	groupKey := mustKey(t)
	first, second := mustSecret(t), mustSecret(t)
	registry := New(&fakeRelay{})

	stated(t, registry, groupKey, first, "Björn")
	if _, err := registry.State(groupKey, second, "Björn"); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("a name another member holds was answered %v", err)
	}

	// Nothing is stored on a refusal, so the group is the one member that claimed the name.
	view := registry.View(groupKey)
	if len(view.Members) != 1 || view.Members[0].MemberID != groupKey.MemberID(first) {
		t.Errorf("a refused claim left %+v", view.Members)
	}
}

// A refresh is a member claiming the name it already holds,
// which is the case a first-claim-wins rule has to let through.
func TestAMemberKeepsItsOwnNameOnEveryRefresh(t *testing.T) {
	groupKey, secret := mustKey(t), mustSecret(t)
	registry := New(&fakeRelay{})

	stated(t, registry, groupKey, secret, "Björn")
	if _, err := registry.State(groupKey, secret, "Björn"); err != nil {
		t.Errorf("a member was refused its own name: %v", err)
	}
}

// A name whose holder's lease lapsed is a name nobody holds,
// so the group does not keep it reserved for an app that stopped refreshing.
func TestANameALapsedMemberHeldIsFree(t *testing.T) {
	groupKey := mustKey(t)
	first, second := mustSecret(t), mustSecret(t)
	registry := New(&fakeRelay{})

	stated(t, registry, groupKey, first, "Björn")
	at := time.Now().Add(Lease + time.Second)
	registry.now = func() time.Time { return at }

	if _, err := registry.State(groupKey, second, "Björn"); err != nil {
		t.Errorf("a name whose holder lapsed was refused: %v", err)
	}
}

// A member states a name to be known by, so a claim carrying none is refused and nothing is stored.
func TestAMemberWithNoDisplayNameIsRefused(t *testing.T) {
	groupKey := mustKey(t)
	registry := New(&fakeRelay{})

	if _, err := registry.State(groupKey, mustSecret(t), "  "); err == nil {
		t.Fatal("a member with no display name was taken")
	}
	if len(registry.View(groupKey).Members) != 0 {
		t.Errorf("a refused claim left %+v", registry.View(groupKey).Members)
	}
}

// The whole point: somebody holding no lease loses what they hold.
func TestMembershipClosesWhatANonMemberHolds(t *testing.T) {
	groupKey, secret := mustKey(t), mustSecret(t)
	live := &fakeRelay{}
	registry := New(live)

	stranger := relay.Session{Segment: "srtconns", ID: "theirs", Path: groupKey.Prefix() + "desk",
		User: "whoever", State: "read", RemoteAddr: "10.0.0.4:5000"}
	live.live = []relay.Session{
		{Segment: "srtconns", ID: "mine", Path: groupKey.Prefix() + "desk", User: groupKey.MemberID(secret), State: "read"},
		stranger,
	}

	answered := stated(t, registry, groupKey, secret, "Björn")
	if !slices.Equal(live.kicked, []string{"theirs"}) {
		t.Fatalf("stating presence closed %v", live.kicked)
	}
	if len(answered.Members) != 1 {
		t.Errorf("the answer names %+v", answered.Members)
	}

	// The relay's session id and the address it came from are an operator's view of the relay,
	// and a caller here holds a group key rather than an operator's credential.
	// Taken off the run that closes a stranger's reconnect, that being the answer carrying one.
	live.live = append(live.live, stranger)
	past(registry)
	result := registry.Reconcile(groupKey.Prefix())
	rendered, _ := json.Marshal(result)
	if len(result.Kicked) != 1 {
		t.Fatalf("the run closed %s", rendered)
	}
	if strings.Contains(string(rendered), "10.0.0.") || strings.Contains(string(rendered), "theirs") {
		t.Errorf("an answer carried another member's address or the relay's own session id: %s", rendered)
	}
	if result.Kept != 1 {
		t.Errorf("one member was left connected, and the answer counts %d", result.Kept)
	}
}

// A member's own connections are what membership is for.
// Closing one because a sweep ran would make every refresh a glitch for everybody on it.
func TestAMemberIsLeftAlone(t *testing.T) {
	groupKey, secret := mustKey(t), mustSecret(t)
	live := &fakeRelay{}
	registry := New(live)

	live.live = []relay.Session{
		{Segment: "rtspsessions", ID: "watching", Path: groupKey.Prefix() + "desk",
			User: groupKey.MemberID(secret), State: "read"},
	}
	stated(t, registry, groupKey, secret, "Björn")

	if len(live.kicked) != 0 {
		t.Errorf("a member's own connection was closed: %v", live.kicked)
	}
}

// Enforcement runs on every pass of the poll and on every read the relay reports,
// so a run that changes nothing has to cost nothing.
func TestASecondRunOverUnchangedLeasesClosesNothing(t *testing.T) {
	groupKey, secret := mustKey(t), mustSecret(t)
	live := &fakeRelay{live: []relay.Session{
		{Segment: "srtconns", ID: "goes", Path: groupKey.Prefix() + "desk", User: "whoever", State: "read"},
	}}
	registry := New(live)

	stated(t, registry, groupKey, secret, "Björn")
	before := len(live.kicked)

	past(registry)
	result := registry.Reconcile(groupKey.Prefix())
	if len(live.kicked) != before {
		t.Errorf("a second run closed %v", live.kicked[before:])
	}
	if len(result.Kicked) != 0 {
		t.Errorf("a second run reported %+v as closed", result.Kicked)
	}
}

// One relay carries every group, and a sweep reads all of them at once.
// Acting on a connection outside the group being enforced
// would let one group close another's streams.
func TestAConnectionOutsideTheGroupIsNotTouched(t *testing.T) {
	here, elsewhere := mustKey(t), mustKey(t)
	live := &fakeRelay{live: []relay.Session{
		{Segment: "srtconns", ID: "theirs", Path: elsewhere.Prefix() + "desk", User: "somebody", State: "read"},
		{Segment: "srtconns", ID: "reader", Path: elsewhere.Prefix() + "standup", User: "anybody", State: "read"},
	}}

	stated(t, New(live), here, mustSecret(t), "Björn")
	if len(live.kicked) != 0 {
		t.Errorf("enforcing one group closed %v", live.kicked)
	}
}

// Membership nobody stated is not the same as a group nobody is in.
// Enforcing the empty case would close the connections of an app that has not stated its presence.
func TestAGroupWithNoLiveMembersIsNotEnforced(t *testing.T) {
	groupKey := mustKey(t)
	live := &fakeRelay{live: []relay.Session{
		{Segment: "srtconns", ID: "unknown", Path: groupKey.Prefix() + "desk", User: "whoever", State: "read"},
	}}

	result := New(live).Reconcile(groupKey.Prefix())
	if result.Enforced {
		t.Error("a group nobody stated presence in reported as enforced")
	}
	if len(live.kicked) != 0 {
		t.Errorf("a group with no live members closed %v", live.kicked)
	}
}

// A lease that lapsed is a member who left, so what they were sharing goes with them,
// the same way leaving a voice channel ends what was being shared into it.
func TestALapsedLeaseLosesBothWhatItWatchesAndWhatItShares(t *testing.T) {
	groupKey := mustKey(t)
	leaving, staying := mustSecret(t), mustSecret(t)
	live := &fakeRelay{}
	registry := New(live)

	stated(t, registry, groupKey, leaving, "Bob")
	stated(t, registry, groupKey, staying, "Alice")
	live.live = []relay.Session{
		{Segment: "rtspsessions", ID: "bob-publish", Path: groupKey.Prefix() + "bob",
			User: groupKey.MemberID(leaving), State: "publish"},
		{Segment: "hlssessions", ID: "bob-watching", Path: groupKey.Prefix() + "alice",
			User: groupKey.MemberID(leaving), State: "read"},
		{Segment: "hlssessions", ID: "alice-watching", Path: groupKey.Prefix() + "alice",
			User: groupKey.MemberID(staying), State: "read"},
	}

	// Only the leaver's lease lapses: the one that stays is refreshed at the later moment.
	at := time.Now().Add(Lease + time.Second)
	registry.now = func() time.Time { return at }
	stated(t, registry, groupKey, staying, "Alice")

	slices.Sort(live.kicked)
	if !slices.Equal(live.kicked, []string{"bob-publish", "bob-watching"}) {
		t.Errorf("a lapsed lease kept something: closed %v", live.kicked)
	}
}

// The call that notices a lapse is the one that closes it,
// so a member who stopped refreshing is gone by the next pass of another member's poll
// rather than at the next sweep of the reaper.
func TestALapsedLeaseIsClosedByTheCallThatNoticesIt(t *testing.T) {
	groupKey := mustKey(t)
	lapsing, watching := mustSecret(t), mustSecret(t)
	live := &fakeRelay{}
	registry := New(live)

	stated(t, registry, groupKey, lapsing, "Bob")
	stated(t, registry, groupKey, watching, "Alice")
	live.live = []relay.Session{
		{Segment: "srtconns", ID: "bobs", Path: groupKey.Prefix() + "desk", User: groupKey.MemberID(lapsing), State: "read"},
	}

	at := time.Now().Add(Lease + time.Second)
	registry.now = func() time.Time { return at }
	answered := stated(t, registry, groupKey, watching, "Alice")

	if !slices.Equal(live.kicked, []string{"bobs"}) {
		t.Errorf("the call that noticed the lapse closed %v", live.kicked)
	}
	if len(answered.Members) != 1 {
		t.Errorf("a lapsed member is still in the group: %+v", answered.Members)
	}
}

// A member watching three streams holds three connections, and lapsing takes all of them.
func TestEveryConnectionOneNonMemberHoldsGoes(t *testing.T) {
	groupKey := mustKey(t)
	live := &fakeRelay{live: []relay.Session{
		{Segment: "srtconns", ID: "one", Path: groupKey.Prefix() + "a", User: "bob", State: "read"},
		{Segment: "rtspsessions", ID: "two", Path: groupKey.Prefix() + "b", User: "bob", State: "read"},
		{Segment: "webrtcsessions", ID: "three", Path: groupKey.Prefix() + "c", User: "bob", State: "read"},
	}}

	stated(t, New(live), groupKey, mustSecret(t), "Alice")
	if len(live.kicked) != 3 {
		t.Errorf("one non-member on three streams left %d connections open: %v", 3-len(live.kicked), live.kicked)
	}
}

// A kick the relay would not perform is a non-member still watching,
// so it is reported rather than counted as a removal.
func TestAKickTheRelayRefusedIsReported(t *testing.T) {
	groupKey := mustKey(t)
	live := &fakeRelay{
		live: []relay.Session{
			{Segment: "srtconns", ID: "stubborn", Path: groupKey.Prefix() + "desk", User: "nobody", State: "read"},
		},
		refuse: map[string]error{"stubborn": errors.New("session not found")},
	}
	registry := New(live)

	stated(t, registry, groupKey, mustSecret(t), "Alice")
	result := registry.Reconcile(groupKey.Prefix())
	if len(result.Kicked) != 0 {
		t.Errorf("a refused kick was reported as closed: %+v", result.Kicked)
	}
	if len(result.Failed) != 1 || result.Failed[0].Stream != "desk" {
		t.Fatalf("a refused kick was not reported: %+v", result.Failed)
	}
	if !strings.Contains(result.Failed[0].Reason, "session not found") {
		t.Errorf("a refused kick lost the relay's words: %s", result.Failed[0].Reason)
	}
}

// A sweep that could not read every list may have missed the one connection that mattered,
// so the answer carries that rather than reporting a clean enforcement.
func TestListsThatCouldNotBeReadTravelWithTheAnswer(t *testing.T) {
	groupKey := mustKey(t)
	live := &fakeRelay{listErr: errors.New("the relay answered 500")}
	registry := New(live)

	stated(t, registry, groupKey, mustSecret(t), "Alice")
	if result := registry.Reconcile(groupKey.Prefix()); len(result.Unread) != 1 {
		t.Fatalf("a list that could not be read was dropped: %+v", result)
	}
}

// Releasing is what closing the app means, and it is the same removal a lapse is without the wait.
func TestReleasingAMemberClosesWhatItHeld(t *testing.T) {
	groupKey := mustKey(t)
	leaving, staying := mustSecret(t), mustSecret(t)
	live := &fakeRelay{}
	registry := New(live)

	stated(t, registry, groupKey, leaving, "Bob")
	stated(t, registry, groupKey, staying, "Alice")
	live.live = []relay.Session{
		{Segment: "srtconns", ID: "bobs", Path: groupKey.Prefix() + "desk", User: groupKey.MemberID(leaving), State: "read"},
		{Segment: "srtconns", ID: "alices", Path: groupKey.Prefix() + "desk", User: groupKey.MemberID(staying), State: "read"},
	}
	past(registry)

	answered, err := registry.Release(groupKey, leaving)
	if err != nil {
		t.Fatalf("releasing a member: %v", err)
	}
	if !answered.Released {
		t.Error("releasing a member who held a lease answered that it held none")
	}
	if !slices.Equal(live.kicked, []string{"bobs"}) {
		t.Errorf("releasing one member closed %v", live.kicked)
	}
}

// A release closes what the leaver itself holds, by its own id, whether or not anybody is left.
// The carve-out is about a run: a group with no live member is not enforced,
// so a connection nobody released stays open until somebody states presence again.
func TestTheLastMemberLeavingClosesWhatItHeld(t *testing.T) {
	groupKey, secret := mustKey(t), mustSecret(t)
	live := &fakeRelay{}
	registry := New(live)

	stated(t, registry, groupKey, secret, "Björn")
	live.live = []relay.Session{
		{Segment: "srtconns", ID: "mine", Path: groupKey.Prefix() + "desk", User: groupKey.MemberID(secret), State: "read"},
		{Segment: "srtconns", ID: "unstated", Path: groupKey.Prefix() + "desk", User: "whoever", State: "read"},
	}
	past(registry)

	if _, err := registry.Release(groupKey, secret); err != nil {
		t.Fatalf("releasing a member: %v", err)
	}
	if !slices.Equal(live.kicked, []string{"mine"}) {
		t.Errorf("the last member leaving closed %v, want what it held itself", live.kicked)
	}

	past(registry)
	if registry.Reconcile(groupKey.Prefix()).Enforced {
		t.Error("a group the last member left reported as enforced")
	}
	if !slices.Equal(live.kicked, []string{"mine"}) {
		t.Errorf("a group with no live member closed %v", live.kicked)
	}
}

// Reading the relay is a round trip,
// and a member that states presence inside one holds a lease by the moment a kick would land.
// The leases are read again for each connection rather than off the snapshot the run started from.
func TestAMemberStatingPresenceDuringTheRelayReadIsLeftAlone(t *testing.T) {
	groupKey := mustKey(t)
	holding, returning := mustSecret(t), mustSecret(t)
	live := &during{}
	registry := New(live)

	stated(t, registry, groupKey, holding, "Alice")
	live.live = []relay.Session{
		{Segment: "srtconns", ID: "bobs", Path: groupKey.Prefix() + "desk",
			User: groupKey.MemberID(returning), State: "read"},
	}
	past(registry)

	back := make(chan struct{})
	live.crossing = func() {
		go func() {
			registry.State(groupKey, returning, "Bob")
			close(back)
		}()
		until(t, func() bool { return holds(registry, groupKey, returning) })
	}

	registry.Reconcile(groupKey.Prefix())
	<-back
	if len(live.kicked) != 0 {
		t.Errorf("a member that stated presence while the relay was read lost %v", live.kicked)
	}
}

// during is a relay that lets a second request reach the registry
// while its connection lists are read,
// which is the window a run's snapshot of the leases is stale for.
type during struct {
	live   []relay.Session
	kicked []string
	// crossing runs inside the first read and is dropped,
	// the request it starts reading the relay for itself rather than waiting on the read it crosses.
	crossing func()
}

func (d *during) Sessions() ([]relay.Session, []relay.Unread) {
	if d.crossing != nil {
		crossing := d.crossing
		d.crossing = nil
		crossing()
	}
	return slices.Clone(d.live), nil
}

func (d *during) Kick(_, id string) error {
	d.kicked = append(d.kicked, id)
	d.live = slices.DeleteFunc(d.live, func(session relay.Session) bool { return session.ID == id })
	return nil
}

// holds reports whether this member's lease stands at this moment.
func holds(r *Registry, groupKey group.Key, secret group.MemberSecret) bool {
	_, member := r.member(groupKey.Prefix(), groupKey.MemberID(secret))
	return member
}

// until waits for what a crossing request brings about,
// that request holding a lock this one has to let go of first.
func until(t *testing.T, reached func() bool) {
	t.Helper()
	for range 10_000 {
		if reached() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("a crossing request never reached the registry")
}

// A read of the relay is a request per connection list, each paged,
// and every statement of presence and every view reaches one.
// One look per group inside the window, and every request arriving in it is answered off that look.
func TestOneLookAtTheRelayAnswersEveryRequestInsideTheWindow(t *testing.T) {
	groupKey, secret := mustKey(t), mustSecret(t)
	live := &fakeRelay{}
	registry := New(live)

	stated(t, registry, groupKey, secret, "Björn")
	if live.reads != 1 {
		t.Fatalf("stating presence read the relay %d times", live.reads)
	}

	stated(t, registry, groupKey, secret, "Björn")
	registry.View(groupKey)
	registry.Reconcile(groupKey.Prefix())
	if live.reads != 1 {
		t.Errorf("four requests inside one window read the relay %d times", live.reads)
	}

	past(registry)
	registry.View(groupKey)
	if live.reads != 2 {
		t.Errorf("a request past the window read the relay %d times", live.reads)
	}
}

// A list that would not answer leaves every member on it publishing nothing,
// which reads exactly like a member sending nothing.
// What could not be read travels with the answer, so an app can tell the two apart.
func TestAnAnswerCarriesTheListsThatWouldNotAnswer(t *testing.T) {
	groupKey, secret := mustKey(t), mustSecret(t)
	live := &fakeRelay{listErr: errors.New("the relay answered 500")}
	registry := New(live)

	answered := stated(t, registry, groupKey, secret, "Björn")
	if len(answered.Unread) != 1 {
		t.Errorf("a statement answered %+v, naming no list that would not answer", answered.Unread)
	}

	past(registry)
	if view := registry.View(groupKey); len(view.Unread) != 1 {
		t.Errorf("a view answered %+v, naming no list that would not answer", view.Unread)
	}
}

// Releasing names the state it wants true, so a member who holds no lease is already in it.
func TestReleasingAMemberWhoHoldsNoLeaseIsASuccess(t *testing.T) {
	groupKey, secret := mustKey(t), mustSecret(t)
	registry := New(&fakeRelay{})

	answered, err := registry.Release(groupKey, secret)
	if err != nil {
		t.Fatalf("releasing a member who holds no lease: %v", err)
	}
	if answered.Released {
		t.Error("releasing a member who holds no lease answered that it held one")
	}
	if answered.MemberID != groupKey.MemberID(secret) {
		t.Errorf("the answer names %s, where the secret derives %s", answered.MemberID, groupKey.MemberID(secret))
	}

	stated(t, registry, groupKey, secret, "Björn")
	if _, err := registry.Release(groupKey, secret); err != nil {
		t.Fatalf("releasing a member: %v", err)
	}
	if again, _ := registry.Release(groupKey, secret); again.Released {
		t.Error("releasing the same member twice reported two removals")
	}
}

// Who is publishing is the relay's fact, read through on every answer,
// so a member whose publish dropped stops being shown as publishing without anybody stating it.
func TestPublishingIsReadOffTheRelay(t *testing.T) {
	groupKey := mustKey(t)
	publishing, watching := mustSecret(t), mustSecret(t)
	live := &fakeRelay{}
	registry := New(live)

	stated(t, registry, groupKey, publishing, "Bob")
	stated(t, registry, groupKey, watching, "Alice")
	live.live = []relay.Session{
		{Segment: "rtspsessions", ID: "bobs", Path: groupKey.Prefix() + "bob",
			User: groupKey.MemberID(publishing), State: "publish"},
		{Segment: "hlssessions", ID: "alices", Path: groupKey.Prefix() + "bob",
			User: groupKey.MemberID(watching), State: "read"},
	}
	past(registry)

	held := map[string]bool{}
	for _, member := range registry.View(groupKey).Members {
		held[member.DisplayName] = member.Publishing
	}
	if !held["Bob"] {
		t.Error("a member with a publish open is not shown publishing")
	}
	if held["Alice"] {
		t.Error("a member who is only watching is shown publishing")
	}

	live.live = nil
	past(registry)
	for _, member := range registry.View(groupKey).Members {
		if member.Publishing {
			t.Errorf("%s is shown publishing with nothing open at the relay", member.DisplayName)
		}
	}
}

// The view answers who is in the group without stating anything,
// which is what a reader holding the group key asks for.
func TestTheViewStatesNothing(t *testing.T) {
	groupKey, secret := mustKey(t), mustSecret(t)
	live := &fakeRelay{live: []relay.Session{
		{Segment: "srtconns", ID: "stranger", Path: groupKey.Prefix() + "desk", User: "whoever", State: "read"},
	}}
	registry := New(live)

	if members := registry.View(groupKey).Members; len(members) != 0 {
		t.Errorf("a group nobody stated presence in carries %+v", members)
	}
	if len(live.kicked) != 0 {
		t.Errorf("a view closed %v", live.kicked)
	}

	stated(t, registry, groupKey, secret, "Björn")
	view := registry.View(groupKey)
	if len(view.Members) != 1 || view.Members[0].DisplayName != "Björn" {
		t.Errorf("the view carries %+v", view.Members)
	}
	if view.Lease != 0 {
		t.Errorf("a view stated a lease of %s", view.Lease)
	}
}

// The reaper is the only timer,
// and what it sweeps is a lease nobody refreshed while the members around it went on holding theirs.
func TestTheReaperClosesWhatALapsedLeaseHeld(t *testing.T) {
	groupKey := mustKey(t)
	lapsing, staying := mustSecret(t), mustSecret(t)
	live := &fakeRelay{}
	registry := New(live)

	started := time.Now()
	at := started
	registry.now = func() time.Time { return at }

	stated(t, registry, groupKey, lapsing, "Bob")
	at = started.Add(Lease / 2)
	stated(t, registry, groupKey, staying, "Alice")

	live.live = []relay.Session{
		{Segment: "srtconns", ID: "bobs", Path: groupKey.Prefix() + "desk", User: groupKey.MemberID(lapsing), State: "read"},
		{Segment: "srtconns", ID: "alices", Path: groupKey.Prefix() + "desk", User: groupKey.MemberID(staying), State: "read"},
	}

	// Inside every lease nothing lapsed, so the sweep has no group to run.
	if swept := registry.Reap(at); len(swept) != 0 {
		t.Errorf("a sweep inside every lease ran against %+v", swept)
	}
	if len(live.kicked) != 0 {
		t.Errorf("a sweep inside every lease closed %v", live.kicked)
	}

	at = started.Add(Lease + time.Second)
	if swept := registry.Reap(at); len(swept) != 1 {
		t.Fatalf("a sweep past one lease ran against %+v", swept)
	}
	if !slices.Equal(live.kicked, []string{"bobs"}) {
		t.Errorf("a sweep past one lease closed %v", live.kicked)
	}

	members := registry.View(groupKey).Members
	if len(members) != 1 || members[0].DisplayName != "Alice" {
		t.Errorf("a sweep left the group as %+v", members)
	}
}

// A connection is named by the stream inside the group, which is the name the index answers under,
// so the two join without a caller deriving the prefix rule a second time.
func TestAClosedConnectionIsNamedTheWayTheIndexNamesIt(t *testing.T) {
	groupKey := mustKey(t)
	live := &fakeRelay{live: []relay.Session{
		{Segment: "srtconns", Transport: "srt", ID: "one", Path: groupKey.Prefix() + "desk",
			User: "stranger", State: "read"},
	}}
	registry := New(live)

	stated(t, registry, groupKey, mustSecret(t), "Alice")
	result := registry.Reconcile(groupKey.Prefix())
	if len(result.Kicked) != 0 {
		t.Fatalf("a second run closed %+v", result.Kicked)
	}

	live.live = []relay.Session{
		{Segment: "srtconns", Transport: "srt", ID: "two", Path: groupKey.Prefix() + "desk",
			User: "stranger", State: "read"},
	}
	past(registry)
	closed := registry.Reconcile(groupKey.Prefix()).Kicked
	if len(closed) != 1 {
		t.Fatalf("the run closed %+v", closed)
	}
	if closed[0].Stream != "desk" {
		t.Errorf("the answer names the stream %q, where the index answers \"desk\"", closed[0].Stream)
	}
	if closed[0].Transport != "srt" {
		t.Errorf("the answer names the leg %q in the relay's vocabulary rather than this app's",
			closed[0].Transport)
	}
}

// A release reaching the registry while a statement is between its lock and its answer
// is two well-formed requests crossing, which is what this app does to itself on every leave:
// a group key taken out of the settings sends DELETE /members
// while the 2 s poll has a PUT /members in flight.
// The statement answers the group without this member in it rather than ending the process.
func TestAStatementCrossingAReleaseAnswers(t *testing.T) {
	groupKey, secret := mustKey(t), mustSecret(t)
	live := &during{}
	registry := New(live)

	released := make(chan struct{})
	live.crossing = func() {
		go func() {
			registry.Release(groupKey, secret)
			close(released)
		}()
		until(t, func() bool { return !holds(registry, groupKey, secret) })
	}

	if _, err := registry.State(groupKey, secret, "Björn"); err != nil {
		t.Fatalf("stating presence across a release: %v", err)
	}
	<-released
}

// The same pair the other way round:
// the poll's statement lands while the leave it crossed is reading the relay,
// and the release answers rather than ending the process.
func TestAReleaseCrossingAStatementAnswers(t *testing.T) {
	groupKey := mustKey(t)
	leaving, staying := mustSecret(t), mustSecret(t)
	live := &during{}
	registry := New(live)

	stated(t, registry, groupKey, staying, "Alice")
	stated(t, registry, groupKey, leaving, "Bob")
	past(registry)

	restated := make(chan struct{})
	live.crossing = func() {
		go func() {
			registry.State(groupKey, leaving, "Bob")
			close(restated)
		}()
		until(t, func() bool { return holds(registry, groupKey, leaving) })
	}

	if _, err := registry.Release(groupKey, leaving); err != nil {
		t.Fatalf("releasing a member across a statement: %v", err)
	}
	<-restated
}

// Token issuance asks whether a run would close what it is about to sign for,
// which is the question a run asks of every connection it reads,
// so both read one set of leases and a credential outlives its subject's presence by nothing.
func TestASubjectNoLiveMemberHoldsIsSwept(t *testing.T) {
	groupKey, secret, absent := mustKey(t), mustSecret(t), mustSecret(t)
	registry := New(&fakeRelay{})

	// A group nobody states presence in is one a run leaves alone, whatever the subject.
	if registry.Swept(groupKey, groupKey.MemberID(absent)) {
		t.Error("a group nobody stated presence in swept a subject")
	}

	stated(t, registry, groupKey, secret, "Björn")
	if registry.Swept(groupKey, groupKey.MemberID(secret)) {
		t.Error("a member holding a lease was swept")
	}
	if !registry.Swept(groupKey, groupKey.MemberID(absent)) {
		t.Error("a member holding no lease stood in a group that states its members")
	}
	// The group's own id is the subject a token naming no member carries.
	if !registry.Swept(groupKey, groupKey.ID()) {
		t.Error("the group's own id stood in a group that states its members")
	}

	at := time.Now().Add(Lease + time.Second)
	registry.now = func() time.Time { return at }
	if registry.Swept(groupKey, groupKey.MemberID(absent)) {
		t.Error("a group whose only lease lapsed swept a subject")
	}
}
