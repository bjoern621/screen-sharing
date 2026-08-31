package transport

import (
	"bjoernblessin.de/screenshare/internal/settings"
)

// What a relay's certificate is measured against, for the two legs that terminate TLS at the relay
// itself: RTSP and RTMP (deploy/mediamtx-groups.yml).
//
// A relay on a trusted network holds the self-signed pair scripts/relay.sh draws for it.
// Nothing issued that pair and no certificate store carries it, so a leg validating it opens
// nothing.
// Relaxed there, and there alone: a relay across somebody else's network holds a certificate issued
// for the name it is reached by, and a leg taking whatever certificate arrives takes the one
// an interception offers.
//
// Both branches are spelled out rather than left to an element's or a muxer's default.
// ffmpeg's tls protocol verifies nothing unless told to, so the encrypted case is a setting rather
// than an omission, and the two engines' defaults are opposite ends of this question.

// gstTlsValidation is the property rtspclientsink, rtspsrc and rtmp2src take, all three spelling it
// alike.
func gstTlsValidation(s settings.Settings) string {
	if s.Relay.OnTrustedNetwork() {
		return "tls-validation-flags=no-flags"
	}
	return "tls-validation-flags=validate-all"
}

// ffmpegTlsVerify is the same question as ffmpeg's tls protocol takes it, an output option
// that reaches the protocol under the muxer.
func ffmpegTlsVerify(s settings.Settings) []string {
	if s.Relay.OnTrustedNetwork() {
		return []string{"-tls_verify", "0"}
	}
	return []string{"-tls_verify", "1"}
}
