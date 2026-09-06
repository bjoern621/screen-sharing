package app

import (
	"errors"
	"fmt"

	"bjoernblessin.de/screenshare/internal/applink"
	"bjoernblessin.de/screenshare/internal/settings"
)

// A link is how a stream of this app is named outside it (internal/applink).
// What follows one is a decode like any other, so this reads the link and answers the pair
// StartReceive takes, leaving the tile to whoever asked (docs/ipc-api.md, "The rule").

// errLinkNotOnRelay refuses a link naming a stream the relay is not carrying.
// Its own refusal because the reader can act on it:
// a link outlives the share it names, and what is wrong is the moment rather than the link.
var errLinkNotOnRelay = errors.New("nothing is publishing that stream right now: ask whoever shared the link to start sharing again")

// ResolveLink reads a link and answers the stream it names.
//
// Nothing is started here.
// The decode is StartReceive's and the tile is the shell's,
// so a link followed twice opens what is already open, which is the state it names.
func (a *App) ResolveLink(raw string) (string, error) {
	link, err := applink.Parse(raw)
	if err != nil {
		return "", err
	}

	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	// The group this machine is in, as the last statement of presence had it.
	// Read rather than derived from the settings: in Discord mode the group follows the channel,
	// and a link is followed against the channel this machine stands in now (discord.go).
	group := a.membership().Group
	if group == "" {
		if s.Relay.DiscordMode {
			return "", a.discordRefusal(s)
		}
		return "", errNoGroup
	}
	if link.Group != group {
		return "", errOtherGroup(s)
	}

	// A relay that answered says whether it carries the stream, and one out of reach says nothing:
	// the decode reports its own failure, where a refusal here would name the link for an outage.
	if carried, known := a.relayCarries(link.Stream); known && !carried {
		return "", errLinkNotOnRelay
	}

	return link.Stream, nil
}

// errOtherGroup refuses a link into a group this machine is not in,
// naming what would put it there in the mode it is in.
//
// Two sentences of one shape: what is wrong, and the one move that fixes it.
// In Discord mode that move is a voice channel, membership following it (docs/discord-mode.md).
func errOtherGroup(s settings.Settings) error {
	if s.Relay.DiscordMode {
		return errors.New("this stream is shared in another voice channel: join that channel in Discord to watch it")
	}
	return fmt.Errorf("this stream is in another group: paste that group's key under Relay to watch it")
}
