package app

import (
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

			// What the command answers is another test's: the settings are held before it runs.
			_ = hold(a, stale)

			if got := a.GetSettings().Relay.DiscordLink; got != "link-secret" {
				t.Fatalf("%s left the link %q, want the stored secret", name, got)
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
