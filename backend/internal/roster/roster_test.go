package roster

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/relay"
)

// A roster is who a group's members are, and enforcing it means closing what anybody else holds.
// These hold the two halves that make it safe to run on every membership change: it acts on the
// group it was given and no other, and a second run with an unchanged roster does nothing.

// fakeRelay stands in for the relay's connection lists and its kicks.
type fakeRelay struct {
	live    []relay.Session
	unread  []relay.Unread
	kicked  []string
	refuse  map[string]error
	listErr error
}

// Sessions answers a snapshot, the way the client does: a kick closes a connection at the relay and
// never edits a listing a caller is already reading.
func (f *fakeRelay) Sessions() ([]relay.Session, []relay.Unread) {
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
	key, err := group.NewKey()
	if err != nil {
		t.Fatalf("drawing a group key: %v", err)
	}
	return key
}

func mustMemberID(t *testing.T, key group.Key, name string) string {
	t.Helper()
	id, err := key.MemberID(name)
	if err != nil {
		t.Fatalf("deriving a member id: %v", err)
	}
	return id
}

// The whole point: somebody who is no longer a member loses what they hold.
func TestARosterClosesWhatANonMemberHolds(t *testing.T) {
	key := mustKey(t)
	live := &fakeRelay{live: []relay.Session{
		{Segment: "srtconns", ID: "stays", Path: key.Prefix() + "desk", User: mustMemberID(t, key, "alice"), State: "read"},
		{Segment: "srtconns", ID: "goes", Path: key.Prefix() + "desk", User: mustMemberID(t, key, "bob"), State: "read"},
	}}
	registry := New(live)

	result, err := registry.Set(key, []string{"alice"})
	if err != nil {
		t.Fatalf("setting a roster: %v", err)
	}
	if !slices.Equal(live.kicked, []string{"goes"}) {
		t.Fatalf("the roster closed %v", live.kicked)
	}
	if len(result.Kicked) != 1 || result.Kicked[0].Stream != "desk" {
		t.Errorf("the answer names %+v as closed", result.Kicked)
	}
	// The relay's session id and the address it came from are an operator's view of the relay, and a
	// caller here holds a group key rather than an operator's credential.
	if answered, _ := json.Marshal(result); strings.Contains(string(answered), "10.0.0.") {
		t.Errorf("an answer carried another member's address: %s", answered)
	}
	if result.Kept != 1 {
		t.Errorf("one member was left connected, and the answer counts %d", result.Kept)
	}
}

// A member's own connections are what membership is for.
// Closing one because a sweep ran would make every roster change a glitch for everybody on it.
func TestAMemberIsLeftAlone(t *testing.T) {
	key := mustKey(t)
	live := &fakeRelay{live: []relay.Session{
		{Segment: "rtspsessions", ID: "watching", Path: key.Prefix() + "desk", User: mustMemberID(t, key, "alice"), State: "read"},
	}}

	if _, err := New(live).Set(key, []string{"alice", "bob"}); err != nil {
		t.Fatalf("setting a roster: %v", err)
	}
	if len(live.kicked) != 0 {
		t.Errorf("a member's own connection was closed: %v", live.kicked)
	}
}

// Enforcement runs on every membership change and on every read the relay reports, so a run that
// changes nothing has to cost nothing.
func TestASecondRunWithAnUnchangedRosterClosesNothing(t *testing.T) {
	key := mustKey(t)
	live := &fakeRelay{live: []relay.Session{
		{Segment: "srtconns", ID: "goes", Path: key.Prefix() + "desk", User: mustMemberID(t, key, "bob"), State: "read"},
		{Segment: "srtconns", ID: "stays", Path: key.Prefix() + "desk", User: mustMemberID(t, key, "alice"), State: "read"},
	}}
	registry := New(live)

	if _, err := registry.Set(key, []string{"alice"}); err != nil {
		t.Fatalf("setting a roster: %v", err)
	}
	before := len(live.kicked)

	result := registry.Reconcile(key.Prefix())
	if len(live.kicked) != before {
		t.Errorf("a second run closed %v", live.kicked[before:])
	}
	if len(result.Kicked) != 0 {
		t.Errorf("a second run reported %+v as closed", result.Kicked)
	}
}

// One relay carries every group, and a sweep reads all of them at once.
// Acting on a connection outside the group being enforced would let one voice channel close
// another's streams.
func TestAConnectionOutsideTheGroupIsNotTouched(t *testing.T) {
	here, elsewhere := mustKey(t), mustKey(t)
	live := &fakeRelay{live: []relay.Session{
		{Segment: "srtconns", ID: "theirs", Path: elsewhere.Prefix() + "desk", User: "somebody", State: "read"},
		{Segment: "srtconns", ID: "public", Path: group.PublicPrefix + "desk", User: "public", State: "read"},
	}}

	if _, err := New(live).Set(here, []string{"alice"}); err != nil {
		t.Fatalf("setting a roster: %v", err)
	}
	if len(live.kicked) != 0 {
		t.Errorf("enforcing one group closed %v", live.kicked)
	}
}

