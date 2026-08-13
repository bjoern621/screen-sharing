package transport

import (
	"bjoernblessin.de/go-utils/util/assert"

	"fmt"
	"slices"
	"strconv"
	"strings"

	"bjoernblessin.de/screenshare/internal/settings"
)

// WatchOption.Kind values, what a UI switches on to pick a widget.
const (
	OptionInt    = "int"
	OptionChoice = "choice"
)

// minWatchLatencyMs is the floor under every latency knob.
// settings.Load reads a non-positive latency as unset and puts the default back, so a zero would
// not survive the settings it is written into.
const minWatchLatencyMs = 1

// WatchOption is one knob of a transport's watch leg: the key a change names it by, how to present
// it, and its value in the settings it was read from.
//
// Values travel as text, so one shape carries every kind and the transport declaring the key is the
// only place that parses it.
// A viewer offering the knobs therefore names no transport and keeps no table of its own.
type WatchOption struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Tip     string   `json:"tip"`
	Kind    string   `json:"kind"`
	Value   string   `json:"value"`
	Min     int      `json:"min,omitempty"`
	Choices []string `json:"choices,omitempty"`
}

// WatchTunable is a transport whose watch leg has knobs a viewer changes per stream, declared
// beside the code that reads them.
type WatchTunable interface {
	// WatchOptions is the knob set, carrying the values s holds.
	WatchOptions(s settings.Settings) []WatchOption
	// SetWatchOption writes one knob of this transport's into s.
	// An undeclared key and an unusable value are both errors, and both leave s untouched.
	SetWatchOption(s *settings.Settings, key, value string) error
}

// WatchOptions is the named transport's watch-leg knobs.
// Empty for a transport declaring none, and for a name the registry does not know.
func WatchOptions(name string, s settings.Settings) []WatchOption {
	t, ok := Get(name)
	if !ok {
		return nil
	}
	w, ok := t.(WatchTunable)
	if !ok {
		return nil
	}
	return w.WatchOptions(s)
}

// SetWatchOption writes one watch-leg knob of the named transport into s.
// An unknown transport, one with no knobs, an undeclared key and an unusable value are all errors,
// and a refused change leaves s as it was rather than taking a value nobody asked for.
func SetWatchOption(name string, s *settings.Settings, key, value string) error {
	assert.IsNotNil(s, "a watch option is written into settings", name, key)

	t, ok := Get(name)
	if !ok {
		return fmt.Errorf("unknown transport %q", name)
	}
	w, ok := t.(WatchTunable)
	if !ok {
		return fmt.Errorf("transport %q has no watch options", name)
	}
	return w.SetWatchOption(s, key, value)
}

// watchKnob is one declared knob: what a viewer shows, where the value is read, and where an
// accepted one is written.
// A transport declares its knobs once and serves both WatchTunable methods off that list.
type watchKnob struct {
	option WatchOption
	read   func(s *settings.Settings) string
	write  func(s *settings.Settings, value string) error
}

// intKnob refuses a value below min rather than clamping it, so a viewer is told instead of shown a
// number it did not ask for.
func intKnob(key, label, tip string, min int, field func(*settings.Settings) *int) watchKnob {
	assert.Assert(key != "", "a knob is declared under a key", label)
	assert.IsNotNil(field, "a knob names the settings field it reads and writes", key)

	return watchKnob{
		option: WatchOption{Key: key, Label: label, Tip: tip, Kind: OptionInt, Min: min},
		read:   func(s *settings.Settings) string { return strconv.Itoa(*field(s)) },
		write: func(s *settings.Settings, value string) error {
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("%s: %q is not a number", key, value)
			}
			if n < min {
				return fmt.Errorf("%s: %d is below the minimum of %d", key, n, min)
			}
			*field(s) = n
			return nil
		},
	}
}

// choiceKnob refuses a value outside choices, which is the same list the viewer offers.
func choiceKnob(key, label, tip string, choices []string, field func(*settings.Settings) *string) watchKnob {
	assert.Assert(key != "", "a knob is declared under a key", label)
	assert.Assert(len(choices) > 0, "a choice knob offers something to choose", key)
	assert.IsNotNil(field, "a knob names the settings field it reads and writes", key)

	return watchKnob{
		option: WatchOption{Key: key, Label: label, Tip: tip, Kind: OptionChoice, Choices: choices},
		read:   func(s *settings.Settings) string { return *field(s) },
		write: func(s *settings.Settings, value string) error {
			if !slices.Contains(choices, value) {
				return fmt.Errorf("%s: %q is not one of %s", key, value, strings.Join(choices, ", "))
			}
			*field(s) = value
			return nil
		},
	}
}

// knobOptions renders a knob list against s.
// s is a copy, so the pointers the knobs take point into it and no write reaches the caller's
// settings.
func knobOptions(knobs []watchKnob, s settings.Settings) []WatchOption {
	out := make([]WatchOption, 0, len(knobs))
	for _, k := range knobs {
		o := k.option
		o.Value = k.read(&s)
		out = append(out, o)
	}
	return out
}

// knobSet writes one knob of a list, and names the transport where the key is not on it, a key
// belonging to the transport that declared it.
func knobSet(transport string, knobs []watchKnob, s *settings.Settings, key, value string) error {
	assert.IsNotNil(s, "a knob is written into settings", transport, key)

	for _, k := range knobs {
		if k.option.Key == key {
			return k.write(s, value)
		}
	}
	return fmt.Errorf("transport %q has no watch option %q", transport, key)
}
