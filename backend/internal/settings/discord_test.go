package settings

import (
	"os"
	"testing"

	"bjoernblessin.de/screenshare/internal/group"
)

// relayInDiscordMode is a relay whose settings still hold a manual key,
// which Discord mode must leave unread.
func relayInDiscordMode(t *testing.T) Relay {
	t.Helper()
	key, err := group.NewKey()
	if err != nil {
		t.Fatalf("drawing a key: %v", err)
	}
	return Relay{
		Host:        "streamrelay.example.com",
		DiscordMode: true,
		GroupKey:    key.String(),
		DisplayName: "Bob",
	}
}

func TestDiscordModeLeavesTheManualKeyUnread(t *testing.T) {
	r := relayInDiscordMode(t)

	if r.InGroup() {
		t.Fatal("a manual key states no membership while the group follows the voice channel")
	}
	if path := r.Path("monitor-0"); path != "monitor-0" {
		t.Fatalf("a manual key derives no path in Discord mode, got %q", path)
	}
	if r.SrtPassphrase() != "" {
		t.Fatal("a manual key derives no passphrase in Discord mode")
	}
}

func TestBrokeredGroupCarriesTheDerivedFacts(t *testing.T) {
	r := relayInDiscordMode(t).WithBrokeredGroup("PFX/", "passphrase", "Bobby")

	if !r.InGroup() {
		t.Fatal("a brokered group is membership")
	}
	if path := r.Path("monitor-0"); path != "PFX/monitor-0" {
		t.Fatalf("a path lives under the brokered prefix, got %q", path)
	}
	if r.SrtPassphrase() != "passphrase" {
		t.Fatalf("the brokered passphrase keys the SRT legs, got %q", r.SrtPassphrase())
	}
	if r.Prefix() != "PFX/" {
		t.Fatalf("the prefix reads back off the path, got %q", r.Prefix())
	}
	if r.DisplayName != "Bobby" {
		t.Fatalf("the brokered name is what this machine is called, got %q", r.DisplayName)
	}
}

func TestDiscordServiceFollowsTheDeployment(t *testing.T) {
	direct := Relay{Host: "192.168.1.9"}
	base, ok := direct.DiscordService()
	if !ok || base != "http://192.168.1.9:9444" {
		t.Fatalf("a relay on this network is asked on discordd's own port, got %q ok=%v", base, ok)
	}

	proxied := Relay{Host: "streamrelay.example.com"}
	base, ok = proxied.DiscordService()
	if !ok || base != "https://streamrelay.example.com/discord" {
		t.Fatalf("a public relay is asked through the proxy under /discord, got %q ok=%v", base, ok)
	}

	if _, ok := (Relay{}).DiscordService(); ok {
		t.Fatal("no relay is no manager to ask")
	}
}

// Discord hands over the name a person picked for themselves,
// so a brokered path is spelled like every other (internal/group, SpellName):
// a separator lands inside the member's own segment, and a byte outside the alphabet is spelled.
func TestABrokeredPathSpellsTheNameDiscordHandedOver(t *testing.T) {
	r := relayInDiscordMode(t).WithBrokeredGroup("PFX/", "passphrase", "DJ/Rex")

	if path := r.Path("a/b/c"); path != "PFX/a_2fb/c" {
		t.Fatalf("a name carrying a separator reaches %q", path)
	}
	if path := r.Path("Björn/monitor-0"); path != "PFX/Bj_c3_b6rn/monitor-0" {
		t.Fatalf("a name outside the alphabet reaches %q", path)
	}
}

func TestAFreshInstallationStatesItsShare(t *testing.T) {
	if !Defaults().App.DiscordRichPresence {
		t.Error("a fresh installation states its share on the Discord client beside it")
	}
}

func TestTheStatedShareStaysOffOnceTurnedOff(t *testing.T) {
	isolateConfig(t)
	s := Defaults()
	s.App.DiscordRichPresence = false

	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if mustLoad(t).App.DiscordRichPresence {
		t.Error("a stored refusal outlives a default the decode starts from")
	}
}

// The stated share sits with the app, and a file written while it sat with the relay
// carries it under the relay group alone.
// A fresh installation states its share, so a stored refusal is what the decode would put back on.
func TestARefusalStoredUnderTheRelayGroupOutlivesTheMove(t *testing.T) {
	isolateConfig(t)

	path := mustSettingsPath(t)
	const body = `{"relay":{"host":"relay.lan","discordRichPresence":false}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("seed the stored file: %v", err)
	}

	if mustLoad(t).App.DiscordRichPresence {
		t.Error("a refusal stored under the relay group came back on under the app group")
	}
}
