package app

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/events"
	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/groupclient"
	"bjoernblessin.de/screenshare/internal/member"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/text"
	"bjoernblessin.de/screenshare/internal/wire"
)

// The relay closes a connection without stating a reason, so a stream that stopped reads as a
// membership the group stopped honouring or as an ordinary drop only against the presence this
// machine last stated.
// These tests are that reading, one arm at a time, and the statements that produce it.

// The group these tests are in, derived from a key of zeroes: what a member holds is the id, and
// nothing here asks what the key is worth.
var (
	aGroupKey = group.Key(make([]byte, group.KeyBytes)).String()
	aGroupID  = group.Key(make([]byte, group.KeyBytes)).ID()
)

// aMemberID is the member the faked service answers this machine with.
const aMemberID = "MFZWIZLTOQ2DGNBV"

// isolateConfig points os.UserConfigDir at a fresh temp directory, so an identity a test draws lands
// there rather than in the developer's own config directory.
//
// All three variables, os.UserConfigDir reading a different one per platform: XDG_CONFIG_HOME on
// Linux, AppData on Windows, HOME on macOS.
func isolateConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("AppData", dir)
	t.Setenv("HOME", dir)

	got, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("isolating the config directory: %v", err)
	}
	if !strings.HasPrefix(got, dir) {
		t.Fatalf("os.UserConfigDir is %s, outside the temp directory %s: this test would write into the real config directory", got, dir)
	}
}

// fakeGroups answers as a group service would and records what was asked of it, in order.
//
// Presence, join and leave are one statement in one order, so two of them at the service at once is
// the defect that order exists to prevent, and overlapped is where it shows.
type fakeGroups struct {
	// state answers a statement of presence, nil answering the lease of a group this machine is alone
	// in.
	// Set before the goroutines that reach it start.
	state func() (groupclient.Membership, error)

	mu         sync.Mutex
	calls      []string
	running    string
	overlapped bool
	forgets    int
}

func (f *fakeGroups) begin(what string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.running != "" {
		f.overlapped = true
	}
	f.running = what
	f.calls = append(f.calls, what)
}

func (f *fakeGroups) end() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.running = ""
}

func (f *fakeGroups) State(base, groupKey, memberSecret, displayName string) (groupclient.Membership, error) {
	f.begin("state")
	defer f.end()

	if f.state != nil {
		return f.state()
	}
	return takenPresence(displayName), nil
}

func (f *fakeGroups) Release(base, groupKey, memberSecret string) error {
	f.begin("release")
	defer f.end()
	return nil
}

func (f *fakeGroups) Token(base, groupKey, memberSecret string) (string, error) {
	return "a token", nil
}

func (f *fakeGroups) Streams(base, groupKey string) ([]groupclient.Stream, error) { return nil, nil }

func (f *fakeGroups) CreateGroup(base string) (string, string, error) {
	return aGroupKey, aGroupID, nil
}

func (f *fakeGroups) Forget() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forgets++
}

// asked is the calls the service took, in the order it took them.
func (f *fakeGroups) asked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.calls...)
}

// dropped is how many times the held relay token was thrown away.
func (f *fakeGroups) dropped() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.forgets
}

// takenPresence is a group of one, as the service answers a member whose presence it took.
func takenPresence(displayName string) groupclient.Membership {
	return groupclient.Membership{
		MemberID:     aMemberID,
		DisplayName:  displayName,
		LeaseSeconds: 20,
		Members:      []groupclient.Member{{MemberID: aMemberID, DisplayName: displayName}},
	}
}

// notAnswering is what a call to a service that is not there leaves: a transport failure, and
// nothing the service stated (internal/groupclient).
func notAnswering() error {
	return errors.New("the group service at 127.0.0.1 cannot be reached: connection refused")
}

// nameTaken is the one refusal this app has a code for.
func nameTaken() error {
	return &groupclient.Refusal{Status: 409, Reason: "that name is taken in this group"}
}

