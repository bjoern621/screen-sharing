// Package discordavatar addresses a Discord account's picture on Discord's CDN.
//
// One owner of the address shape: the link flow reads an account's own hash (internal/discordoauth)
// and the gateway reads a channel member's (internal/discordgateway),
// and both hand on what this builds.
// Nothing here reaches the network; the app is what fetches an address (internal/avatars).
package discordavatar

import (
	"fmt"
	"strconv"

	"bjoernblessin.de/go-utils/util/assert"
)

// Host is Discord's CDN, and the one host an address here names.
// The app refuses to fetch anything else, a manager naming the address it fetches.
const Host = "cdn.discordapp.com"

// size is the edge of the square asked for, in pixels.
// Twice the widest row a shell draws one in, so a doubled display scale still has pixels to spend.
const size = 64

// User is one account's own picture, or the default Discord draws for an account holding none.
// Empty where neither can be addressed.
func User(userID, hash string) string {
	if userID == "" {
		return ""
	}
	if hash == "" {
		return defaultOf(userID)
	}
	return fmt.Sprintf("https://%s/avatars/%s/%s.png?size=%d", Host, userID, hash, size)
}

// GuildMember is the picture one account carries in one guild alone.
// It outranks User where a member holds one, as a nick outranks a username.
func GuildMember(guildID, userID, hash string) string {
	assert.Assert(hash != "", "a guild picture is addressed by the hash the member carries", guildID, userID)

	if guildID == "" || userID == "" {
		return ""
	}
	return fmt.Sprintf("https://%s/guilds/%s/users/%s/avatars/%s.png?size=%d", Host, guildID, userID, hash, size)
}

// defaultOf is the picture Discord draws for an account that set none,
// one of six chosen by the id's own bits.
//
// The index is the pomelo rule, an account still holding a discriminator choosing among five by that instead.
// A legacy account therefore gets a default of the wrong colour, which costs a hue and nothing else.
// An id that is no snowflake addresses nothing, being an Umgebungsfehler from another process.
func defaultOf(userID string) string {
	id, err := strconv.ParseUint(userID, 10, 64)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("https://%s/embed/avatars/%d.png?size=%d", Host, (id>>22)%6, size)
}
