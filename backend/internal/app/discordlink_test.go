package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"bjoernblessin.de/screenshare/internal/settings"
)

// The link secret is the link flow's to write and nothing else on this side reads it back off a shell
// (discordlink.go).
// A shell's copy of the settings can be older than a link that landed since it was read,
// so every path taking one has to leave the stored secret standing.

func TestNoShellCopyUnlinksThisInstall(t *testing.T) {
	isolateConfig(t)

	held := map[string]func(*App, settings.Settings) error{
		"a save":      (*App).SaveSettings,
		"a publish":   (*App).StartPublish,
		"a republish": (*App).Republish,
	}

	for name, hold := range held {
		t.Run(name, func(t *testing.T) {
			a := discordApp(&fakeDiscord{})
			stale := a.GetSettings()
			stale.Relay.DiscordLink = ""
			stale.Relay.DiscordAccount = ""

			// What the command answers is another test's: the settings are held before it runs.
			_ = hold(a, stale)

			if got := a.GetSettings().Relay.DiscordLink; got != "link-secret" {
				t.Fatalf("%s left the link %q, want the stored secret", name, got)
			}
			if got := a.GetSettings().Relay.DiscordAccount; got != "bob" {
				t.Fatalf("%s left the account %q, want the stored one", name, got)
			}
			if !a.discordWire().Linked {
				t.Fatalf("%s drew this install as unlinked", name)
			}
		})
	}
}

// A draft off the wire carries whatever the shell last read, so the resolve reads the stored secret
// rather than the copy: the diagnostic asking for a link is the one that names the next move.
func TestADraftIsResolvedAgainstTheStoredLink(t *testing.T) {
	a := discordApp(&fakeDiscord{})

	draft := settings.Settings{Relay: settings.Relay{Host: "127.0.0.1", DiscordMode: true}}

	if got := a.withBrokered(draft).Relay.DiscordLink; got != "link-secret" {
		t.Fatalf("the draft carries the link %q, want the stored secret", got)
	}
}

// A command is built from the settings a shell hands over, and those carry no secret at all
// (internal/wire, ToRelay), so the trade reads the stored one.
func TestACommandIsBuiltOnTheStoredLink(t *testing.T) {
	fake := &fakeDiscord{answer: inChannel(), token: "a-token", prefix: aPrefix}
	a := discordApp(fake)
	a.discordPass()

	draft := a.GetSettings()
	draft.Relay.DiscordLink = ""

	got, err := a.settingsForCommand(draft)
	if err != nil {
		t.Fatalf("a command on the settings a shell handed over answered %v, want the brokered trade", err)
	}
	if got.Relay.Token != "a-token" {
		t.Fatalf("the command carries the token %q, want the one the manager brokered", got.Relay.Token)
	}
}

// The account the link was drawn for lands beside the secret,
// so a reader sees which Discord account the link belongs to.

func TestTheBrowserLegLandsTheSecretAndTheAccount(t *testing.T) {
	landed := make(chan landedLink, 1)

	w := httptest.NewRecorder()
	linkHandler(landed)(w, httptest.NewRequest(http.MethodGet, "/?linkSecret=fresh&account=bob", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("the browser leg answers %d, want %d", w.Code, http.StatusOK)
	}
	got := <-landed
	if got.secret != "fresh" || got.account != "bob" {
		t.Fatalf("the leg landed %+v, want the secret and the account the redirect carried", got)
	}
}

// A manager that names no account still links the install: the name is a label on the link.
func TestALandedLinkWithoutAnAccountStillLinks(t *testing.T) {
	isolateConfig(t)
	a := discordApp(&fakeDiscord{})

	a.storeDiscordLink(landedLink{secret: "fresh"})

	if !a.discordWire().Linked {
		t.Fatal("an unnamed account left this install unlinked")
	}
	if got := a.discordWire().AccountName; got != "" {
		t.Fatalf("the state names the account %q, want none", got)
	}
}

func TestTheLinkedAccountReachesAShell(t *testing.T) {
	isolateConfig(t)

	// Both modes: the account labels the link the settings hold, and no pass answers it.
	for name, mode := range map[string]bool{"in Discord mode": true, "with the mode off": false} {
		t.Run(name, func(t *testing.T) {
			a := discordApp(&fakeDiscord{})
			a.settings.Relay.DiscordMode = mode

			a.storeDiscordLink(landedLink{secret: "fresh", account: "bob"})

			if got := a.GetSettings().Relay.DiscordAccount; got != "bob" {
				t.Fatalf("the settings hold the account %q, want the one the link was drawn for", got)
			}
			if got := a.discordWire().AccountName; got != "bob" {
				t.Fatalf("the state names the account %q, want the linked one", got)
			}
		})
	}
}
