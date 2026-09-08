package form

import (
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	groupdomain "bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The Discord source on the form: the choice naming it, what it greys,
// and the refusals it re-anchors.
// The group follows the voice channel under it,
// so the manual group controls grey and the audience refusals name Discord's own missing halves.

func TestTheGroupSourceIsARelayRadioOverBothSources(t *testing.T) {
	f := fieldRowFor(t, KeyGroupSource)
	if f.group != GroupRelay {
		t.Errorf("the choice sits in %q, want the relay group", f.group)
	}
	if f.control != screensharev1.ControlKind_CONTROL_KIND_RADIO {
		t.Errorf("the choice renders as %v, want a radio", f.control)
	}

	offered := f.options(fieldTestDeps(), settings.Defaults())
	if len(offered) != len(groupdomain.Sources) {
		t.Fatalf("the choice offers %d entries, and the table declares %d sources",
			len(offered), len(groupdomain.Sources))
	}
	for i, o := range offered {
		if o.GetValue() != groupdomain.Sources[i] {
			t.Errorf("entry %d is %q, want the table's %q", i, o.GetValue(), groupdomain.Sources[i])
		}
	}

	s := settings.Defaults()
	s.Relay.GroupSource = groupdomain.SourceDiscord
	if got := f.value(s).GetText(); got != groupdomain.SourceDiscord {
		t.Errorf("the choice reads %q, want the stored source", got)
	}
}

// What this machine states about itself is no property of the relay it publishes to,
// so the stated share sits with the app and the mode it reads sits with the relay.
func TestTheStatedShareIsAnAppField(t *testing.T) {
	f := fieldRowFor(t, KeyDiscordRichPresence)
	if f.group != GroupApp {
		t.Errorf("the stated share sits in %q, want the app group", f.group)
	}
	if f.control != screensharev1.ControlKind_CONTROL_KIND_TOGGLE {
		t.Errorf("the stated share renders as %v, want a toggle", f.control)
	}

	s := settings.Defaults()
	s.App.DiscordRichPresence = false
	if f.value(s).GetFlag() {
		t.Error("the toggle reads the stored refusal")
	}
}

func TestTheDiscordSourceGreysTheManualGroupControls(t *testing.T) {
	s := settings.Defaults()
	s.Relay.GroupSource = groupdomain.SourceDiscord
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
	s.Relay.GroupSource = groupdomain.SourceDiscord
	s.Relay.DiscordLink = ""

	diags := diagnostics(d, s, estimate(d, s), nil)
	if publishable(diags) {
		t.Error("an unlinked install has no account to read a channel off, and this draft was publishable")
	}

	refusal := diagnosticTestNaming(diags, discordNotLinked)
	if refusal == nil {
		t.Fatalf("no statement names the missing link: %v", diags)
	}
	if refusal.GetFieldKey() != KeyGroupSource {
		t.Errorf("the refusal anchors on %q, want the Discord toggle", refusal.GetFieldKey())
	}
}

func TestALinkedDraftOutsideAnyChannelIsRefused(t *testing.T) {
	d := diagnosticTestDeps()
	s := diagnosticTestStream()
	s.Relay.GroupSource = groupdomain.SourceDiscord
	s.Relay.DiscordLink = "link-secret"

	diags := diagnostics(d, s, estimate(d, s), nil)
	if publishable(diags) {
		t.Error("outside any voice channel there is no group, and this draft was publishable")
	}

	refusal := diagnosticTestNaming(diags, discordNoVoiceChannel)
	if refusal == nil {
		t.Fatalf("no statement names the missing channel: %v", diags)
	}
	if refusal.GetFieldKey() != KeyGroupSource {
		t.Errorf("the refusal anchors on %q, want the Discord toggle", refusal.GetFieldKey())
	}
}

// A link the manager declines is cleared by linking again, and joining a channel does nothing for it,
// so the refusal names the move that works.
func TestARefusedDiscordLinkIsRefusedByName(t *testing.T) {
	d := diagnosticTestDeps()
	d.DiscordRefused = true
	s := diagnosticTestStream()
	s.Relay.GroupSource = groupdomain.SourceDiscord
	s.Relay.DiscordLink = "link-secret"

	diags := diagnostics(d, s, estimate(d, s), nil)
	if publishable(diags) {
		t.Error("a link the manager declines draws no group, and this draft was publishable")
	}

	refusal := diagnosticTestNaming(diags, discordLinkRefused)
	if refusal == nil {
		t.Fatalf("no statement names the refused link: %v", diags)
	}
	if refusal.GetFieldKey() != KeyGroupSource {
		t.Errorf("the refusal anchors on %q, want the Discord toggle", refusal.GetFieldKey())
	}
}

func TestABrokeredDraftInsideAChannelPublishes(t *testing.T) {
	d := diagnosticTestDeps()
	s := diagnosticTestStream()
	s.Relay.GroupSource = groupdomain.SourceDiscord
	s.Relay.DiscordLink = "link-secret"
	s.Relay = s.Relay.WithBrokeredGroup("PREFIX/", "passphrase", "Bob")

	diags := diagnostics(d, s, estimate(d, s), nil)
	if !publishable(diags) {
		t.Errorf("a brokered group is membership, and this draft was refused: %v", diags)
	}
}

// A resolve describes the draft it was handed, the link and the brokered group with it.
// Repair walks the settings as a wire message and neither rides one,
// so a draft losing them on the way through is refused as unlinked
// while the app states the link and the channel beside it.
func TestAResolvedBrokeredDraftPublishes(t *testing.T) {
	d := diagnosticTestDeps()
	s := diagnosticTestStream()
	s.Relay.GroupSource = groupdomain.SourceDiscord
	s.Relay.DiscordLink = "link-secret"
	s.Relay = s.Relay.WithBrokeredGroup("PREFIX/", "passphrase", "Bob")

	form := Resolve(d, s)
	if !form.GetPublishable() {
		t.Errorf("a brokered group is membership, and the resolve refused it: %v", form.GetDiagnostics())
	}
}
