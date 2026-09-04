// Discord mode's half of Relay: the link to the manager,
// and the brokered facts that stand in for what a group key would derive
// (docs/discord-mode.md).
package settings

import (
	"fmt"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"
)

// DiscordServicePort is where discordd answers on a relay this network reaches directly,
// its own default (cmd/discordd, -listen).
// Behind the proxy the routes sit under /discord on the one name (deploy/Caddyfile).
const DiscordServicePort = 9444

// BrokeredGroup is what the manager derived for the current voice channel:
// the two facts a group key would otherwise derive locally.
// The key itself never reaches this side, which is the mode's whole security story.
type BrokeredGroup struct {
	Prefix        string
	SrtPassphrase string
}

// WithBrokeredGroup is r carrying the manager's answer for this pass or command.
//
// Runtime like Token: written per pass at the sites that fetched it, never stored,
// the json-less field keeping it out of the file.
// The display name rides along because the manager claims it from the Discord nick,
// and StreamName reads the field wherever the claim came from.
func (r Relay) WithBrokeredGroup(prefix, passphrase, displayName string) Relay {
	assert.Assert(strings.HasSuffix(prefix, "/"), "a prefix ends at its separator", prefix)
	assert.Assert(passphrase != "", "a brokered group keys its SRT legs")
	assert.Assert(displayName != "", "a member is listed under a name")

	r.brokered = &BrokeredGroup{Prefix: prefix, SrtPassphrase: passphrase}
	r.DisplayName = displayName
	return r
}

// DiscordService is where the Discord manager answers, ok=false where no relay is named.
//
// The deployment decides the address the way GroupService's does:
// through the proxy under its /discord prefix, or on discordd's own port where the relay
// is reached directly.
func (r Relay) DiscordService() (base string, ok bool) {
	if r.Host == "" {
		return "", false
	}
	if r.OnTrustedNetwork() {
		return fmt.Sprintf("http://%s:%d", r.Host, DiscordServicePort), true
	}
	return "https://" + r.Host + "/discord", true
}
