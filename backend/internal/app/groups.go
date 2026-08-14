package app

import (
	"fmt"

	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The group service (internal/groupclient) holds the credential a relay wants beside an address.
//
// Two things read it: every relay command carries the token its group key was traded for, and the
// stream list comes off the service's index rather than the relay's API, which a member's token does
// not reach.
//
// A relay that authenticates nothing names no group service, so no token is minted and every command
// is built without one.

// settingsForCommand returns s carrying the token this relay connection needs.
// The one site that attaches one, so it reaches neither the held settings nor the store.
//
// A relay with a group service is asked for a token whether or not there is a group key, an empty
// key being a request for the public prefix (internal/groupclient).
// Without that, a publisher who set no key would build a command with no credential and meet a
// refusal at the handshake with nothing naming the cause.
//
// An unreachable service leaves as an error rather than as a credential-less command: that command
// dies at the relay's handshake, and "the group service cannot be reached" is the reason a user can
// act on.
func (a *App) settingsForCommand(s settings.Settings) (settings.Settings, error) {
	base, ok := s.Relay.GroupService()
	if !ok {
		// Nowhere to trade. Whether a credential is needed at all is the relay's answer.
		return s, nil
	}

	token, err := a.groups.Token(base, s.Relay.GroupKey)
	if err != nil {
		return s, fmt.Errorf("no relay token for this group: %w", err)
	}
	s.Relay.Token = token
	return s, nil
}

// forgetRelayToken drops the held credential, so the next command trades the group key again.
// Called where the relay refused a connection this app built, the one sign a held token is spent:
// it expires on a clock this app does not read, and the service may have restarted on a new key.
func (a *App) forgetRelayToken() {
	a.groups.Forget()
}

// groupIndexStatus is what is live, as the group service's index answers it.
// The relay's own API is not a member's to read: a group token grants publish and read under one
// prefix and names no API action (docs/plan.md).
//
// The index answers no reader count and no rate, so those figures stay zero and the snapshot marks
// its source (relay.Status.FromIndex).
// Otherwise a reader shows "no viewers" for "not answered here".
func (a *App) groupIndexStatus(s settings.Settings, base string) relay.Status {
	streams, err := a.groups.Streams(base, s.Relay.GroupKey)
	if err != nil {
		return relay.Status{Reachable: false, FromIndex: true, Error: err.Error()}
	}

	paths := make([]relay.Path, 0, len(streams))
	for _, stream := range streams {
		paths = append(paths, relay.Path{
			// The index answers the name inside the group, and a viewer opens the whole relay path,
			// group prefix included.
			Name:   s.Relay.Path(stream.Name),
			Ready:  stream.Ready,
			Tracks: stream.Tracks,
			Format: stream.Format,
		})
	}
	return relay.Status{Reachable: true, FromIndex: true, Paths: paths}
}

// relayStatusFor is one snapshot, off whichever source this relay has.
// The deployment decides: a relay behind the proxy answers its API to nobody, and one without a
// proxy has no index to ask.
func (a *App) relayStatusFor(s settings.Settings) relay.Status {
	if base, ok := s.Relay.GroupService(); ok {
		return a.groupIndexStatus(s, base)
	}
	return a.relay.Fetch(s.Relay.Host, s.Relay.ApiPort)
}

// CreateGroup draws a group key at this relay's service and hands it back with the prefix it
// derives.
//
// Nothing is stored and nothing is applied. Possession of the key is membership, so a key this app
// adopted on its own would move the machine into a group nobody else can reach and say so only
// after the first stream went somewhere nobody was looking.
// The caller writes it to the group key field, which is the one write that changes a machine's
// group and is a settings write like any other.
//
// A relay with no group service is one where there are no groups to draw from, which is the LAN
// shape, and its message says so rather than naming a service that would be asked.
func (a *App) CreateGroup(relay settings.Relay) (key, id string, err error) {
	base, ok := relay.GroupService()
	if !ok {
		return "", "", fmt.Errorf("a group key is drawn by a group service, and a relay reached without a TLS proxy has none")
	}
	return a.groups.CreateKey(base)
}
