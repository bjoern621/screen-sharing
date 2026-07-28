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
const legTip = "Protocol streams are received over, the watch leg (relay to viewer). " +
	"It is independent of how a stream was published, since the relay re-serves each stream on every listener that carries its format. " +
	"Changing it is a setting of the app's: it reconnects every tile and is what the next launch opens on."

// legView is the watch-leg control a row holds: the button that opens the
// popover, and inside it the transports the app offers for that stream and the
// knobs of whichever one the dropdown shows. The app declares both, so this side
// renders a control per entry and knows what none of them mean.
//
// Two directions, one function each. draw is the only path from the model into
// these widgets, write the only one out of them, and it puts a change to the app
// rather than to a widget. Ending an edit any other way than Apply, the popover
// closing among it, is another draw against the entry that still holds, so a
// discarded edit and a refused change both leave the controls on the values in
// force.
//
// The knobs follow the dropdown rather than the leg in force, which is what lets
// picking another transport swap them at once: the roster declares a knob set per
// offered leg, so the controls of the leg being moved to are already here when it
// is picked. What Apply asks for is the whole leg the popover stands on, the
// transport and the values of the knobs shown with it.
//
// The controls are rebuilt only when a push changes their shape, so a value
// arriving while the popover is open lands in the widget the pointer is on
// instead of replacing it. A change leaves on Apply rather than per keystroke,
// because it reconnects the stream and a half-typed latency is not a leg anybody
// asked for.
type legView struct {
	button  *gtk.MenuButton
	popover *gtk.Popover
	// head holds the transport dropdown and knobs the controls of the leg that
	// dropdown shows. They are separate boxes because the two are rebuilt on
	// different events: the offered legs change with the roster, the knobs with
	// every pick.
	head  *gtk.Box
	knobs *gtk.Box
	// stream is the last drawn roster entry: what the controls are written from
	// and what a discarded change returns to.
	stream roster.Stream
	legs   *gtk.DropDown
	// offered is the transport list legs holds. A push that leaves it alone keeps
	// the dropdown and the selection on it.
	offered []string
	// shown is the leg the knobs on screen belong to, and keys the option keys
	// they were built for. It follows the dropdown, so it is the leg in force only
	// until someone picks another one.
	shown  string
	keys   []string
	fields []legField
	// syncing suppresses the dropdown's own signal while a draw writes the model's
	// leg into it, so a redraw cannot read as a viewer picking one.
	syncing bool
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
		head:    gtk.NewBox(gtk.OrientationVertical, legSpacing),
		knobs:   gtk.NewBox(gtk.OrientationVertical, legSpacing),
		ask:     ask,
	}

	apply := gtk.NewButtonWithLabel("Apply")
	apply.AddCSSClass("suggested-action")
	apply.SetTooltipText("Reconnect on the chosen leg and keep it as the app's watch setting.")
	apply.ConnectClicked(v.write)

	body := gtk.NewBox(gtk.OrientationVertical, legSpacing)
	body.SetMarginTop(legMargin)
	body.SetMarginBottom(legMargin)
	body.SetMarginStart(legMargin)
	body.SetMarginEnd(legMargin)
	body.Append(v.head)
	body.Append(v.knobs)
	body.Append(apply)

	v.popover.SetChild(body)
	// Abandoning an edit is drawing the entry that holds. A popover closes only after
	// it opened, and it opens on a button a draw made visible, so the entry is there.
	v.popover.ConnectClosed(func() { v.draw(v.stream) })

	v.button.AddCSSClass("flat")
	v.button.SetChild(icons.Image("adjustments-horizontal", legIconSize, theme.Muted))
	v.button.SetTooltipText("Watch-leg settings: the protocol streams are received over, and the knobs that protocol offers.")
	v.button.SetPopover(v.popover)
	return v
}

// Widget is the button the row holds.
func (v *legView) Widget() gtk.Widgetter { return v.button }

