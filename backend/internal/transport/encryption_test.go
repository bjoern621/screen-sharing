package transport

import (
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/settings"
)

// A relay reached over TLS carries every leg encrypted, and each protocol encrypts differently:
// RTSP and RTMP terminate TLS themselves on listeners of their own, SRT has no TLS at all and is
// keyed by a passphrase, and WebRTC is DTLS-SRTP whatever anything here does.
//
// These hold the two halves of that: what a URL says, and what the publish refuses rather than
// sending in the clear.

// encrypted is settings pointed at a relay behind a TLS proxy, keyed and grouped like a real one.
func encrypted() settings.Settings {
	s := settings.Defaults()
	s.Relay.Host = "relay.example"
	s.Relay.SrtPassphrase = "a-passphrase-long-enough"
	s.Publish.Name = "standup"
	return s
}

// The scheme is the whole difference between a session somebody on the path can read and one they
// cannot, so it follows the relay rather than a second setting nobody would find.
func TestEncryptedRelayAddressesTlsListeners(t *testing.T) {
	s := encrypted()

	rtsp := rtspURL(s, s.Relay.Path(s.Publish.Name))
	if !strings.HasPrefix(rtsp, "rtsps://relay.example:8322/") {
		t.Errorf("RTSP addresses %q, want the rtsps listener", rtsp)
	}

	rtmp := rtmpURL(s, s.Relay.Path(s.Publish.Name))
	if !strings.HasPrefix(rtmp, "rtmps://relay.example:1936/") {
		t.Errorf("RTMP addresses %q, want the rtmps listener", rtmp)
	}
}

// A relay on the local network is the deployment where cleartext is a real choice, and the one the
// repository's own mediamtx.yml describes. Its URLs are unchanged.
func TestPlainRelayKeepsItsCleartextListeners(t *testing.T) {
	s := encrypted()
	s.Relay.Host = "192.168.1.9"

	if got := rtspURL(s, "standup"); !strings.HasPrefix(got, "rtsp://192.168.1.9:8554/") {
		t.Errorf("an unencrypted relay addresses %q, want its rtsp listener", got)
	}
	if got := rtmpURL(s, "standup"); !strings.HasPrefix(got, "rtmp://192.168.1.9:1935/") {
		t.Errorf("an unencrypted relay addresses %q, want its rtmp listener", got)
	}
}

// RTSPS wraps the control connection and nothing else, so RTP over UDP would travel beside it in
// the clear.
// Refused rather than interleaved behind the user's back: the control says udp, and a publish that
// quietly did otherwise would leave it saying so.
func TestEncryptedRtspRefusesUdp(t *testing.T) {
	s := encrypted()
	s.Publish.RtspPublishProtocol = "udp"

	if err := (RTSP{}).ValidatePublishSettings(s); err == nil {
		t.Fatal("an encrypted relay accepted RTP over UDP, which crosses the network unencrypted")
	}

	s.Publish.RtspPublishProtocol = EncryptedRtspProtocol
	if err := (RTSP{}).ValidatePublishSettings(s); err != nil {
		t.Errorf("interleaved RTP was refused on an encrypted relay: %v", err)
	}
}

// The same choice is free on a relay this network reaches directly, there being no TLS session for
// the media to be outside of.
func TestPlainRtspTakesEitherLowerTransport(t *testing.T) {
	s := encrypted()
	s.Relay.Host = "192.168.1.9"

	for _, protocol := range RtspProtocols {
		s.Publish.RtspPublishProtocol = protocol
		if err := (RTSP{}).ValidatePublishSettings(s); err != nil {
			t.Errorf("an unencrypted relay refused %s: %v", protocol, err)
		}
	}
}

// SRT is UDP with no TLS, so the passphrase is not one credential among several: it is the whole of
// what makes the leg unreadable, and an empty one on a relay across the internet is the picture in
// the clear.
func TestEncryptedSrtRefusesAnEmptyPassphrase(t *testing.T) {
	s := encrypted()
	s.Relay.SrtPassphrase = ""

	if err := (SRT{}).ValidatePublishSettings(s); err == nil {
		t.Fatal("an encrypted relay accepted SRT with no passphrase, which sends the stream in the clear")
	}

	s.Relay.SrtPassphrase = "a-passphrase-long-enough"
	if err := (SRT{}).ValidatePublishSettings(s); err != nil {
		t.Errorf("a keyed SRT publish was refused: %v", err)
	}
}
