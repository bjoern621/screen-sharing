package app

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

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
			Host:           "127.0.0.1",
			DiscordMode:    true,
			DiscordLink:    "link-secret",
			DiscordAccount: "bob",
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
	if !d.InChannel || d.Prefix != aPrefix || d.ChannelName != "General" {
		t.Fatalf("the pass lands the brokered facts, got %+v", d)
	}
}

func TestOutsideAnyChannelTheGroupIsEmpty(t *testing.T) {
	a := discordApp(&fakeDiscord{answer: discordclient.Answer{}})

	a.discordPass()

	if a.membership().Joined {
		t.Fatal("outside any channel there is no membership")
	}
	if a.discordState().InChannel {
		t.Fatal("outside any channel the state is adrift")
	}
	if !a.discordWire().Linked {
		t.Fatal("standing in no channel is a linked install all the same")
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
	if a.discordWire().Linked {
		t.Fatal("an install without a secret is unlinked")
	}
}

func TestARefusedLinkLandsUnlinked(t *testing.T) {
	fake := &fakeDiscord{err: &groupclient.Refusal{Status: http.StatusUnauthorized, Reason: "unknown"}}
	a := discordApp(fake)

	a.discordPass()

	d := a.discordState()
	if !d.Refused || d.Stale {
		t.Fatalf("a refusal is the answer rather than one left standing, got %+v", d)
	}
	if a.discordWire().Linked {
		t.Fatal("a secret the manager will not resolve links this install to nothing")
	}
}

// A link is stored the moment the browser leg lands, and a pass runs only in Discord mode,
// so a state read off the pass alone draws a linked install as unlinked until the toggle goes on.
func TestALinkedInstallDrawsAsLinkedBeforeAnyPass(t *testing.T) {
	a := discordApp(&fakeDiscord{answer: inChannel()})
	a.settings.Relay.DiscordMode = false

	if !a.discordWire().Linked {
		t.Fatal("a stored link is a linked install with no pass behind it")
	}
}

// With the mode off nothing follows a voice channel,
// so the channel a pass landed before it went off is not a state to draw.
func TestWithTheModeOffTheLinkIsTheWholeState(t *testing.T) {
	a := discordApp(&fakeDiscord{answer: inChannel()})
	a.discordPass()

	a.settings.Relay.DiscordMode = false

	d := a.discordWire()
	if !d.Linked || d.InChannel || d.ChannelName != "" {
		t.Fatalf("with the mode off the link is the whole state, got %+v", d)
	}
}

// The link lands where no pass is running, so the announcement is the whole of what moves a shell.
func TestStoringALinkAnnouncesIt(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := discordApp(&fakeDiscord{})
	a.settings.Relay.DiscordMode = false
	a.settings.Relay.DiscordLink = ""

	stream, cancel, err := a.events.Subscribe(nil)
	if err != nil {
		t.Fatalf("subscribing to the broker: %v", err)
	}
	defer cancel()

	a.storeDiscordLink(landedLink{secret: "fresh-secret", account: "bob"})

	if !a.discordWire().Linked {
		t.Fatal("a stored link draws as linked")
	}
	if !linkedAnnounced(stream) {
		t.Fatal("no Discord state announced the link")
	}
}

// linkedAnnounced is whether a linked Discord state reaches the stream before the wait runs out.
func linkedAnnounced(stream <-chan *screensharev1.Event) bool {
	deadline := time.After(2 * time.Second)
	for {
		select {
		case event := <-stream:
			if d := event.GetDiscordState(); d != nil && d.GetLinked() {
				return true
			}
		case <-deadline:
			return false
		}
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
