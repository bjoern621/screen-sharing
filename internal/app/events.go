package app

import (
	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/settings"
)

// Announcing a change is one act, and this file is where it happens.
//
// Every change reaches the shells as the contract's own message on the broker
// (docs/ipc-api.md, "Events"), whole rather than as a delta, so a shell that acted and
// a shell that did not learn it the same way and a duplicate is harmless.
//
// There used to be a second surface here, a Wails frontend subscribing to runtime
// events under names of its own, and emit told both. It is gone, and with it the table
// pairing a contract kind to a runtime name: one announcement path is what the pairing
// existed to guarantee, and one surface reaches it without a table.

// emit announces one change to every shell.
func (a *App) emit(event *screensharev1.Event) {
	assert.IsNotNil(event, "an announced change is a contract event")
	assert.IsNotNil(a.events, "an app announces its changes on a broker")

	a.events.Publish(event)
}

// PublishState is what the app reports about the publish in force. It goes out on
// every change, whoever made it, and a shell that has just connected reads the same
// shape rather than a second one built for the query.
//
// The control contract's own shape for the same state is wire.PublishSnapshot, and
// control.go carries one to the other.
type PublishState struct {
	Publishing bool `json:"publishing"`
	// Settings are what the running pipeline was built from, null while nothing
	// publishes. The form reverts to them, so what they describe is the stream the
	// viewers are watching rather than what the form currently shows.
	Settings *settings.Settings `json:"settings"`
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
