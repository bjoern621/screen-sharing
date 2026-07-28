package sidebar

import (
	"slices"
	"strconv"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/theme"
)

// The watch-leg popover's geometry. A millisecond knob needs a ceiling and a
// step its declaration does not carry: a minute of buffering is past any delay
// a viewer would sit through, and these knobs are tuned in 50 ms steps.
const (
	legIconSize  = 14
	legSpacing   = 8
	legMargin    = 12
	legFieldMax  = 60000
	legFieldStep = 50
)

// legTip describes the transport dropdown, the one control the roster does not
// carry a tip for: it is the leg itself rather than one of its knobs.
const legTip = "Protocol this stream is received over, the watch leg (relay to viewer). " +
	"It is independent of how the stream was published, since the relay re-serves each stream on every listener that carries its format. " +
	"Changing it reconnects this stream and leaves the other tiles alone."

// legField is one knob's control: the key it answers under, the value it holds,
// and how a value from the app is written into it.
type legField struct {
	key   string
	read  func() string
	write func(value string)
}

// legView is one row's watch-leg control: the button that opens the popover,
// and inside it the transports the app offers for that stream and the knobs of
// the one it is on. The app declares both, so this side renders a control per
// entry and knows what none of them mean.
//
// The controls follow the roster and are rebuilt only when a push changes their
// shape, so a value arriving while the popover is open lands in the widget the
// pointer is on instead of replacing it. A change leaves on Apply rather than
// per keystroke, because it reconnects the stream and a half-typed latency is
// not a leg anybody asked for. Closing the popover any other way puts the
// model's values back.
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
	v.popover.ConnectClosed(v.write)

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
func (v *legView) draw(st roster.Stream) {
	v.stream = st
	v.button.SetVisible(len(st.Transports) > 1 || len(st.Options) > 0)

	if !slices.Equal(legTransports(st), v.offered) || !slices.Equal(optionKeys(st.Options), v.keys) {
		v.rebuild()
	}
	v.write()
}

// rebuild replaces the popover's controls with the ones the current entry
// describes, which is what a stream moved to another transport needs: the knobs
// belong to the leg, not to the stream.
func (v *legView) rebuild() {
	for child := v.body.FirstChild(); child != nil; child = v.body.FirstChild() {
		v.body.Remove(child)
	}
	v.offered = legTransports(v.stream)
	v.keys = optionKeys(v.stream.Options)
	v.fields = nil

	v.legs = gtk.NewDropDownFromStrings(v.offered)
	v.body.Append(legRow("watch over", legTip, v.legs))

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
	apply.ConnectClicked(func() {
		v.popover.Popdown()
		v.ask(v.selected(), v.values())
	})
	v.body.Append(apply)
}

// write puts the entry's values into the controls. It runs on every draw and
// when the popover closes, so a change the app refused is visible as the values
// that still hold rather than as the ones that were asked for.
func (v *legView) write() {
	i := slices.Index(v.offered, v.stream.Transport)
	assert.Assert(i >= 0, "the dropdown offers the leg the stream is on", v.stream.Transport)
	v.legs.SetSelected(uint(i))

	for _, f := range v.fields {
		for _, o := range v.stream.Options {
			if o.Key == f.key {
				f.write(o.Value)
				break
			}
		}
	}
}

// selected is the transport the dropdown shows.
func (v *legView) selected() string {
	i := int(v.legs.Selected())
	assert.Assert(i >= 0 && i < len(v.offered), "the dropdown shows one of the offered legs", i, len(v.offered))

	return v.offered[i]
}

// values reads every knob back, the whole leg rather than the control that
// moved: the app replaces what it held for this stream.
func (v *legView) values() map[string]string {
	out := make(map[string]string, len(v.fields))
	for _, f := range v.fields {
		out[f.key] = f.read()
	}
	return out
}

// legTransports is what the dropdown offers: the transports the app named for
// the stream, plus the one it is on when that is not among them. A stream whose
// format the window's transport cannot carry is exactly that case, and a
// dropdown that cannot show the current leg would read as a leg nobody set.
func legTransports(st roster.Stream) []string {
	if slices.Contains(st.Transports, st.Transport) {
		return st.Transports
	}
	return append([]string{st.Transport}, st.Transports...)
}

// optionKeys is the shape of an option set, which is what decides whether the
// controls are rebuilt or only written.
func optionKeys(options []roster.Option) []string {
	keys := make([]string, 0, len(options))
	for _, o := range options {
		keys = append(keys, o.Key)
	}
	return keys
}

// legControl builds the control for one declared knob. A kind this build does
// not render is skipped rather than fatal: the app on the other side of the
// pipe can be newer than the window, and one unknown knob is worth less than
// the rest of the popover.
func legControl(o roster.Option) (gtk.Widgetter, legField, bool) {
	switch o.Kind {
	case roster.OptionInt:
		spin := gtk.NewSpinButtonWithRange(float64(o.Min), legFieldMax, legFieldStep)
		return spin, legField{
			key:  o.Key,
			read: func() string { return strconv.Itoa(int(spin.Value())) },
			write: func(value string) {
				n, err := strconv.Atoi(value)
				if err != nil {
					logger.Warnf("watch option %s: %q is not a number", o.Key, value)
					return
				}
				spin.SetValue(float64(n))
			},
		}, true

	case roster.OptionChoice:
		drop := gtk.NewDropDownFromStrings(o.Choices)
		return drop, legField{
			key:  o.Key,
			read: func() string { return o.Choices[drop.Selected()] },
			write: func(value string) {
				i := slices.Index(o.Choices, value)
				if i < 0 {
					logger.Warnf("watch option %s: %q is not one of its choices", o.Key, value)
					return
				}
				drop.SetSelected(uint(i))
			},
		}, true
	}

	logger.Warnf("watch option %s: nothing here renders a %q", o.Key, o.Kind)
	return nil, legField{}, false
}

// legRow is one labelled control, the shape every line of the popover takes.
func legRow(label, tip string, control gtk.Widgetter) gtk.Widgetter {
	l := gtk.NewLabel(label)
	l.SetXAlign(0)
	l.SetHExpand(true)

	box := gtk.NewBox(gtk.OrientationHorizontal, legSpacing)
	box.Append(l)
	box.Append(control)
	box.SetTooltipText(tip)
	return box
}
