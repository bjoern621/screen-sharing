package transport

import (
	"net/url"

	"bjoernblessin.de/screenshare/internal/settings"
)

// Where a relay token rides is per protocol, and every difference is MediaMTX 1.20's answer,
// measured:
//
//   - RTSP, RTMP: query, "?jwt=<token>". Both engines pass a URL query through untouched.
//   - SRT: stream id, "publish:<path>:<user>:<token>". No query, no header.
//   - HLS, WebRTC: "Authorization: Bearer <token>". Their query form is a browser's - relay answers
//     it with a cookie and a redirect, and refuses a client that kept no cookie.
//
// Every form is empty without a token, which is what a LAN relay takes.
// Whether that suffices is the relay's answer; refusing to build the address would decide a policy
// this side cannot see.

// Token as the HTTP legs take it. ok=false without one.
// Handed out rather than written into a URL: the caller carries it, as a player option or an
// element property (internal/watch, webrtc.go).
func CredentialHeader(s settings.Settings) (name, value string, ok bool) {
	token, ok := credentialToken(s)
	if !ok {
		return "", "", false
	}
	return "Authorization", "Bearer " + token, true
}

// Bare token, for callers that write the header themselves.
// ffmpeg's WHIP muxer and the WHIP and WHEP elements add the scheme word, so a rendered header
// would send "Bearer Bearer <token>".
func credentialToken(s settings.Settings) (string, bool) {
	if s.Relay.Token == "" {
		return "", false
	}
	return s.Relay.Token, true
}

// Token as a query on an address-carrying URL, joined with the separator the caller's URL needs.
// Read by RTSP, RTMP, and the relay's own player page, which runs in a browser and answers the
// cookie.
func credentialQuery(s settings.Settings, separator string) string {
	if s.Relay.Token == "" {
		return ""
	}
	return separator + "jwt=" + url.QueryEscape(s.Relay.Token)
}

// Whole SRT stream id: mode, path, credential where there is one.
// "publish:<path>" without a token, "publish:<path>:jwt:<token>" with.
//
// MediaMTX reads the token as the password of "<mode>:<path>:<user>:<password>", so the user field
// cannot be dropped: the password is found by position.
//
// SRT truncates the id at srtStreamIDBytes rather than refusing it, which token.MaxTokenBytes and
// the stream-name bound are measured against. A halved token is a signature error at the relay, not
// a length error here.
func srtStreamID(s settings.Settings, mode, path string) string {
	id := mode + ":" + path
	if s.Relay.Token == "" {
		return id
	}
	return id + ":" + srtIDUser + ":" + s.Relay.Token
}

// Fills the user field of a stream id carrying a token.
// Any value does: JWT auth reads the password beside it, never this.
const srtIDUser = "jwt"

// Whether an assembled stream id survives the wire.
// A question and not an assert: the watch leg builds one from a path the relay named, and an
// over-long path there is that relay's doing.
func srtStreamIDFits(id string) bool {
	return len(id) <= srtStreamIDBytes
}

// Cap every SRT implementation puts on a stream id.
const srtStreamIDBytes = 512
