package settings

import (
	"strings"
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

func TestBrokeredGroupRefusesABadStreamName(t *testing.T) {
	r := relayInDiscordMode(t).WithBrokeredGroup("PFX/", "passphrase", "Bob")

	if path := r.Path("a/b/c"); strings.HasPrefix(path, "PFX/") {
		t.Fatalf("a name deeper than a stream reaches no brokered path, got %q", path)
	}
}
