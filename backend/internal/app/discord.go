package app

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/discordclient"
	"bjoernblessin.de/screenshare/internal/groupclient"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// Discord mode replaces the pass the poll runs (docs/discord-mode.md):
// one manager round trip answers channel, brokered group, members and streams together,
// where manual mode asks the index and states presence in two.
// The group key never reaches this side,
// so every derivation a command needs rides in as brokered facts on the settings copy.

// discordService is what Discord mode asks of the manager,
// held as an interface at the caller so a test states the answers without a manager running.
// One implementation, *discordclient.Client.
type discordService interface {
	Presence(base, linkSecret string) (discordclient.Answer, error)
	Token(base, linkSecret string) (token, prefix string, err error)
}

// storedLink is Discord's half of the settings: whether this install holds a link,
// and the account it was drawn for.
// No pass answers either, so every landing carries it beside the pass's own snapshot.
type storedLink struct {
	Linked  bool
	Account string
}

// discordSnapshot is what the last Discord pass landed, the zero value before one has run.
//
// Held whole and read wherever a command needs the brokered facts,
// so a build path waits on nothing.
type discordSnapshot struct {
	// InChannel is whether the linked account stands in a voice channel.
	InChannel bool
	// GuildName and ChannelName label the channel for a reader, empty outside one.
	GuildName   string
	ChannelName string
	// Prefix, SrtPassphrase and DisplayName are the brokered facts commands build with,
	// empty outside a channel.
	Prefix        string
	SrtPassphrase string
	DisplayName   string
	// Application is the Discord application the manager links through,
	// which an activity on this machine's own Discord client is drawn under (richpresence.go).
	Application string
	// Refused marks a manager that will not resolve the link the settings hold.
	// Polling again cannot clear it; linking again is what does.
	Refused bool
	// Stale marks an answer a pass left standing because it did not reach the manager.
	Stale bool
}

// discordState is what the last Discord pass landed, the zero value before one has run.
func (a *App) discordState() discordSnapshot {
	if last := a.discordLast.Load(); last != nil {
		return *last
	}
	return discordSnapshot{}
}

// pollPass is one pass of the relay poll, in whichever mode the settings hold.
func (a *App) pollPass() {
	a.settingsMu.Lock()
	mode := a.settings.Relay.DiscordMode
	a.settingsMu.Unlock()

	if mode {
		a.discordPass()
	} else {
		a.fetchRelay()
		a.statePresence()
	}

	// Off what the pass above landed, so what Discord shows and what this app holds are one read
	// apart (richpresence.go).
	a.statePresenceOnDiscord()
}

// discordPass states presence at the manager and lands everything the answer carries:
// the Discord snapshot, the membership and the relay snapshot, each where its readers look.
//
// One round trip on the loop that already polls,
// so presence, the index and the channel move together or not at all.
// A manager this pass could not reach leaves the last answer standing under its lease,
// as a manual pass leaves membership (members.go, standing).
func (a *App) discordPass() {
	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	// Out here because settingsMu is not held while membersMu is taken (app.go),
	// and every landing below announces the link beside the channel.
	held := storedLink{Linked: s.Relay.DiscordLink != "", Account: s.Relay.DiscordAccount}

	a.membersMu.Lock()
	defer a.membersMu.Unlock()

	base, okBase := s.Relay.DiscordService()
	if !okBase || !held.Linked {
		a.landDiscord(held, discordSnapshot{}, membership{}, relay.Status{})
		return
	}

	answer, err := a.discord.Presence(base, s.Relay.DiscordLink)
	if err != nil {
		a.discordPassFailed(held, err)
		return
	}

	if answer.Group == nil {
		// Standing in no voice channel: no group, and an index nothing can ask.
		a.landDiscord(held, discordSnapshot{Application: answer.Application}, membership{}, relay.Status{})
		return
	}

	snap := discordSnapshot{
		InChannel: true,
		GuildName: answer.Channel.Guild, ChannelName: answer.Channel.Name,
		Prefix:        answer.Group.Prefix,
		SrtPassphrase: answer.Group.SrtPassphrase,
		DisplayName:   answer.Group.DisplayName,
		Application:   answer.Application,
	}

	last := a.discordState()
	if last.Prefix != snap.Prefix {
		logger.Infof("the group follows the voice channel %s in %s", snap.ChannelName, snap.GuildName)
	}

	taken := presenceTaken(groupIDOfPrefix(snap.Prefix), groupclient.Membership{
		MemberID:         answer.Group.MemberID,
		DisplayName:      answer.Group.DisplayName,
		LeaseSeconds:     answer.Group.LeaseSeconds,
		Members:          answer.Group.Members,
		PublishingUnread: answer.Group.PublishingUnread,
	})
	status := relay.Status{Reachable: true, FromIndex: true, Paths: indexPaths(answer.Group.Streams)}
	a.landDiscord(held, snap, taken, status)
}

// discordPassFailed leaves the last answer standing where the manager did not refuse it,
// and lands the refusal where it did: a 401 is the manager declining to resolve the link.
func (a *App) discordPassFailed(held storedLink, err error) {
	last := a.discordState()

	var refusal *groupclient.Refusal
	if errors.As(err, &refusal) && refusal.Status == http.StatusUnauthorized {
		if !last.Refused {
			logger.Warnf("the manager does not know this install's link, so Discord mode has no group until it is linked again: %v", err)
		}
		a.landDiscord(held, discordSnapshot{Refused: true}, membership{}, relay.Status{})
		return
	}

	if !last.Stale {
		logger.Warnf("the Discord pass did not land, the answer already read standing until its lease runs out: %v", err)
	}
	last.Stale = true
	a.discordLast.Store(&last)
	a.emit(wire.DiscordStateEvent(last.wire(held)))

	// Membership stands as the manual pass leaves it: the last taken answer, until its lease lapses.
	heldMembers := a.membership()
	heldMembers.Stale = true
	a.landMembership(heldMembers)
}

