package transport

import (
	"net/url"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/settings"
)

// Where a relay token rides is per protocol, and every difference is MediaMTX's answer, measured:
//
//   - RTSP: query, "?jwt=<token>", except for rtspsrc, which takes the pair rtspCredential builds.
//   - RTMP: query, "?jwt=<token>". Both engines pass a URL query through untouched.
//   - SRT: stream id, "publish:<path>:<user>:<token>". No query, no header.
//   - HLS, WebRTC, MoQ: "Authorization: Bearer <token>", and nothing else. The HTTP servers read
//     no query at all, measured against v1.20.0: the same address with "?jwt=" is answered 401.
//     A browser cannot set a header on an address it is handed, which is what credentialUserinfo
//     covers.
//
// Every form is empty without a token, one coming from the group service beside the relay
// (settings.Relay.GroupService).
// Whether that suffices is the relay's answer, and refusing to build the address would decide
// a policy this side cannot see.

// Redacted is what stands in a log where a secret was.
const Redacted = "<redacted>"

// Redact replaces every secret s carries wherever it appears in text.
//
// Keyed on the value and never on the spelling around it: the token and the passphrase reach
// a child as a query parameter, as an element property and as a field of an SRT stream id, so
// a redaction written per form misses whichever form is added next.
// Both spellings of the same bytes are covered, since a query carries the value percent-encoded.
//
// What a reader wants a run log for survives it: the relay, the path, the element that failed and
// its own wording are all still there, and only the two values that are worth stealing are not.
// The passphrase in particular is the group's and outlives every token, so a log offered to whoever
// is helping is exactly how it leaves.
func Redact(s settings.Settings, text string) string {
	for _, secret := range []string{s.Relay.Token, s.Relay.SrtPassphrase()} {
		if secret == "" {
			continue
		}
		text = strings.ReplaceAll(text, secret, Redacted)
		if escaped := url.QueryEscape(secret); escaped != secret {
			text = strings.ReplaceAll(text, escaped, Redacted)
		}
	}
	return text
}

// CredentialHeader is the token as the HTTP legs take it, ok=false without one.
// Handed out rather than written into a URL, the caller carrying it as a player option or
// an element property (internal/watch, webrtc.go).
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

// credentialUserinfo is the token as the userinfo of a player-page address, "jwt:<token>@", empty
// without one.
//
// The one credential form a browser carries on an address it is handed.
// The relay's HTTP servers read a token off a header and out of no query, and a page opened
// by the desktop is a plain navigation that sets none, so it rides where the browser builds
// the header itself: MediaMTX reads a JWT as the password of a Basic pair under any user, the way
// it reads the SRT stream id.
//
// Without it an authenticated relay answers the page 401, and the dialog the browser then shows
// asks a person to type a JWT.
func credentialUserinfo(s settings.Settings) string {
	token, ok := credentialToken(s)
	if !ok {
		return ""
	}
	return url.UserPassword(browserAuthUser, token).String() + "@"
}

// browserAuthUser fills the user half of that pair, MediaMTX reading the password beside it.
const browserAuthUser = "jwt"

// withCredential splices the userinfo into an origin, "http://relay:8888" becoming
// "http://jwt:<token>@relay:8888", and hands back an origin with no token unchanged.
//
// One splice for every player page, so a leg cannot carry the credential in a shape of its own:
// the pages differ in scheme and port and in nothing else a credential touches.
func withCredential(s settings.Settings, origin string) string {
	assert.Assert(origin != "", "a player page is built on an origin")

	userinfo := credentialUserinfo(s)
	if userinfo == "" {
		return origin
	}
	scheme, host, ok := strings.Cut(origin, "://")
	assert.Assert(ok, "a player page origin names its scheme", origin)
	return scheme + "://" + userinfo + host
}

// httpPageOrigin is where the relay answers a player page on one of its HTTP listeners, credential
// included.
func httpPageOrigin(s settings.Settings, directPort int) string {
	return withCredential(s, s.Relay.HTTPOrigin(directPort))
}

// credentialQuery is the token as a query, joined with the separator the caller's URL needs ("?",
// "&").
// RTSP and RTMP alone: the HTTP legs read no query, and their form is credentialUserinfo.
func credentialQuery(s settings.Settings, separator string) string {
	if s.Relay.Token == "" {
		return ""
	}
	return separator + "jwt=" + url.QueryEscape(s.Relay.Token)
}

// rtspCredential is the token as an RTSP session's user and password, ok=false without one.
//
// rtspsrc alone: it sets up a track at the SDP's control attribute joined onto the session URL, and
// that join keeps neither the query nor the last path segment, so ".../<group>/desk?jwt=<token>"
// is set up as ".../<group>/trackID=0" and the relay answers 401 Unauthorized on a path nothing
// serves.
// A credential the element holds as properties is sent with every request instead, SETUP included.
// ffmpeg's reader and rtspclientsink join the same pair losing neither half, so the query form
// stays on the legs they drive.
//
// MediaMTX reads the token out of the password and never the user beside it, the way it reads
// the SRT stream id.
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
// SRT truncates the id at srtStreamIDBytes rather than refusing it, and token.MaxTokenBytes and
// the stream-name bound are measured against that.
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
//
// A question rather than an assert, what overflows being a stream name and a token, both values
// a deployment supplies rather than anything this code computes.
// Called from ValidatePublishSettings alone, so the bound is stated where a publish is refused
// with a reason: the watch leg builds its id from a path the relay named and has no error to answer
// with, leaving an over-long one to truncate on the wire and reach the relay as a signature error.
func srtStreamIDFits(id string) bool {
	return len(id) <= srtStreamIDBytes
}

// srtStreamIDBytes is the cap every SRT implementation puts on a stream id.
const srtStreamIDBytes = 512
