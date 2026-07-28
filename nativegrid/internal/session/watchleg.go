package session

import (
	"slices"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
)

// RequestWatchLeg asks the app to receive stream i over a transport, with that
// transport's knobs at the given values, keyed as the roster declared them.
//
// Nothing changes here: the app decides what the leg means, and its answer is
// the roster push that carries the new source fragment, which SetRoster
// restarts a watched stream on. A refused request is answered too, with the
// values that still hold, so the controls that asked follow the model back.
//
// The leg and the keys are the ones the stream declared. The app rejects a leg
// it did not offer and a knob the transport does not have, so a request for
// either is a control that asked for something nothing on the other side can
// answer.
func (s *Session) RequestWatchLeg(i int, transport string, options map[string]string) {
	e := s.at(i)
	// The leg the stream is on counts as offered whether the app named it again
	// or not: a stream whose format the window's transport cannot carry is on a
	// leg outside its own list, and the sidebar offers that one beside the rest.
	assert.Assert(transport == e.stream.Transport || slices.Contains(e.stream.Transports, transport),
		"a watch leg is the one a stream is on or one it offers", transport, e.stream.Transports)
	for key := range options {
		assert.Assert(slices.ContainsFunc(e.stream.Options, func(o roster.Option) bool { return o.Key == key }),
			"a watch option is one the stream declares", key, e.stream.Name)
	}

	logger.Infof("asking to watch %q over %s", e.stream.Name, transport)
	s.send(roster.Request{Stream: e.stream.Name, Transport: transport, Options: options})
}
