package app

import (
	"time"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/discordrpc"
	"bjoernblessin.de/screenshare/internal/relay"
)

// What this machine states on the Discord client running beside it (docs/discord-mode.md).
//
// One pass of the relay poll states one activity, off what that pass landed:
// the channel from the Discord snapshot, the audience from the group's index and its members,
// and the timer from the child carrying the stream.
// Nothing is kept between passes,
// the connection alone holding what it last stated so an unchanged pass sends nothing
// (internal/discordrpc).
//
// The words here are this side's, which is a departure from every word on screen being a shell's
// (docs/ipc-api.md, "The rule").
// Discord draws them on a profile, and no shell stands in that path to write them.

// richPresenceDetails is the first line Discord draws, under this app's own name.
const richPresenceDetails = "Sharing a screen"

// richPresence is the poll loop's own state, read and written on that goroutine alone.
type richPresence struct {
	client *discordrpc.Client
	// quiet keeps a machine with no Discord client running to one log line
	// rather than one every two seconds.
	quiet bool
}

// sharing is the publish an activity is drawn from.
// The zero value is a machine sharing nothing, which states no activity.
type sharing struct {
	live      bool
	name      string
	startedAt time.Time
}

// statePresenceOnDiscord states the activity this pass's facts describe,
// and clears where they describe none.
//
// Idempotent end to end: the desired activity is named on every pass,
// and the connection sends only where it differs from what it already states.
func (a *App) statePresenceOnDiscord() {
	a.settingsMu.Lock()
	r := a.settings.Relay
	a.settingsMu.Unlock()

	// The channel and the audience are Discord mode's answers,
	// so the mode going off leaves the last pass's snapshot describing a group nothing follows
	// (discord.go, discordWire).
	if !r.DiscordMode || !r.DiscordRichPresence {
		a.dropRichPresence()
		return
	}

	d := a.discordState()
	activity, stating := richPresenceActivity(d, a.sharingNow(), a.lastRelayStatus(), a.membership())
	if !stating && a.presence.client == nil {
		// Nothing to state, and no connection stating anything.
		return
	}

	client, ok := a.discordClient(d.Application)
	if !ok {
		return
	}

	var err error
	if stating {
		err = client.SetActivity(activity)
	} else {
		err = client.ClearActivity()
	}

	if err != nil {
		logger.Warnf("the activity did not reach the Discord client, so the next pass opens a fresh connection: %v", err)
		a.dropRichPresence()
	}
}

// richPresenceActivity is the activity a machine in this state describes,
// and false where it describes none.
//
// An activity says that this machine is sharing, where, and to how many,
// so a machine publishing nothing states none at all.
// The timer dates the child carrying the stream, which a relaunch after a failure restarts:
// what it measures is the picture a viewer is watching.
func richPresenceActivity(d discordSnapshot, live sharing, status relay.Status, m membership) (discordrpc.Activity, bool) {
	if !d.InChannel || !live.live {
		return discordrpc.Activity{}, false
	}

	return discordrpc.Activity{
		Details: richPresenceDetails,
		State:   d.ChannelName,
		Readers: readersOf(status, live.name),
		Members: len(m.Members),
		Start:   live.startedAt,
	}, true
}

// readersOf is how many the relay serves this machine's own stream to.
//
// Zero where no snapshot names the stream, which covers a relay out of reach
// and the passes between a launch and the relay listing what it carries.
func readersOf(status relay.Status, name string) int {
	for _, path := range status.Paths {
		if path.Name == name {
			return path.Readers
		}
	}
	return 0
}

// sharingNow is the child carrying a stream, read under procMu and used after it is released.
//
// A publish waiting out a backoff answers as sharing nothing.
// It carries no child and no picture reaches a viewer through it,
// where PublishState answers publishing across that wait, the stream being in force (publish.go).
func (a *App) sharingNow() sharing {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	if a.run == nil || !a.run.handle.Running() {
		return sharing{}
	}
	return sharing{live: true, name: a.run.settings.Publish.Name(), startedAt: a.run.startedAt}
}

// discordClient is the connection this pass states on, opening one where none is held.
//
// A machine with no Discord client running answers false and is tried again on the next pass:
// a socket that is not there refuses at once, so the retry costs the poll nothing worth spacing out.
// An empty application is a manager that has answered no pass yet.
func (a *App) discordClient(application string) (*discordrpc.Client, bool) {
	if a.presence.client != nil {
		return a.presence.client, true
	}
	if application == "" {
		return nil, false
	}

	client, err := discordrpc.Connect(application)
	if err != nil {
		if !a.presence.quiet {
			logger.Warnf("this machine states no activity on Discord: %v", err)
			a.presence.quiet = true
		}
		return nil, false
	}

	a.presence.quiet = false
	a.presence.client = client
	return client, true
}

// dropRichPresence closes the connection, which is what takes the activity off the profile:
// Discord clears what a connection stated when that connection goes.
// Idempotent, holding nothing being the state it names.
func (a *App) dropRichPresence() {
	if a.presence.client == nil {
		return
	}

	a.presence.client.Close()
	a.presence.client = nil
}
