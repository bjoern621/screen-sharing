package settings

import (
	"os"
	"testing"

	"bjoernblessin.de/screenshare/internal/group"
)

// Where a group comes from, as a stored file names it.
// The flag a file may carry is the same fact, so both of its states upgrade,
// and anything else lands on the source a fresh installation carries.

// seedStore writes a settings file directly, for a shape Save would never produce.
func seedStore(t *testing.T, body string) {
	t.Helper()
	if err := os.WriteFile(mustSettingsPath(t), []byte(body), 0o600); err != nil {
		t.Fatalf("seeding the settings file: %v", err)
	}
}

func TestAFreshInstallationTakesItsGroupFromTheKey(t *testing.T) {
	if got := Defaults().Relay.GroupSource; got != group.SourceKey {
		t.Errorf("a fresh installation names %q, want the key it can be handed", got)
	}
}

func TestAStoredFlagUpgradesToTheSourceItNamed(t *testing.T) {
	for _, c := range []struct {
		flag string
		want string
	}{
		{flag: "true", want: group.SourceDiscord},
		{flag: "false", want: group.SourceKey},
	} {
		t.Run(c.flag, func(t *testing.T) {
			isolateConfig(t)
			seedStore(t, `{"relay":{"host":"relay.example.com","discordMode":`+c.flag+`}}`)

			if got := mustLoad(t).Relay.GroupSource; got != c.want {
				t.Errorf("a stored flag of %s reads as %q, want %q", c.flag, got, c.want)
			}
		})
	}
}

func TestAFileNamingNoSourceAtAllLandsOnTheKey(t *testing.T) {
	isolateConfig(t)
	seedStore(t, `{"relay":{"host":"relay.example.com","groupKey":"a-key"}}`)

	if got := mustLoad(t).Relay.GroupSource; got != group.SourceKey {
		t.Errorf("a file naming no source reads as %q, want the key its own field holds", got)
	}
}

func TestASourceThisBuildDoesNotKnowIsRepaired(t *testing.T) {
	isolateConfig(t)
	seedStore(t, `{"relay":{"host":"relay.example.com","groupSource":"voice-chat"}}`)

	if got := mustLoad(t).Relay.GroupSource; got != group.SourceKey {
		t.Errorf("an unknown source reads as %q, want the shipped one", got)
	}
}

// The stored key stays where it is under Discord, so moving back reaches the same group.
func TestTheDiscordSourceLeavesTheStoredKeyAlone(t *testing.T) {
	isolateConfig(t)
	seedStore(t, `{"relay":{"host":"relay.example.com","groupKey":"a-key","discordMode":true}}`)

	s := mustLoad(t)
	if s.Relay.GroupSource != group.SourceDiscord || s.Relay.GroupKey != "a-key" {
		t.Errorf("the upgrade holds source %q and key %q", s.Relay.GroupSource, s.Relay.GroupKey)
	}
}
