package transport

import (
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/settings"
)

// A group is a path prefix, so joining one moves every leg at once.
// These hold that no transport builds its URL from the stream's bare name: one that did would
// publish outside the group its own token grants, which the relay refuses and which reaches the
// user as a stream that will not start.

// grouped is settings publishing under a group, with the key they joined with.
func grouped(t *testing.T) (settings.Settings, group.Key) {
	t.Helper()
	key, err := group.NewKey()
	if err != nil {
		t.Fatalf("drawing a group key: %v", err)
	}
	s := settings.Defaults()
	s.Relay.Host = "relay.example"
	s.Publish.Name = "standup"
	s.Relay.GroupKey = key.String()
	return s, key
}

// Both legs carry the prefix: a publisher pushing into the group and a viewer pulling out of it
// name one path.
func TestEveryTransportPublishesInsideTheGroup(t *testing.T) {
	s, key := grouped(t)

	for _, name := range Names() {
		tr, ok := Get(name)
		if !ok {
			t.Fatalf("the registry names %s and carries no row for it", name)
		}

		if p, ok := tr.(FFmpegPublisher); ok {
			assertGrouped(t, name+" publish", strings.Join(p.PublishArgs(s), " "), key)
		}
		if w, ok := tr.(Watcher); ok {
			assertGrouped(t, name+" watch", w.WatchURL(s, s.Relay.Path(s.Publish.Name)), key)
		}
	}
}

// A machine in no group publishes under the bare name on a relay that runs no group service, which
// is what a relay with no auth configured serves.
// A prefix appears because a key was joined with or because the relay has a public one, never
// because the app invented one.
func TestAMachineInNoGroupPublishesUnderTheName(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = "10.0.0.5"
	s.Publish.Name = "standup"
	s.Relay.GroupKey = ""

	if got := s.Relay.Path(s.Publish.Name); got != "standup" {
		t.Errorf("a machine in no group publishes to %q, want the bare name", got)
	}
}

// A key the app cannot read leaves the path alone rather than deriving from nonsense: a prefix
// computed off a broken key is a path no member is watching, which is worse than the typed name.
func TestAKeyTheAppCannotReadMovesNothing(t *testing.T) {
	s := settings.Defaults()
	s.Publish.Name = "standup"
	s.Relay.GroupKey = "not a key"

	if got := s.Relay.Path(s.Publish.Name); got != "standup" {
		t.Errorf("an unreadable key publishes to %q, want the bare name", got)
	}
}

// A relay with a group service and no key publishes where anybody may watch, which is a stream the
// user chose to leave open rather than one that failed to find its group.
func TestNoGroupOnAGroupRelayPublishesPublicly(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = "relay.example"
	s.Publish.Name = "standup"
	s.Relay.GroupKey = ""

	if got := s.Relay.Path(s.Publish.Name); got != group.PublicPrefix+"standup" {
		t.Errorf("a keyless publish goes to %q, want the public prefix", got)
	}
}

// The audience is never widened on the strength of a key that came back damaged.
// Somebody who set a key meant to restrict who watches, so a broken one publishes where the relay
// refuses it rather than where everybody can see it.
func TestABrokenKeyNeverFallsToThePublicPrefix(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = "relay.example"
	s.Publish.Name = "standup"
	s.Relay.GroupKey = "not a key"

	if got := s.Relay.Path(s.Publish.Name); got != "standup" {
		t.Errorf("a broken key publishes to %q, want the bare name and never the public prefix", got)
	}
}

// assertGrouped fails on a rendered leg that does not carry the group's prefix.
func assertGrouped(t *testing.T, leg, rendered string, key group.Key) {
	t.Helper()
	if !strings.Contains(rendered, key.Prefix()) {
		t.Errorf("%s renders %q, which is outside the group %s", leg, rendered, key.Prefix())
	}
}
