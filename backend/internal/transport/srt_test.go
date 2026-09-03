package transport

import (
	"net/url"
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/settings"
)

func testStream() settings.Settings {
	return settings.Settings{
		Relay: settings.Relay{
			Host:    "10.0.0.5",
			SrtPort: 8890,
		},
		Publish: settings.Publish{
			Transport:           "srt",
			SrtPublishLatencyMs: 300,
		},
		Viewer: settings.Viewer{
			SrtWatchLatencyMs: 1200,
		},
	}
}

// mustGroupKey fails the test on a drawing failure rather than carrying it into an assertion.
func mustGroupKey(t *testing.T) group.Key {
	t.Helper()
	key, err := group.NewKey()
	if err != nil {
		t.Fatalf("drawing a group key: %v", err)
	}
	return key
}

func TestSRTRegistered(t *testing.T) {
	tr, ok := Get("srt")
	if !ok {
		t.Fatal("srt transport not registered")
	}
	if tr.Name() != "srt" {
		t.Fatalf("Name() = %q, want srt", tr.Name())
	}
	if !slices.Contains(Names(), "srt") {
		t.Errorf("Names() = %v, missing srt", Names())
	}
}

func TestSRTPublishArgs(t *testing.T) {
	args := SRT{}.PublishArgs(testStream())

	if len(args) != 3 || args[0] != "-f" || args[1] != "mpegts" {
		t.Fatalf("PublishArgs prefix = %v, want [-f mpegts URL]", args)
	}

	url := args[2]
	for _, want := range []string{
		"srt://10.0.0.5:8890",
		"streamid=publish:public/monitor-0",
		"pkt_size=1316",
		"latency=300000", // ffmpeg's srt protocol counts microseconds
	} {
		if !strings.Contains(url, want) {
			t.Errorf("publish URL %q missing %q", url, want)
		}
	}
}

func TestSRTGstSource(t *testing.T) {
	src := SRT{}.GstSource(testStream(), "bob")

	for _, want := range []string{
		"srtsrc",
		"uri=srt://10.0.0.5:8890",
		"mode=caller",
		"streamid=read:bob",
		"latency=1200", // srtsrc counts milliseconds, unlike the ffmpeg URL
	} {
		if !slices.Contains(src, want) {
			t.Errorf("GstSource = %v, missing %q", src, want)
		}
	}
}

// Every SRT leg is keyed with the passphrase the settings derive, both engines and both directions,
// and a member's legs carry the group's own where a keyless machine's carry the public one.
// A leg missing it connects to a relay that refuses the handshake, and one carrying another group's
// plays nothing.
func TestEverySRTLegCarriesTheDerivedPassphrase(t *testing.T) {
	member := testStream()
	member.Relay.GroupKey = mustGroupKey(t).String()

	for deployment, s := range map[string]settings.Settings{
		"a member's machine": member,
		"a keyless machine":  testStream(),
	} {
		passphrase := s.Relay.SrtPassphrase()
		if passphrase == "" {
			t.Fatalf("%s derives no passphrase", deployment)
		}

		for leg, address := range map[string]string{
			"the publish command": SRT{}.PublishArgs(s)[2],
			"the watch URL":       SRT{}.WatchURL(s, "bob"),
		} {
			if !strings.Contains(address, "passphrase="+url.QueryEscape(passphrase)) {
				t.Errorf("%s of %s carries no passphrase: %q", leg, deployment, address)
			}
		}
		for leg, elements := range map[string][]string{
			"the publish sink":   SRT{}.GstSink(s),
			"the receive source": SRT{}.GstSource(s, "bob"),
		} {
			if !slices.Contains(elements, "passphrase="+passphrase) {
				t.Errorf("%s of %s carries no passphrase: %v", leg, deployment, elements)
			}
		}
	}
}

func TestSRTWatchURL(t *testing.T) {
	url := SRT{}.WatchURL(testStream(), "bob")

	for _, want := range []string{
		"srt://10.0.0.5:8890",
		"streamid=read:bob",
		"latency=1200000", // the watch knob in microseconds too
	} {
		if !strings.Contains(url, want) {
			t.Errorf("watch URL %q missing %q", url, want)
		}
	}
}
