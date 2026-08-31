package control

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/events"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// inGroup is a machine with a group to join and a name to join it under.
// Neither value is read past the refusals decided off them,
// the group service being where a key is traded and a name claimed.
var inGroup = settings.Settings{Relay: settings.Relay{
	Host:        "relay.example.com",
	GroupKey:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	DisplayName: "Björn",
}}

// A machine with no group key names no group, and one with no display name has nothing to claim in it:
// both are the world not being ready rather than a request naming something that cannot exist.
func TestJoiningTakesAGroupAndAName(t *testing.T) {
	for _, missing := range []struct {
		what string
		s    settings.Settings
	}{
		{"group key", settings.Settings{Relay: settings.Relay{DisplayName: "Björn"}}},
		{"display name", settings.Settings{Relay: settings.Relay{GroupKey: inGroup.Relay.GroupKey}}},
	} {
		backend := &fakeBackend{settings: missing.s}
		server := New(backend, events.New(), "test")

		_, err := server.JoinGroup(context.Background(), &screensharev1.JoinGroupRequest{})
		if got := status.Code(err); got != codes.FailedPrecondition {
			t.Errorf("joining with no %s answered %v, want a failed precondition", missing.what, got)
		}
		if backend.joins != 0 {
			t.Errorf("joining with no %s reached the backend %d times, want none", missing.what, backend.joins)
		}
	}
}

// A name another member holds is the request naming something it cannot have,
// which the backend answers with a Refused,
// the group service being the only side that knows who holds what.
func TestATakenNameRefusesTheRequest(t *testing.T) {
	backend := &fakeBackend{settings: inGroup, err: Refuse("that name is taken in this group")}
	server := New(backend, events.New(), "test")

	_, err := server.JoinGroup(context.Background(), &screensharev1.JoinGroupRequest{})
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("joining under a name another member holds answered %v, want an invalid argument", got)
	}
}

// A group service that did not answer is the world failing at something legal to ask for,
// and the name in the request had nothing to do with it.
func TestAnUnreachableServiceLeavesJoiningUnavailable(t *testing.T) {
	backend := &fakeBackend{settings: inGroup, err: errors.New("the group service cannot be reached")}
	server := New(backend, events.New(), "test")

	_, err := server.JoinGroup(context.Background(), &screensharev1.JoinGroupRequest{})
	if got := status.Code(err); got != codes.Unavailable {
		t.Errorf("joining through a service that did not answer gave %v, want unavailable", got)
	}
}

// A second join is the state the first one reached, so it succeeds and reaches the backend,
// where drawing nothing a second time is decided.
// A shell whose answer went missing has asking again as its only move,
// and a refusal here would take it away.
func TestJoiningTwiceSucceedsTwice(t *testing.T) {
	backend := &fakeBackend{settings: inGroup}
	server := New(backend, events.New(), "test")

	for attempt := 1; attempt <= 2; attempt++ {
		if _, err := server.JoinGroup(context.Background(), &screensharev1.JoinGroupRequest{}); err != nil {
			t.Fatalf("join %d answered %v, want the group joined", attempt, err)
		}
	}
	if backend.joins != 2 {
		t.Errorf("two joins reached the backend %d times, want both", backend.joins)
	}
}

// Leaving names what is to be true afterwards, so a machine in no group is already there.
// Nothing about the settings is read: a leave by a machine with no group key still leaves it in none.
func TestLeavingTwiceSucceedsTwice(t *testing.T) {
	backend := &fakeBackend{}
	server := New(backend, events.New(), "test")

	for attempt := 1; attempt <= 2; attempt++ {
		if _, err := server.LeaveGroup(context.Background(), &screensharev1.LeaveGroupRequest{}); err != nil {
			t.Fatalf("leave %d answered %v, want the group left", attempt, err)
		}
	}
	if backend.leaves != 2 {
		t.Errorf("two leaves reached the backend %d times, want both", backend.leaves)
	}
}

// The read is the presence loop's last reading and states nothing of its own,
// so a shell that mounts and one that listened are told the same thing.
func TestGetMembersStateAnswersWithTheGroupAsItWasRead(t *testing.T) {
	backend := &fakeBackend{members: wire.MembersSnapshot{
		Joined:  true,
		Members: []wire.Member{{MemberID: "MFZWIZLTOQ2DGNBV", DisplayName: "Björn", Self: true}},
	}}
	server := New(backend, events.New(), "test")

	got, err := server.GetMembersState(context.Background(), &screensharev1.GetMembersStateRequest{})
	if err != nil {
		t.Fatalf("reading the group answered %v, want the membership", err)
	}
	if !got.GetJoined() {
		t.Error("a machine in a group reads as out of it")
	}
	if len(got.GetMembers()) != 1 || !got.GetMembers()[0].GetSelf() {
		t.Errorf("the group reads as %v, want this machine's own row", got.GetMembers())
	}
}
