// Package discordgateway feeds the voice roster from Discord's gateway.
//
// A thin adapter: discordgo owns the connection, the resume and the cache,
// and everything here translates its events into roster statements.
// No test drives it, its only oracle being Discord itself;
// what stands in is the roster's own tests and a run against a real guild.
//
// Intents: Guilds for the channel and guild names, GuildVoiceStates for who stands where.
// Both unprivileged, so a bot invite is the whole of the setup.
package discordgateway

import (
	"fmt"

	"github.com/bwmarrin/discordgo"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/voiceroster"
)

// Gateway is one bot connection feeding one roster.
type Gateway struct {
	session *discordgo.Session
}

// Connect opens the gateway and feeds roster until Close.
//
// An unreachable Discord or a refused token is an Umgebungsfehler and leaves as an error,
// the caller deciding whether a manager without a bot serves anything.
func Connect(botToken string, roster *voiceroster.Roster) (*Gateway, error) {
	assert.Assert(botToken != "", "a gateway authenticates as a bot")
	assert.IsNotNil(roster, "a gateway feeds a roster")

	session, err := discordgo.New("Bot " + botToken)
	if err != nil {
		return nil, fmt.Errorf("preparing the Discord session: %w", err)
	}
	session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildVoiceStates

	session.AddHandler(func(s *discordgo.Session, e *discordgo.GuildCreate) {
		// The guild's whole voice occupancy arrives with it,
		// which is what seeds the roster at connect and after every resume.
		for _, vs := range e.VoiceStates {
			roster.Apply(presenceFor(s, e.Guild, vs))
		}
	})

	session.AddHandler(func(s *discordgo.Session, e *discordgo.VoiceStateUpdate) {
		if e.ChannelID == "" {
			roster.Apply(voiceroster.Presence{UserID: e.UserID})
			return
		}
		guild, err := s.State.Guild(e.GuildID)
		if err != nil {
			logger.Warnf("a voice state names guild %s the state does not hold: %v", e.GuildID, err)
			return
		}
		roster.Apply(presenceFor(s, guild, e.VoiceState))
	})

	session.AddHandler(func(s *discordgo.Session, e *discordgo.GuildDelete) {
		roster.DropGuild(e.ID)
	})

	if err := session.Open(); err != nil {
		return nil, fmt.Errorf("opening the Discord gateway: %w", err)
	}
	return &Gateway{session: session}, nil
}

// Close drops the gateway connection.
func (g *Gateway) Close() {
	assert.IsNotNil(g.session, "a gateway holds the session it closes")

	if err := g.session.Close(); err != nil {
		logger.Warnf("closing the Discord gateway: %v", err)
	}
}

// presenceFor is one voice state as the roster takes it, names resolved.
func presenceFor(s *discordgo.Session, guild *discordgo.Guild, vs *discordgo.VoiceState) voiceroster.Presence {
	assert.IsNotNil(guild, "a presence stands in a guild")
	assert.IsNotNil(vs, "a presence reads off a voice state")

	channelName := ""
	if channel, err := s.State.Channel(vs.ChannelID); err == nil {
		channelName = channel.Name
	}

	return voiceroster.Presence{
		UserID:      vs.UserID,
		GuildID:     guild.ID,
		ChannelID:   vs.ChannelID,
		DisplayName: displayName(s, guild.ID, vs),
		GuildName:   guild.Name,
		ChannelName: channelName,
	}
}

// displayName is the user's name as the channel shows it: nick, global name, username,
// the first of those the member carries.
//
// The member rides most voice states; where it does not, the state cache answers,
// and a user neither holds is named by id until a later event carries more.
// A wrong name costs a label, so nothing here reaches Discord's REST API for one.
func displayName(s *discordgo.Session, guildID string, vs *discordgo.VoiceState) string {
	member := vs.Member
	if member == nil {
		if cached, err := s.State.Member(guildID, vs.UserID); err == nil {
			member = cached
		}
	}
	if member == nil || member.User == nil {
		return vs.UserID
	}
	switch {
	case member.Nick != "":
		return member.Nick
	case member.User.GlobalName != "":
		return member.User.GlobalName
	default:
		return member.User.Username
	}
}
