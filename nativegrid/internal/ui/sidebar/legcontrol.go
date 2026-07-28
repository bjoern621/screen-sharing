package sidebar

import (
	"slices"
	"strconv"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
)

// A millisecond knob needs a ceiling and a step its declaration does not carry: a
// minute of buffering is past any delay a viewer would sit through, and these knobs
// are tuned in 50 ms steps.
const (
	legFieldMax  = 60000
	legFieldStep = 50
)

// legField is one knob's control: the key it answers under, the value it holds,
// and how a value from the app is written into it.
type legField struct {
	key   string
	read  func() string
	write func(value string)
}

// legControl builds the control for one declared knob.
//
// The kinds are an environment input: the app on the other side of the pipe can be
// newer than this window and name a kind this build has never heard of. That is a
// condition to survive rather than a bug in this code, so an unknown kind is warned
// about and its knob dropped, and the rest of the popover stands. An assert here
// would take the window down over an option it only had to leave out.
//
// What an option carries is input on the same terms, so a declaration this build knows
// the kind of but cannot build a control from leaves by the same door.
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
		// A choice naming nothing to choose is malformed the same way an unknown kind is,
		// and it goes out the same door: a dropdown over an empty list has no selection,
		// so the read below would have nothing to answer with.
		if len(o.Choices) == 0 {
			logger.Warnf("watch option %s: a choice offers nothing to choose", o.Key)
			return nil, legField{}, false
		}
		drop := gtk.NewDropDownFromStrings(o.Choices)
		return drop, legField{
			key: o.Key,
			read: func() string {
				i := int(drop.Selected())
				assert.Assert(i >= 0 && i < len(o.Choices), "a dropdown shows one of the choices it was built from", i, len(o.Choices))

				return o.Choices[i]
			},
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

// clearBox empties a container the popover refills, so nothing accumulates over
// rebuilds and every widget the removed children held goes with them.
func clearBox(box *gtk.Box) {
	assert.IsNotNil(box, "a cleared container exists")

	for child := box.FirstChild(); child != nil; child = box.FirstChild() {
		box.Remove(child)
	}
	assert.Assert(box.FirstChild() == nil, "a cleared container holds nothing")
}

// legRow is one labelled control, the shape every line of the popover takes.
func legRow(label, tip string, control gtk.Widgetter) gtk.Widgetter {
	assert.IsNotNil(control, "a labelled popover line holds a control", label)

	l := gtk.NewLabel(label)
	l.SetXAlign(0)
	l.SetHExpand(true)

	box := gtk.NewBox(gtk.OrientationHorizontal, legSpacing)
	box.Append(l)
	box.Append(control)
	box.SetTooltipText(tip)
	return box
}

// legTransports is what the dropdown offers: the transports the app named for
// the stream, plus the one it is on when that is not among them. A stream whose
// format the window's transport cannot carry is exactly that case, and a
// dropdown that cannot show the current leg would read as a leg nobody set.
func legTransports(st roster.Stream) []string {
	if slices.Contains(st.Transports, st.Transport) {
		return st.Transports
	}
	// The selection a draw makes is read back from this list, so the leg in force
	// being in it is what that read stands on.
	offered := append([]string{st.Transport}, st.Transports...)
	assert.Assert(slices.Contains(offered, st.Transport), "the offered legs hold the one the stream is on", st.Transport)
	return offered
}

// optionKeys is the shape of an option set, which is what decides whether the
// controls are rebuilt or only written.
func optionKeys(options []roster.Option) []string {
	keys := make([]string, 0, len(options))
	for _, o := range options {
		keys = append(keys, o.Key)
	}
	assert.Assert(len(keys) == len(options), "a key per option", len(keys), len(options))
	return keys
}

// optionValue is the value an entry carries under one key.
func optionValue(options []roster.Option, key string) (string, bool) {
	for _, o := range options {
		if o.Key == key {
			return o.Value, true
		}
	}
	return "", false
}
