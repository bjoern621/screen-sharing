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
// An unreachable service leaves as an error rather than as a credential-less command: that command
// dies at the relay's handshake, and "the group service cannot be reached" is the reason a user can
// act on.
func (a *App) settingsForCommand(s settings.Settings) (settings.Settings, error) {
	base, ok := s.Relay.GroupService()
	if !ok || s.Relay.GroupKey == "" {
		// Nothing to trade, or nowhere to trade it.
		// Whether a credential is needed at all is the relay's answer.
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

// Drawing a group key is not here.
// The service draws one over HTTP and the settings field takes it, so a group is joined by pasting
// what its members were handed.
// A method for it would need a control-contract call and a control to press it from (docs/plan.md).
