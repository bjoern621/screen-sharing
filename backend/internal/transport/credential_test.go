package transport

import (
	"net/url"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/settings"
)

// The run log holds the child's whole command line and the app offers to open it, so a secret
// spelled there leaves with every log a user forwards.
// The passphrase is the one that matters most: it is the group's and expires with nothing.
func TestASecretIsHiddenInEveryCarriageForm(t *testing.T) {
	s := testStream()
	s.Relay.Token = "eyJhbGciOiJFUzI1NiJ9.payload.signature"
	s.Relay.GroupKey = mustGroupKey(t).String()
	passphrase := s.Relay.SrtPassphrase()

	// One line per carriage the transports build: a query, a field of an SRT stream id, and an element
	// property (credential.go, srt.go, webrtc.go).
	lines := []string{
		"rtsps://relay:8322/g/alice?jwt=" + s.Relay.Token,
		"srt://relay:8890?streamid=publish:g/alice:jwt:" + s.Relay.Token,
		"passphrase=" + passphrase,
		"srt://relay:8890?passphrase=" + url.QueryEscape(passphrase),
		"signaller::auth-token=" + s.Relay.Token,
		"https://jwt:" + s.Relay.Token + "@relay:8892/g/alice/",
	}
	for _, line := range lines {
		got := Redact(s, line)
		if strings.Contains(got, s.Relay.Token) {
			t.Errorf("%q kept the token: %q", line, got)
		}
		if strings.Contains(got, passphrase) {
			t.Errorf("%q kept the passphrase: %q", line, got)
		}
		if !strings.Contains(got, Redacted) {
			t.Errorf("%q named no redaction: %q", line, got)
		}
	}
}

// What a reader opens a run log for has to survive the redaction, or the log stops answering
// the question it exists for.
func TestRedactingKeepsTheAddressAroundTheSecret(t *testing.T) {
	s := testStream()
	s.Relay.Token = "a-token"

	got := Redact(s, "rtsps://relay:8322/g/alice?jwt=a-token")
	for _, keep := range []string{"rtsps", "relay:8322", "g/alice"} {
		if !strings.Contains(got, keep) {
			t.Errorf("redacting dropped %q: %q", keep, got)
		}
	}
}

// Every player page carries the credential the same way, and it is the one way a browser carries
// one at all: the relay's HTTP servers read a bearer token off a header and no query, measured, and
// an address handed to the desktop sets no header.
// A page addressed with a query would be answered 401 on every authenticated relay.
func TestEveryPlayerPageCarriesTheCredentialAsUserinfo(t *testing.T) {
	s := settings.Settings{
		Relay: settings.Relay{
			Host: "10.0.0.5", HlsPort: 8888, WebrtcPort: 8889, MoqPort: 8892,
			Token: "a.b.c",
		},
	}

	for _, name := range WatchNames(EngineBrowser) {
		page, ok := BrowserURL(name, s, "bob")
		if !ok {
			t.Fatalf("%s states a browser carriage and yields no page address", name)
		}
		if !strings.Contains(page, "://jwt:"+s.Relay.Token+"@") {
			t.Errorf("%s page carries no credential a browser can send: %q", name, page)
		}
		if strings.Contains(page, "jwt="+s.Relay.Token) {
			t.Errorf("%s page carries the credential as a query, which the relay answers 401: %q", name, page)
		}
	}
}

// An empty secret is a substring of every position in the line, so a relay carrying none must leave
// the line alone rather than redact between every character.
// A machine outside a group holds neither token nor passphrase, which is the settings this reads.
func TestSettingsCarryingNoSecretChangeNothing(t *testing.T) {
	s := settings.Settings{Relay: settings.Relay{Host: "10.0.0.5", SrtPort: 8890}}
	line := "srt://relay:8890?streamid=publish:alice"
	if got := Redact(s, line); got != line {
		t.Errorf("a run carrying no secret was rewritten: %q", got)
	}
}
