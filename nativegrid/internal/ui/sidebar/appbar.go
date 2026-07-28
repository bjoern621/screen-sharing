package sidebar

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/theme"
)

// The app bar's geometry. The settings glyph is drawn at the size a row's status
// face is, so the sidebar carries one icon size.
const (
	appIconSize = 16
	appSpacing  = 6
	appMargin   = 6
)

// The two publish faces, the wording the settings form's own button uses.
const (
	publishLabel = "Start publishing"
	stopLabel    = "Stop publishing"
)

// The tooltips of the app bar's controls. They say what pressing does and what
// it leaves alone, which a glyph and a two-word label cannot.
const (
	settingsTip = "Bring the app's window to the front, where the stream settings are. This window keeps playing."
	publishTip  = "Publish this machine's screen to the relay, on the settings the app's form holds. The button beside this one opens that form."
	stopTip     = "Stop publishing this machine's screen. Tiles watching other publishers keep playing."
)

// appBar is the sidebar's foot: the app's settings form, and this machine's own
// publish, the two controls of the app that are reachable without leaving the
// grid.
//
// It is drawn from the app state a push carries and never from the click that
// asked for it, so the button says what holds rather than what was asked for,
// and a refused command leaves its reason on the bar instead of a button that
// quietly springs back.
//
// The whole bar is hidden for a run with no app behind it: the demo window has
// nobody to ask.
type appBar struct {
	root    *gtk.Box
	publish *gtk.Button
	// dot is the live state on the stop face, the design language's small
	// pulsing dot in the button's own foreground.
	dot   *gtk.Box
	label *gtk.Label
	// failure carries the reason the last command was refused, hidden while
	// there is none.
	failure *gtk.Label
	run     func(command string)
	// drawn is the state the bar last drew, and what a click acts on: the button
	// asks for the opposite of the state it shows.
	drawn roster.App
	// present says a draw filled drawn from an app rather than from the zero state
	// a run without one leaves on the hidden bar.
	present bool
}

// newAppBar builds the bar. run carries one command to the app, which answers
// with the push that puts it into force.
func newAppBar(icons *theme.Icons, run func(command string)) *appBar {
	assert.IsNotNil(icons, "the app bar's icons belong to a set")
	assert.IsNotNil(run, "the app bar's buttons send a command somewhere")

	b := &appBar{
		root:    gtk.NewBox(gtk.OrientationVertical, appSpacing),
		publish: gtk.NewButton(),
		dot:     statusDot(true),
		label:   gtk.NewLabel(publishLabel),
		failure: gtk.NewLabel(""),
		run:     run,
	}

	settings := gtk.NewButton()
	settings.AddCSSClass("flat")
	settings.SetChild(icons.Image("settings", appIconSize, theme.Foreground))
	settings.SetTooltipText(settingsTip)
	settings.ConnectClicked(func() { b.run(roster.CommandShowSettings) })

	// The dot rides inside the button beside its label, so the live state sits on
	// the control it belongs to rather than beside it. On a filled button it
	// takes the button's foreground instead of the sidebar accent.
	b.dot.AddCSSClass("on-button")
	face := gtk.NewBox(gtk.OrientationHorizontal, appSpacing)
	face.SetHAlign(gtk.AlignCenter)
	face.Append(b.dot)
	face.Append(b.label)
	b.publish.SetChild(face)
	b.publish.SetHExpand(true)
	b.publish.AddCSSClass("publish-button")
	b.publish.ConnectClicked(b.toggle)

	b.failure.AddCSSClass("app-error")
	b.failure.SetXAlign(0)
	b.failure.SetWrap(true)

	row := gtk.NewBox(gtk.OrientationHorizontal, appSpacing)
	row.Append(settings)
	row.Append(b.publish)

	b.root.Append(row)
	b.root.Append(b.failure)
	b.root.SetMarginTop(appMargin)
	b.root.SetMarginBottom(appMargin)
	b.root.SetMarginStart(appMargin)
	b.root.SetMarginEnd(appMargin)
	return b
}

// Widget is the bar, the sidebar's bottom bar.
func (b *appBar) Widget() gtk.Widgetter { return b.root }

// draw puts the app state on the bar. present is whether there is an app behind
// the window at all; a run with none hides the bar.
//
// The controls are set from app on either branch, which without an app is the zero
// state, so a bar that becomes visible again shows what was drawn into it rather
// than the publish face of an app that has since gone.
func (b *appBar) draw(app roster.App, present bool) {
	b.drawn, b.present = app, present
	b.root.SetVisible(present)

	b.dot.SetVisible(app.Publishing)
	if app.Publishing {
		b.label.SetText(stopLabel)
		b.publish.SetTooltipText(stopTip)
		b.publish.AddCSSClass("stop")
	} else {
		b.label.SetText(publishLabel)
		b.publish.SetTooltipText(publishTip)
		b.publish.RemoveCSSClass("stop")
	}

	b.failure.SetText(app.PublishError)
	b.failure.SetVisible(app.PublishError != "")
}

// toggle asks for the publish state the button does not show. A button drawn
// from a state the app has since left asks for one the app is already in, which
// it refuses and answers with the state that holds.
//
// The click reads the cache and only a draw fills it. The bar stays hidden until a
// draw carries an app, so there is no press to answer before there is a state to
// answer it from.
func (b *appBar) toggle() {
	assert.Assert(b.present, "the publish button acts on an app state a draw put on the bar")

	if b.drawn.Publishing {
		b.run(roster.CommandStopPublish)
		return
	}
	b.run(roster.CommandStartPublish)
}
