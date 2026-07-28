package roster

import (
	"io"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// Request is the watch leg the window wants one stream on: the transport and
// every knob that transport declares. It is the whole leg rather than the one
// control that moved, so the app replaces what it held and neither side merges.
//
// It travels as a KindWatchLeg line; the consuming half is
// watch.ParseGridRequest in desktop/watch/gridrequest.go.
type Request struct {
	Stream    string            `json:"stream"`
	Transport string            `json:"transport"`
	Options   map[string]string `json:"options"`
}

// Send delivers one request to the app. The window sends and reads on: the leg
// a stream runs on changes when the app answers with a push, not here.
type Send func(Request)

// Sender writes requests to w, one JSON line each. Messages are the only thing
// the window puts on stdout, which is why logging goes to stderr.
func Sender(w io.Writer) Send {
	assert.IsNotNil(w, "requests need a writer")

	return func(r Request) {
		if err := writeLine(w, legLine{Kind: KindWatchLeg, Request: r}); err != nil {
			logger.Warnf("watch leg request for %q not sent: %v", r.Stream, err)
		}
	}
}

// Discard drops every request, for a run with no app behind it: the demo
// streams carry no watch options, so nothing can ask.
func Discard(Request) {}
