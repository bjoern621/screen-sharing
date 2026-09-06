package app

import (
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// An activity is what one pass of the poll makes of its own facts,
// so these tests state the facts and read the activity, with no Discord client in the picture.

// inChannelSnapshot is the Discord snapshot a landed pass leaves for a member standing in General.
func inChannelSnapshot() discordSnapshot {
	return discordSnapshot{
		InChannel:   true,
		GuildName:   "Guild",
		ChannelName: "General",
		Prefix:      aPrefix,
		Application: "an-application",
	}
}

// aShare is this machine publishing one stream, started a minute ago.
func aShare() sharing {
	return sharing{live: true, name: "bob/monitor-0", startedAt: time.Now().Add(-time.Minute)}
}

// watchedBy is a relay snapshot serving this machine's stream to that many readers.
func watchedBy(readers int) relay.Status {
	return relay.Status{
		Reachable: true,
		Paths:     []relay.Path{{Name: "bob/monitor-0", Ready: true, Readers: readers}},
	}
}

// ofMembers is a group of that many members.
func ofMembers(count int) membership {
	m := membership{Group: aGroupID, Joined: true}
	for i := range count {
		m.Members = append(m.Members, wire.Member{DisplayName: string(rune('a' + i))})
	}
	return m
}

func TestAnActivityCountsTheReadersOfTheMembers(t *testing.T) {
	activity, stating := richPresenceActivity(inChannelSnapshot(), aShare(), watchedBy(1), ofMembers(4))

	if !stating {
		t.Fatal("a machine sharing in a voice channel states an activity")
	}
	if activity.Readers != 1 || activity.Members != 4 {
		t.Errorf("the party is %d of %d, and one of four are watching", activity.Readers, activity.Members)
	}
}

func TestAnActivityNamesTheVoiceChannel(t *testing.T) {
	activity, _ := richPresenceActivity(inChannelSnapshot(), aShare(), watchedBy(0), ofMembers(2))

	if activity.State != "General" {
		t.Errorf("the activity states %q, and the channel is General", activity.State)
	}
	if activity.Details == "" {
		t.Error("the activity says what this machine is doing")
	}
}

func TestAnActivityDatesTheShare(t *testing.T) {
	share := aShare()
	activity, _ := richPresenceActivity(inChannelSnapshot(), share, watchedBy(0), ofMembers(2))

	if !activity.Start.Equal(share.startedAt) {
		t.Errorf("the activity starts at %s, and the share started at %s", activity.Start, share.startedAt)
	}
}

func TestAMachineSharingNothingStatesNoActivity(t *testing.T) {
	_, stating := richPresenceActivity(inChannelSnapshot(), sharing{}, watchedBy(0), ofMembers(4))

	if stating {
		t.Error("an activity says this machine is sharing, so a machine sharing nothing states none")
	}
}

func TestAMachineOutsideAChannelStatesNoActivity(t *testing.T) {
	_, stating := richPresenceActivity(discordSnapshot{Application: "an-application"}, aShare(), watchedBy(1), ofMembers(4))

	if stating {
		t.Error("an activity names a voice channel, so a machine standing in none states no activity")
	}
}

func TestAnUnlistedStreamIsWatchedByNobody(t *testing.T) {
	activity, stating := richPresenceActivity(inChannelSnapshot(), aShare(), relay.Status{}, ofMembers(4))

	if !stating {
		t.Fatal("a relay that answered nothing does not stop a share from being stated")
	}
	if activity.Readers != 0 {
		t.Errorf("%d readers off a snapshot naming no stream", activity.Readers)
	}
}

func TestTheActivityFollowsDiscordMode(t *testing.T) {
	r := followsDiscord(settings.Relay{}, settings.Relay{DiscordRichPresence: true})

	if !r.DiscordMode {
		t.Error("the activity is drawn from a voice channel, so asking for it asks for Discord mode")
	}
}

func TestDiscordModeIsLeftAloneWithoutTheActivity(t *testing.T) {
	r := followsDiscord(settings.Relay{}, settings.Relay{GroupKey: "a-key"})

	if r.DiscordMode {
		t.Error("a machine asking for no activity is left in the mode its own settings name")
	}
}

func TestDiscordModeIsLeftAloneWhileTheActivityDoesNotMove(t *testing.T) {
	on := settings.Relay{DiscordRichPresence: true}

	if followsDiscord(on, on).DiscordMode {
		t.Error("a machine that asked for no mode keeps the one its settings name, the stated share a fresh installation carries included")
	}
}
