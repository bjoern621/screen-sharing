package sidebar

import (
	"slices"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/theme"
)

// The watch-leg popover's geometry.
const (
	legIconSize = 14
	legSpacing  = 8
	legMargin   = 12
)

// legTip describes the transport dropdown, the one control the roster does not
// carry a tip for: it is the leg itself rather than one of its knobs.
const legTip = "Protocol this stream is received over, the watch leg (relay to viewer). " +
	"It is independent of how the stream was published, since the relay re-serves each stream on every listener that carries its format. " +
	"Changing it reconnects this stream and leaves the other tiles alone."

// legView is one row's watch-leg control: the button that opens the popover,
// and inside it the transports the app offers for that stream and the knobs of
// the one it is on. The app declares both, so this side renders a control per
// entry and knows what none of them mean.
//
// Two directions, one function each. draw is the only path from the model into
// these widgets, write the only one out of them, and it puts a change to the app
// rather than to a widget. Ending an edit any other way than Apply, the popover
// closing among it, is another draw against the entry that still holds, so a
// discarded edit and a refused change both leave the controls on the values in
// force.
//
// The controls follow the roster and are rebuilt only when a push changes their
// shape, so a value arriving while the popover is open lands in the widget the
// pointer is on instead of replacing it. A change leaves on Apply rather than
// per keystroke, because it reconnects the stream and a half-typed latency is
// not a leg anybody asked for.
type legView struct {
	button  *gtk.MenuButton
	popover *gtk.Popover
	body    *gtk.Box
	// stream is the last drawn roster entry: what the controls are written from
	// and what a discarded change returns to.
	stream roster.Stream
	legs   *gtk.DropDown
	// offered is the transport list legs holds and keys the option keys fields
	// was built for. A push that changes neither only writes values.
	offered []string
	keys    []string
	fields  []legField
	ask     func(transport string, options map[string]string)
}

// newLegView builds the control. ask carries a change to the app, which answers
// with the roster push that puts it into force.
func newLegView(icons *theme.Icons, ask func(transport string, options map[string]string)) *legView {
	assert.IsNotNil(icons, "the watch-leg button's icon belongs to a set")
	assert.IsNotNil(ask, "a watch-leg change goes somewhere")

	v := &legView{
		button:  gtk.NewMenuButton(),
		popover: gtk.NewPopover(),
		body:    gtk.NewBox(gtk.OrientationVertical, legSpacing),
		ask:     ask,
	}
	v.body.SetMarginTop(legMargin)
	v.body.SetMarginBottom(legMargin)
	v.body.SetMarginStart(legMargin)
	v.body.SetMarginEnd(legMargin)

	v.popover.SetChild(v.body)
	// Abandoning an edit is drawing the entry that holds. A popover closes only after
	// it opened, and it opens on a button a draw made visible, so the entry is there.
	v.popover.ConnectClosed(func() { v.draw(v.stream) })

	v.button.AddCSSClass("flat")
	v.button.SetChild(icons.Image("adjustments-horizontal", legIconSize, theme.Muted))
	v.button.SetTooltipText("Watch-leg settings for this stream: the protocol it is received over, and the knobs that protocol offers.")
	v.button.SetPopover(v.popover)
	return v
}

// Widget is the button the row holds.
func (v *legView) Widget() gtk.Widgetter { return v.button }

// draw puts one roster entry on the control. A stream on a leg with nothing to
// choose and nothing to tune hides the button: the app offers it no watch
// options, so there is nothing behind it.
//
// It sets every control from the entry on every pass, so a second draw on an
// unchanged entry rebuilds nothing and changes nothing.
func (v *legView) draw(st roster.Stream) {
	v.stream = st
	v.button.SetVisible(len(st.Transports) > 1 || len(st.Options) > 0)

	if !slices.Equal(legTransports(st), v.offered) || !slices.Equal(optionKeys(st.Options), v.keys) {
		v.rebuild()
	}

	i := slices.Index(v.offered, st.Transport)
	assert.Assert(i >= 0, "the dropdown offers the leg the stream is on", st.Transport)
	v.legs.SetSelected(uint(i))

	for _, f := range v.fields {
		value, ok := optionValue(st.Options, f.key)
		assert.Assert(ok, "a knob answers under a key the entry carries", f.key)
		f.write(value)
	}
}

// rebuild replaces the popover's controls with the ones the current entry
// describes, which is what a stream moved to another transport needs: the knobs
// belong to the leg, not to the stream. Only draw calls it, and draw fills what it
// builds: the controls leave here empty.
//
// Clear then fill, so nothing accumulates over rebuilds. The previous Apply button
// leaves with the body's other children and takes its clicked handler with it,
// nothing outside the body having held it, and the fields that closed over the
// previous knobs go with the slice.
func (v *legView) rebuild() {
	for child := v.body.FirstChild(); child != nil; child = v.body.FirstChild() {
		v.body.Remove(child)
	}
	assert.Assert(v.body.FirstChild() == nil, "a rebuild starts on an empty popover")

	v.offered = legTransports(v.stream)
	v.keys = optionKeys(v.stream.Options)
	v.fields = nil

	v.legs = gtk.NewDropDownFromStrings(v.offered)
	v.body.Append(legRow("Watch over", legTip, v.legs))

	for _, o := range v.stream.Options {
		control, field, ok := legControl(o)
		if !ok {
			continue
		}
		v.body.Append(legRow(o.Label, o.Tip, control))
		v.fields = append(v.fields, field)
	}

	apply := gtk.NewButtonWithLabel("Apply")
	apply.AddCSSClass("suggested-action")
	apply.SetTooltipText("Reconnect this stream on the chosen leg. The other tiles keep playing.")
	apply.ConnectClicked(v.write)
	v.body.Append(apply)
}

// write asks the app for the leg the controls stand on, the whole leg rather than
// the knob that moved: the app replaces what it held for this stream. The answer is
// the next push, which is what moves these widgets.
//
// A change that leaves the transport carries no knobs.
// The ones on screen belong to the transport being left, keyed as it declares them,
// and the transport being moved to declares a set of its own this side has never been sent.
// Stating the old keys under the new leg asks it for knobs it does not have,
// which the app refuses as a whole leg, so the move would be lost along with them.
// The push that answers carries the new leg's knobs at the values the app holds,
// and the draw it triggers builds the controls for them.
//
// The controls are read before the popdown, because closing draws the entry that
// still holds back into them and a read after it would carry the leg the change was
// meant to replace.
func (v *legView) write() {
	assert.IsNotNil(v.legs, "a watch-leg change is read off controls a draw built")

	transport := v.selected()
	options := map[string]string{}
	if transport == v.stream.Transport {
		options = v.values()
	}
	v.popover.Popdown()
	v.ask(transport, options)
}

// selected is the transport the dropdown shows.
func (v *legView) selected() string {
	i := int(v.legs.Selected())
	assert.Assert(i >= 0 && i < len(v.offered), "the dropdown shows one of the offered legs", i, len(v.offered))

	return v.offered[i]
}

// values reads every knob back.
func (v *legView) values() map[string]string {
	out := make(map[string]string, len(v.fields))
	for _, f := range v.fields {
		out[f.key] = f.read()
	}
	return out
}