// A group nobody stated a roster for is one whose membership this service does not know.
// Kicking there would close every connection on a relay the moment one group started enforcing.
func TestAGroupWithNoRosterIsNotEnforced(t *testing.T) {
	key := mustKey(t)
	live := &fakeRelay{live: []relay.Session{
		{Segment: "srtconns", ID: "unknown", Path: key.Prefix() + "desk", User: "whoever", State: "read"},
	}}

	result := New(live).Reconcile(key.Prefix())
	if result.Enforced {
		t.Error("a group nobody stated a roster for reported as enforced")
	}
	if len(live.kicked) != 0 {
		t.Errorf("a group with no roster closed %v", live.kicked)
	}
}

// A member who left is out of the group, so what they were sharing goes with them, the same way
// leaving a voice channel ends what was being shared into it.
func TestALeaverLosesBothWhatTheyWatchAndWhatTheyShare(t *testing.T) {
	key := mustKey(t)
	bob := mustMemberID(t, key, "bob")
	live := &fakeRelay{live: []relay.Session{
		{Segment: "rtspsessions", ID: "bob-publish", Path: key.Prefix() + "bob", User: bob, State: "publish"},
		{Segment: "hlssessions", ID: "bob-watching", Path: key.Prefix() + "alice", User: bob, State: "read"},
	}}

	if _, err := New(live).Set(key, []string{"alice"}); err != nil {
		t.Fatalf("setting a roster: %v", err)
	}
	slices.Sort(live.kicked)
	if !slices.Equal(live.kicked, []string{"bob-publish", "bob-watching"}) {
		t.Errorf("a leaver kept something: closed %v", live.kicked)
	}
}

// A member watching three streams holds three connections, and leaving takes all of them.
func TestEveryConnectionOneNonMemberHoldsGoes(t *testing.T) {
	key := mustKey(t)
	bob := mustMemberID(t, key, "bob")
	live := &fakeRelay{live: []relay.Session{
		{Segment: "srtconns", ID: "one", Path: key.Prefix() + "a", User: bob, State: "read"},
		{Segment: "rtspsessions", ID: "two", Path: key.Prefix() + "b", User: bob, State: "read"},
		{Segment: "webrtcsessions", ID: "three", Path: key.Prefix() + "c", User: bob, State: "read"},
	}}

	if _, err := New(live).Set(key, []string{"alice"}); err != nil {
		t.Fatalf("setting a roster: %v", err)
	}
	if len(live.kicked) != 3 {
		t.Errorf("one member on three streams left %d connections open: %v", 3-len(live.kicked), live.kicked)
	}
}

