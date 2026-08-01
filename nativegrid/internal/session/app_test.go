package session

import (
	"testing"

	"bjoernblessin.de/screenshare-nativegrid/internal/layout"
	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
)

// commands collects what the model asked the app to run, in place of the pipe
// the window writes on.
type commands struct{ sent []roster.Command }

func (c *commands) run(cmd roster.Command) { c.sent = append(c.sent, cmd) }

// newAppSession is a model with an app behind it whose commands are collected.
func newAppSession(t *testing.T, app *roster.App) (*Session, *commands) {
	t.Helper()

	ran := &commands{}
	backend := &stubBackend{}
	s := New(roster.Config{App: app}, backend.factory, backend.chains, &layout.Memory{},
		roster.Discard, roster.DiscardReport, ran.run, func(f func()) { f() })
	testTimings(s)
	return s, ran
}

// The app state opens with the config and travels no other way: a window that
// kept a copy of its own would draw a publish state nobody set.
func TestAppStateComesFromTheConfig(t *testing.T) {
	s, _ := newAppSession(t, &roster.App{Publishing: true})

	app, ok := s.App()
	if !ok {
		t.Fatal("the config carried app state and the model reports none")
	}
	if !app.Publishing {
		t.Error("the model reports an idle app, want a publishing one")
	}
}

// A config with no app state is the demo run, where the sidebar draws no control
// that could send a command.
func TestNoAppStateIsReported(t *testing.T) {
	s, _ := newAppSession(t, nil)

	if _, ok := s.App(); ok {
		t.Error("the model reports an app for a run that has none")
	}
}

// A push replaces the app state and tells the views, which is the only way the
// publish control changes what it says.
func TestSetAppNotifies(t *testing.T) {
	s, _ := newAppSession(t, &roster.App{})

	kinds := 0
	s.Observe(ObserverFunc(func(c Change) {
		if c.Kind == AppChanged {
			kinds++
		}
	}))
	s.SetApp(&roster.App{Publishing: true, PublishError: "no encoder"})

	if kinds != 1 {
		t.Errorf("%d app changes reported, want 1", kinds)
	}
	app, _ := s.App()
	if !app.Publishing || app.PublishError != "no encoder" {
		t.Errorf("app = %+v, want the pushed state", app)
	}
}

// A push that carries no app state says nothing about the app. Reading it as an
// app that went away would take the publish control off a window whose app is
// still on the other end of the pipe.
func TestSetAppKeepsTheStateAPushOmits(t *testing.T) {
	s, _ := newAppSession(t, &roster.App{Publishing: true})

	changes := 0
	s.Observe(ObserverFunc(func(c Change) {
		if c.Kind == AppChanged {
			changes++
		}
	}))
	s.SetApp(nil)

	app, ok := s.App()
	if !ok {
		t.Fatal("a push without app state dropped the app")
	}
	if !app.Publishing {
		t.Error("the model reports an idle app, want the state that still holds")
	}
	if changes != 0 {
		t.Errorf("%d app changes reported, want none for a push that says nothing", changes)
	}
}

// The roster is pushed on a poll, so the state in force arrives over and over
// and a view has nothing to redraw for it.
func TestSetAppRepeatingTheStateInForceIsSilent(t *testing.T) {
	s, _ := newAppSession(t, &roster.App{Publishing: true})

	changes := 0
	s.Observe(ObserverFunc(func(c Change) {
		if c.Kind == AppChanged {
			changes++
		}
	}))
	s.SetApp(&roster.App{Publishing: true})

	if changes != 0 {
		t.Errorf("%d app changes reported, want none for the state already in force", changes)
	}
}

// A command goes out as it was named and changes nothing here: what it did comes
// back as the next push.
func TestRunAppCommandSends(t *testing.T) {
	s, ran := newAppSession(t, &roster.App{})

	s.RunAppCommand(roster.CommandStartPublish)

	if len(ran.sent) != 1 || ran.sent[0].Name != roster.CommandStartPublish {
		t.Fatalf("sent %+v, want one %s command", ran.sent, roster.CommandStartPublish)
	}
	if app, _ := s.App(); app.Publishing {
		t.Error("the model turned publishing on by itself, want the push to say so")
	}
}
