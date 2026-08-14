package transport

import (
	"net/url"

	"bjoernblessin.de/screenshare/internal/settings"
)

// Where a relay token rides is per protocol, and every difference is MediaMTX's answer, measured:
//
//   - RTSP: query, "?jwt=<token>", except for rtspsrc, which takes the pair rtspCredential builds.
//   - RTMP: query, "?jwt=<token>". Both engines pass a URL query through untouched.
//   - SRT: stream id, "publish:<path>:<user>:<token>". No query, no header.
//   - HLS, WebRTC: "Authorization: Bearer <token>". Their query form is a browser's, answered with
//     a cookie and a redirect, and a client that kept no cookie is refused.
//
// Every form is empty without a token, which is what a LAN relay takes.
// Whether that suffices is the relay's answer, and refusing to build the address would decide a
// policy this side cannot see.

// CredentialHeader is the token as the HTTP legs take it, ok=false without one.
// Handed out rather than written into a URL, because the caller carries it as a player option or an
// element property (internal/watch, webrtc.go).
func CredentialHeader(s settings.Settings) (name, value string, ok bool) {
	token, ok := credentialToken(s)
	if !ok {
		return "", "", false
	}
	return "Authorization", "Bearer " + token, true
}

// credentialToken is the bare token, for callers that write the header themselves.
// ffmpeg's WHIP muxer and the WHIP and WHEP elements add the scheme word, so a rendered header
// would send "Bearer Bearer <token>".
func credentialToken(s settings.Settings) (string, bool) {
	if s.Relay.Token == "" {
		return "", false
	}
	return s.Relay.Token, true
}

// credentialQuery is the token as a query, joined with the separator the caller's URL needs ("?",
// "&").
// Read by RTSP, RTMP, and the relay's player page, which runs in a browser and answers the cookie.
func credentialQuery(s settings.Settings, separator string) string {
	if s.Relay.Token == "" {
		return ""
	}
	return separator + "jwt=" + url.QueryEscape(s.Relay.Token)
}

// rtspCredential is the token as an RTSP session's user and password, ok=false without one.
//
// rtspsrc alone: it sets up a track at the SDP's control attribute joined onto the session URL, and
// that join keeps neither the query nor the last path segment, so ".../public/desk?jwt=<token>" is
// set up as ".../public/trackID=0" and the relay answers 401 Unauthorized on a path nothing serves.
// A credential the element holds as properties is sent with every request instead, SETUP included.
// ffmpeg's reader and rtspclientsink join the same pair losing neither half, which is why the query
// form stays on the legs they drive.
//
// MediaMTX reads the token out of the password and never the user beside it, the way it reads the
// SRT stream id.
func rtspCredential(s settings.Settings) (user, password string, ok bool) {
	token, ok := credentialToken(s)
	if !ok {
		return "", "", false
	}
	return rtspAuthUser, token, true
}

// rtspAuthUser fills the user half of that pair.
// Empty is not a value it takes: rtspsrc sends no credential at all unless both halves are set.
const rtspAuthUser = "jwt"

// srtStreamID is the whole stream id: "publish:<path>" without a token,
// "publish:<path>:jwt:<token>" with one.
//
// MediaMTX reads the token as the password of "<mode>:<path>:<user>:<password>", found by position,
// so the user field cannot be dropped.
//
// SRT truncates the id at srtStreamIDBytes rather than refusing it, and token.MaxTokenBytes and the
// stream-name bound are measured against that.
// A halved token reaches the relay as a signature error, never as a length error here.
func srtStreamID(s settings.Settings, mode, path string) string {
	id := mode + ":" + path
	if s.Relay.Token == "" {
		return id
	}
	return id + ":" + srtIDUser + ":" + s.Relay.Token
}

// srtIDUser fills the user field of a stream id carrying a token.
// Any value does: JWT auth reads the password beside it and never this.
const srtIDUser = "jwt"

// srtStreamIDFits reports whether an assembled id survives the wire.
// A question rather than an assert, because the watch leg builds an id from a path the relay named,
// and an over-long one there is that relay's doing.
func srtStreamIDFits(id string) bool {
	return len(id) <= srtStreamIDBytes
}

// srtStreamIDBytes is the cap every SRT implementation puts on a stream id.
const srtStreamIDBytes = 512