// inGroupApp is an app whose settings name a relay and a group, with groups answering for the
// service and the identity directory pointed at a temp directory.
func inGroupApp(t *testing.T, groups groupService) *App {
	t.Helper()
	isolateConfig(t)

	return &App{
		events: events.New(),
		groups: groups,
		settings: settings.Settings{Relay: settings.Relay{
			Host:        "127.0.0.1",
			GroupKey:    aGroupKey,
			DisplayName: "Björn",
		}},
	}
}

// joinIdentity draws what a machine that has joined this group holds.
func joinIdentity(t *testing.T) {
	t.Helper()
	if _, err := member.Join(aGroupID, "Björn"); err != nil {
		t.Fatalf("drawing this machine's identity in group %s: %v", aGroupID, err)
	}
}

// A member whose presence the group service took holds a lease the relay honours, so a stream that
// stopped stopped for something this snapshot cannot speak for, and the child's own words are the
// whole of what is known.
func TestAHeldPresenceNamesNoCauseForAStoppedStream(t *testing.T) {
	m := membership{Group: aGroupID, Joined: true, Taken: time.Now(), Lease: 20 * time.Second}

	if failure := m.failure(); failure != nil {
		t.Errorf("a held presence reads a stopped stream as %v, want the child's own words alone", failure.GetCode())
	}
}

// A machine in no group has no membership behind a stream that stopped, so there is nothing here to
// say about it.
func TestAMachineInNoGroupNamesNoCauseForAStoppedStream(t *testing.T) {
	if failure := (membership{}).failure(); failure != nil {
		t.Errorf("a machine in no group reads a stopped stream as %v, want the child's own words alone", failure.GetCode())
	}
}

// A machine with a group key and no identity in that group trades a token as the group itself, which
// the relay closes the moment any member states presence, and joining is what fixes it.
func TestAGroupNeverJoinedReadsAStoppedStreamAsALapsedMembership(t *testing.T) {
	m := membership{Group: aGroupID}

	if got := m.failure().GetCode(); got != screensharev1.TextCode_TEXT_CODE_GROUP_MEMBERSHIP_LAPSED {
		t.Errorf("a group this machine never joined reads a stopped stream as %v, want a lapsed membership", got)
	}
}

// A name another member holds leaves this machine without a member id the relay honours, which is
// what turns a closed connection into a sentence a reader can act on.
func TestARefusedNameReadsAStoppedStreamAsALapsedMembership(t *testing.T) {
	m := membership{
		Group:   aGroupID,
		Joined:  true,
		Refusal: text.Of(screensharev1.TextCode_TEXT_CODE_GROUP_NAME_TAKEN),
	}

	if got := m.failure().GetCode(); got != screensharev1.TextCode_TEXT_CODE_GROUP_MEMBERSHIP_LAPSED {
		t.Errorf("a refused name reads a stopped stream as %v, want a lapsed membership", got)
	}
}

// Presence the service took and stopped taking is a lease that ran out, which the relay answers by
// closing what this machine holds.
func TestPresenceOlderThanItsLeaseReadsAStoppedStreamAsALapsedMembership(t *testing.T) {
	m := membership{Group: aGroupID, Joined: true, Taken: time.Now().Add(-time.Minute), Lease: 20 * time.Second}

	if !m.lapsed() {
		t.Fatalf("presence taken a minute ago on a %s lease reports standing, want it lapsed", m.Lease)
	}
	if got := m.failure().GetCode(); got != screensharev1.TextCode_TEXT_CODE_GROUP_MEMBERSHIP_LAPSED {
		t.Errorf("a lapsed lease reads a stopped stream as %v, want a lapsed membership", got)
	}
}

// Presence stated on the last pass stands for the whole lease, so the pass after it is not a lapse.
func TestPresenceInsideItsLeaseHasNotLapsed(t *testing.T) {
	m := membership{Group: aGroupID, Joined: true, Taken: time.Now().Add(-2 * time.Second), Lease: 20 * time.Second}

	if m.lapsed() {
		t.Errorf("presence taken 2s ago on a %s lease reports lapsed, want it standing", m.Lease)
	}
}

