package channelgroup

import (
	"errors"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/groupclient"
	"bjoernblessin.de/screenshare/internal/linkstore"
	"bjoernblessin.de/screenshare/internal/voiceroster"
)

// fakeGroups answers as groupd would, off real key derivations,
// so a test reads prefixes and member ids the way the relay would see them.
type fakeGroups struct {
	mu       sync.Mutex
	created  int
	stated   [][2]string // groupKey, displayName per State call
	released []string    // memberSecret per Release call
	// names holds each group's claimed display names by member id, the first claim holding.
	names map[string]map[string]string
}

func newFakeGroups() *fakeGroups {
	return &fakeGroups{names: map[string]map[string]string{}}
}

func (f *fakeGroups) CreateGroup(base string) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created++
	key, err := group.NewKey()
	if err != nil {
		return "", "", err
	}
	return key.String(), key.ID(), nil
}

func (f *fakeGroups) State(base, groupKey, memberSecret, displayName string) (groupclient.Membership, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key, err := group.ParseKey(groupKey)
	if err != nil {
		return groupclient.Membership{}, err
	}
	secret, err := group.ParseMemberSecret(memberSecret)
	if err != nil {
		return groupclient.Membership{}, err
	}
	memberID := key.MemberID(secret)

	claimed := f.names[groupKey]
	if claimed == nil {
		claimed = map[string]string{}
		f.names[groupKey] = claimed
	}
	for other, name := range claimed {
		if name == displayName && other != memberID {
			return groupclient.Membership{}, &groupclient.Refusal{
				Status: http.StatusConflict,
				Reason: "another member of this group is already called that",
			}
		}
	}
	if _, held := claimed[memberID]; !held {
		claimed[memberID] = displayName
	}

	f.stated = append(f.stated, [2]string{groupKey, displayName})

	members := make([]groupclient.Member, 0, len(claimed))
	for id, name := range claimed {
		members = append(members, groupclient.Member{MemberID: id, DisplayName: name})
	}
	return groupclient.Membership{
		MemberID:     memberID,
		DisplayName:  claimed[memberID],
		LeaseSeconds: 20,
		Members:      members,
	}, nil
}

func (f *fakeGroups) Release(base, groupKey, memberSecret string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	key, _ := group.ParseKey(groupKey)
	secret, _ := group.ParseMemberSecret(memberSecret)
	if key != nil && secret != nil {
		delete(f.names[groupKey], key.MemberID(secret))
	}
	f.released = append(f.released, memberSecret)
	return nil
}

func (f *fakeGroups) Streams(base, groupKey string) ([]groupclient.Stream, error) {
	return []groupclient.Stream{{Name: "bob/monitor-0", Ready: true}}, nil
}

func (f *fakeGroups) Token(base, groupKey, memberSecret string) (string, error) {
	return "token-for-" + groupKey[:8], nil
}

// rig is one broker with everything it reads, and the clock the test turns.
type rig struct {
	broker *Broker
	groups *fakeGroups
	roster *voiceroster.Roster
	links  *linkstore.Store
	now    time.Time
}

func newRig(t *testing.T) *rig {
	t.Helper()

	links, err := linkstore.Open(filepath.Join(t.TempDir(), "links.json"))
	if err != nil {
		t.Fatalf("opening the link store: %v", err)
	}

	r := &rig{groups: newFakeGroups(), links: links, now: time.Unix(1000, 0)}
	r.broker = New(r.groups, "http://groupd", links, func() time.Time { return r.now })
	r.roster = voiceroster.New(r.broker.Leave)
	r.broker.ReadOccupancy(r.roster)
	return r
}

// link draws a link secret for a user and puts them in a channel.
func (r *rig) link(t *testing.T, userID string) string {
	t.Helper()
	secret, err := r.links.Draw(userID)
	if err != nil {
		t.Fatalf("drawing a link: %v", err)
	}
	return secret
}

func (r *rig) enter(userID, channelID, nick string) {
	r.roster.Apply(voiceroster.Presence{
		UserID: userID, GuildID: "g1", ChannelID: channelID, DisplayName: nick,
		GuildName: "Guild", ChannelName: "General",
	})
}

func (r *rig) leave(userID string) {
	r.roster.Apply(voiceroster.Presence{UserID: userID})
}

func TestPresenceForAnUnknownSecretIsRefused(t *testing.T) {
	r := newRig(t)

	_, err := r.broker.Presence("bm90LWEtc2VjcmV0")
	if !errors.Is(err, ErrUnlinked) {
		t.Fatalf("a secret nothing drew is unlinked, got %v", err)
	}
}

