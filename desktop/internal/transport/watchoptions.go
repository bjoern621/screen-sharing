package transport

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"bjoernblessin.de/screenshare/internal/settings"
)

// The kinds a WatchOption takes, which is what a UI switches on to pick a widget.
const (
	OptionInt    = "int"
	OptionChoice = "choice"
)

// minWatchLatencyMs is the floor of every latency knob. settings.Load reads a
// non-positive latency as unset and replaces it with the default, so a zero
// would not survive the settings it is written into.
const minWatchLatencyMs = 1

// WatchOption is one knob of a transport's watch leg: the key a change names it
// by, how to present it, and its value in the settings it was read from.
//
// Values travel as text so one shape carries every kind, and the transport that
// declares the key is the only place that parses it. A viewer offering the knobs
// therefore names no transport and holds no table of its own.
type WatchOption struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Tip     string   `json:"tip"`
	Kind    string   `json:"kind"`
	Value   string   `json:"value"`
	Min     int      `json:"min,omitempty"`
	Choices []string `json:"choices,omitempty"`
}

// WatchTunable is a transport whose watch leg has knobs a viewer can change per
// stream, declared beside the code that reads them.
type WatchTunable interface {
	// WatchOptions is the knob set carrying the values s holds.
	WatchOptions(s settings.Stream) []WatchOption
	// SetWatchOption writes one knob into s. A key the transport does not
	// declare and a value it cannot use are both errors, and leave s untouched.
	SetWatchOption(s *settings.Stream, key, value string) error
}

// WatchOptions is the named transport's watch-leg knobs, empty for a transport
// that declares none and for a name the registry does not know.
func WatchOptions(name string, s settings.Stream) []WatchOption {
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

// SetWatchOption writes one of the named transport's watch-leg knobs into s. An
// unknown transport, one with no knobs, an undeclared key and an unusable value
// are all errors: a rejected change leaves s as it was instead of taking a
// value nobody asked for.
func SetWatchOption(name string, s *settings.Stream, key, value string) error {
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

// watchKnob is one declared knob: what a viewer shows, where the value is read,
// and where an accepted one is written. A transport lists its knobs once and
// serves both WatchTunable methods off that list.
type watchKnob struct {
	option WatchOption
	read   func(s *settings.Stream) string
	write  func(s *settings.Stream, value string) error
}

// intKnob declares a whole-number knob with a floor.
func intKnob(key, label, tip string, min int, field func(*settings.Stream) *int) watchKnob {
	return watchKnob{
		option: WatchOption{Key: key, Label: label, Tip: tip, Kind: OptionInt, Min: min},
		read:   func(s *settings.Stream) string { return strconv.Itoa(*field(s)) },
		write: func(s *settings.Stream, value string) error {
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

// choiceKnob declares a knob taking one of a fixed set of names.
func choiceKnob(key, label, tip string, choices []string, field func(*settings.Stream) *string) watchKnob {
	return watchKnob{
		option: WatchOption{Key: key, Label: label, Tip: tip, Kind: OptionChoice, Choices: choices},
		read:   func(s *settings.Stream) string { return *field(s) },
		write: func(s *settings.Stream, value string) error {
			if !slices.Contains(choices, value) {
				return fmt.Errorf("%s: %q is not one of %s", key, value, strings.Join(choices, ", "))
			}
			*field(s) = value
			return nil
		},
	}
}

// knobOptions reads a knob list off s. s is a copy, so the pointers the knobs
// take point into it and nothing here reaches the caller's settings.
func knobOptions(knobs []watchKnob, s settings.Stream) []WatchOption {
	out := make([]WatchOption, 0, len(knobs))
	for _, k := range knobs {
		o := k.option
		o.Value = k.read(&s)
		out = append(out, o)
	}
	return out
}

// knobSet writes one knob of a list, naming the transport in the error a key
// outside the list produces.
func knobSet(transport string, knobs []watchKnob, s *settings.Stream, key, value string) error {
	for _, k := range knobs {
		if k.option.Key == key {
			return k.write(s, value)
		}
	}
	return fmt.Errorf("transport %q has no watch option %q", transport, key)
}
