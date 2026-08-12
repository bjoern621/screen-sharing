package form

import (
	"slices"
	"strconv"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// Repair walks a draft to the nearest legal value and names the fields it moved.
//
// It is the other half of availability, and the reason the two live in one package: a
// form that greys an option and a repair that picks a different replacement are two
// encodings of one rule, and the failure mode is a control offering a value the publish
// then refuses. Here both read optionState, so an option the form would grey is an option
// repair has already taken away.
//
// What it does not do is invent. A dimension with nothing legal left keeps the value it
// has and the field stays disabled with its reason, which is the same answer from both
// sides rather than a form offering what the encoder rejects
// (docs/domain-model.md, "What derives from the tables").
//
// Two kinds of field are repaired, and the difference between them is what the number
// is held against.
//
// A field with options is walked to the first entry availability leaves enabled. A
// number is left alone against its slider range, which is a sane end rather than a limit
// anything enforces - but clamped against the ceilings the capability table states,
// because those are refusals. A codec whose bitrate ceiling is under the settings'
// default is a stream that cannot be published and a form with nothing greyed to explain
// it, since a number has no entry to grey: the control looks live, the value looks
// ordinary, and only the launch says no. Clamping is what turns that into a repair the
// form can name.
func Repair(d Deps, draft settings.Settings) (settings.Settings, []string) {
	// The walk runs on the wire message rather than on the Go struct, because a field key
	// is an address into that message: the group before the dot, the field after it
	// (settingsField). That is what lets one loop repair every field there is - a
	// per-field setter table would be a second list to keep in step with the first, and a
	// field added to the contract would be repaired by nobody until someone remembered.
	m := wire.Settings(draft)

	var repaired []string
	moved := map[string]bool{}

	// The walk runs to a fixed point rather than once, because the dependencies between
	// fields run both ways down the table: the capture backend strands the codec below it,
	// and the publish leg strands the audio codec above it. A single pass leaves the second
	// kind for the next call, and a form that repaired something new every time it was
	// asked about the draft it had just returned would never settle.
	//
	// The bound is the number of settings a round can move. Each round either moves at
	// least one of them or is the last, and none can move without a round having moved
	// another, so a walk that took more rounds than that would be one cycling between two
	// answers - which is an availability rule contradicting itself, not a draft to keep
	// chewing on.
	//
	// It is the form's fields plus what the ladder repair moves without a control of its
	// own. The tune step is one: it is a setting the repair puts back on the selected
	// codec's ladder, and the form has no row for it yet, so counting fields alone would
	// bound the walk one round short of what it can legitimately do.
	movable := len(fieldTable) + len(repairKeysWithoutFields)
	rounds := 0
	for changed := true; changed; rounds++ {
		changed = false
		assert.Assert(rounds <= movable,
			"a repair settles in fewer rounds than the settings it can move", rounds, movable)

		// The table's order is the screen's order, which makes each round left to right and
		// top to bottom: a field is repaired against the values the fields before it were
		// left on, so a codec repaired by the capture backend is what the chroma is then
		// held to.
		for i := range fieldTable {
			f := &fieldTable[i]
			if f.options == nil {
				continue
			}

			// The draft as it stands after every earlier repair. Read back each time rather
			// than carried, because an option list is a function of the whole draft and the
			// list this field is held against has to be the one it would be offered.
			s := wire.ToSettings(m)
			if repairSkips(f.key, s) {
				continue
			}
			held := optionValue(f.value(s))

			next, walked := legalOption(d, s, f, held)
			if !walked {
				continue
			}

			group, descriptor, found := settingsField(m, f.key)
			assert.Assert(found, "a form field names a group and a settings field in it", f.key)

			value, ok := repairValue(descriptor, next)
			assert.Assert(ok, "a repaired option carries the type its settings field holds",
				f.key, descriptor.Kind().String(), next)

			group.Set(descriptor, value)
			changed = true
			if !moved[f.key] {
				moved[f.key] = true
				repaired = append(repaired, f.key)
			}
		}

		// The ladder steps and the ceilings last in the round, for the same reason: both
		// are the codec's own facts and the codec may have moved above. Holding either
		// first would measure the draft against the codec it arrived on and leave it
		// against the one it left on.
		for _, key := range repairLadders(m) {
			changed = true
			if !moved[key] {
				moved[key] = true
				repaired = append(repaired, key)
			}
		}
		for _, key := range repairCeilings(d, m) {
			changed = true
			if !moved[key] {
				moved[key] = true
				repaired = append(repaired, key)
			}
		}
	}

	// Straight off the message. The contract carries every settings field now, so what
	// the walk ran on is the whole draft rather than the part of it a shell could see:
	// there is nothing left for a base draft to restore, which is what the split bought
	// here.
	out := wire.ToSettings(m)
	assert.Assert(len(repaired) <= len(fieldTable), "a field is named repaired at most once", len(repaired))
	return out, repaired
}

// repairSkips reports whether a field is left as it stands however the rest of the draft
// reads.
//
// One field is, and it is a rule rather than an exception. The audio codec is read only
// where a source is selected: on a silent stream it reaches no encoder and no transport,
// so a publish leg that cannot carry Opus does not make a stored "opus" wrong - there is
// no track for it to be wrong about. Repairing it anyway would rewrite a choice the user
// made, for a stream it changes nothing about, and hand the form a repaired field to
// announce that nothing on screen would explain.
func repairSkips(key string, s settings.Settings) bool {
	return key == KeyAudioCodec && s.Publish.AudioTrack() == capabilities.AudioNone
}

// repairKeysWithoutFields are the settings the repair moves that the form declares no
// control for.
//
// It exists so the walk's bound counts what can move rather than what is drawn: a setting
// the user cannot see and the repair rewrites is a setting whose change nobody can read.
// It is empty, and every entry it gains is a control the form owes.
var repairKeysWithoutFields []string

// repairLadders puts the two ladder steps back on the selected codec's own ladders, and
// returns the keys it moved.
//
// The ladders do not correspond, which is the whole reason this exists. A step is the
// encoder's own identifier - x264 counts in names, SVT-AV1 in numbers to 13, NVENC from p1
// to p7 - so a draft that changes codec is holding a step the new encoder never heard of,
// and there is no position to carry across: "slow" is not preset 8, and a number that
// looked equivalent would land the user on a different real setting than the one they had.
//
// So a step off the ladder is reset to the one that codec's row declares for the mode
// rather than mapped. That is a value the user did not choose, which is why the field is
// named in the repaired list: a shell says so, where a silent rewrite would leave them
// reading a step they never picked.
//
// A codec declaring no ladder leaves the field standing. There is no step to be off, the
// control is greyed with that reason, and clearing it would throw away what the draft
// carries for whichever codec it came from.
func repairLadders(m *screensharev1.Settings) []string {
	s := wire.ToSettings(m)
	c, known := capabilities.Get(s.Publish.Codec)
	if !known {
		// A codec no table carries is one the codec field's own repair moves on a later
		// round. Nothing here knows which ladder to hold the steps against until it has.
		return nil
	}

	// A move that would write what is already there is not a move. The declared step for a
	// mode can be the empty one - a tune ladder leaves most modes untuned, and a mode the
	// codec gaps declares nothing at all - and the empty value is not a step of any ladder,
	// so resetting to it and then judging it off the ladder again is a round that never
	// settles. The repair is a fixed point or it is a loop.
	var moved []string
	if len(c.Effort.Steps) > 0 && !c.Effort.Has(s.Publish.Effort) {
		if step, _ := c.Effort.StepFor(s.Publish.Mode); step != s.Publish.Effort {
			s.Publish.Effort = step
			moved = append(moved, KeyEffort)
		}
	}
	if len(c.Tune.Steps) > 0 && !c.Tune.Has(s.Publish.Tune) {
		if step, _ := c.Tune.StepFor(s.Publish.Mode); step != s.Publish.Tune {
			s.Publish.Tune = step
			moved = append(moved, KeyTune)
		}
	}
	if len(moved) == 0 {
		return nil
	}
	proto.Reset(m)
	proto.Merge(m, wire.Settings(s))
	return moved
}

// repairCeilings holds the numeric settings to the limits the capability table states for
// the codec and engine the draft names, and returns the keys it moved.
//
// These are the numbers with a refusal behind them rather than a slider end: a bitrate
// above the codec's ceiling and a quantizer off the top of its scale are both values
// capabilities.Validate rejects, so leaving them standing is leaving a draft that cannot
// be published with nothing on the form saying so. A ceiling of zero is an engine that
// states none, which is not a ceiling of nothing.
//
// Only the ceilings are enforced, and only downward. A figure under a limit is the user's
// to choose, and this is a repair rather than an opinion about what a good bitrate is.
func repairCeilings(d Deps, m *screensharev1.Settings) []string {
	// Read off the message alone, unlike the walk above. Everything this function holds a
	// figure against - the codec, the bitrate, the ceiling - is on the contract, so there
	// is no off-contract field for a base draft to restore.
	s := wire.ToSettings(m)
	// The ends come off the same evaluation the form offers the control within, so a
	// figure this holds down and a figure the slider stops at cannot disagree. A draft
	// naming a codec no table carries narrows nothing and is left alone, which is what the
	// codec field's own repair moves on a later round.
	v := verdictsOf(d, s)

	var moved []string
	if _, ceiling := v.Bounds(KeyCq, 0, capabilities.WidestCqScale()); s.Publish.Cq > ceiling {
		s.Publish.Cq = ceiling
		moved = append(moved, KeyCq)
	}
	if _, ceiling := v.Bounds(KeyBitrateM, 0, fieldRateCeiling); ceiling < fieldRateCeiling {
		if s.Publish.BitrateM > ceiling {
			s.Publish.BitrateM = ceiling
			moved = append(moved, KeyBitrateM)
		}
		// The burst ceiling is held to the same limit, which the control's own range
		// deliberately is not: a ceiling is not a target, so the form offers it past the
		// encoder's limit on purpose. What a repair may not leave standing is a draft the
		// publish refuses, and a maxrate above the limit is one.
		if s.Publish.MaxrateM > ceiling {
			s.Publish.MaxrateM = ceiling
			moved = append(moved, KeyMaxrateM)
		}
	}
	// A burst ceiling under the target it is a ceiling for is not a ceiling. It is raised
	// rather than the target lowered, because the target is the figure the user chose and
	// the ceiling is the one that follows it.
	if s.Publish.MaxrateM > 0 && s.Publish.MaxrateM < s.Publish.BitrateM {
		s.Publish.MaxrateM = s.Publish.BitrateM
		if !slices.Contains(moved, KeyMaxrateM) {
			moved = append(moved, KeyMaxrateM)
		}
	}

	if len(moved) == 0 {
		return nil
	}
	proto.Reset(m)
	proto.Merge(m, wire.Settings(s))
	return moved
}

// The two dimensions whose walk order is a judgement rather than a list.
//
// Everywhere else "the first legal entry" is the right replacement, because an option
// list is written in the order a reader should reach for its entries. These two are
// stated separately because a downgrade forced on the user is not a choice they made, and
// the order decides how much it costs them.
//
// Chroma descends by how much colour detail is kept, so a combination that rules out
// planar RGB lands on 4:4:4 rather than on whatever the encoder happens to list first.
// The 10-bit layout is last: it is more tonal resolution at 4:2:0 chroma, which is a
// different trade from the four above it and not a step down the same ladder.
var repairChromaOrder = []string{"gbrp", "yuv444p", "yuv422p", "yuv420p", "p010le"}

// Colour range prefers full range, which is what a desktop already is: the limited-range
// alternative maps the captured values into a narrower window, so it is a conversion the
// picture did not need and the repair reaches for it only when full range is refused.
var repairColorRangeOrder = []string{"pc", "tv"}

// The capture backends in the order a repair reaches for them, which is not the order the
// form lists them in.
//
// A repair moves a backend the user did not ask to move, so it reaches for the one that
// asks least of the machine it lands on. The portal and the two X11 grabbers capture a
// desktop the way the user's old backend did; kmsgrab reads scanout buffers below the
// compositor and needs CAP_SYS_ADMIN to do it, so a repair that landed there would answer
// a Wayland session with a backend that fails for a reason the form never mentioned.
//
// Within that, an engine's own two backends sit together, because moving engine is what
// changes which codecs and transports the rest of the form offers - and a repair that
// changed engine when it did not have to would strand more than it fixed.
var repairCaptureOrder = []string{
	"ddagrab", "d3d11screencapturesrc",
	"x11grab", "ximagesrc",
	"portal",
	"avfoundation", "avfvideosrc",
	"gdigrab",
	"kmsgrab",
}

// repairOrders is the walk order per field, for the fields that state one. A field with
// no row here is walked in the order its option list offers.
var repairOrders = map[string][]string{
	KeyCapture:    repairCaptureOrder,
	KeyChroma:     repairChromaOrder,
	KeyColorRange: repairColorRangeOrder,
}

// legalOption answers what this field should hold instead of held, and whether that is a
// change.
//
// The order is the whole of the rule. A held value that is offered and not greyed stands,
// whatever else the list contains; only a value that is absent or greyed is walked, and it
// is walked to the first entry the same evaluation leaves enabled. First rather than
// nearest, because the option lists are written in the order a reader should reach for
// them - the recommended entry leads - so first is nearest by the only measure this
// package has.
func legalOption(d Deps, s settings.Settings, f *field, held string) (string, bool) {
	options := f.options(d, s)
	if len(options) == 0 {
		// A field whose list came out empty describes a machine that offers nothing here.
		// The held value stays: there is nothing to walk to, and a cleared setting would
		// be this package inventing the absence of a choice.
		return held, false
	}

	first := ""
	for _, value := range repairWalk(f.key, options) {
		enabled, _ := optionState(d, s, f.key, value)
		if value == held && enabled {
			return held, false
		}
		if enabled && first == "" && value != held {
			first = value
		}
	}

	if first == "" {
		// Every entry is greyed. The value stands and the field keeps its reason, which is
		// the one case the contract explicitly allows a control to show a value its own
		// evaluation would refuse.
		return held, false
	}
	return first, true
}

// repairWalk is the order this field's entries are tried in: the stated one where the
// field has one, narrowed to what the list actually offers, and the list's own order
// otherwise.
//
// The narrowing is what keeps the two in step. A stated order is a preference among
// values, not a claim that every one of them is on offer here, so an entry the list left
// out is one no repair may reach for; and an entry the list carries that the order does
// not name is appended rather than dropped, since a value a machine offers has to be
// reachable even when this file has no opinion about where it sits.
func repairWalk(key string, options []*screensharev1.FieldOption) []string {
	offered := make([]string, 0, len(options))
	for _, o := range options {
		offered = append(offered, o.GetValue())
	}

	order, ok := repairOrders[key]
	if !ok {
		return offered
	}

	walk := make([]string, 0, len(offered))
	seen := make(map[string]bool, len(order))
	for _, value := range order {
		for _, have := range offered {
			if have == value {
				walk = append(walk, value)
				seen[value] = true
				break
			}
		}
	}
	for _, have := range offered {
		if !seen[have] {
			walk = append(walk, have)
		}
	}
	return walk
}

// optionValue is a field's current value as its option list spells it.
//
// An option value is a string on the wire whatever the settings field holds, so this is
// the one place the two meet on the way in - and repairValue is the one place they meet on
// the way back. A field whose control offers options and whose value is a number is an
// ordinary case, the monitor index being one: what makes it a select is that the machine
// has a list of them, not that the value is a name.
func optionValue(v *screensharev1.FieldValue) string {
	switch k := v.GetKind().(type) {
	case *screensharev1.FieldValue_Text:
		return k.Text
	case *screensharev1.FieldValue_Number:
		return strconv.FormatInt(k.Number, 10)
	case *screensharev1.FieldValue_Decimal:
		return strconv.FormatFloat(k.Decimal, 'g', -1, 64)
	case *screensharev1.FieldValue_Flag:
		return strconv.FormatBool(k.Flag)
	default:
		return ""
	}
}

// repairValue is the option's string back in the type the settings field holds it, and
// false where the string does not fit that type.
//
// False is a broken contract rather than a condition to survive: every value that reaches
// here came out of an option list this package built for this field. The caller asserts on
// it, which is what makes a list built with the wrong spelling fail where it was written
// rather than three layers away.
func repairValue(descriptor protoreflect.FieldDescriptor, value string) (protoreflect.Value, bool) {
	switch descriptor.Kind() {
	case protoreflect.StringKind:
		return protoreflect.ValueOfString(value), true
	case protoreflect.Int32Kind:
		n, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return protoreflect.Value{}, false
		}
		return protoreflect.ValueOfInt32(int32(n)), true
	case protoreflect.Int64Kind:
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return protoreflect.Value{}, false
		}
		return protoreflect.ValueOfInt64(n), true
	case protoreflect.BoolKind:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return protoreflect.Value{}, false
		}
		return protoreflect.ValueOfBool(b), true
	default:
		return protoreflect.Value{}, false
	}
}
