package transport

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/settings"
)

// Every relay this repository runs terminates TLS on the two legs no proxy can carry, RTSP and
// RTMP, and its HTTP legs are reached through the proxy or on its own ports
// (deploy/mediamtx-groups.yml, deploy/Caddyfile).
//
// These hold what falls out of that: which listener an address names, what the certificate on it
// is measured against, and what the publish refuses rather than sending in the clear.

// behindTheProxy is a relay across somebody else's network, named like a real one.
func behindTheProxy() settings.Settings {
	s := settings.Defaults()
	s.Relay.Host = "relay.example"
	s.Publish.Name = "standup"
	return s
}

// onThisNetwork is a relay this network reaches directly, the shape scripts/relay.sh starts.
func onThisNetwork() settings.Settings {
	s := behindTheProxy()
	s.Relay.Host = "192.168.1.9"
	return s
}

// relays are the two deployments, named as a failure should name them.
func relays() map[string]settings.Settings {
	return map[string]settings.Settings{
		"a relay behind the proxy": behindTheProxy(),
		"a relay on this network":  onThisNetwork(),
	}
}

// RTSP and RTMP are not HTTP, so no proxy carries either and the relay terminates TLS on a listener
// of its own wherever it runs.
// Both are addressed on the encrypted listener on every relay, at the port the settings name, there
// being no cleartext listener anywhere to address.
func TestEveryRelayAddressesItsEncryptedMediaListeners(t *testing.T) {
	for deployment, s := range relays() {
		rtsp := rtspURL(s, s.Relay.Path(s.Publish.Name))
		if want := fmt.Sprintf("rtsps://%s:%d/", s.Relay.Host, s.Relay.RtspPort); !strings.HasPrefix(rtsp, want) {
			t.Errorf("%s addresses RTSP as %q, want the encrypted listener at %q", deployment, rtsp, want)
		}

		rtmp := rtmpURL(s, s.Relay.Path(s.Publish.Name))
		if want := fmt.Sprintf("rtmps://%s:%d/", s.Relay.Host, s.Relay.RtmpPort); !strings.HasPrefix(rtmp, want) {
			t.Errorf("%s addresses RTMP as %q, want the encrypted listener at %q", deployment, rtmp, want)
		}
	}
}

// The certificate a relay on a trusted network holds is the self-signed pair scripts/relay.sh
// draws, which nothing issued and no store carries, so a leg that validates it opens nothing
// at all.
//
// Relaxed there and nowhere else.
// A relay across somebody else's network holds a certificate issued for its name, and taking
// whatever certificate arrives on that leg takes the one an interception offers.
func TestOnlyARelayOnThisNetworkHasItsCertificateTakenAsItStands(t *testing.T) {
	direct, proxied := onThisNetwork(), behindTheProxy()

	gst := map[string][]string{
		"the rtsps publish sink":   RTSP{}.GstSink(direct),
		"the rtsps receive source": RTSP{}.GstSource(direct, "bob"),
		"the rtmps receive source": RTMP{}.GstSource(direct, "bob"),
	}
	for leg, elements := range gst {
		if !slices.Contains(elements, "tls-validation-flags=no-flags") {
			t.Errorf("%s of a relay on this network is %v, which validates a certificate nothing issued",
				leg, elements)
		}
	}
	for leg, elements := range map[string][]string{
		"the rtsps publish sink":   RTSP{}.GstSink(proxied),
		"the rtsps receive source": RTSP{}.GstSource(proxied, "bob"),
		"the rtmps receive source": RTMP{}.GstSource(proxied, "bob"),
	} {
		if !slices.Contains(elements, "tls-validation-flags=validate-all") {
			t.Errorf("%s of a relay across the internet is %v, which takes whichever certificate arrives",
				leg, elements)
		}
	}

	for leg, args := range map[string][]string{
		"the rtsps publish command": RTSP{}.PublishArgs(direct),
		"the rtmps publish command": RTMP{}.PublishArgs(direct),
	} {
		if !containsPair(args, "-tls_verify", "0") {
			t.Errorf("%s to a relay on this network is %v, which validates a certificate nothing issued", leg, args)
		}
	}
	for leg, args := range map[string][]string{
		"the rtsps publish command": RTSP{}.PublishArgs(proxied),
		"the rtmps publish command": RTMP{}.PublishArgs(proxied),
	} {
		if !containsPair(args, "-tls_verify", "1") {
			t.Errorf("%s to a relay across the internet is %v, which takes whichever certificate arrives", leg, args)
		}
	}
}

