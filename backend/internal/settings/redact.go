package settings

// RedactedMark stands where a secret stood, so a report says one was set without carrying it.
const RedactedMark = "redacted"

// Redacted is s with every secret blanked, for a bundle leaving this machine (internal/report).
//
// The two stored secrets are the relay group's: the group key is membership itself,
// and whoever reads the Discord link watches that account's channels.
// The minted token is blanked too, though the store already leaves it out.
func (s Settings) Redacted() Settings {
	if s.Relay.GroupKey != "" {
		s.Relay.GroupKey = RedactedMark
	}
	if s.Relay.DiscordLink != "" {
		s.Relay.DiscordLink = RedactedMark
	}
	s.Relay.Token = ""
	return s
}
