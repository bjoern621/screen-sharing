package publish

import (
	"slices"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/gstrun"
	"bjoernblessin.de/screenshare/internal/rules"
	"bjoernblessin.de/screenshare/internal/settings"
)

// What a running pipeline takes, and what it does not.
//
// A publish that changes one of these stays the same pipeline: the value reaches the encoder over
// the child's control socket and every viewer keeps watching, where a relaunch costs each of them a
// reconnect.
// Everything else is a different pipeline and says so by changing the rendered command
// (SamePipeline).
//
// Live takes no field away.
// Every settings field stays drawn and editable while a stream publishes, and a live row is what
// lets a form state the cost of an edit before it is made rather than after the picture blinked
// (docs/field-availability.md, "A live stream blocks no field").
//
// One table, read three ways.
// LiveFields answers which fields a configuration carries live, so LiveOnly can hold a proposal
// against a running stream.
// liveRules registers the same rows into the evaluator, so a form saying a control costs no
// reconnect and an apply that costs none are one fact.
// gstLiveState turns them into the writes the child converges to.
// A list of field names per consumer is what this replaces, and the consumer that fell behind would
// have been the one deciding whether to kill a stream.

// liveField is one settings field a running pipeline takes a new value for.
type liveField struct {
	// key is the settings field, spelled as the form addresses the control and as a rule names the
	// axis, which are one identifier.
	key string
	// when is what has to hold for a running pipeline to take it: the engine whose child carries the
	// write, and whatever else decides the value reaches the encoder at all.
	// A field whose value the encoder never sees is ignored rather than live, and reporting that edit
	// as applied would be a lie either way.
	when map[string]rules.Match
	// hold copies this field's value from one settings object onto another.
	// It is what lets LiveOnly ask its question by rendering rather than by listing: put the running
	// values back and see whether anything else moved.
	hold func(from settings.Settings, onto *settings.Settings)
	// write is what the running child is told, as the property writes it converges to.
	// Each row's own when names the engine whose child carries them; a field another engine took live
	// would carry that engine's writes here and be gated the same way.
	write func(settings.Settings) []gstrun.Property
}

// liveFields is what a running pipeline takes, ordered as a form draws the controls.
var liveFields = []liveField{{
	key: rules.AxisBitrateM,
	// An encoder takes a new rate while it runs, and every element here has a property for one.
	//
	// A mode that sends the encoder no rate is not live in it.
	// Constant quality aims at a quantizer and lossless at exactness, so a rate written under either
	// is a figure the element carries and never spends.
	when: map[string]rules.Match{
		rules.AxisEngine: rules.OneOf(EngineGst),
		rules.AxisCodec:  rules.OneOf(gstLiveBitrateCodecs()...),
		rules.AxisMode:   rules.OneOf(capabilities.BitrateModes()...),
	},
	hold: func(from settings.Settings, onto *settings.Settings) {
		onto.Publish.BitrateM = from.Publish.BitrateM
	},
	write: gstLiveBitrateWrite,
}, {
	// The level of every source in the mix, and the silence with it.
	// An element that multiplies takes both as one value, so a muted source is one at zero and
	// unmuting is a write to the running pipeline rather than a rebuild of the audio graph.
	//
	// The whole list is one field, because the mixer is one graph: an entry added or taken off is a
	// different graph and a relaunch, and what is live is the level each existing branch runs at.
	key:   rules.FieldAudioGain,
	when:  map[string]rules.Match{rules.AxisEngine: rules.OneOf(EngineGst)},
	hold:  holdAudioLevels,
	write: gstLiveGainWrite,
}}

// holdAudioLevels puts the running levels back onto a proposal, so what is left differing is
// everything the mixer's shape depends on.
//
// The list is cloned before anything is written to it.
// A settings value is copied by assignment everywhere else in this package, and a slice copied that
// way shares its entries: writing through the copy would move the caller's own settings,
// which for LiveOnly's probe means answering a question by changing it.
func holdAudioLevels(from settings.Settings, onto *settings.Settings) {
	assert.IsNotNil(onto, "a held level is written onto a proposal")

	onto.Publish.AudioSources = slices.Clone(onto.Publish.AudioSources)
	for i := range onto.Publish.AudioSources {
		if i >= len(from.Publish.AudioSources) {
			break
		}
		onto.Publish.AudioSources[i].Gain = from.Publish.AudioSources[i].Gain
		onto.Publish.AudioSources[i].Mute = from.Publish.AudioSources[i].Mute
	}
}