// A service that answered nothing this app can act on is a different fact from a membership that
// lapsed: the refusal travels whole, so the ground it carries reaches the reader with it.
func TestAServiceThatRefusedReadsAStoppedStreamAsItsOwnRefusal(t *testing.T) {
	m := membership{
		Group:   aGroupID,
		Joined:  true,
		Refusal: text.Of(screensharev1.TextCode_TEXT_CODE_GROUP_SERVICE_REFUSED),
	}

	if got := m.failure().GetCode(); got != screensharev1.TextCode_TEXT_CODE_GROUP_SERVICE_REFUSED {
		t.Errorf("a refusing service reads a stopped stream as %v, want the service's own refusal", got)
	}
}

// A refusal is something the service stated. A name clash is the one ground it carries a code for,
// every other stated ground is the service saying no in words this app cannot name, and a service
// that could not be reached refused nothing at all.
func TestOnlyARefusalTheServiceStatedIsOne(t *testing.T) {
	if got := membershipRefusal(nameTaken()).GetCode(); got != screensharev1.TextCode_TEXT_CODE_GROUP_NAME_TAKEN {
		t.Errorf("a 409 states %v, want the name being taken", got)
	}

	other := &groupclient.Refusal{Status: 400, Reason: "this group states its members, and this request names none"}
	if got := membershipRefusal(other).GetCode(); got != screensharev1.TextCode_TEXT_CODE_GROUP_SERVICE_REFUSED {
		t.Errorf("a refusal on another ground states %v, want the service refusing", got)
	}

	if refusal := membershipRefusal(notAnswering()); refusal != nil {
		t.Errorf("a service that could not be reached states %v, want no refusal", refusal.GetCode())
	}
}

// A pass that did not reach the service refused nothing, so the presence the service took last
// stands with the lease that came with it: a 20 s lease survives nine passes that never landed.
func TestAPassThatDidNotReachTheServiceKeepsTheLeaseItGranted(t *testing.T) {
	groups := &fakeGroups{state: func() (groupclient.Membership, error) {
		return groupclient.Membership{}, notAnswering()
	}}
	a := inGroupApp(t, groups)
	joinIdentity(t)
	a.setMembership(membership{
		Group:   aGroupID,
		Joined:  true,
		Taken:   time.Now(),
		Lease:   20 * time.Second,
		Members: []wire.Member{{MemberID: aMemberID, DisplayName: "Björn", Self: true}},
	})

	a.statePresence()

	held := a.membership()
	if !held.Joined || len(held.Members) != 1 {
		t.Errorf("a pass that did not reach the service landed joined=%v with %d members, want the group it last read",
			held.Joined, len(held.Members))
	}
	if held.Refusal != nil {
		t.Errorf("a service that could not be reached lands the refusal %v, want none", held.Refusal.GetCode())
	}
	if failure := held.failure(); failure != nil {
		t.Errorf("a stopped stream under a lease that still stands reads as %v, want the child's own words alone",
			failure.GetCode())
	}
}

// The lease is what membership lapses on, so presence older than the one it was granted lapses
// whether or not the service is answering.
func TestALeaseNoPassRestatedLapses(t *testing.T) {
	groups := &fakeGroups{state: func() (groupclient.Membership, error) {
		return groupclient.Membership{}, notAnswering()
	}}
	a := inGroupApp(t, groups)
	joinIdentity(t)
	a.setMembership(membership{
		Group:  aGroupID,
		Joined: true,
		Taken:  time.Now().Add(-25 * time.Second),
		Lease:  20 * time.Second,
	})

	a.statePresence()

	held := a.membership()
	if !held.lapsed() {
		t.Fatalf("presence taken 25s ago on a %s lease reports standing, want it lapsed", held.Lease)
	}
	if got := held.failure().GetCode(); got != screensharev1.TextCode_TEXT_CODE_GROUP_MEMBERSHIP_LAPSED {
		t.Errorf("a lease no pass restated reads a stopped stream as %v, want a lapsed membership", got)
	}
}

