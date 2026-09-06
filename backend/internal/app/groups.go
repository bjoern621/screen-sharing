package app

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"bjoernblessin.de/screenshare/internal/groupclient"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The group service (internal/groupclient) holds the credential a relay wants beside an address.
//
// Two things read it: every relay command carries the token its group key was traded for,
// and the stream list comes off the service's index rather than the relay's API,
// which a member's token does not reach.
//
// A relay that authenticates nothing names no group service,
// so no token is minted and every command is built without one.

// groupService is what this app asks of the service,
// held as an interface at the caller so a test states the answers without a service running.
// One implementation, *groupclient.Client.
type groupService interface {
	Token(base, groupKey, memberSecret string) (string, error)
	Forget()
	Streams(base, groupKey string) ([]groupclient.Stream, error)
	State(base, groupKey, memberSecret, displayName string) (groupclient.Membership, error)
	Release(base, groupKey, memberSecret string) error
	CreateGroup(base string) (groupKey, groupID string, err error)
	SendReport(base string, bundle io.Reader) (string, error)
}

// settingsForCommand returns s carrying the token this relay connection needs.
// The one site that attaches one, so it reaches neither the held settings nor the store.
//
// A relay with a group service is asked for a token, the group key being what buys one.
// Without it a publisher would build a command with no credential,
// and meet a refusal at the handshake with nothing naming the cause.
//
// The trade names this machine's member secret,
// so the token's subject is the member id the relay closes a connection against.
// A group this machine has not joined names none,
// the bare trade a group being created makes (members.go).
//
// An unreachable service leaves as an error rather than as a credential-less command:
// that command dies at the relay's handshake,
// and "the group service cannot be reached" is the reason a user can act on.
func (a *App) settingsForCommand(s settings.Settings) (settings.Settings, error) {
	// The caller's copy came off the contract, which carries no link secret (internal/wire, ToRelay).
	s = a.withStoredLink(s)

	if s.Relay.DiscordMode {
		// The manager brokers the trade and the brokered facts ride the same copy (discord.go).
		return a.discordSettingsForCommand(s)
	}

	base, ok := s.Relay.GroupService()
	if !ok {
		// Nowhere to trade.
		// Whether a credential is needed is the relay's answer.
		return s, nil
	}

	token, err := a.groups.Token(base, s.Relay.GroupKey, a.memberSecret(s.Relay))
	if err != nil {
		return s, fmt.Errorf("no relay token for this group: %w", err)
	}
	s.Relay.Token = token
	return s, nil
}

// errNoGroup refuses everything this machine would publish while it is in no group.
//
// One sentence for the publish and for the synthetic set, both being streams on the relay,
// and both refused on settings.Relay.InGroup.
var errNoGroup = errors.New("this computer is in no group, so there is nowhere to publish: set a group key and a name under Relay")

// forgetRelayToken drops the held credential, so the next command trades the group key again.
// Called where the relay refused a connection this app built, the one sign a held token is spent:
// it expires on a clock this app does not read, and the service may have restarted on another key.
func (a *App) forgetRelayToken() {
	a.groups.Forget()
}

// groupIndexStatus is what is live, as the group service's index answers it.
// The relay's own API is not a member's to read:
// a group token grants publish and read under one prefix and names no API action (docs/plan.md).
//
// The index answers the reader count and the ingest rate, and never the roster,
// so the snapshot marks its source and a row draws its figures with an empty roster
// (relay.Status.FromIndex).
func (a *App) groupIndexStatus(s settings.Settings, base string) relay.Status {
	streams, err := a.groups.Streams(base, s.Relay.GroupKey)
	if err != nil {
		return relay.Status{Reachable: false, FromIndex: true, Error: err.Error()}
	}

	return relay.Status{Reachable: true, FromIndex: true, Paths: indexPaths(streams)}
}

// indexPaths is the index's rows in the snapshot's shape,
// shared by the manual fetch and the Discord pass, whose answers carry the same rows.
func indexPaths(streams []groupclient.Stream) []relay.Path {
	paths := make([]relay.Path, 0, len(streams))
	for _, stream := range streams {
		paths = append(paths, relay.Path{
			// The index answers the name inside the group, which is the name a snapshot carries.
			Name:    stream.Name,
			Ready:   stream.Ready,
			Tracks:  stream.Tracks,
			Format:  stream.Format,
			InMbps:  stream.InMbps,
			Readers: stream.Readers,
		})
	}
	return paths
}

// relayStatusFor is one snapshot, off the group service's index.
//
// A relay nobody named is asked nothing, and the zero snapshot is the answer:
// unreachable, with no failure to name about a relay that is not there (watch.go, lastRelayStatus).
// A machine holding no group key is the same case:
// a listing is a group's, so there is nothing to ask about
// and the refusal that came back would be read out as a relay that is down.
func (a *App) relayStatusFor(s settings.Settings) relay.Status {
	base, ok := s.Relay.GroupService()
	if !ok || s.Relay.GroupKey == "" {
		return relay.Status{}
	}
	return insidePrefix(a.groupIndexStatus(s, base), s.Relay.Prefix())
}

// insidePrefix names every path by what it is called inside prefix.
//
// One name per stream, and it is the one a publish states (settings.Settings.StreamName):
// a group is a path prefix, so every row of a member's list carries it and separates none of them,
// and a name matched against a live publish under two spellings matches under neither.
// What reaches the relay puts the prefix back on (settings.Settings.WatchPath).
//
// Trimmed rather than cut at the first separator:
// a path under somebody else's prefix would otherwise print under a name of this group's.
func insidePrefix(status relay.Status, prefix string) relay.Status {
	for i, path := range status.Paths {
		inside := strings.TrimPrefix(path.Name, prefix)
		if inside == "" {
			// Nothing but the prefix names no stream, so there is nothing to shorten to.
			inside = path.Name
		}
		status.Paths[i].Name = inside
	}
	return status
}

// CreateGroup draws a group key at this relay's service
// and hands it back with the prefix it derives.
//
// Nothing is stored and nothing is applied.
// Possession of the key is membership,
// so a key this app adopted on its own would move the machine into a group nobody else can reach,
// and say so only after the first stream went somewhere nobody was looking.
// The caller writes it to the group key field,
// the one write that changes a machine's group and a settings write like any other.
//
// A relay nobody named has no service to draw at,
// and the message says so rather than naming one that would be asked.
func (a *App) CreateGroup(relay settings.Relay) (groupKey, groupID string, err error) {
	base, ok := relay.GroupService()
	if !ok {
		return "", "", fmt.Errorf("a group key is drawn by a group service, and this relay names none")
	}
	return a.groups.CreateGroup(base)
}
