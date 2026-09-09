package transport

import (
	"net/url"
	"regexp"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/settings"
)

// How a publish child is handed the secrets its pipeline needs.
//
// A process's arguments are readable by every process on the machine,
// /proc/<pid>/cmdline being world readable and ps listing them, where its environment is not.
// So a pipeline crosses carrying a placeholder per secret,
// and the child puts the values back out of variables only its owner reads
// (cmd/backend, runPipeline).
// The passphrase is the one that matters most: it is the group's and outlives every token.
//
// A child this app wrote is what this covers.
// An ffmpeg publish parses the address it is handed, so those legs carry the values on argv.

// The variables a child reads its secrets out of.
const (
	TokenVar      = "SCREENSHARE_RELAY_TOKEN"
	PassphraseVar = "SCREENSHARE_SRT_PASSPHRASE"
)

// childSecret is one value a child needs, and the variable it arrives in.
type childSecret struct {
	name  string
	value string
}

// secretsOf is every secret these settings carry, each beside the variable it crosses in.
// One owner of which values are secret, read by the redaction and by the hiding alike.
func secretsOf(s settings.Settings) []childSecret {
	held := []childSecret{
		{name: TokenVar, value: s.Relay.Token},
		{name: PassphraseVar, value: s.Relay.SrtPassphrase()},
	}

	out := make([]childSecret, 0, len(held))
	for _, secret := range held {
		if secret.value != "" {
			out = append(out, secret)
		}
	}
	return out
}

// placeholder stands in an argument where a secret stood.
// Named after the variable holding it, so an argument that reached a listing says which secret
// was there rather than what it was.
func placeholder(name string) string {
	return "${" + name + "}"
}

// placeholderPattern finds a placeholder and captures the variable naming it.
// A GStreamer pipeline description spells no variable of its own,
// so this shape stands for one this side put there.
var placeholderPattern = regexp.MustCompile(`\$\{([A-Z0-9_]+)\}`)

// Hidden is args with every secret these settings carry replaced by its placeholder.
//
// Keyed on the value, as Redact is:
// a secret reaches a child as a query parameter, an element property and a field of an SRT stream id,
// so a rule written per form misses whichever form is added next.
func Hidden(s settings.Settings, args []string) []string {
	out := make([]string, len(args))
	copy(out, args)

	for _, secret := range secretsOf(s) {
		// One replacement covers a value however it travels,
		// both secrets being spelled in an alphabet a query leaves alone.
		// One spelled outside it would need its escaped form too.
		assert.Assert(url.QueryEscape(secret.value) == secret.value,
			"a secret crossing to a child is spelled in characters a query leaves alone", secret.name)

		for i, arg := range out {
			out[i] = strings.ReplaceAll(arg, secret.value, placeholder(secret.name))
		}
	}
	return out
}

// SecretEnv is the environment a child reads the hidden values back out of,
// one entry per secret these settings carry.
func SecretEnv(s settings.Settings) []string {
	out := make([]string, 0, 2)
	for _, secret := range secretsOf(s) {
		out = append(out, secret.name+"="+secret.value)
	}
	return out
}

// Revealed is args with every placeholder replaced by what lookup answers for its variable.
//
// A placeholder whose variable is unset is left alone.
// It reaches the relay as a credential that is refused, which names the leg that lost its secret,
// where a pipeline quietly missing one names nothing.
func Revealed(args []string, lookup func(name string) (string, bool)) []string {
	assert.IsNotNil(lookup, "a child reads its secrets out of somewhere")

	out := make([]string, len(args))
	for i, arg := range args {
		out[i] = placeholderPattern.ReplaceAllStringFunc(arg, func(match string) string {
			value, held := lookup(placeholderPattern.FindStringSubmatch(match)[1])
			if !held {
				return match
			}
			return value
		})
	}
	return out
}
