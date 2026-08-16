package main

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"

	v1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// A field key is its group and the field name the settings message spells: "publish.encoder".
// An entry of a repeated group carries its index and the field inside it:
// "publish.audio_sources[0].source".
// Writing through protoreflect keeps this probe off a second copy of that mapping, which is the
// table it exists to test.
func writeField(settings *v1.Settings, key string, write func(protoreflect.FieldDescriptor) (protoreflect.Value, bool)) error {
	message, desc, err := locate(settings, key, true)
	if err != nil {
		return err
	}

	value, ok := write(desc)
	if !ok {
		return nil
	}
	message.Set(desc, value)
	return nil
}

// locate walks a field key to the message that holds it and the descriptor it names.
//
// A read never creates the group on the way: an absent message and an empty one are different
// drafts, and a probe comparing two of them would report its own reading as a difference.
func locate(settings *v1.Settings, key string, create bool) (protoreflect.Message, protoreflect.FieldDescriptor, error) {
	group, rest, ok := strings.Cut(key, ".")
	if !ok {
		return nil, nil, fmt.Errorf("field key %q names no group", key)
	}

	root := settings.ProtoReflect()
	groupDesc := root.Descriptor().Fields().ByName(protoreflect.Name(group))
	if groupDesc == nil || groupDesc.Kind() != protoreflect.MessageKind {
		return nil, nil, fmt.Errorf("field key %q names no settings group", key)
	}
	message := root.Get(groupDesc).Message()
	if create {
		message = root.Mutable(groupDesc).Message()
	}

	for {
		name, index, tail, indexed := splitEntry(rest)
		desc := message.Descriptor().Fields().ByName(protoreflect.Name(name))
		if desc == nil {
			return nil, nil, fmt.Errorf("field key %q names no field of %s", key, message.Descriptor().FullName())
		}
		if !indexed {
			return message, desc, nil
		}
		list := message.Get(desc).List()
		if create {
			list = message.Mutable(desc).List()
			// The row past the end is the one a reader fills to add an entry, so writing it appends.
			for list.Len() <= index {
				list.Append(list.NewElement())
			}
		}
		if index >= list.Len() {
			return nil, nil, fmt.Errorf("field key %q names entry %d of a list holding %d", key, index, list.Len())
		}
		message, rest = list.Get(index).Message(), tail
	}
}

// splitEntry reads "audio_sources[0].source" as the list, the entry and what follows it.
func splitEntry(path string) (name string, index int, tail string, indexed bool) {
	head, rest, more := strings.Cut(path, ".")
	open := strings.Index(head, "[")
	if open < 0 || !strings.HasSuffix(head, "]") || !more {
		return head, 0, "", false
	}
	n, err := strconv.Atoi(head[open+1 : len(head)-1])
	if err != nil {
		return head, 0, "", false
	}
	return head[:open], n, rest, true
}

// setOption writes a select's chosen entry, in whatever scalar that field is spelled as.
func setOption(settings *v1.Settings, key, option string) error {
	return writeField(settings, key, func(desc protoreflect.FieldDescriptor) (protoreflect.Value, bool) {
		switch desc.Kind() {
		case protoreflect.StringKind:
			return protoreflect.ValueOfString(option), true
		case protoreflect.Int32Kind:
			var n int32
			if _, err := fmt.Sscan(option, &n); err != nil {
				return protoreflect.Value{}, false
			}
			return protoreflect.ValueOfInt32(n), true
		case protoreflect.BoolKind:
			return protoreflect.ValueOfBool(option == "true"), true
		default:
			return protoreflect.Value{}, false
		}
	})
}

func setNumber(settings *v1.Settings, key string, n int64) error {
	return writeField(settings, key, func(desc protoreflect.FieldDescriptor) (protoreflect.Value, bool) {
		switch desc.Kind() {
		case protoreflect.Int32Kind:
			return protoreflect.ValueOfInt32(int32(n)), true
		case protoreflect.Int64Kind:
			return protoreflect.ValueOfInt64(n), true
		case protoreflect.DoubleKind:
			return protoreflect.ValueOfFloat64(float64(n)), true
		case protoreflect.StringKind:
			return protoreflect.ValueOfString(fmt.Sprint(n)), true
		default:
			return protoreflect.Value{}, false
		}
	})
}

func setFlag(settings *v1.Settings, key string, on bool) error {
	return writeField(settings, key, func(desc protoreflect.FieldDescriptor) (protoreflect.Value, bool) {
		if desc.Kind() != protoreflect.BoolKind {
			return protoreflect.Value{}, false
		}
		return protoreflect.ValueOfBool(on), true
	})
}

// A field the probe may move, and what it may move it to.
//
// A number-select carries both an entry list and a band, the entries being shortcuts rather than
// the domain, so both fill here and a move picks between them.
type mutable struct {
	key     string
	control v1.ControlKind
	options []string
	low     int64
	high    int64
	step    int64
	value   *v1.FieldValue
}

