package voiceroster

import "testing"

// occupy is one Apply in the shape the gateway feeds: a user standing in a channel.
func occupy(user, guild, channel, name string) Presence {
	return Presence{UserID: user, GuildID: guild, ChannelID: channel, DisplayName: name}
}

// gone is one Apply reporting a user in no voice channel.
func gone(user string) Presence {
	return Presence{UserID: user}
}

func TestApplyStatesWhereAUserIs(t *testing.T) {
	r := New(nil)

	r.Apply(occupy("u1", "g1", "c1", "Bob"))

	where, ok := r.Where("u1")
	if !ok {
		t.Fatal("an applied presence answers Where")
	}
	if where.GuildID != "g1" || where.ChannelID != "c1" || where.DisplayName != "Bob" {
		t.Fatalf("Where answers the applied presence, got %+v", where)
	}
}

func TestWhereAnswersFalseForAnUnknownUser(t *testing.T) {
	r := New(nil)

	if _, ok := r.Where("nobody"); ok {
		t.Fatal("a user nothing placed is nowhere")
	}
}

func TestApplyingTheSameStateTwiceFiresNoLeave(t *testing.T) {
	var left []Presence
	r := New(func(p Presence) { left = append(left, p) })

	r.Apply(occupy("u1", "g1", "c1", "Bob"))
	r.Apply(occupy("u1", "g1", "c1", "Bob"))

	if len(left) != 0 {
		t.Fatalf("a state already true fires nothing, got %d leaves", len(left))
	}
}

func TestMovingChannelsLeavesTheOldOne(t *testing.T) {
	var left []Presence
	r := New(func(p Presence) { left = append(left, p) })

	r.Apply(occupy("u1", "g1", "c1", "Bob"))
	r.Apply(occupy("u1", "g1", "c2", "Bob"))

	if len(left) != 1 || left[0].ChannelID != "c1" {
		t.Fatalf("a move leaves the channel the user stood in, got %+v", left)
	}
	where, _ := r.Where("u1")
	if where.ChannelID != "c2" {
		t.Fatalf("a move lands in the new channel, got %+v", where)
	}
}

func TestDisconnectingLeavesAndForgets(t *testing.T) {
	var left []Presence
	r := New(func(p Presence) { left = append(left, p) })

	r.Apply(occupy("u1", "g1", "c1", "Bob"))
	r.Apply(gone("u1"))

	if len(left) != 1 || left[0].ChannelID != "c1" || left[0].UserID != "u1" {
		t.Fatalf("a disconnect leaves the channel the user stood in, got %+v", left)
	}
	if _, ok := r.Where("u1"); ok {
		t.Fatal("a disconnected user is nowhere")
	}
}

func TestDisconnectingAnUnknownUserFiresNothing(t *testing.T) {
	var left []Presence
	r := New(func(p Presence) { left = append(left, p) })

	r.Apply(gone("u1"))

	if len(left) != 0 {
		t.Fatalf("a user already nowhere is the state named, got %d leaves", len(left))
	}
}

func TestANickChangeInPlaceFiresNoLeave(t *testing.T) {
	var left []Presence
	r := New(func(p Presence) { left = append(left, p) })

	r.Apply(occupy("u1", "g1", "c1", "Bob"))
	r.Apply(occupy("u1", "g1", "c1", "Bobby"))

	if len(left) != 0 {
		t.Fatalf("a rename in place leaves nothing, got %d leaves", len(left))
	}
	where, _ := r.Where("u1")
	if where.DisplayName != "Bobby" {
		t.Fatalf("a rename lands, got %q", where.DisplayName)
	}
}

func TestDroppingAGuildLeavesEveryoneInIt(t *testing.T) {
	var left []Presence
	r := New(func(p Presence) { left = append(left, p) })

	r.Apply(occupy("u1", "g1", "c1", "Bob"))
	r.Apply(occupy("u2", "g1", "c2", "Eve"))
	r.Apply(occupy("u3", "g2", "c3", "Kim"))

	r.DropGuild("g1")

	if len(left) != 2 {
		t.Fatalf("a dropped guild leaves each of its occupants, got %d leaves", len(left))
	}
	if _, ok := r.Where("u1"); ok {
		t.Fatal("a dropped guild's occupant is nowhere")
	}
	if _, ok := r.Where("u3"); !ok {
		t.Fatal("another guild's occupant stays")
	}
}

func TestOccupantsCountsAChannel(t *testing.T) {
	r := New(nil)

	r.Apply(occupy("u1", "g1", "c1", "Bob"))
	r.Apply(occupy("u2", "g1", "c1", "Eve"))
	r.Apply(occupy("u3", "g1", "c2", "Kim"))

	if n := r.Occupants("g1", "c1"); n != 2 {
		t.Fatalf("two users stand in c1, counted %d", n)
	}

	r.Apply(occupy("u2", "g1", "c2", "Eve"))
	if n := r.Occupants("g1", "c1"); n != 1 {
		t.Fatalf("a move decrements the channel left, counted %d", n)
	}
	if n := r.Occupants("g1", "c3"); n != 0 {
		t.Fatalf("an empty channel counts zero, counted %d", n)
	}
}
