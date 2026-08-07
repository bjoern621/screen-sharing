package app

import (
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/events"
	"bjoernblessin.de/screenshare/internal/settings"
)

// Announcing a change is one act, and this file is where it happens.
//
// The backend has two surfaces in front of it and they learn the same things: the
// Wails frontend subscribes to runtime events, and every control shell receives the
// contract's own messages off the broker (docs/ipc-api.md, "Events"). Both are told
// through emit, so a state that reached one and not the other is not a mistake this
// code can make - there is no second place to forget.
//
// The two names for one change are spelled differently on purpose. One is the string
// the frontend subscribes with, the other the oneof field events.proto declares, and
// nothing in either language connects them. eventNames is that connection, stated
// once: a caller hands emit the change in the contract's shape and the table decides
// which runtime event carries it, so no call site names both and none can name a pair
// that does not go together.

// The Wails runtime event names, which are the strings the frontend subscribes with.
// They are constants rather than literals at the emit sites because eventNames pairs
// each of them with a contract kind, and a table of two literals is a table that can
// disagree with itself.
const (
	eventPublishState   = "publish:state"
	eventPublishStats   = "publish:stats"
	eventPublishExit    = "publish:exit"
	eventWatchExit      = "watch:exit"
	eventGridState      = "nativegrid:watching"
	eventGridExit       = "nativegrid:exit"
	eventTestStreamExit = "teststream:exit"
	eventSettingsChged  = "settings:changed"
	eventShowSettings   = "app:show-settings"
)

// eventNames maps each contract event kind to the runtime event the frontend learns
// the same change under, empty for a kind that surface has no subscription for.
//
// Four rows are empty, and they are all the same fact: this frontend is obsolete and
// polls for what a control shell is pushed. It reads the relay snapshot through Live()
// and the viewer and test-stream counts through their own getters on its own interval,
// and it has no reader for the catalog at all. A control shell is pushed each of them,
// because the contract's rule is that a shell that acted still waits for the event and
// nothing here may leave an effect unannounced.
//
// Every kind has a row, including the empty ones. A kind with none would be a change
// the broker carries and this table cannot describe, and emit asserts the lookup so
// the kind added without a decision fails at the first announcement instead of
// reaching one surface silently.
var eventNames = map[screensharev1.EventKind]string{
	screensharev1.EventKind_EVENT_KIND_PUBLISH_STATE:     eventPublishState,
	screensharev1.EventKind_EVENT_KIND_PUBLISH_STATS:     eventPublishStats,
	screensharev1.EventKind_EVENT_KIND_PUBLISH_EXIT:      eventPublishExit,
	screensharev1.EventKind_EVENT_KIND_RELAY_STATUS:      "",
	screensharev1.EventKind_EVENT_KIND_VIEWER_STATE:      "",
	screensharev1.EventKind_EVENT_KIND_VIEWER_EXIT:       eventWatchExit,
	screensharev1.EventKind_EVENT_KIND_TEST_STREAM_STATE: "",
	screensharev1.EventKind_EVENT_KIND_TEST_STREAM_EXIT:  eventTestStreamExit,
	screensharev1.EventKind_EVENT_KIND_CATALOG:           "",
	screensharev1.EventKind_EVENT_KIND_SETTINGS_CHANGED:  eventSettingsChged,
	screensharev1.EventKind_EVENT_KIND_SHOW_SETTINGS:     eventShowSettings,
}

// The table is checked against the broker's own list at load time, because the
// failure it guards against is otherwise invisible until the kind is first announced:
// a kind the broker carries and this table has no row for reaches the shells and
// never the frontend, and the frontend's silence looks like a backend with nothing
// happening.
func init() {
	for _, kind := range events.Kinds {
		_, decided := eventNames[kind]
		assert.Assert(decided, "every announced kind has a runtime name decided for it", kind)
	}
	assert.Assert(len(eventNames) == len(events.Kinds), "a row per kind the broker carries", len(eventNames), len(events.Kinds))
}

// emit announces one change on both surfaces.
//
// event is the change in the contract's shape and decides which runtime event carries
// it; payload is what that runtime event carries, and is passed through variadically
// so the two changes that go out with no data still go out with none. A kind the
// frontend has no subscription for is announced to the shells alone.
//
// The order is the frontend first. Nothing depends on it - both are non-blocking, and
// every event carries a whole state, so a subscriber that receives them the other way
// round is not stale for it - but the frontend is the surface a user is looking at.
func (a *App) emit(event *screensharev1.Event, payload ...any) {
	assert.IsNotNil(event, "an announced change is a contract event")

	kind := events.KindOf(event)
	name, decided := eventNames[kind]
	assert.Assert(decided, "an announced change has a runtime name decided for its kind", kind)

	if name != "" {
		runtime.EventsEmit(a.ctx, name, payload...)
	}

	assert.IsNotNil(a.events, "an app announces its changes on a broker")
	a.events.Publish(event)
}

// emitLocal announces a change to this frontend alone, for the changes the control
// contract does not describe.
//
// There is one producer: the GTK4 grid window, whose state used to cross the contract
// and no longer does. How a viewer arranges what it receives is the shell's, and the
// window is obsolete besides, so the contract carries neither its tiles nor its exit -
// while this frontend, which is obsolete with it, still draws both.
//
// Nothing reaches the broker from here on purpose. A control shell that received an
// event for a window it cannot open, cannot close and cannot read the state of would
// be holding a state with no method behind it, which is worse than not being told.
func (a *App) emitLocal(name string, payload ...any) {
	assert.Assert(name != "", "a frontend-only change is announced under a name")

	runtime.EventsEmit(a.ctx, name, payload...)
}

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
// events beside it, which cross it as payloads alone. The control contract's own shape
// for the same state is wire.PublishSnapshot, and control.go carries one to the other.
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
