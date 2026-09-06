package app

import (
	"testing"

	"bjoernblessin.de/screenshare/internal/discordrpc"
)

// What takes the activity off a profile, one test per way it ends.
// Each states the world after the change and runs one pass, the pass being where every answer lands.

// fakePresence is a Discord connection that records what a pass stated on it.
type fakePresence struct {
	stated  []discordrpc.Activity
	cleared int
	closed  int
}

func (f *fakePresence) SetActivity(a discordrpc.Activity) error {
	f.stated = append(f.stated, a)
	return nil
}

func (f *fakePresence) ClearActivity() error {
	f.cleared++
	return nil
}

func (f *fakePresence) Close() error {
	f.closed++
	return nil
}

// sharing is an app in a voice channel, publishing, with a connection already open.
func sharingApp(t *testing.T) (*App, *fakePresence) {
	t.Helper()

	a := discordApp(&fakeDiscord{answer: inChannel()})
	a.settings.Relay.DiscordRichPresence = true
	a.discordPass()

	held := &fakePresence{}
	a.presence.client = held
	return a, held
}

func TestSharingStatesAnActivity(t *testing.T) {
	a, held := sharingApp(t)
	a.run = &publishRun{settings: a.settings, handle: liveHandle{}}

	a.statePresenceOnDiscord()

	if len(held.stated) != 1 {
		t.Fatalf("%d activities stated, and a machine sharing in a channel states one", len(held.stated))
	}
	if held.stated[0].State != "General" {
		t.Errorf("the activity names %q, and the channel is General", held.stated[0].State)
	}
}

func TestStoppingTheStreamTakesTheActivityOff(t *testing.T) {
	a, held := sharingApp(t)

	// Nothing publishes, which is where a stopped stream leaves this app.
	a.statePresenceOnDiscord()

	if held.cleared != 1 {
		t.Errorf("a stopped stream clears the activity, cleared %d times", held.cleared)
	}
	if held.closed != 0 {
		t.Errorf("a stopped stream keeps the connection, closed %d times", held.closed)
	}
}

func TestLeavingTheVoiceChannelTakesTheActivityOff(t *testing.T) {
	a, held := sharingApp(t)

	// The channel is left, which the next pass lands as a snapshot standing in none.
	a.discord = &fakeDiscord{}
	a.discordPass()
	a.statePresenceOnDiscord()

	if held.cleared == 0 {
		t.Error("a machine that left the channel clears its activity")
	}
}

func TestTurningDiscordModeOffTakesTheActivityOff(t *testing.T) {
	a, held := sharingApp(t)
	a.settings.Relay.DiscordMode = false

	a.statePresenceOnDiscord()

	if held.closed != 1 {
		t.Errorf("the mode going off closes the connection, closed %d times", held.closed)
	}
	if a.presence.client != nil {
		t.Error("a closed connection is let go of, so the next pass opens a fresh one")
	}
}

func TestTurningTheActivityOffTakesItOff(t *testing.T) {
	a, held := sharingApp(t)
	a.settings.Relay.DiscordRichPresence = false

	a.statePresenceOnDiscord()

	if held.closed != 1 {
		t.Errorf("the setting going off closes the connection, closed %d times", held.closed)
	}
}

func TestClosingTheAppTakesTheActivityOff(t *testing.T) {
	a, held := sharingApp(t)

	a.dropRichPresence()

	if held.closed != 1 {
		t.Errorf("a shutdown closes the connection, closed %d times", held.closed)
	}
	if a.presence.client != nil {
		t.Error("a shutdown lets go of the connection it closed")
	}
}

// A pass over a machine that states nothing and holds no connection reaches no Discord client,
// so an app outside Discord mode never opens one.
func TestAMachineStatingNothingOpensNoConnection(t *testing.T) {
	a := discordApp(&fakeDiscord{answer: inChannel()})
	a.settings.Relay.DiscordRichPresence = true
	a.discordPass()

	a.statePresenceOnDiscord()

	if a.presence.client != nil {
		t.Error("a machine sharing nothing opens no connection to state it on")
	}
}
