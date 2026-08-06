package roster

import (
	"io"
	"slices"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// The commands the window can send. Each names an action and carries no
// arguments: what it acts on is the app's own state, which the window has no
// second copy of.
const (
	// CommandShowSettings brings the app's window to the front, where the
	// settings form is.
	CommandShowSettings = "show-settings"
	// CommandStartPublish publishes this machine's capture on the settings in
	// force in the app.
	CommandStartPublish = "start-publish"
	// CommandStopPublish stops that publish.
	CommandStopPublish = "stop-publish"
)

// Commands is the whole vocabulary the app answers to, which is what a sender
// is held to.
var Commands = []string{CommandShowSettings, CommandStartPublish, CommandStopPublish}

// IsCommand reports whether name is one of the declared commands.
func IsCommand(name string) bool { return slices.Contains(Commands, name) }

// Command is one action the window asks the app to take.
//
// The two publish commands name the state they want rather than a toggle, so a
// button drawn from a push that has since been overtaken cannot flip the state
// the other way.
//
// It travels as a KindCommand line; the consuming half is watch.GridCommand in
// desktop/internal/watch/grid.go.
type Command struct {
	Name string `json:"command"`
}

// Run delivers one command to the app, which is where it runs. The window sends
// and reads on: what a command changed arrives as the next push, like a watch
// leg does.
type Run func(Command)

// Runner writes commands to w, one JSON line each.
func Runner(w io.Writer) Run {
	assert.IsNotNil(w, "commands need a writer")

	return func(c Command) {
		if err := writeLine(w, commandLine{Kind: KindCommand, Command: c}); err != nil {
			logger.Warnf("command %q not sent: %v", c.Name, err)
		}
	}
}

// DiscardCommand drops every command, for a run with no app behind it: the demo
// config carries no app state, so the window draws no control that could send
// one.
func DiscardCommand(Command) {}
