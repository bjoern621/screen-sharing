// The settings a contract message does not carry, and how a caller walking a draft as one keeps them.
package settings

// WithUnsent is s carrying the fields no contract message holds, read off the settings s was built from.
//
// Five, each a credential or a runtime fact rather than a setting a shell edits:
// the Discord link, the account it names, the brokered group, the relay token,
// and a stream name an override set (internal/wire, RelaySettings).
// A walk over the wire message drops all five,
// and a draft read back bare describes an install that is unlinked and in no group.
func (s Settings) WithUnsent(from Settings) Settings {
	s.streamName = from.streamName
	s.Relay.DiscordLink = from.Relay.DiscordLink
	s.Relay.DiscordAccount = from.Relay.DiscordAccount
	s.Relay.brokered = from.Relay.brokered
	s.Relay.Token = from.Relay.Token
	return s
}
