package transport

import (
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/settings"
)

// A group is a path prefix, so joining one has to move every leg at once.
// What these hold is that no transport builds its URL from the stream's bare name:
// one that did would publish outside the group its own token grants, which the relay refuses and
// which reads to the user as a stream that will not start.

// grouped is settings publishing under a group, and the key it joined with.
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

// Every URL a transport builds carries the prefix, on both legs: a publisher pushing inside the
// group and a viewer pulling from inside it are the same path.
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

// A machine in no group publishes under the bare name, which is what every stream did before groups
// existed and what a relay with no auth configured serves.
// The prefix appears because a key was joined with, not because the app invented one.
func TestAMachineInNoGroupPublishesUnderTheName(t *testing.T) {
	s := settings.Defaults()
	s.Publish.Name = "standup"
	s.Relay.GroupKey = ""

	if got := s.Relay.Path(s.Publish.Name); got != "standup" {
		t.Errorf("a machine in no group publishes to %q, want the bare name", got)
	}
}

// A key the app cannot read leaves the path where it was rather than deriving from nonsense:
// a prefix computed off a broken key is a path no member is watching, which is worse than the name
// the user typed.
func TestAKeyTheAppCannotReadMovesNothing(t *testing.T) {
	s := settings.Defaults()
	s.Publish.Name = "standup"
	s.Relay.GroupKey = "not a key"

	if got := s.Relay.Path(s.Publish.Name); got != "standup" {
		t.Errorf("an unreadable key publishes to %q, want the bare name", got)
	}
}

// assertGrouped fails where a rendered leg does not carry the group's prefix.
func assertGrouped(t *testing.T, leg, rendered string, key group.Key) {
	t.Helper()
	if !strings.Contains(rendered, key.Prefix()) {
		t.Errorf("%s renders %q, which is outside the group %s", leg, rendered, key.Prefix())
	}
}
