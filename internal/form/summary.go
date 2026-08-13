package form

import (
	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
)

// summarize is what the whole form settled on.
//
// Two things a shell shows without deriving either: the command these settings would run,
// and the prediction behind the diagnostics.
// The estimate is the one it was handed rather than a fresh computation - a second computation is a
// second answer, and a summary that disagreed with the diagnostic beside it would be the fork this
// package exists to remove.
//
// What is no longer here is the shorthand.
// This function used to compose a headline - "cq 21 - 1080p60" - and one line per group,
// out of the same values the fields already carry.
// Both were screen copy: they pick a separator, an abbreviation and a length,
// all of which belong where the strip they sit in is laid out, and a surface that can name a codec
// can compose its own at its own width.
// The values are on Form.settings and the vocabulary is the surface's, so nothing was lost in the
// move but a second voice (api/proto/screenshare/v1/form.proto).
func summarize(_ Deps, s settings.Settings, est *screensharev1.Estimate) *screensharev1.Summary {
	command, reason := formCommand(s)

	sum := &screensharev1.Summary{
		Command:      command,
		CommandError: reason,
		Estimate:     est,
	}

	assert.Assert((sum.GetCommand() == "") != (sum.GetCommandError() == ""),
		"a summary carries a command or the reason there is none", sum.GetCommand(), sum.GetCommandError())
	return sum
}

// formCommand renders the pipeline these settings would run, and the reason none could be rendered.
//
// It is the one place either is asked for, so the command the summary displays and the error the
// diagnostics rank cannot be two different answers about one draft.
// publish.Command renders a string and starts nothing: the ffmpeg engine builds an argument list,
// the GStreamer engine describes its capture chain rather than opening it,
// and neither spawns a process or acquires a portal session.
// It does read this machine - the X display, the monitor list, the render nodes - because a command
// the button beside it would refuse is worse than no command, and those reads are what refuse it.
//
// The refusal crosses as prose, and is one of the three places on this contract that does.
// It is an operational failure rather than a fact about the domain: the same text crosses as a gRPC
// status the moment the publish is attempted, it is shown verbatim, and nothing matches against it
// (docs/ipc-api.md, "Errors").
func formCommand(s settings.Settings) (command string, reason string) {
	cmd, err := publish.Command(s)
	if err != nil {
		return "", err.Error()
	}
	return cmd, ""
}