// A refusal this app made for want of a name is answered by a pass that read one, so it does not
// survive a pass that could not reach the service.
func TestAPassThatReadANameDropsTheRefusalForWantOfOne(t *testing.T) {
	groups := &fakeGroups{state: func() (groupclient.Membership, error) {
		return groupclient.Membership{}, notAnswering()
	}}
	a := inGroupApp(t, groups)
	joinIdentity(t)
	a.setMembership(membership{
		Group:   aGroupID,
		Joined:  true,
		Refusal: text.Of(screensharev1.TextCode_TEXT_CODE_GROUP_NAME_MISSING),
	})

	a.statePresence()

	if refusal := a.membership().Refusal; refusal != nil {
		t.Errorf("a pass over a machine that has a name lands %v, want no refusal", refusal.GetCode())
	}
}

// A refusal the service stated is held as one, which is what a stopped stream is read against.
func TestARefusalTheServiceStatedIsLanded(t *testing.T) {
	groups := &fakeGroups{state: func() (groupclient.Membership, error) {
		return groupclient.Membership{}, nameTaken()
	}}
	a := inGroupApp(t, groups)
	joinIdentity(t)

	a.statePresence()

	held := a.membership()
	if got := held.Refusal.GetCode(); got != screensharev1.TextCode_TEXT_CODE_GROUP_NAME_TAKEN {
		t.Errorf("a name another member holds lands %v, want the name being taken", got)
	}
	if got := held.failure().GetCode(); got != screensharev1.TextCode_TEXT_CODE_GROUP_MEMBERSHIP_LAPSED {
		t.Errorf("a refused name reads a stopped stream as %v, want a lapsed membership", got)
	}
}

// The pass reads the identity file, so a group key with no identity beside it lands the fact that
// this machine is in the group's paths and not in the group.
// Nothing is stated at the service: there is no member to state.
func TestAPassOverAGroupNeverJoinedLandsThatItIsNotIn(t *testing.T) {
	groups := &fakeGroups{}
	a := inGroupApp(t, groups)

	a.statePresence()

	held := a.membership()
	if held.Joined {
		t.Error("a machine that never joined reports being in the group")
	}
	if held.Group != aGroupID {
		t.Errorf("a pass over group %s landed group %q, want the group the settings name", aGroupID, held.Group)
	}
	if got := held.failure().GetCode(); got != screensharev1.TextCode_TEXT_CODE_GROUP_MEMBERSHIP_LAPSED {
		t.Errorf("a machine that never joined reads a stopped stream as %v, want a lapsed membership", got)
	}
	if asked := groups.asked(); len(asked) != 0 {
		t.Errorf("a machine with no member secret asked the service %v, want nothing stated", asked)
	}
}

// A tile that is not live says why where the reason is known, and says nothing while a decode is
// merely opening.
// The relay snapshot is the whole source: a stream the relay stopped carrying is one no decode is
// going to receive.
func TestATileSaysTheStreamLeftTheRelay(t *testing.T) {
	a := &App{}
	a.relayLast.Store(&relay.Status{Reachable: true, Paths: []relay.Path{{Name: "alice"}}})

	if got := a.receiveFailure("bob", false).GetCode(); got != screensharev1.TextCode_TEXT_CODE_STREAM_LEFT_THE_RELAY {
		t.Errorf("a stream the relay does not carry says %v, want that it left the relay", got)
	}
	if failure := a.receiveFailure("alice", false); failure != nil {
		t.Errorf("a decode opening on a stream the relay carries says %v, want nothing", failure.GetCode())
	}
	if failure := a.receiveFailure("bob", true); failure != nil {
		t.Errorf("a decode carrying frames says %v, want nothing", failure.GetCode())
	}
}

// A snapshot nothing has answered says nothing about any stream, which reads differently from one
// that answered and did not carry it.
func TestATileSaysNothingWhileTheRelayHasNotAnswered(t *testing.T) {
	a := &App{}

	if failure := a.receiveFailure("bob", false); failure != nil {
		t.Errorf("a decode on an unpolled relay says %v, want nothing", failure.GetCode())
	}
}