// banded says whether this field offers a band to move inside.
func (m mutable) banded() bool { return m.high > m.low }

// mutables is every field a reader could move on this form.
//
// Only the enabled and visible ones: a greyed control is what the shell refuses to offer, and a
// probe writing through it would be testing a path no user reaches.
func mutables(form *v1.Form, skip map[string]bool) []mutable {
	var out []mutable
	for _, group := range form.GetGroups() {
		for _, field := range group.GetFields() {
			if !field.GetVisible() || !field.GetEnabled() || skip[field.GetKey()] {
				continue
			}
			// Where the relay is belongs to the deployment this run was pointed at.
			// A probe moving a listener's port publishes at a port nothing serves, which measures the
			// refusal rather than anything the settings decide.
			if strings.HasPrefix(field.GetKey(), "relay.") {
				continue
			}
			m := mutable{key: field.GetKey(), control: field.GetControl(), value: field.GetValue(), step: 1}
			if r := field.GetRange(); r != nil {
				m.low, m.high, m.step = r.GetMin(), r.GetMax(), r.GetStep()
				if m.step < 1 {
					m.step = 1
				}
			}
			for _, option := range field.GetOptions() {
				if option.GetEnabled() {
					m.options = append(m.options, option.GetValue())
				}
			}

			switch field.GetControl() {
			case v1.ControlKind_CONTROL_KIND_SELECT, v1.ControlKind_CONTROL_KIND_RADIO:
				if len(m.options) == 0 {
					continue
				}
			case v1.ControlKind_CONTROL_KIND_NUMBER, v1.ControlKind_CONTROL_KIND_SLIDER,
				v1.ControlKind_CONTROL_KIND_NUMBER_SELECT:
				if len(m.options) == 0 && !m.banded() {
					continue
				}
			case v1.ControlKind_CONTROL_KIND_TOGGLE:
			default:
				continue
			}
			out = append(out, m)
		}
	}
	return out
}

// mutate moves one field of the draft and names what it wrote.
//
// One field per resolve, so a repair that lands on it is a contradiction rather than a
// consequence: the option was enabled on the form this draft came from, and nothing else moved.
func mutate(rng *rand.Rand, settings *v1.Settings, m mutable) (string, error) {
	switch m.control {
	case v1.ControlKind_CONTROL_KIND_SELECT, v1.ControlKind_CONTROL_KIND_RADIO:
		option := m.options[rng.Intn(len(m.options))]
		return option, setOption(settings, m.key, option)
	case v1.ControlKind_CONTROL_KIND_NUMBER_SELECT:
		// An entry outside the band is the one thing this control says that a band alone cannot, and
		// the band holds positions no entry names, so a walk reaching only one of the two leaves half
		// the control untried.
		if len(m.options) > 0 && (!m.banded() || rng.Intn(2) == 0) {
			option := m.options[rng.Intn(len(m.options))]
			return option, setOption(settings, m.key, option)
		}
		n := pick(rng, m)
		return fmt.Sprint(n), setNumber(settings, m.key, n)
	case v1.ControlKind_CONTROL_KIND_NUMBER, v1.ControlKind_CONTROL_KIND_SLIDER:
		n := pick(rng, m)
		return fmt.Sprint(n), setNumber(settings, m.key, n)
	case v1.ControlKind_CONTROL_KIND_TOGGLE:
		on := !m.value.GetFlag()
		return fmt.Sprint(on), setFlag(settings, m.key, on)
	}
	return "", nil
}

// pick is one position a drag can land on: a multiple of the step inside the band, or either end.
//
// The ladder is the round figures rather than a grid counted off the floor, so a 20 ms floor
// stepping by 50 stops on 20, 50, 100 (avalonia/.../Fields/ViewModel/FieldViewModel.cs, Ticks).
// A probe writing between two stops would be asking for a value no reader can reach, and every
// answer to it would be a finding about nothing.
//
// Both ends come up often: a range whose top no encoder takes is found by asking for the top, and a
// draw spread evenly over a thousand positions asks for it about never.
// The bottom eighth takes most of the rest, so a range whose top is refused leaves the run somewhere
// to keep working from rather than spending itself on one finding.
func pick(rng *rand.Rand, m mutable) int64 {
	step := m.step
	if step < 1 {
		step = 1
	}
	// The stops are step, 2*step and so on, so the first one inside the band is the floor rounded up
	// to a multiple and the last is the ceiling rounded down.
	first := (m.low + step - 1) / step
	last := m.high / step
	if first > last {
		return m.low
	}
	stops := last - first

	switch draw := rng.Intn(8); {
	case draw == 0:
		return m.low
	case draw == 1:
		return m.high
	case draw < 6:
		return (first + rng.Int63n(stops/8+1)) * step
	default:
		return (first + rng.Int63n(stops+1)) * step
	}
}

// readField reads what the draft holds for a key, as the string a report prints.
func readField(settings *v1.Settings, key string) string {
	message, desc, err := locate(settings, key, false)
	if err != nil {
		return ""
	}
	return message.Get(desc).String()
}
