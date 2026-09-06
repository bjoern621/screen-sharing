package applink

import "testing"

func TestALinkRoundTrips(t *testing.T) {
	cases := []struct {
		name   string
		group  string
		stream string
	}{
		{"a stream named inside a group", "abc123", "bob/monitor-0"},
		{"a display name carrying a space", "abc123", "Bob's desk/monitor-0"},
		{"a display name carrying a separator", "abc123", "bob/two/monitor-0"},
		{"a stream with no display name", "abc123", "monitor-0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			read, err := Parse(FormatWatch(tc.group, tc.stream))
			if err != nil {
				t.Fatalf("reading back %s: %v", FormatWatch(tc.group, tc.stream), err)
			}
			if read.Group != tc.group || read.Stream != tc.stream {
				t.Errorf("read %q of %q, made from %q of %q", read.Stream, read.Group, tc.stream, tc.group)
			}
		})
	}
}

func TestALinkNamesThisApp(t *testing.T) {
	refused := []struct {
		name string
		raw  string
	}{
		{"nothing at all", ""},
		{"another program's scheme", "spotify://watch/abc123/bob/monitor-0"},
		{"a web address", "https://example.test/watch/abc123/bob/monitor-0"},
		{"another action", "mirrorme://publish/abc123/bob/monitor-0"},
		{"no stream beside the group", "mirrorme://watch/abc123"},
		{"no group at all", "mirrorme://watch/"},
	}

	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(tc.raw); err == nil {
				t.Errorf("%q is not a link this app opens", tc.raw)
			}
		})
	}
}

// The one shape a person reads off a link, so it is stated rather than derived in a test.
func TestALinkReadsAsItIsWritten(t *testing.T) {
	if got := FormatWatch("abc123", "bob/monitor-0"); got != "mirrorme://watch/abc123/bob/monitor-0" {
		t.Errorf("a link reads %q", got)
	}
}
