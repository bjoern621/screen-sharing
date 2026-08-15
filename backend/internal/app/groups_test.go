package app

import (
	"testing"

	"bjoernblessin.de/screenshare/internal/relay"
)

// The prefix every path of one group carries is drawn nowhere: a member's list is a list under one
// prefix, so repeating it on each row spends the width the stream's own name needs.
// The whole path stays on Name, which is what a viewer is opened with.
func TestAnOwnNameIsThePathInsideThisMachinesPrefix(t *testing.T) {
	const prefix = "MFZWIZLTOQ2DGNBVGY3TQOJQGE/"

	status := ownNames(relay.Status{Reachable: true, Paths: []relay.Path{
		{Name: prefix + "desk"},
		{Name: "OTHERGROUPIDOTHERGROUPIDXX/desk"},
		{Name: prefix},
	}}, prefix)

	for i, want := range []string{"desk", "OTHERGROUPIDOTHERGROUPIDXX/desk", prefix} {
		if got := status.Paths[i].OwnName; got != want {
			t.Errorf("path %q is listed as %q, want %q", status.Paths[i].Name, got, want)
		}
	}
}

// A relay that authenticates nobody carries bare names, so there is nothing to take off and the
// whole name is the stream's own.
func TestWithNoPrefixEveryNameIsAlreadyItsOwn(t *testing.T) {
	status := ownNames(relay.Status{Reachable: true, Paths: []relay.Path{{Name: "desk"}}}, "")

	if got := status.Paths[0].OwnName; got != "desk" {
		t.Errorf("a bare path is listed as %q, want its own name", got)
	}
}