// draw puts one roster entry on the control. A stream on a leg with nothing to
// choose and nothing to tune hides the button: the app offers it no watch
// options, so there is nothing behind it.
//
// It sets every control from the entry on every pass, the dropdown back to the
// leg in force among them, so a second draw on an unchanged entry rebuilds
// nothing and changes nothing, and a push arriving mid-edit returns the popover
// to what the app holds.
func (v *legView) draw(st roster.Stream) {
	v.stream = st
	v.button.SetVisible(len(st.Transports) > 1 || len(st.Options[st.Transport]) > 0)

	if offered := legTransports(st); !slices.Equal(offered, v.offered) {
		v.rebuildLegs(offered)
	}

	i := slices.Index(v.offered, st.Transport)
	assert.Assert(i >= 0, "the dropdown offers the leg the stream is on", st.Transport)
	// The write below is the model reaching the widget, not a viewer picking a leg,
	// so the handler that swaps the knobs stays out of it. showKnobs does that job
	// here, against the leg in force rather than against the selection.
	v.syncing = true
	v.legs.SetSelected(uint(i))
	v.syncing = false

	v.showKnobs(st.Transport)
}

// rebuildLegs replaces the transport dropdown with one over the legs the current
// entry offers. Only draw calls it, and only where that list moved: the
// selection is written afterwards, so the dropdown leaves here without one.
//
// The previous dropdown leaves with the head box and takes its notify handler
// with it, nothing outside the box having held either.
func (v *legView) rebuildLegs(offered []string) {
	clearBox(v.head)
	assert.Assert(len(offered) > 0, "a stream offers a leg to receive it over", v.stream.Name)

	v.offered = offered
	v.legs = gtk.NewDropDownFromStrings(offered)
	v.legs.NotifyProperty("selected", func() {
		if v.syncing {
			return
		}
		v.showKnobs(v.selected())
	})
	v.head.Append(legRow("Watch over", legTip, v.legs))
}

// showKnobs puts the knobs of one leg on the popover: the controls the entry
// declares for it, at the values the entry carries.
//
// The controls are rebuilt only where the leg or its knob set changed, so a draw
// on an unchanged entry writes values into the widgets already there. Moving to
// another leg and back rebuilds twice and lands on the declared values both
// times, which is what makes a leg someone looked at and left cost nothing.
func (v *legView) showKnobs(name string) {
	declared := v.stream.Options[name]
	if v.shown != name || !slices.Equal(optionKeys(declared), v.keys) {
		v.rebuildKnobs(name, declared)
	}

	for _, f := range v.fields {
		value, ok := optionValue(declared, f.key)
		assert.Assert(ok, "a knob answers under a key the entry carries", f.key, name)
		f.write(value)
	}
}

// rebuildKnobs replaces the knob controls with the ones declared for a leg. Only
// showKnobs calls it, and showKnobs fills what it builds: the controls leave here
// empty.
//
// Clear then fill, so nothing accumulates over rebuilds. The fields that closed
// over the previous knobs go with the slice.
func (v *legView) rebuildKnobs(name string, declared []roster.Option) {
	clearBox(v.knobs)

	v.shown = name
	v.keys = optionKeys(declared)
	v.fields = nil
	for _, o := range declared {
		control, field, ok := legControl(o)
		if !ok {
			continue
		}
		v.knobs.Append(legRow(o.Label, o.Tip, control))
		v.fields = append(v.fields, field)
	}
}

// write asks the app for the leg the popover stands on, the whole leg rather than
// the control that moved: the app replaces what it held. The answer is the next
// push, which is what moves these widgets.
//
// The transport and the knobs travel together and belong to each other: the
// controls on screen are the ones the entry declared for the transport the
// dropdown shows, whether or not that is the leg in force. A move therefore
// carries the values the viewer set on the leg being moved to, and the app has no
// message to merge with another.
//
// The controls are read before the popdown, because closing draws the entry that
// still holds back into them and a read after it would carry the leg the change was
// meant to replace.
func (v *legView) write() {
	assert.IsNotNil(v.legs, "a watch-leg change is read off controls a draw built")

	transport := v.selected()
	assert.Assert(transport == v.shown, "the knobs on screen belong to the leg the dropdown shows", transport, v.shown)

	options := v.values()
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
