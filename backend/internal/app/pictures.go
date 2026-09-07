package app

import (
	"bjoernblessin.de/go-utils/util/assert"

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
