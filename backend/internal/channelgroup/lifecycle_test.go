package channelgroup

import (
	"errors"
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/voiceroster"
)

func TestLeavingReleasesTheMember(t *testing.T) {
	r := newRig(t)
	secret := r.link(t, "u1")
	r.enter("u1", "c1", "Bob")
	r.broker.Presence(secret)

	r.leave("u1")

	if len(r.groups.released) != 1 {
		t.Fatalf("a leave releases the leaver's presence, released %d", len(r.groups.released))
	}
}

func TestComingBackIsANewMember(t *testing.T) {
	r := newRig(t)
	secret := r.link(t, "u1")
	r.enter("u1", "c1", "Bob")
	before, _ := r.broker.Presence(secret)

	r.leave("u1")
	r.enter("u1", "c1", "Bob")
	after, err := r.broker.Presence(secret)
	if err != nil {
		t.Fatalf("rejoining: %v", err)
	}

	if before.Group.MemberID == after.Group.MemberID {
		t.Fatal("the member that left is gone, so coming back draws a fresh one")
	}
	if r.groups.created != 1 {
		t.Fatalf("a rejoin inside the retire window keeps the group, created %d", r.groups.created)
	}
}

func TestMovingChannelsMovesTheGroup(t *testing.T) {
	r := newRig(t)
	secret := r.link(t, "u1")
	r.enter("u1", "c1", "Bob")
	before, _ := r.broker.Presence(secret)

	r.enter("u1", "c2", "Bob")
	after, err := r.broker.Presence(secret)
	if err != nil {
		t.Fatalf("stating presence in the new channel: %v", err)
	}

	if len(r.groups.released) != 1 {
		t.Fatalf("a move releases the member in the channel left, released %d", len(r.groups.released))
	}
	if before.Group.Prefix == after.Group.Prefix {
		t.Fatal("two channels are two groups")
	}
}

func TestAMissedLeaveIsCaughtByTheNextPass(t *testing.T) {
	r := newRig(t)
	secret := r.link(t, "u1")
	r.enter("u1", "c1", "Bob")
	r.broker.Presence(secret)

	// An empty roster the broker never saw a leave from,
	// which is what a missed gateway event leaves behind.
	r.broker.ReadOccupancy(voiceroster.New(nil))

	answer, err := r.broker.Presence(secret)
	if err != nil {
		t.Fatalf("a pass over a stale member still answers: %v", err)
	}
	if answer.Group != nil {
		t.Fatal("outside any channel there is no group")
	}
	if len(r.groups.released) != 1 {
		t.Fatalf("the pass releases what the missed event left, released %d", len(r.groups.released))
	}
}

func TestAChannelEmptyPastTheWindowRetires(t *testing.T) {
	r := newRig(t)
	secret := r.link(t, "u1")
	r.enter("u1", "c1", "Bob")
	before, _ := r.broker.Presence(secret)
	r.leave("u1")

	r.now = r.now.Add(RetireAfter / 2)
	r.broker.Sweep()
	r.enter("u1", "c1", "Bob")
	kept, _ := r.broker.Presence(secret)
	if kept.Group.Prefix != before.Group.Prefix {
		t.Fatal("a channel empty for less than the window keeps its group")
	}

	r.leave("u1")
	r.broker.Sweep()
	r.now = r.now.Add(RetireAfter + time.Second)
	r.broker.Sweep()

	r.enter("u1", "c1", "Bob")
	after, err := r.broker.Presence(secret)
	if err != nil {
		t.Fatalf("occupying a retired channel: %v", err)
	}
	if after.Group.Prefix == before.Group.Prefix {
		t.Fatal("the next occupancy after a retire draws a fresh group")
	}
}

func TestTokenIsBrokeredForAMemberInAChannel(t *testing.T) {
	r := newRig(t)
	secret := r.link(t, "u1")
	r.enter("u1", "c1", "Bob")
	answer, _ := r.broker.Presence(secret)

	token, prefix, err := r.broker.Token(secret)
	if err != nil {
		t.Fatalf("brokering a token: %v", err)
	}
	if token == "" || prefix != answer.Group.Prefix {
		t.Fatalf("the trade answers a token under the group's prefix, got %q %q", token, prefix)
	}
}

func TestTokenOutsideAnyChannelIsRefused(t *testing.T) {
	r := newRig(t)
	secret := r.link(t, "u1")

	if _, _, err := r.broker.Token(secret); !errors.Is(err, ErrNoChannel) {
		t.Fatalf("no channel means no group to grant, got %v", err)
	}
}