func TestPresenceOutsideAnyChannelIsEmpty(t *testing.T) {
	r := newRig(t)
	secret := r.link(t, "u1")

	answer, err := r.broker.Presence(secret)
	if err != nil {
		t.Fatalf("an empty answer is still an answer: %v", err)
	}
	if answer.Channel != nil || answer.Group != nil {
		t.Fatalf("outside any channel there is no channel and no group, got %+v", answer)
	}
	if r.groups.created != 0 {
		t.Fatalf("no channel spawns no group, created %d", r.groups.created)
	}
}

func TestPresenceInAChannelJoinsItsGroup(t *testing.T) {
	r := newRig(t)
	secret := r.link(t, "u1")
	r.enter("u1", "c1", "Bob")

	answer, err := r.broker.Presence(secret)
	if err != nil {
		t.Fatalf("stating presence: %v", err)
	}

	if answer.Channel == nil || answer.Channel.Guild != "Guild" || answer.Channel.Name != "General" {
		t.Fatalf("the answer names the channel, got %+v", answer.Channel)
	}
	g := answer.Group
	if g == nil {
		t.Fatal("standing in a channel is standing in its group")
	}
	if g.Prefix == "" || g.SrtPassphrase == "" || g.MemberID == "" {
		t.Fatalf("the answer carries the derived facts, got %+v", g)
	}
	if g.DisplayName != "Bob" {
		t.Fatalf("the member is called by their Discord name, got %q", g.DisplayName)
	}
	if g.LeaseSeconds != 20 || len(g.Members) != 1 || len(g.Streams) != 1 {
		t.Fatalf("the answer carries lease, members and streams, got %+v", g)
	}
	if r.groups.created != 1 {
		t.Fatalf("the first member draws the channel's group, created %d", r.groups.created)
	}
}

func TestASecondPassRefreshesTheSameMember(t *testing.T) {
	r := newRig(t)
	secret := r.link(t, "u1")
	r.enter("u1", "c1", "Bob")

	first, _ := r.broker.Presence(secret)
	second, err := r.broker.Presence(secret)
	if err != nil {
		t.Fatalf("restating presence: %v", err)
	}

	if r.groups.created != 1 {
		t.Fatalf("a state already true draws nothing, created %d", r.groups.created)
	}
	if len(r.groups.stated) != 2 {
		t.Fatalf("every pass restates presence, stated %d", len(r.groups.stated))
	}
	if first.Group.MemberID != second.Group.MemberID {
		t.Fatal("one install in one channel is one member")
	}
}

func TestTwoUsersInOneChannelShareTheGroup(t *testing.T) {
	r := newRig(t)
	bob := r.link(t, "u1")
	eve := r.link(t, "u2")
	r.enter("u1", "c1", "Bob")
	r.enter("u2", "c1", "Eve")

	first, _ := r.broker.Presence(bob)
	second, err := r.broker.Presence(eve)
	if err != nil {
		t.Fatalf("stating the second presence: %v", err)
	}

	if r.groups.created != 1 {
		t.Fatalf("one channel is one group, created %d", r.groups.created)
	}
	if first.Group.Prefix != second.Group.Prefix {
		t.Fatal("both members stand under the channel's one prefix")
	}
	if first.Group.MemberID == second.Group.MemberID {
		t.Fatal("two installs are two members")
	}
}

func TestANameBothClaimGetsASuffix(t *testing.T) {
	r := newRig(t)
	first := r.link(t, "u1")
	second := r.link(t, "u2")
	r.enter("u1", "c1", "Bob")
	r.enter("u2", "c1", "Bob")

	r.broker.Presence(first)
	answer, err := r.broker.Presence(second)
	if err != nil {
		t.Fatalf("stating the colliding presence: %v", err)
	}

	if answer.Group.DisplayName != "Bob 2" {
		t.Fatalf("the second claim lands under a suffix, got %q", answer.Group.DisplayName)
	}
}

func TestANickChangeKeepsTheClaimedName(t *testing.T) {
	r := newRig(t)
	secret := r.link(t, "u1")
	r.enter("u1", "c1", "Bob")

	r.broker.Presence(secret)
	r.enter("u1", "c1", "Bobby")
	answer, err := r.broker.Presence(secret)
	if err != nil {
		t.Fatalf("restating presence: %v", err)
	}

	if answer.Group.DisplayName != "Bob" {
		t.Fatalf("the first claim holds until the group retires, got %q", answer.Group.DisplayName)
	}
}
