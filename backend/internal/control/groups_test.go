package control

import (
	"context"
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/events"
	"bjoernblessin.de/screenshare/internal/wire"
)

// The read is the presence loop's last reading and states nothing of its own,
// so a shell that mounts and one that listened are told the same thing.
// There is no membership call beside it:
// the group key and the display name in the settings are what put this machine in a group.
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