// containsPair reports whether args names this flag with this value beside it.
func containsPair(args []string, flag, value string) bool {
	at := slices.Index(args, flag)
	return at >= 0 && at+1 < len(args) && args[at+1] == value
}

// RTSPS wraps the control connection and nothing else, so RTP over UDP travels beside it
// in the clear on every relay, a LAN one included.
// Refused rather than interleaved behind the user's back: the control says udp, and a publish
// that quietly did otherwise would leave it saying so.
func TestRtspRefusesUdpOnEveryRelay(t *testing.T) {
	for deployment, s := range relays() {
		s.Publish.RtspPublishProtocol = "udp"
		if err := (RTSP{}).ValidatePublishSettings(s); err == nil {
			t.Errorf("%s accepted RTP over UDP, which crosses the network unencrypted", deployment)
		}

		s.Publish.RtspPublishProtocol = EncryptedRtspProtocol
		if err := (RTSP{}).ValidatePublishSettings(s); err != nil {
			t.Errorf("%s refused interleaved RTP: %v", deployment, err)
		}
	}
}

// The watch leg is the same session from the other end, so it takes the same lower transport.
func TestTheRtspWatchLegRefusesUdpOnEveryRelay(t *testing.T) {
	for deployment, s := range relays() {
		if err := (RTSP{}).SetWatchOption(&s, rtspWatchProtocolKey, "udp"); err == nil {
			t.Errorf("%s accepted a viewer receiving RTP over UDP, which arrives unencrypted", deployment)
		}
		if err := (RTSP{}).SetWatchOption(&s, rtspWatchProtocolKey, EncryptedRtspProtocol); err != nil {
			t.Errorf("%s refused a viewer receiving interleaved RTP: %v", deployment, err)
		}
	}
}

// SRT is UDP with no TLS, so the passphrase is not one credential among several: it is the whole
// of what makes the leg unreadable.
// It derives from the group key, so the one machine none derives for is one whose stored key will
// not read back, and across the internet that publish is refused rather than sent in the clear.
func TestSrtAcrossTheInternetRefusesAnUnderivablePassphrase(t *testing.T) {
	s := behindTheProxy()
	s.Relay.GroupKey = "not a group key"

	if err := (SRT{}).ValidatePublishSettings(s); err == nil {
		t.Fatal("a relay across the internet accepted SRT with no passphrase, which sends the stream in the clear")
	}

	// A member and a keyless machine both derive one, so both publish.
	s.Relay.GroupKey = mustGroupKey(t).String()
	if err := (SRT{}).ValidatePublishSettings(s); err != nil {
		t.Errorf("a member's SRT publish was refused: %v", err)
	}
	s.Relay.GroupKey = ""
	if err := (SRT{}).ValidatePublishSettings(s); err != nil {
		t.Errorf("a keyless machine's SRT publish was refused: %v", err)
	}
}

// HLS and WebRTC are HTTP, so the proxy carries them under one name on the standard port, and
// a relay this network reaches directly answers each on its own listener.
func TestTheHttpLegsFollowTheProxy(t *testing.T) {
	proxied, direct := behindTheProxy(), onThisNetwork()

	for leg, address := range map[string]string{
		"hls":    (HLS{}).WatchURL(proxied, "bob"),
		"webrtc": whepURL(proxied, "bob"),
	} {
		if !strings.HasPrefix(address, "https://relay.example/") {
			t.Errorf("the %s leg of a relay behind the proxy addresses %q, want the proxy's own name", leg, address)
		}
	}

	if got, want := (HLS{}).WatchURL(direct, "bob"),
		fmt.Sprintf("http://%s:%d/bob/index.m3u8", direct.Relay.Host, direct.Relay.HlsPort); got != want {
		t.Errorf("the hls leg of a relay on this network addresses %q, want %q", got, want)
	}
	if got, want := whepURL(direct, "bob"),
		fmt.Sprintf("http://%s:%d/bob/whep", direct.Relay.Host, direct.Relay.WebrtcPort); got != want {
		t.Errorf("the webrtc leg of a relay on this network addresses %q, want %q", got, want)
	}
}
