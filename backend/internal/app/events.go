package app

import (
	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// Announcing a change is one act, and this file is where it happens.
//
// A change reaches the shells as the contract's own message on the broker, whole and never as a
// delta (docs/ipc-api.md, "Events").
// The shell that acted and the shell that did not are told the same thing, and a duplicate costs a
// reader nothing.

func (a *App) emit(event *screensharev1.Event) {
	assert.IsNotNil(event, "an announced change is a contract event")
	assert.IsNotNil(a.events, "an app announces its changes on a broker")

	a.events.Publish(event)
}

// PublishState is what the app reports about the publish in force.
// The same shape goes out on every change, whoever made it, and answers a shell that has just
// connected, so no second shape exists for the query.
//
// The contract carries this state as wire.PublishSnapshot, and publishSnapshot in control.go is the
// one conversion between them.
type PublishState struct {
	Publishing bool `json:"publishing"`
	// What the running pipeline was built from, null while nothing publishes.
	// The form reverts to these, so they describe the stream the viewers are watching and not what the
	// form shows.
	Settings *settings.Settings `json:"settings"`
	// The settings the app holds build a different pipeline than the running one, so the stream
	// carries values the form no longer shows.
	Pending bool `json:"pending"`
	// The pipeline died on its own and a backoff is running before the next launch.
	// Publishing stays true across that wait, so this is what tells a stream carrying frames from one
	// between attempts.
	Retrying bool `json:"retrying"`
	// Attempt is which relaunch is pending, counting from 1, and Budget how many the app spends before
	// it gives up.
	// Both are zero while nothing retries.
	Attempt int `json:"attempt"`
	Budget  int `json:"budget"`
	// Cause is this app's statement about what ended the pipeline the pending relaunch follows, and
	// Message that pipeline's own last words.
	// On the state and not on the exit event alone, so a shell that mounts mid-backoff reads why as well
	// as one that was listening when the pipeline died.
	// Both are empty while nothing retries.
	Cause   *screensharev1.Text `json:"cause"`
	Message string              `json:"message"`
	// What the local preview of this stream turned out to be, null while nothing publishes and while a
	// publish runs without one.
	// Here rather than beside the running decodes, because the pipeline behind it belongs to the
	// publish and nothing else keys it (preview.go).
	Preview *wire.PreviewSnapshot `json:"preview"`
}
