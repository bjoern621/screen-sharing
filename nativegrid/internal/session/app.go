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
func (s *Session) SetApp(app *roster.App) {
	s.app = app
	logger.Debugf("app state applied: %+v", app)
	s.notify(Change{Kind: AppChanged, Index: noStream})
}

// RunAppCommand asks the app to run one command, named as roster declares it.
// Nothing is recorded here: the app's state is the app's, and this window draws
// the push that follows.
func (s *Session) RunAppCommand(name string) {
	_, ok := s.App()
	assert.Assert(ok, "a command is sent to an app that is there", name)

	logger.Infof("asking the app to %s", name)
	s.run(roster.Command{Name: name})
}
