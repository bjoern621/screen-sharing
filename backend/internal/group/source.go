package group

// Where a machine's membership comes from.
//
// Two answers to one question rather than a flag on the Discord half:
// a group has one source at a time, and the controls under it are read by that source alone.
const (
	// SourceKey is the secret in the settings, handed over by whoever drew it.
	SourceKey = "key"
	// SourceDiscord is the voice channel this install's linked Discord account sits in,
	// with the key, the prefix and the passphrase brokered per channel (docs/discord-mode.md).
	SourceDiscord = "discord"
)

// Sources lists every source, in the order a form offers them.
//
// The key leads because it is what a fresh installation carries,
// and a draft naming no source it recognises is repaired onto the first entry (internal/form).
var Sources = []string{SourceKey, SourceDiscord}

// KnownSource reports whether source is one of Sources.
// Every string is legal input, the value coming off the settings file or another process,
// so unknown is an answer rather than a broken contract.
func KnownSource(source string) bool {
	for _, s := range Sources {
		if s == source {
			return true
		}
	}
	return false
}
