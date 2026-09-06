package form

import (
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/settings"
)

// Discord mode on the form: the toggle, what it greys, and the refusals it re-anchors.
// The group follows the voice channel while it is on,
// so the manual group controls grey and the audience refusals name Discord's own missing halves.

func TestTheDiscordToggleIsARelayField(t *testing.T) {
	f := fieldRowFor(t, KeyDiscordMode)
	if f.group != GroupRelay {
		t.Errorf("the toggle sits in %q, want the relay group", f.group)
	}
	if f.control != screensharev1.ControlKind_CONTROL_KIND_TOGGLE {
		t.Errorf("the toggle renders as %v, want a toggle", f.control)
	}

	s := settings.Defaults()
	s.Relay.DiscordMode = true
	if !f.value(s).GetFlag() {
		t.Error("the toggle reads the stored mode")
	}
}

func TestDiscordModeGreysTheManualGroupControls(t *testing.T) {
	s := settings.Defaults()
	s.Relay.DiscordMode = true
	form := Resolve(fieldTestDeps(), s)

	for _, key := range []string{KeyGroupKey, KeyDisplayName} {
		drawn := fieldDrawnFor(t, form, key)
		if drawn.GetEnabled() {
			t.Errorf("%s is live while the group follows the voice channel", key)
		}
		if drawn.GetReason().GetCode() != groupFollowsDiscord {
			t.Errorf("%s greys for %v, want the statement naming Discord", key, drawn.GetReason().GetCode())
		}
	}
}

func TestManualModeLeavesTheGroupControlsLive(t *testing.T) {
	form := Resolve(fieldTestDeps(), settings.Defaults())

	for _, key := range []string{KeyGroupKey, KeyDisplayName} {
		if !fieldDrawnFor(t, form, key).GetEnabled() {
			t.Errorf("%s is greyed outside Discord mode", key)
		}
	}
}

func TestAnUnlinkedDiscordDraftIsRefused(t *testing.T) {
	d := diagnosticTestDeps()
	s := diagnosticTestStream()
	s.Relay.DiscordMode = true
	s.Relay.DiscordLink = ""

	diags := diagnostics(d, s, estimate(d, s))
	if publishable(diags) {
		t.Error("an unlinked install has no account to read a channel off, and this draft was publishable")
	}

	refusal := diagnosticTestNaming(diags, discordNotLinked)
	if refusal == nil {
		t.Fatalf("no statement names the missing link: %v", diags)
	}
	if refusal.GetFieldKey() != KeyDiscordMode {
		t.Errorf("the refusal anchors on %q, want the Discord toggle", refusal.GetFieldKey())
	}
}

func TestALinkedDraftOutsideAnyChannelIsRefused(t *testing.T) {
	d := diagnosticTestDeps()
	s := diagnosticTestStream()
	s.Relay.DiscordMode = true
	s.Relay.DiscordLink = "link-secret"

	diags := diagnostics(d, s, estimate(d, s))
	if publishable(diags) {
		t.Error("outside any voice channel there is no group, and this draft was publishable")
	}

	refusal := diagnosticTestNaming(diags, discordNoVoiceChannel)
	if refusal == nil {
		t.Fatalf("no statement names the missing channel: %v", diags)
	}
	if refusal.GetFieldKey() != KeyDiscordMode {
		t.Errorf("the refusal anchors on %q, want the Discord toggle", refusal.GetFieldKey())
	}
}

// A link the manager declines is cleared by linking again, and joining a channel does nothing for it,
// so the refusal names the move that works.
func TestARefusedDiscordLinkIsRefusedByName(t *testing.T) {
	d := diagnosticTestDeps()
	d.DiscordRefused = true
	s := diagnosticTestStream()
	s.Relay.DiscordMode = true
	s.Relay.DiscordLink = "link-secret"

	diags := diagnostics(d, s, estimate(d, s))
	if publishable(diags) {
		t.Error("a link the manager declines draws no group, and this draft was publishable")
	}

	refusal := diagnosticTestNaming(diags, discordLinkRefused)
	if refusal == nil {
		t.Fatalf("no statement names the refused link: %v", diags)
	}
	if refusal.GetFieldKey() != KeyDiscordMode {
		t.Errorf("the refusal anchors on %q, want the Discord toggle", refusal.GetFieldKey())
	}
}

func TestABrokeredDraftInsideAChannelPublishes(t *testing.T) {
	d := diagnosticTestDeps()
	s := diagnosticTestStream()
	s.Relay.DiscordMode = true
	s.Relay.DiscordLink = "link-secret"
	s.Relay = s.Relay.WithBrokeredGroup("PREFIX/", "passphrase", "Bob")

	diags := diagnostics(d, s, estimate(d, s))
	if !publishable(diags) {
		t.Errorf("a brokered group is membership, and this draft was refused: %v", diags)
	}
}
