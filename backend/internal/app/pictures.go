package app

import (
	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/discordclient"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// The pictures a shell draws people with are read here and nowhere else.
// A state carries bytes and an address stays on this side,
// the backend being what reaches Discord's CDN (docs/ipc-api.md).

// pictureSource answers a picture by its address, empty until a read lands.
// Held as an interface at the caller so a test answers without a CDN.
// One implementation, *avatars.Cache.
type pictureSource interface {
	Bytes(address string) []byte
}

// storedLink is Discord's half of the settings, the linked account's picture read with it.
func (a *App) storedLink(r settings.Relay) storedLink {
	assert.IsNotNil(a.avatars, "an app reads the pictures it draws people with")

	return storedLink{
		Linked:  r.DiscordLink != "",
		Account: r.DiscordAccount,
		Avatar:  a.avatars.Bytes(r.DiscordAvatar),
	}
}

// membersWire is the group as a shell draws it, every member's picture read with it.
//
// The rows are copied rather than written through:
// the membership behind them is read by every path a stopped stream takes (members.go).
func (a *App) membersWire(m membership) wire.MembersSnapshot {
	assert.IsNotNil(a.avatars, "an app reads the pictures it draws people with")

	snap := m.snapshot()

	members := make([]wire.Member, len(snap.Members))
	for i, member := range snap.Members {
		member.Avatar = a.avatars.Bytes(member.AvatarURL)
		members[i] = member
	}
	snap.Members = members
	return snap
}

// announcePictures states the two snapshots a picture is drawn in, one read having landed.
//
// A read starts on a pass that answered without it,
// so the landing is what puts a picture in front of a reader rather than the pass after it.
func (a *App) announcePictures() {
	a.emit(wire.MembersStateEvent(a.MembersState()))
	a.emit(wire.DiscordStateEvent(a.discordWire()))
}

// ownPicture addresses the picture on this install's own row of a brokered group.
// Empty where the manager named none, which is every manager answering a group before pictures.
func ownPicture(g *discordclient.Group) string {
	assert.IsNotNil(g, "an own row is read off a group")

	for _, m := range g.Members {
		if m.MemberID == g.MemberID {
			return m.AvatarURL
		}
	}
	return ""
}

// rememberAccountPicture stores the picture the last pass answered for this install's own row.
//
// The link flow lands one where it draws the link, and a link drawn before the manager answered
// pictures carries none, so the pass is what fills it in.
// Stored rather than read per pass: a link stands with Discord mode off, where no pass runs
// (docs/discord-mode.md).
//
// Idempotent: an address already stored is written again by no pass,
// which is what keeps the poll off the settings file.
func (a *App) rememberAccountPicture() {
	address := a.discordState().AccountAvatar
	if address == "" {
		return
	}

	a.settingsMu.Lock()
	if a.settings.Relay.DiscordAvatar == address {
		a.settingsMu.Unlock()
		return
	}
	a.settings.Relay.DiscordAvatar = address
	s := a.settings
	a.settingsMu.Unlock()

	if err := settings.Save(s); err != nil {
		logger.Warnf("the account's picture is not persisted, so it is read again on the next pass: %v", err)
	}
	a.emit(wire.SettingsChangedEvent())
	a.emit(wire.DiscordStateEvent(a.discordWire()))
}