// A kick the relay would not perform is a member still watching, so it is reported rather than
// counted as a removal.
func TestAKickTheRelayRefusedIsReported(t *testing.T) {
	key := mustKey(t)
	live := &fakeRelay{
		live: []relay.Session{
			{Segment: "srtconns", ID: "stubborn", Path: key.Prefix() + "desk", User: "nobody", State: "read"},
		},
		refuse: map[string]error{"stubborn": errors.New("session not found")},
	}

	result, err := New(live).Set(key, []string{"alice"})
	if err != nil {
		t.Fatalf("setting a roster: %v", err)
	}
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

// A sweep that could not read every list may have missed the one connection that mattered, so the
// answer carries that rather than reporting a clean enforcement.
func TestListsThatCouldNotBeReadTravelWithTheAnswer(t *testing.T) {
	key := mustKey(t)
	live := &fakeRelay{listErr: errors.New("the relay answered 500")}

	result, err := New(live).Set(key, []string{"alice"})
	if err != nil {
		t.Fatalf("setting a roster: %v", err)
	}
	if len(result.Unread) != 1 {
		t.Fatalf("a list that could not be read was dropped: %+v", result)
	}
}

// Clearing is how a group stops being enforced, which is what a voice channel emptying means.
func TestClearingARosterStopsEnforcement(t *testing.T) {
	key := mustKey(t)
	live := &fakeRelay{live: []relay.Session{
		{Segment: "srtconns", ID: "after", Path: key.Prefix() + "desk", User: "nobody", State: "read"},
	}}
	registry := New(live)

	if _, err := registry.Set(key, []string{"alice"}); err != nil {
		t.Fatalf("setting a roster: %v", err)
	}
	if !registry.Clear(key) {
		t.Error("clearing a roster that was set reported nothing to clear")
	}

	live.live = []relay.Session{
		{Segment: "srtconns", ID: "later", Path: key.Prefix() + "desk", User: "nobody", State: "read"},
	}
	result := registry.Reconcile(key.Prefix())
	if result.Enforced {
		t.Error("a cleared group is still enforced")
	}
	if slices.Contains(live.kicked, "later") {
		t.Error("a cleared group still closed a connection")
	}
}

// A member with no name derives no id, so a roster carrying one would silently hold a member nothing
// can match.
func TestAMemberWithNoNameIsRefused(t *testing.T) {
	key := mustKey(t)

	if _, err := New(&fakeRelay{}).Set(key, []string{"alice", "  "}); err == nil {
		t.Error("a roster took a member with no name")
	}
}

// The view is what a reader is shown of a group: who is on the roster, and who each live connection
// belongs to.
func TestTheViewNamesWhoHoldsEachConnection(t *testing.T) {
	key := mustKey(t)
	live := &fakeRelay{live: []relay.Session{
		{Segment: "srtconns", ID: "hers", Path: key.Prefix() + "desk", User: mustMemberID(t, key, "alice"), State: "read"},
		{Segment: "srtconns", ID: "nobodys", Path: key.Prefix() + "desk", User: "stranger", State: "read"},
	}}
	registry := New(live)
	if _, err := registry.Set(key, []string{"alice"}); err != nil {
		t.Fatalf("setting a roster: %v", err)
	}

	// Set closed the stranger's, so the view is taken of what a stranger's reconnect looks like.
	live.live = append(live.live, relay.Session{
		Segment: "srtconns", ID: "nobodys", Path: key.Prefix() + "desk", User: "stranger", State: "read",
	})

	view := registry.View(key)
	if !view.Enforced || !slices.Equal(view.Members, []string{"alice"}) {
		t.Fatalf("the view carries %+v", view)
	}

	held := map[string]Connection{}
	for _, seen := range view.Sessions {
		held[seen.Member] = seen
	}
	if held["alice"].Stream != "desk" {
		t.Errorf("a member's own connection came back as %+v", held["alice"])
	}
	// The subject no member derives is named by nobody, which is what a stranger looks like between
	// the sweep that finds them and the kick that closes them.
	if _, stranger := held[""]; !stranger {
		t.Errorf("a subject no member derives was named: %+v", view.Sessions)
	}
}

// A view names each connection by the stream inside the group, which is the name the index answers
// under, so the two join without a caller deriving the prefix rule a second time.
func TestTheViewNamesStreamsTheWayTheIndexDoes(t *testing.T) {
	key := mustKey(t)
	live := &fakeRelay{live: []relay.Session{
		{Segment: "srtconns", Transport: "srt", ID: "one", Path: key.Prefix() + "desk",
			User: mustMemberID(t, key, "alice"), State: "read"},
	}}
	registry := New(live)
	if _, err := registry.Set(key, []string{"alice"}); err != nil {
		t.Fatalf("setting a roster: %v", err)
	}

	view := registry.View(key)
	if len(view.Sessions) != 1 {
		t.Fatalf("the view carries %+v", view.Sessions)
	}
	if view.Sessions[0].Stream != "desk" {
		t.Errorf("the view names the stream %q, where the index answers \"desk\"", view.Sessions[0].Stream)
	}
	if view.Sessions[0].Transport != "srt" {
		t.Errorf("the view names the leg %q in the relay's vocabulary rather than this app's",
			view.Sessions[0].Transport)
	}
}

// Streams under the public prefix are watchable by anybody, so there is no membership to hold them
// against and a run there closes nothing.
func TestThePublicPrefixIsNeverEnforced(t *testing.T) {
	live := &fakeRelay{live: []relay.Session{
		{Segment: "srtconns", ID: "watcher", Path: group.PublicPrefix + "desk", User: "public", State: "read"},
	}}

	result := New(live).Reconcile(group.PublicPrefix)
	if result.Enforced {
		t.Error("the public prefix reported as enforced")
	}
	if len(live.kicked) != 0 {
		t.Errorf("a run on the public prefix closed %v", live.kicked)
	}
}

// A member watching a public stream is watching something their group does not own, so their own
// group's roster leaves it alone.
func TestAMembersPublicViewingIsNotTheGroupsToClose(t *testing.T) {
	key := mustKey(t)
	live := &fakeRelay{live: []relay.Session{
		{Segment: "srtconns", ID: "public-read", Path: group.PublicPrefix + "desk",
			User: mustMemberID(t, key, "bob"), State: "read"},
	}}

	result, err := New(live).Set(key, []string{"alice"})
	if err != nil {
		t.Fatalf("setting a roster: %v", err)
	}
	if len(live.kicked) != 0 {
		t.Errorf("enforcing a group closed a public stream's reader: %v", live.kicked)
	}
	if result.Kept != 0 {
		t.Errorf("a public stream's reader was counted as the group's: %+v", result)
	}
}
