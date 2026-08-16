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

// grouped is settings publishing under a group, with the group key they joined with.
func grouped(t *testing.T) (settings.Settings, group.Key) {
	t.Helper()
	groupKey, err := group.NewKey()
	if err != nil {
		t.Fatalf("drawing a group key: %v", err)
	}
	s := settings.Defaults()
	s.Relay.Host = "relay.example"
	s.Publish.Name = "standup"
	s.Relay.GroupKey = groupKey.String()
	return s, groupKey
}

// Both legs carry the prefix: a publisher pushing into the group and a viewer pulling out of it
// name one path.
func TestEveryTransportPublishesInsideTheGroup(t *testing.T) {
	s, groupKey := grouped(t)

	for _, name := range Names() {
		tr, ok := Get(name)
		if !ok {
			t.Fatalf("the registry names %s and carries no row for it", name)
		}

		if p, ok := tr.(FFmpegPublisher); ok {
			assertGrouped(t, name+" publish", strings.Join(p.PublishArgs(s), " "), groupKey)
		}
		if w, ok := tr.(Watcher); ok {
			assertGrouped(t, name+" watch", w.WatchURL(s, s.Relay.Path(s.Publish.Name)), groupKey)
		}
	}
}

// A relay nobody named has no prefix to derive, that prefix being a group service's answer and there
// being no service to ask.
// A prefix appears because a group key was joined with or because the relay has a public one, never
// because the app invented one.
func TestAnUnnamedRelayPublishesUnderTheName(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = ""
	s.Publish.Name = "standup"
	s.Relay.GroupKey = ""

	if got := s.Relay.Path(s.Publish.Name); got != "standup" {
		t.Errorf("a relay nobody named publishes to %q, want the bare name", got)
	}
}

// A group key the app cannot read leaves the path alone rather than deriving from nonsense: a prefix
// computed off a broken group key is a path no member is watching, which is worse than the typed name.
func TestAKeyTheAppCannotReadMovesNothing(t *testing.T) {
	s := settings.Defaults()
	s.Publish.Name = "standup"
	s.Relay.GroupKey = "not a group key"

	if got := s.Relay.Path(s.Publish.Name); got != "standup" {
		t.Errorf("an unreadable group key publishes to %q, want the bare name", got)
	}
}

// A machine in no group publishes where anybody may watch, which is a stream the user chose to leave
// open rather than one that failed to find its group.
// On every relay, each of them having a group service beside it and refusing a path under no prefix.
func TestNoGroupPublishesPublicly(t *testing.T) {
	for _, host := range []string{"relay.example", "192.168.1.9"} {
		s := settings.Defaults()
		s.Relay.Host = host
		s.Publish.Name = "standup"
		s.Relay.GroupKey = ""

		if got := s.Relay.Path(s.Publish.Name); got != group.PublicPrefix+"standup" {
			t.Errorf("a keyless publish to %s goes to %q, want the public prefix", host, got)
		}
	}
}

// The audience is never widened on the strength of a group key that came back damaged.
// Somebody who set a group key meant to restrict who watches, so a broken one publishes where the relay
// refuses it rather than where everybody can see it.
func TestABrokenKeyNeverFallsToThePublicPrefix(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = "relay.example"
	s.Publish.Name = "standup"
	s.Relay.GroupKey = "not a group key"

	if got := s.Relay.Path(s.Publish.Name); got != "standup" {
		t.Errorf("a broken group key publishes to %q, want the bare name and never the public prefix", got)
	}
}

// assertGrouped fails on a rendered leg that does not carry the group's prefix.
func assertGrouped(t *testing.T, leg, rendered string, groupKey group.Key) {
	t.Helper()
	if !strings.Contains(rendered, groupKey.Prefix()) {
		t.Errorf("%s renders %q, which is outside the group %s", leg, rendered, groupKey.Prefix())
	}
}
