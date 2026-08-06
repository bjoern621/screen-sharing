package app

import "bjoernblessin.de/screenshare/internal/settings"

// exitEvent is the payload of the "publish:exit", "teststream:exit" and
// "nativegrid:exit" events: the (possibly empty) error message and the path of
// the full run log.
type exitEvent struct {
	Message string `json:"message"`
	LogPath string `json:"logPath"`
}

// PublishStateEvent is the payload of the "publish:state" event and the answer
// App.PublishState returns. It goes out on every change, whoever made it, which is
// what keeps a window that did not ask for one from missing it, and a window that has
// just mounted reads the same shape rather than a second one built for the query.
//
// It is exported because it crosses the binding boundary as a return value, unlike the
// events beside it, which cross it as payloads alone.
type PublishStateEvent struct {
	Publishing bool `json:"publishing"`
	// Settings are what the running pipeline was built from, null while nothing
	// publishes. The form reverts to them, so what they describe is the stream the
	// viewers are watching rather than what the form currently shows.
	Settings *settings.Stream `json:"settings"`
	// Pending reports that the settings the app holds build a different pipeline than
	// the running one, so the stream is carrying values the form no longer shows.
	Pending bool `json:"pending"`
	// Retrying reports that the pipeline died on its own and the app is waiting out a
	// backoff before starting it again. Publishing stays true across that wait, so the
	// three together separate a stream carrying frames from one between attempts.
	Retrying bool `json:"retrying"`
	// Attempt is which relaunch the pending one is, counting from one, and Budget how
	// many the app will spend before it gives up. Both are zero while nothing retries.
	Attempt int `json:"attempt"`
	Budget  int `json:"budget"`
}

// watchExitEvent is the payload of the "watch:exit" event. Name and Transport
// together identify which viewer exited, so the UI clears the connecting state
// of the right (stream, transport) rather than every viewer of the stream.
type watchExitEvent struct {
	Name      string `json:"name"`
	Transport string `json:"transport"`
	Message   string `json:"message"`
	LogPath   string `json:"logPath"`
}