// The table decides whether a change kills a stream, so a malformed row is an Entwicklungsfehler
// and fails at load rather than at the moment a reader presses apply.
//
// A row missing its hold makes LiveOnly answer that a field differs when it does not, and one
// missing its write has the child converge to a state that left the field behind.
// The first costs every viewer a reconnect nobody needed, the second reports an edit that never
// reached the encoder.
func init() {
	seen := make(map[string]bool, len(liveFields))
	for _, f := range liveFields {
		assert.Assert(f.key != "", "a live field names the control it belongs to")
		assert.Assert(!seen[f.key], "a live field is declared once", f.key)
		assert.Assert(len(f.when) > 0, "a live field states what has to hold for it to be live", f.key)
		assert.IsNotNil(f.hold, "a live field states how its value is held across a proposal", f.key)
		assert.IsNotNil(f.write, "a live field states what the running child is told", f.key)
		seen[f.key] = true
	}

	rules.Register(liveRules()...)
}

// liveRules is the table as rules, so the form's answer and the apply path's answer come off one
// statement.
// A live rule names no value and carries no reason: it grants the control a property rather than
// taking anything away from it (rules.Live, docs/field-availability.md).
func liveRules() []rules.Rule {
	out := make([]rules.Rule, 0, len(liveFields))
	for _, f := range liveFields {
		out = append(out, rules.Rule{When: f.when, Verdict: rules.Live, Field: f.key})
	}
	return out
}

// LiveFields is every settings field a pipeline built from s takes a new value for while it runs,
// in table order.
//
// It evaluates against the facts the rows name and no more, which is what lets this package answer
// without the machine's own answers: the engine follows from the capture backend, and the codec and
// the mode are the draft's.
// A capture backend no publisher runs names no engine and matches no row, so an unbuildable
// configuration answers as not live rather than being guessed at.
func LiveFields(s settings.Settings) []string {
	v := rules.EvaluateRules(liveFacts(s), liveRules())
	out := make([]string, 0, len(liveFields))
	for _, f := range liveFields {
		if v.Live(f.key) {
			out = append(out, f.key)
		}
	}
	return out
}

// liveFacts is the configuration a liveness question is about, as the rows read it.
// An unknown capture backend names no engine, which is a fact no row matches.
func liveFacts(s settings.Settings) rules.Facts {
	engine, err := EngineFor(s.Publish.Capture)
	if err != nil {
		engine = ""
	}
	return rules.Facts{
		rules.AxisEngine: rules.TextValue(engine),
		rules.AxisCodec:  rules.TextValue(s.Publish.Codec),
		rules.AxisMode:   rules.TextValue(s.Publish.Mode),
	}
}

// LiveOnly reports whether next differs from running in nothing but what a running pipeline takes.
//
// Asked rather than derived from a list of field names: the running settings' live values go back
// onto the proposal, and if the two then render the same pipeline, everything that differs is
// something the socket can carry.
// A field no builder reads changes neither rendering and is no reason to relaunch either,
// which is the answer SamePipeline already gives.
//
// The live set is the running stream's and not the proposal's.
// What decides whether a change can be applied is what the running child accepts, and a proposal
// moving onto a codec with a live bitrate does not make the playing pipeline take one.
func LiveOnly(running, next settings.Settings) (bool, error) {
	probe := next
	for _, key := range LiveFields(running) {
		f, ok := liveFieldFor(key)
		assert.Assert(ok, "a live field is one the table declares", key)
		f.hold(running, &probe)
	}
	return SamePipeline(running, probe)
}

func liveFieldFor(key string) (liveField, bool) {
	for _, f := range liveFields {
		if f.key == key {
			return f, true
		}
	}
	return liveField{}, false
}
