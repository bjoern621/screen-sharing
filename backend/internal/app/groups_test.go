package app

import (
	"testing"

	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The prefix every path of one group carries is drawn nowhere: one prefix over the whole list,
// so repeating it per row spends the width the stream's own name needs.
func TestASnapshotNamesEachPathInsideThisMachinesPrefix(t *testing.T) {
	const prefix = "MFZWIZLTOQ2DGNBVGY3TQOJQGE/"

	status := insidePrefix(relay.Status{Reachable: true, Paths: []relay.Path{
		{Name: prefix + "desk"},
		{Name: "OTHERGROUPIDOTHERGROUPIDXX/desk"},
		{Name: prefix},
	}}, prefix)

	for i, want := range []string{"desk", "OTHERGROUPIDOTHERGROUPIDXX/desk", prefix} {
		if got := status.Paths[i].Name; got != want {
			t.Errorf("path %d is listed as %q, want %q", i, got, want)
		}
	}
}

// A relay that authenticates nobody carries bare names, so there is nothing to take off.
func TestWithNoPrefixEveryNameIsAlreadyItsOwn(t *testing.T) {
	status := insidePrefix(relay.Status{Reachable: true, Paths: []relay.Path{{Name: "desk"}}}, "")

	if got := status.Paths[0].Name; got != "desk" {
		t.Errorf("a bare path is listed as %q, want its own name", got)
	}
}

// One name for one stream: what a live publish states and what a viewer's list carries agree,
// so a group's prefix never stands between a stream and its own row.
func TestASnapshotNamesAStreamAsItsPublishDoes(t *testing.T) {
	groupKey, err := group.NewKey()
	if err != nil {
		t.Fatalf("drawing a group key: %v", err)
	}
	s := settings.Settings{
		Relay:   settings.Relay{Host: "relay.example", GroupKey: groupKey.String(), DisplayName: "bjoern"},
		Publish: settings.Publish{Monitor: 0},
	}

	status := insidePrefix(relay.Status{Paths: []relay.Path{{Name: s.PublishPath()}}}, s.Relay.Prefix())

	if got, want := status.Paths[0].Name, s.StreamName(); got != want {
		t.Errorf("a viewer's list names this machine's stream %q, and its publish names it %q", got, want)
	}
}
