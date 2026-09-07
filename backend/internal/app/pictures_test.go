package app

import (
	"testing"

	"bjoernblessin.de/screenshare/internal/events"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// aPicture is one address and what a cache holding it answers.
const (
	aPictureAddress = "https://cdn.discordapp.com/avatars/u-bob/h1.png?size=64"
	aPicture        = "bob-png"
)

// fakePictures is a cache with every address already read.
type fakePictures map[string][]byte

func (f fakePictures) Bytes(address string) []byte { return f[address] }

func TestAMembersPictureCrossesAsBytes(t *testing.T) {
	a := &App{events: events.New(), avatars: fakePictures{aPictureAddress: []byte(aPicture)}}
	a.setMembership(membership{
		Group:  aGroupID,
		Joined: true,
		Members: []wire.Member{
			{MemberID: aMemberID, DisplayName: "Bob", AvatarURL: aPictureAddress},
			{MemberID: "m2", DisplayName: "Ann"},
		},
	})

	got := a.MembersState()

	if len(got.Members) != 2 {
		t.Fatalf("the group crossed with %d rows, want the two it holds", len(got.Members))
	}
	if string(got.Members[0].Avatar) != aPicture {
		t.Fatalf("Bob's row carries %q, want the picture the cache holds", got.Members[0].Avatar)
	}
	if len(got.Members[1].Avatar) != 0 {
		t.Fatalf("a member the manager named no picture for carries %q", got.Members[1].Avatar)
	}
}

// The membership behind the rows is read by every path a stopped stream takes,
// so the picture is written onto a copy.
func TestReadingPicturesLeavesTheHeldMembershipAlone(t *testing.T) {
	a := &App{events: events.New(), avatars: fakePictures{aPictureAddress: []byte(aPicture)}}
	a.setMembership(membership{
		Group:   aGroupID,
		Joined:  true,
		Members: []wire.Member{{MemberID: aMemberID, DisplayName: "Bob", AvatarURL: aPictureAddress}},
	})

	a.MembersState()

	if got := a.membership().Members[0].Avatar; len(got) != 0 {
		t.Fatalf("the held membership took on %q, want the rows read through it untouched", got)
	}
}

func TestTheLinkedAccountsPictureCrossesAsBytes(t *testing.T) {
	a := &App{
		events:  events.New(),
		avatars: fakePictures{aPictureAddress: []byte(aPicture)},
		settings: settings.Settings{Relay: settings.Relay{
			DiscordLink:    "link-secret",
			DiscordAccount: "bob",
			DiscordAvatar:  aPictureAddress,
		}},
	}

	got := a.discordWire()

	if !got.Linked || got.AccountName != "bob" {
		t.Fatalf("the link crossed as %+v, want the account the settings name", got)
	}
	if string(got.Avatar) != aPicture {
		t.Fatalf("the link carries the picture %q, want the one the cache holds", got.Avatar)
	}
}

// A link drawn before the manager answered pictures has none stored,
// and the pass this install's own row rides on is what fills it in.
func TestAPassRemembersTheAccountsPicture(t *testing.T) {
	isolateConfig(t)
	a := discordApp(&fakeDiscord{answer: inChannel()})
	a.settings.Relay.DiscordAvatar = ""

	a.pollPass()

	if got := a.settings.Relay.DiscordAvatar; got != aPictureAddress {
		t.Fatalf("the pass stored %q, want the picture on this install's own row", got)
	}
}

// Idempotent: a picture already stored is written again by no pass.
func TestAStoredPictureIsLeftWhereItIs(t *testing.T) {
	isolateConfig(t)
	a := discordApp(&fakeDiscord{answer: inChannel()})
	a.settings.Relay.DiscordAvatar = aPictureAddress

	a.pollPass()

	if got := a.settings.Relay.DiscordAvatar; got != aPictureAddress {
		t.Fatalf("the pass moved the stored picture to %q", got)
	}
}
