package form

import (
	"slices"
	"strconv"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/receive"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// Repair walks a draft onto legal values and names the fields it moved.
//
// The greying and the walk read one evaluation, optionState, so an option the form greys is one the
// repair never lands on and no control offers a value the publish refuses.
//
// A dimension with nothing legal left keeps the value it holds.
// The field stays disabled with its reason rather than taking a value the same evaluation greys
// (docs/domain-model.md, "What derives from the tables").
//
// An option field walks to the first entry availability leaves enabled.
// A number stands against its slider range, a sane end rather than a refusal, and is clamped to the
// ceilings the capability table states, which are refusals: a bitrate over the codec's ceiling has
// no entry to grey, so the control looks live and only the launch says no.
func Repair(d Deps, draft settings.Settings) (settings.Settings, []string) {
	// The walk runs on the wire message rather than the Go struct: a field key addresses that message,
	// group before the dot and field after it (settingsField).
	// One loop then repairs every field there is, where a per-field setter table would be a second
	// list to keep in step and a field added to the contract would be repaired by nobody.
	m := wire.Settings(draft)

	var repaired []string
	moved := map[string]bool{}

	// Run to a fixed point, because dependencies run both ways down the table: the capture backend
	// strands the codec below it and the publish leg strands the audio codec above it.
	// A single pass would leave the upward kind for the next call, so a form asked about the draft it
	// had just returned would repair something new every time.
	//
	// The bound is one round per setting a round can move.
	// Each round moves at least one or is the last, so more rounds than that is an availability rule
	// cycling between two answers rather than a draft still settling.
	//
	// Counted are the form's fields plus the settings the ladder repair moves with no control of their
	// own, the tune step being one, or the bound sits a round under what the walk may legitimately do.
	movable := len(fieldTable) + len(repairKeysWithoutFields)
	rounds := 0
	for changed := true; changed; rounds++ {
		changed = false
		assert.Assert(rounds <= movable,
			"a repair settles in fewer rounds than the settings it can move", rounds, movable)

		// Table order is screen order, so a round runs left to right and top to bottom: a field is
		// repaired against what the fields above it were left on, and the chroma is held to the codec the
		// capture backend just moved.
		for i := range fieldTable {
			f := &fieldTable[i]
			if f.options == nil && f.itemOptions == nil {
				continue
			}

			// A repeated row is walked once per entry the draft holds, never for the row past the end:
			// nothing is stranded on a row the settings do not carry.
			// The entries are re-read each round with everything else.
			for _, entry := range repairEntries(f, wire.ToSettings(m)) {
				// The draft as every earlier repair left it, read back rather than carried: an option list is
				// a function of the whole draft, so the list this field is held against has to be the list it
				// would be offered.
				s := wire.ToSettings(m)
				if repairSkips(f.key, s) {
					continue
				}
				key := f.key
				if f.repeat {
					key = indexedKey(f.key, entry)
				}
				held := optionValue(fieldValue(f, s, entry))

				next, walked := legalOption(availabilityOf(d, s), d, s, f, held, entry)
				if !walked {
					continue
				}

				group, descriptor, found := settingsField(m, key)
				assert.Assert(found, "a form field names a group and a settings field in it", key)

				value, ok := repairValue(descriptor, next)
				assert.Assert(ok, "a repaired option carries the type its settings field holds",
					key, descriptor.Kind().String(), next)

				group.Set(descriptor, value)
				changed = true
				if !moved[key] {
					moved[key] = true
					repaired = append(repaired, key)
				}
			}
		}

		// The audio list last, since what it drops follows the kinds the walk above may have moved:
		// an entry repaired to no kind records nothing and comes off here rather than staying as a row.
		for _, key := range repairAudioSources(m) {
			changed = true
			if !moved[key] {
				moved[key] = true
				repaired = append(repaired, key)
			}
		}

		// The ladder steps and the ceilings last in the round: both are the codec's own facts and the
		// codec may have moved above.
		// Holding either first measures the draft against the codec it arrived on.
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

	// Read straight off the message.
	// The contract carries every settings field, so the walk ran on the whole draft and no
	// off-contract field is left for a base draft to restore.
	out := wire.ToSettings(m)
	assert.Assert(len(repaired) <= len(fieldTable), "a field is named repaired at most once", len(repaired))
	return out, repaired
}

// repairSkips reports whether a field stands as it is however the rest of the draft reads.
//
// The audio codec is the one, and it is a rule rather than an exception.
// It is read only where a source is selected, so on a silent stream it reaches no encoder and no
// transport and a leg that cannot carry Opus makes a stored "opus" wrong about nothing.
// Repairing it would rewrite a choice for a stream it changes nothing about, and announce a
// repaired field no control on screen explains.
func repairSkips(key string, s settings.Settings) bool {
	return key == KeyAudioCodec && s.Publish.AudioTrack() == capabilities.AudioNone
}

// repairKeysWithoutFields are the settings the repair moves that no control of the form draws.
//
// The walk's bound counts what can move rather than what is drawn.
// Every entry here is a control the form owes: a setting the user cannot see and the repair
// rewrites is a change nobody can read.
var repairKeysWithoutFields []string

// repairLadders puts the two ladder steps back on the selected codec's own ladders and returns the
// keys it moved.
//
// The ladders do not correspond.
// A step is the encoder's own identifier, x264 counting in names, SVT-AV1 in numbers to 13 and
// NVENC from p1 to p7, so a draft that changed codec holds a step the new encoder never heard of
// and there is no position to carry across: "slow" is not preset 8.
// A step off the ladder is therefore reset to the one the codec's row declares for the mode rather
// than mapped, and the field is named in the repaired list, since the value is not one the user
// picked.
//
// A codec that declares no ladder leaves the field standing.
// There is no ladder to be off, the control is greyed with that reason, and clearing it would throw
// away the step the draft carries for whichever codec it came from.
func repairLadders(m *screensharev1.Settings) []string {
	s := wire.ToSettings(m)
	c, known := capabilities.Get(s.Publish.Codec)
	if !known {
		// A codec no table carries is the codec field's own repair, on a later round.
		// Until it has moved, nothing here knows which ladder to hold the steps against.
		return nil
	}

	// Writing what is already there is not a move.
	// A declared step can be the empty one, a tune ladder leaving most modes untuned and a gapped mode
	// declaring nothing at all, and the empty value is on no ladder.
	// Resetting to it and then judging it off the ladder again is a round that never settles.
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

// repairCeilings holds the numeric settings down to the limits the capability table states for the
// codec and engine the draft names, and returns the keys it moved.
//
// These are the numbers with a refusal behind them rather than a slider end: a bitrate above the
// codec's ceiling and a quantizer off the top of its scale are both values capabilities.Validate
// rejects, and neither has an entry to grey.
// A ceiling of zero is an engine that states none, not a ceiling of nothing.
//
// Ceilings only, and downward only.
// A figure under a limit is the user's to choose.
func repairCeilings(d Deps, m *screensharev1.Settings) []string {
	// Off the message alone, unlike the walk above: the codec, the bitrate and the ceiling are all on
	// the contract, so no off-contract field is left for a base draft to restore.
	s := wire.ToSettings(m)
	// The ends come off the same evaluation the form offers the control within, so a figure held down
	// here and the end a slider stops at cannot disagree.
	// A draft naming a codec no table carries narrows nothing and stands until the codec field's own
	// repair moves it on a later round.
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
		// The burst ceiling comes down with the target, which its control's range deliberately does not
		// do: fieldMaxrateBounds offers the full scale, because a codec's limit bounds the target the
		// encoder is given rather than the headroom above it.
		// So the two disagree on purpose, and this is the side that keeps the pair reachable: a ceiling
		// left where the target cannot follow is headroom nothing can ever use.
		// Not a refusal being anticipated, though: no rule bands this key and capabilities.Validate
		// weighs the target alone, so a maxrate above the limit is a draft nothing rejects.
		// The move is named in the repaired keys, which is what keeps it from being a silent narrowing.
		if s.Publish.MaxrateM > ceiling {
			s.Publish.MaxrateM = ceiling
			moved = append(moved, KeyMaxrateM)
		}
	}
	// A burst ceiling under its own target is not a ceiling.
	// The ceiling rises rather than the target dropping: the target is the figure the user chose.
	//
	// Constant quality has no target for it to sit above, so the pair cannot disagree there and the
	// bitrate is a figure from another mode: raising a 10 Mbit/s ceiling to a 40 Mbit/s target would
	// hand a quality encode four times the rate it was bounded to.
	if s.Publish.Mode != capabilities.ModeCrf && s.Publish.MaxrateM > 0 && s.Publish.MaxrateM < s.Publish.BitrateM {
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

// The two dimensions whose walk order is a judgement rather than the option list's own.
//
// Elsewhere the first legal entry is the replacement, an option list being written in the order a
// reader should reach for its entries.
// A forced downgrade is no choice of the user's, so for these two the order decides what it costs.
//
// Chroma descends by colour detail kept, so a combination ruling out planar RGB lands on 4:4:4
// rather than on whatever the encoder lists first.
// The 10-bit layout is last: more tonal resolution at 4:2:0 chroma is a different trade from the
// four above it, not a step down the same ladder.
var repairChromaOrder = []string{"gbrp", "yuv444p", "yuv422p", "yuv420p", "p010le"}

// Colour range prefers full, which is what a desktop already is.
// Limited range maps the captured values into a narrower window, a conversion the picture did not
// need, so the repair reaches for it only where full range is refused.
var repairColorRangeOrder = []string{"pc", "tv"}

// The capture backends in the order a repair reaches for them, which is not the form's list order.
//
// A repair moves a backend nobody asked to move, so it reaches for the one that asks least of the
// machine it lands on.
// The portal and the two X11 grabbers capture a desktop the way the backend they replace did;
// kmsgrab reads scanout buffers below the compositor and needs CAP_SYS_ADMIN for them,
// so landing there answers a Wayland session with a backend that fails for a reason no control
// names.
//
// An engine's own two backends sit together, since moving engine changes which codecs and
// transports the rest of the form offers and strands more than it fixes.
var repairCaptureOrder = []string{
	"ddagrab", "d3d11screencapturesrc",
	"x11grab", "ximagesrc",
	"portal",
	"avfoundation", "avfvideosrc",
	"gdigrab",
	"kmsgrab",
}

// The render chain a repair reaches for first: the one a stream renders through where nothing chose
// a chain.
//
// A row of its own because the offer order is the picker's, cheapest conversion first, and the
// default sits inside that list rather than at its head (receive.Chains).
// The walk below reaches for the first legal entry, so without this a chain that lost its elements
// would land on whatever the table happens to open with instead of on what the package answers with
// when asked for nothing.
var repairRenderChainOrder = []string{receive.DefaultChain}

// repairOrders is the walk order of the fields that state one.
// A field with no row here walks in the order its option list offers.
var repairOrders = map[string][]string{
	KeyCapture:     repairCaptureOrder,
	KeyChroma:      repairChromaOrder,
	KeyColorRange:  repairColorRangeOrder,
	KeyRenderChain: repairRenderChainOrder,
}

// legalOption answers what this field holds instead of held, and whether that is a change.
//
// av is the evaluation of s, taken by the caller: the walk asks it of every candidate, and a caller
// weighing several fields of one draft (presetStrands) asks it of every field too.
//
// A held value that is offered and not greyed stands, whatever else the list carries.
// Only an absent or greyed value walks, and it walks to the first entry the same evaluation leaves
// enabled.
// First rather than nearest: an option list is written in the order a reader should reach for it,
// recommended entry leading, so first is nearest by the only measure this package has.
func legalOption(av availability, d Deps, s settings.Settings, f *field, held string, entry int) (string, bool) {
	var options []*screensharev1.FieldOption
	if f.repeat {
		options = f.itemOptions(d, s, audioEntry(s, entry))
	} else {
		options = f.options(d, s)
	}
	if len(options) == 0 {
		// An empty list is a machine that offers nothing here.
		// The held value stays: there is nothing to walk to, and clearing the setting would invent the
		// absence of a choice.
		return held, false
	}

	// Whether one was found is a flag of its own and not the empty string standing in for "none".
	// The empty value is a real option: it is what the unscaled entry of the output-resolution ladder
	// carries, and it leads that list as the recommended one (options.go, optionOutputResolutions).
	// Read as a sentinel it costs both directions at once, leaving a greyed value stranded where the
	// unscaled entry is the only legal one, and walking past the recommended entry to whatever comes
	// after it where a smaller monitor greys the rest.
	first, found := "", false
	for _, value := range repairWalk(f.key, options) {
		enabled, _ := optionStateOf(av, f.key, value, entry)
		if value == held && enabled {
			return held, false
		}
		if enabled && !found && value != held {
			first, found = value, true
		}
	}

	if !found {
		// Every entry greyed.
		// The value stands with the field's reason, the one case the contract allows a control to show a
		// value its own evaluation refuses.
		return held, false
	}
	return first, true
}

// repairWalk is the order a field's entries are tried in: its stated order narrowed to what the
// list offers, and the list's own order where it states none.
//
// A stated order is a preference among values, not a claim that every one is on offer here, so an
// entry the list left out is one no repair may reach for.
// An offered entry the order does not name is appended rather than dropped, since a value the
// machine offers stays reachable even where this file has no opinion about where it sits.
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

// optionValue is a field's value as its option list spells it.
//
// An option value is a string on the wire whatever type the settings field holds, so this is where
// the two meet on the way in and repairValue where they meet on the way back.
// A select holding a number is ordinary, the monitor index being one: what makes a control a select
// is the machine having a list, not the value being a name.
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

// repairValue is an option's string back in the type its settings field holds, and false where the
// string does not fit that type.
//
// False is a broken contract rather than a condition to survive: every value reaching here came out
// of an option list this package built for this field.
// The caller asserts on it, so a list built with the wrong spelling fails where it was written
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

// repairEntries is what one row is walked for: the entries the draft holds on a repeated row,
// and noEntry on every other row.
//
// The row past the end of the list is not walked.
// It is not in the settings, so nothing is stranded on it, and repairing it would write an entry
// the reader never made.
func repairEntries(f *field, s settings.Settings) []int {
	if !f.repeat {
		return []int{noEntry}
	}
	out := make([]int, 0, len(s.Publish.AudioSources))
	for i := range s.Publish.AudioSources {
		out = append(out, i)
	}
	return out
}

// repairAudioSources takes every entry that records nothing off the list, and returns the keys it
// moved.
//
// An entry naming no kind is how a reader turns a source off, and it is what the row past the end
// of the list holds until a kind is picked on it.
// Neither is a source, so leaving them in a stored draft would grow the list by one every time the
// form was asked about the draft it had just returned.
//
// The key named is the entry's own, which is what a shell binds and can say moved.
// Entries after a dropped one shift up; a shell adopts the repaired draft whole and redraws from
// it, so no index survives the shift.
func repairAudioSources(m *screensharev1.Settings) []string {
	s := wire.ToSettings(m)
	kept := s.Publish.Recorded()
	if len(kept) == len(s.Publish.AudioSources) {
		return nil
	}

	var moved []string
	for i, a := range s.Publish.AudioSources {
		if !a.Records() {
			moved = append(moved, indexedKey(KeyAudioSource, i))
		}
	}
	s.Publish.AudioSources = kept
	proto.Reset(m)
	proto.Merge(m, wire.Settings(s))
	return moved
}