// landDiscord records one pass's whole answer and announces the three snapshots shells draw from.
// held is the settings' half the pass does not own (wire).
func (a *App) landDiscord(held storedLink, snap discordSnapshot, m membership, status relay.Status) {
	a.discordLast.Store(&snap)
	a.relayLast.Store(&status)
	a.emit(wire.RelayStatusEvent(status))
	a.emit(wire.DiscordStateEvent(snap.wire(held)))
	a.landMembership(m)
}

// discordWire is Discord mode as a shell draws it:
// the link the settings hold, and the channel the last pass answered.
//
// With the mode off the link is the whole state.
// No pass runs there, so the channel and the refusal standing are an answer about a mode
// nothing is following, and drawing them would name a channel no group is under.
func (a *App) discordWire() wire.DiscordSnapshot {
	a.settingsMu.Lock()
	r := a.settings.Relay
	a.settingsMu.Unlock()

	held := storedLink{Linked: r.DiscordLink != "", Account: r.DiscordAccount}
	if !r.DiscordMode {
		return wire.DiscordSnapshot{Linked: held.Linked, AccountName: held.Account}
	}
	return a.discordState().wire(held)
}

// wire is this snapshot in the contract's shape, the brokered facts staying behind:
// a prefix and a passphrase are the backend's to build with, and a shell draws neither.
//
// held is the settings' half, the pass owning only whether the manager resolves the link.
// A secret stored while Discord mode is off has no pass behind it and links this install all the same,
// which is the state the link flow leaves (discordlink.go).
// The account stands through a refusal: it names the link the manager will not resolve.
func (d discordSnapshot) wire(held storedLink) wire.DiscordSnapshot {
	return wire.DiscordSnapshot{
		Linked:      held.Linked && !d.Refused,
		AccountName: held.Account,
		InChannel:   d.InChannel,
		GuildName:   d.GuildName,
		ChannelName: d.ChannelName,
		Stale:       d.Stale,
	}
}

// withStoredLink is s carrying the link the settings hold, secret and account both.
//
// The link flow is the one writer of them (discordlink.go), so a copy that came from a shell says nothing
// about a link that landed after the shell read it.
// Taking such a copy would unlink this install, and every path holding or resolving one reads through here.
func (a *App) withStoredLink(s settings.Settings) settings.Settings {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()

	s.Relay.DiscordLink = a.settings.Relay.DiscordLink
	s.Relay.DiscordAccount = a.settings.Relay.DiscordAccount
	return s
}

// withBrokered is s carrying the stored link and the last pass's brokered facts,
// the brokered half staying off outside Discord mode or outside any channel.
//
// What lets every reader of InGroup, Path and SrtPassphrase answer in Discord mode
// without holding a group key.
// The one owner of the facts is the pass snapshot, and this reads through it per call.
func (a *App) withBrokered(s settings.Settings) settings.Settings {
	s = a.withStoredLink(s)

	if !s.Relay.DiscordMode {
		return s
	}
	d := a.discordState()
	if !d.InChannel {
		return s
	}
	s.Relay = s.Relay.WithBrokeredGroup(d.Prefix, d.SrtPassphrase, d.DisplayName)
	return s
}

// errDiscordUnlinked refuses a command while no link secret is set.
var errDiscordUnlinked = errors.New("Discord mode is on but this computer is not linked: link Discord under Relay")

// errNoVoiceChannel refuses a command while the linked account stands in no voice channel.
var errNoVoiceChannel = errors.New("not in a voice channel: join one in Discord to get a group")

// discordSettingsForCommand is settingsForCommand's Discord half:
// the token is brokered by the manager, and the brokered facts ride the same copy.
//
// The prefix the trade grants is checked against the facts the last pass landed.
// The two can move apart where the channel's group retired between pass and command,
// and a command built half from each would publish under one group keyed as another,
// so the mismatch waits for the next pass instead.
func (a *App) discordSettingsForCommand(s settings.Settings) (settings.Settings, error) {
	base, ok := s.Relay.DiscordService()
	if !ok {
		return s, errors.New("Discord mode is served by a manager, and no relay is named to reach one at")
	}
	if s.Relay.DiscordLink == "" {
		return s, errDiscordUnlinked
	}
	d := a.discordState()
	if !d.InChannel {
		return s, errNoVoiceChannel
	}

	token, prefix, err := a.discord.Token(base, s.Relay.DiscordLink)
	if err != nil {
		return s, fmt.Errorf("no relay token for this channel's group: %w", err)
	}
	if prefix != d.Prefix {
		return s, fmt.Errorf("the channel's group moved between two reads: try again in a moment")
	}

	s.Relay = s.Relay.WithBrokeredGroup(d.Prefix, d.SrtPassphrase, d.DisplayName)
	s.Relay.Token = token
	return s, nil
}

// discordRefusal names why Discord mode states no membership right now,
// for a command refused on settings.Relay.InGroup.
func (a *App) discordRefusal(s settings.Settings) error {
	if s.Relay.DiscordLink == "" {
		return errDiscordUnlinked
	}
	return errNoVoiceChannel
}

// groupIDOfPrefix is the public group id a brokered prefix carries, the digest before the separator.
func groupIDOfPrefix(prefix string) string {
	return strings.TrimSuffix(prefix, "/")
}
