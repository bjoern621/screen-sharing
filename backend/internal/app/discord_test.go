package app

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/discordclient"
	"bjoernblessin.de/screenshare/internal/events"
	"bjoernblessin.de/screenshare/internal/groupclient"
	"bjoernblessin.de/screenshare/internal/settings"
)

// One Discord pass answers channel, group, members and streams together,
// and these tests are what one landing leaves for every reader:
// the membership, the relay snapshot and the brokered facts commands build with.

// aPrefix is the channel's group prefix the faked manager answers with.
var aPrefix = aGroupID + "/"

// fakeDiscord answers as the manager would.
type fakeDiscord struct {
	answer discordclient.Answer
	err    error

	token    string
	prefix   string
	tokenErr error

	presences int
}

func (f *fakeDiscord) Presence(base, linkSecret string) (discordclient.Answer, error) {
	f.presences++
	return f.answer, f.err
}

func (f *fakeDiscord) Token(base, linkSecret string) (string, string, error) {
	return f.token, f.prefix, f.tokenErr
}

// inChannel is the manager's answer for a member standing in a channel with one stream live.
func inChannel() discordclient.Answer {
	return discordclient.Answer{
		Channel: &discordclient.Channel{Guild: "Guild", Name: "General"},
		Group: &discordclient.Group{
			Prefix:        aPrefix,
			SrtPassphrase: "passphrase",
			MemberID:      aMemberID,
			DisplayName:   "Bob",
			LeaseSeconds:  20,
			Members: []groupclient.Member{
				{MemberID: aMemberID, DisplayName: "Bob", Publishing: true},
			},
			Streams: []groupclient.Stream{{Name: "bob/monitor-0", Ready: true}},
		},
	}
}

// discordApp is an app in Discord mode, linked, asking the faked manager.
func discordApp(fake *fakeDiscord) *App {
	return &App{
		events:  events.New(),
		discord: fake,
		settings: settings.Settings{Relay: settings.Relay{
			Host:        "127.0.0.1",
			DiscordMode: true,
			DiscordLink: "link-secret",
		}},
	}
}

func TestADiscordPassLandsEverything(t *testing.T) {
	a := discordApp(&fakeDiscord{answer: inChannel()})

	a.discordPass()

	m := a.membership()
	if !m.Joined || m.Group != aGroupID {
		t.Fatalf("the pass lands membership in the channel's group, got %+v", m)
	}
	if len(m.Members) != 1 || !m.Members[0].Self || !m.Members[0].Publishing {
		t.Fatalf("the members cross with this machine's row marked, got %+v", m.Members)
	}

	status := a.lastRelayStatus()
	if !status.Reachable || !status.FromIndex || len(status.Paths) != 1 {
		t.Fatalf("the pass lands the relay snapshot off the same answer, got %+v", status)
	}

	d := a.discordState()
	if !d.Linked || !d.InChannel || d.Prefix != aPrefix || d.ChannelName != "General" {
		t.Fatalf("the pass lands the brokered facts, got %+v", d)
	}
}

func TestOutsideAnyChannelTheGroupIsEmpty(t *testing.T) {
	a := discordApp(&fakeDiscord{answer: discordclient.Answer{}})

	a.discordPass()

	if a.membership().Joined {
		t.Fatal("outside any channel there is no membership")
	}
	d := a.discordState()
	if !d.Linked || d.InChannel {
		t.Fatalf("the state is linked and adrift, got %+v", d)
	}
}

func TestAnUnlinkedInstallAsksNothing(t *testing.T) {
	fake := &fakeDiscord{answer: inChannel()}
	a := discordApp(fake)
	a.settings.Relay.DiscordLink = ""

	a.discordPass()

	if fake.presences != 0 {
		t.Fatalf("no link secret means no call, made %d", fake.presences)
	}
	if a.discordState().Linked {
		t.Fatal("an install without a secret is unlinked")
	}
}

func TestARefusedLinkLandsUnlinked(t *testing.T) {
	fake := &fakeDiscord{err: &groupclient.Refusal{Status: http.StatusUnauthorized, Reason: "unknown"}}
	a := discordApp(fake)

	a.discordPass()

	d := a.discordState()
	if d.Linked || d.Stale {
		t.Fatalf("a refused link is a clean unlinked state, got %+v", d)
	}
}

func TestAnUnreachableManagerLeavesTheAnswerStanding(t *testing.T) {
	fake := &fakeDiscord{answer: inChannel()}
	a := discordApp(fake)
	a.discordPass()

	fake.err = errors.New("connection refused")
	a.discordPass()

	m := a.membership()
	if !m.Joined || !m.Stale {
		t.Fatalf("the last answer stands under its lease, got %+v", m)
	}
	if !a.discordState().Stale {
		t.Fatal("the state says it was left standing")
	}
}

func TestDiscordCommandsCarryTheBrokeredFacts(t *testing.T) {
	fake := &fakeDiscord{answer: inChannel(), token: "tok", prefix: aPrefix}
	a := discordApp(fake)
	a.discordPass()

	s, err := a.settingsForCommand(a.GetSettings())
	if err != nil {
		t.Fatalf("building a command: %v", err)
	}
	if s.Relay.Token != "tok" {
		t.Fatalf("the command carries the brokered token, got %q", s.Relay.Token)
	}
	if !strings.HasPrefix(s.PublishPath(), aPrefix) {
		t.Fatalf("the publish path lives under the channel's prefix, got %q", s.PublishPath())
	}
	if s.Relay.SrtPassphrase() != "passphrase" {
		t.Fatalf("the SRT leg is keyed with the brokered passphrase, got %q", s.Relay.SrtPassphrase())
	}
}

func TestDiscordCommandsAreRefusedByState(t *testing.T) {
	unlinked := discordApp(&fakeDiscord{})
	unlinked.settings.Relay.DiscordLink = ""
	if _, err := unlinked.settingsForCommand(unlinked.GetSettings()); !errors.Is(err, errDiscordUnlinked) {
		t.Fatalf("no link is the unlinked refusal, got %v", err)
	}

	adrift := discordApp(&fakeDiscord{answer: discordclient.Answer{}})
	adrift.discordPass()
	if _, err := adrift.settingsForCommand(adrift.GetSettings()); !errors.Is(err, errNoVoiceChannel) {
		t.Fatalf("no channel is the adrift refusal, got %v", err)
	}
}

func TestAMovedGroupRefusesTheCommand(t *testing.T) {
	fake := &fakeDiscord{answer: inChannel(), token: "tok", prefix: "OTHER/"}
	a := discordApp(fake)
	a.discordPass()

	if _, err := a.settingsForCommand(a.GetSettings()); err == nil {
		t.Fatal("a trade under another prefix than the pass landed is refused")
	}
}
