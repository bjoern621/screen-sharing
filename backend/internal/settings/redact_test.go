package settings

import "testing"

// A report bundle carries the settings, so the two secrets go out blanked:
// the group key is membership itself, and whoever reads the Discord link
// watches that account's channels.
func TestRedactedBlanksTheSecretsAndKeepsTheRest(t *testing.T) {
	s := Defaults()
	s.Relay.GroupKey = "the-group-key"
	s.Relay.DiscordLink = "the-link-secret"
	s.Relay.Token = "a-minted-token"

	r := s.Redacted()

	if r.Relay.GroupKey != RedactedMark {
		t.Errorf("a set group key leaves as the mark, got %q", r.Relay.GroupKey)
	}
	if r.Relay.DiscordLink != RedactedMark {
		t.Errorf("a set Discord link leaves as the mark, got %q", r.Relay.DiscordLink)
	}
	if r.Relay.Token != "" {
		t.Errorf("a minted token leaves blank, got %q", r.Relay.Token)
	}
	if r.Relay.Host != s.Relay.Host || r.Publish.Transport != s.Publish.Transport || r.Viewer != s.Viewer {
		t.Error("everything beside the secrets survives redaction")
	}
}

// An empty secret stays empty, so a report says whether one was set at all.
func TestRedactedLeavesAnUnsetSecretEmpty(t *testing.T) {
	r := Defaults().Redacted()

	if r.Relay.GroupKey != "" || r.Relay.DiscordLink != "" {
		t.Errorf("unset secrets stay empty, got %q and %q", r.Relay.GroupKey, r.Relay.DiscordLink)
	}
}