// A machine in no group is already what leaving names, so nothing is released and nothing fails, on
// the first call and on the second.
func TestLeavingAGroupThisMachineIsNotInSucceeds(t *testing.T) {
	a := &App{events: events.New()}

	for attempt := 1; attempt <= 2; attempt++ {
		if err := a.LeaveGroup(); err != nil {
			t.Fatalf("leave %d answered %v, want the machine in no group", attempt, err)
		}
		if held := a.membership(); held.Joined {
			t.Errorf("after leave %d this machine reports being in a group", attempt)
		}
	}
}

// A group is joined by its group key, so a machine with none has nowhere to draw an identity and states no
// presence rather than stating it nowhere.
func TestJoiningWithNoGroupStatesNoPresence(t *testing.T) {
	a := &App{events: events.New()}

	if err := a.JoinGroup(); err == nil {
		t.Fatal("a machine naming no group joined one, want the call refused")
	}
	if held := a.membership(); held.Joined {
		t.Error("a machine naming no group reports being in one")
	}
}

// The pass that polls the relay states presence too, and a machine in no group has none to state.
// What it lands is the empty group rather than whatever was there before, so a group key taken out of
// the settings empties the list a shell draws.
func TestAPassOverNoGroupStatesNothing(t *testing.T) {
	a := &App{events: events.New()}
	a.setMembership(membership{Group: aGroupID, Joined: true, Members: []wire.Member{{MemberID: aMemberID}}})

	a.statePresence()

	held := a.membership()
	if held.Joined || len(held.Members) != 0 || held.Group != "" {
		t.Errorf("a pass over no group landed group %q joined=%v with %d members, want the empty group",
			held.Group, held.Joined, len(held.Members))
	}
}

// Membership outranks the relay snapshot: a machine the group stopped honouring loses every stream
// at once, and "the stream left the relay" would send the reader after the publisher instead.
func TestATileSaysMembershipLapsedBeforeItSaysTheStreamLeft(t *testing.T) {
	a := &App{}
	a.relayLast.Store(&relay.Status{Reachable: true, Paths: []relay.Path{}})
	a.setMembership(membership{
		Group:   aGroupID,
		Joined:  true,
		Refusal: text.Of(screensharev1.TextCode_TEXT_CODE_GROUP_NAME_TAKEN),
	})

	if got := a.receiveFailure("bob", false).GetCode(); got != screensharev1.TextCode_TEXT_CODE_GROUP_MEMBERSHIP_LAPSED {
		t.Errorf("a tile on a machine whose membership lapsed says %v, want the lapse", got)
	}
}

// Presence, join and leave are one statement in one order.
// A pass in flight when a leave arrives lands before the release, so the machine that left is not
// stated back into the group by the answer it was already waiting for.
func TestALeaveIsNotOvertakenByThePassInFlight(t *testing.T) {
	entered := make(chan struct{})
	proceed := make(chan struct{})
	groups := &fakeGroups{}
	groups.state = func() (groupclient.Membership, error) {
		close(entered)
		<-proceed
		return takenPresence("Björn"), nil
	}

	a := inGroupApp(t, groups)
	joinIdentity(t)

	pass := make(chan struct{})
	go func() {
		defer close(pass)
		a.statePresence()
	}()
	<-entered

	left := make(chan error, 1)
	go func() { left <- a.LeaveGroup() }()

	// The window the leave has to overtake the pass in flight, which it takes where nothing holds the
	// two of them in one order.
	time.Sleep(100 * time.Millisecond)
	close(proceed)

	if err := <-left; err != nil {
		t.Fatalf("leaving the group answered %v, want it left", err)
	}
	<-pass

	if held := a.membership(); held.Joined {
		t.Error("a machine that left reports being in the group, so the pass in flight landed after the release")
	}
	if _, joined, err := member.Load(aGroupID); joined || err != nil {
		t.Errorf("a machine that left holds an identity in the group it left: %v", err)
	}
	if groups.overlapped {
		t.Error("a statement of presence and a release ran at the service at once")
	}
	if want := []string{"state", "release"}; strings.Join(groups.asked(), ",") != strings.Join(want, ",") {
		t.Errorf("the service was asked %v, want %v", groups.asked(), want)
	}
}

