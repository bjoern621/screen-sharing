package wire

import (
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// The link secret is the link flow's to write and a shell has nothing to edit about it
// (internal/app, withStoredLink), so it crosses in neither direction.

func TestTheContractCarriesNoLinkSecret(t *testing.T) {
	out := RelaySettings(populatedSettings().Relay)
	if got := out.GetDiscordLink(); got != "" {
		t.Errorf("the contract carries the link secret %q outward, want nothing", got)
	}

	in := ToRelay(&screensharev1.RelaySettings{DiscordLink: "a shell's copy"})
	if got := in.DiscordLink; got != "" {
		t.Errorf("a shell's copy of the link read back as %q, want nothing", got)
	}
}
