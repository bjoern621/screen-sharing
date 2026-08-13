package app

import (
	"fmt"

	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
)

// A relay wants a credential beside an address, and this app gets one from the group service
// (internal/groupclient).
//
// Two things read that service, the two halves of "which streams exist and may I open them": every
// command carries the token its group key was traded for, and the stream list comes from the
// service's index rather than the relay's API, which a member's token does not reach.
//
// A relay that authenticates nothing stays working: no group service configured, no token minted,
// every command built as it was before groups existed.

// Settings a command is built from: what the app holds, plus the credential this connection needs.
//
// The one place a token is attached, so a leg that forgot to ask cannot exist and the token reaches
// neither the held settings nor the store.
//
// An unreachable service leaves as an error rather than as a command built without a credential.
// That command is refused at the handshake, and "the group service cannot be reached" is a reason a
// user can act on where "the relay closed the connection" is not.
func (a *App) settingsForCommand(s settings.Settings) (settings.Settings, error) {
	base, ok := s.Relay.GroupService()
	if !ok || s.Relay.GroupKey == "" {
		// Nothing to trade, or nowhere to trade it. Which of the two it is decides nothing here: both
		// are a relay this app has no credential for, and whether one is needed is that relay's answer.
		return s, nil
	}

	token, err := a.groups.Token(base, s.Relay.GroupKey)
	if err != nil {
		return s, fmt.Errorf("no relay token for this group: %w", err)
	}
	s.Relay.Token = token
	return s, nil
}

// Drops the held credential, so the next command trades the group key again.
// Called where the relay refused a connection this app built, which is the one sign a held token is
// spent: it expires on a clock this app does not read, and the service may have restarted with a
// new key.
func (a *App) forgetRelayToken() {
	a.groups.Forget()
}

// What is live, as the group service answers it.
//
// The snapshot's other source, because the relay's API is not a member's to read: a group token
// grants publish and read under one prefix and names no API action (docs/plan.md).
//
// It cannot answer the operational half - who is reading, at what rate - so those fields stay zero
// and the snapshot says where it came from. Otherwise a reader shows "no viewers" for "not answered
// here" (relay.Status.FromIndex).
func (a *App) groupIndexStatus(s settings.Settings, base string) relay.Status {
	streams, err := a.groups.Streams(base, s.Relay.GroupKey)
	if err != nil {
		return relay.Status{Reachable: false, FromIndex: true, Error: err.Error()}
	}

	paths := make([]relay.Path, 0, len(streams))
	for _, stream := range streams {
		paths = append(paths, relay.Path{
			// The path a viewer opens is the whole one on the relay, where the index answers the name
			// inside the group: the prefix is the group's and every URL carries it.
			Name:   s.Relay.Path(stream.Name),
			Ready:  stream.Ready,
			Tracks: stream.Tracks,
			Format: stream.Format,
		})
	}
	return relay.Status{Reachable: true, FromIndex: true, Paths: paths}
}

// One snapshot, from whichever source this relay has.
// The deployment decides: a relay behind the proxy answers its API to nobody, and one without a
// proxy has no index to ask.
func (a *App) relayStatusFor(s settings.Settings) relay.Status {
	if base, ok := s.Relay.GroupService(); ok {
		return a.groupIndexStatus(s, base)
	}
	return a.relay.Fetch(s.Relay.Host, s.Relay.ApiPort)
}

// Drawing a key is not here.
// The service draws one over HTTP and the settings field takes it, so a group is joined by pasting
// what its members were handed; an app method for it would need a control-contract call and a
// control it could be pressed from, and neither exists yet (docs/plan.md).