// The identity file states the name the group took, so a name another member holds does not land in
// it: the machine goes on stating presence under the name it holds.
func TestAJoinRefusedForItsNameStoresNothing(t *testing.T) {
	groups := &fakeGroups{}
	a := inGroupApp(t, groups)

	if err := a.JoinGroup(); err != nil {
		t.Fatalf("joining the group answered %v, want it joined", err)
	}

	groups.state = func() (groupclient.Membership, error) { return groupclient.Membership{}, nameTaken() }
	a.settings.Relay.DisplayName = "Anna"

	if err := a.JoinGroup(); err == nil {
		t.Fatal("joining under a name another member holds succeeded, want the claim refused")
	}

	held, joined, err := member.Load(aGroupID)
	if err != nil || !joined {
		t.Fatalf("the identity read back as %+v, joined=%v: %v", held, joined, err)
	}
	if held.DisplayName != "Björn" {
		t.Errorf("the identity file states %q, want the name the group took", held.DisplayName)
	}
}

// A name the group took is what the identity file states, so the next pass claims the name this
// machine is known by rather than the one it was known by before.
func TestAJoinUnderANewNameStoresTheNameTheGroupTook(t *testing.T) {
	a := inGroupApp(t, &fakeGroups{})

	if err := a.JoinGroup(); err != nil {
		t.Fatalf("joining the group answered %v, want it joined", err)
	}
	a.settings.Relay.DisplayName = "Anna"
	if err := a.JoinGroup(); err != nil {
		t.Fatalf("joining under another name answered %v, want the name claimed", err)
	}

	held, joined, err := member.Load(aGroupID)
	if err != nil || !joined {
		t.Fatalf("the identity read back as %+v, joined=%v: %v", held, joined, err)
	}
	if held.DisplayName != "Anna" {
		t.Errorf("the identity file states %q, want the name the group took", held.DisplayName)
	}
}

// A second join over unchanged settings changes nothing.
// The relay token names the member id this machine already holds, so dropping it would spend a round
// trip on a credential that was good.
func TestASecondJoinKeepsTheRelayToken(t *testing.T) {
	groups := &fakeGroups{}
	a := inGroupApp(t, groups)

	if err := a.JoinGroup(); err != nil {
		t.Fatalf("first join answered %v, want the group joined", err)
	}
	drawn := groups.dropped()

	if err := a.JoinGroup(); err != nil {
		t.Fatalf("second join answered %v, want the state the first one reached", err)
	}

	if dropped := groups.dropped() - drawn; dropped != 0 {
		t.Errorf("a second join dropped the relay token %d times, want the token it already holds", dropped)
	}
}

// The first join draws the identity every later token names, so the token held before it is spent.
func TestAFirstJoinDropsTheTokenMintedWithoutAMember(t *testing.T) {
	groups := &fakeGroups{}
	a := inGroupApp(t, groups)

	if err := a.JoinGroup(); err != nil {
		t.Fatalf("joining the group answered %v, want it joined", err)
	}

	if dropped := groups.dropped(); dropped != 1 {
		t.Errorf("the join that drew this machine's identity dropped the relay token %d times, want once", dropped)
	}
}

// An identity drawn by a join the service refused is dropped with it, so nothing is left stating
// presence under a name this machine never claimed.
func TestAJoinRefusedDrawsNoIdentity(t *testing.T) {
	groups := &fakeGroups{state: func() (groupclient.Membership, error) {
		return groupclient.Membership{}, nameTaken()
	}}
	a := inGroupApp(t, groups)

	if err := a.JoinGroup(); err == nil {
		t.Fatal("joining under a name another member holds succeeded, want the claim refused")
	}

	if _, joined, err := member.Load(aGroupID); joined || err != nil {
		t.Errorf("a refused join left this machine holding an identity: %v", err)
	}
	if held := a.membership(); held.Joined {
		t.Error("a refused join reports this machine in the group")
	}
}
