package session

import (
	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
)

// App is the app state the last push carried, and whether there is an app behind
// this window at all. A demo run has none, and the controls that would act on it
// are not drawn.
func (s *Session) App() (roster.App, bool) {
	if s.app == nil {
		return roster.App{}, false
	}
	return *s.app, true
}

// SetApp applies the app state of a push. It is the answer to every Run: a
// command changes nothing here until the app states what it did.
//
// A push carrying no app state leaves the state in force standing. Whether there
// is an app behind this window is settled at launch by the config it opened on,
// and every push an app writes carries the field (desktop/internal/watch.GridConfig), so
// a push without one is a run that never had an app rather than an app that
// went away. Dropping the controls on it would take the sidebar's publish button
// off a window whose app is still there.
//
// A state that repeats the one in force is not reported: the roster is pushed on
// a poll, and a view has nothing to redraw for a push that says what it already
// shows.
func (s *Session) SetApp(app *roster.App) {
	if app == nil {
		return
	}
	if s.app != nil && *s.app == *app {
		return
	}
	s.app = app
	logger.Debugf("app state applied: %+v", *app)
	s.notify(Change{Kind: AppChanged, Index: noStream})
}

// RunAppCommand asks the app to run one command, named as roster declares it.
// Nothing is recorded here: the app's state is the app's, and this window draws
// the push that follows.
func (s *Session) RunAppCommand(name string) {
	_, ok := s.App()
	assert.Assert(ok, "a command is sent to an app that is there", name)
	assert.Assert(roster.IsCommand(name), "a command is one the roster declares", name)

	logger.Infof("asking the app to %s", name)
	s.run(roster.Command{Name: name})
}
