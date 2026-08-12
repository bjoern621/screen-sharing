package publish

import (
	"slices"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/gstrun"
	"bjoernblessin.de/screenshare/internal/rules"
	"bjoernblessin.de/screenshare/internal/settings"
)

// What a running pipeline will take, and what it will not.
//
// A publish that changes one of these does not have to become another pipeline: the value
// reaches the encoder over the child's control socket and every viewer keeps watching,
// where a relaunch costs each of them a reconnect. Everything else is a different pipeline
// and says so by changing the rendered command (SamePipeline).
//
// One table, read three ways. LiveFields answers which fields a configuration carries
// live, so LiveOnly can hold a proposal against a running stream; liveRules registers the
// same rows into the evaluator, so a form saying a control costs no reconnect and an apply
// that costs none are one fact; and gstLiveState turns them into the writes the child
// converges to. A list of field names written per consumer is what this replaces, and the
// consumer that fell behind would have been the one deciding whether to kill a stream.

// liveField is one settings field a running pipeline takes a new value for.
type liveField struct {
	// key is the settings field, spelled as the form addresses the control and as a rule
	// names the axis, which are the same identifier.
	key string
	// when is what has to hold for a running pipeline to take it: the engine whose child
	// carries the write, and whatever else decides that the value reaches the encoder at
	// all. A field whose value the encoder never sees is not live, it is ignored, and
	// telling a user their edit went through would be the same lie either way.
	when map[string]rules.Match
	// hold copies this field's value from one settings object onto another. It is what
	// lets LiveOnly ask its question by rendering rather than by listing: put the running
	// values back and see whether anything else moved.
	hold func(from settings.Settings, onto *settings.Settings)
	// write is what the running child is told, in the property writes it converges to.
	// Every row is the GStreamer engine's today, which its own when says; a field another
	// engine took live would carry that engine's writes here and be gated the same way.
	write func(settings.Settings) []gstrun.Property
}

// liveFields is every field a running pipeline takes, in the order a form draws them.
var liveFields = []liveField{{
	key: rules.AxisBitrateM,
	// The bitrate is the socket's first customer rather than the audio gain the design
	// named: an encoder takes a new rate while it runs, every element here has a property
	// for it, and it needs no settings field that does not exist yet.
	//
	// A mode that sends the encoder no rate at all is not live in it. Constant quality
	// aims at a quantizer and lossless at exactness, so a rate written there is a figure
	// the element carries and never spends.
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
	// The level of every source in the mix, and the silence with it. Both are one value to
	// an element that multiplies, so a muted source is one at zero and unmuting is a write
	// to a pipeline that is already running rather than a rebuild of the audio graph.
	//
	// The whole list is one field, because the mixer is one graph: an entry added or taken
	// off is a different graph and a relaunch, and what is live is the level each branch
	// that already exists is running at.
	key:   rules.FieldAudioGain,
	when:  map[string]rules.Match{rules.AxisEngine: rules.OneOf(EngineGst)},
	hold:  holdAudioLevels,
	write: gstLiveGainWrite,
}}

// holdAudioLevels puts the running levels back onto a proposal, so what is left differing is
// everything the mixer's shape depends on.
//
// The list is cloned before anything is written to it. A settings value is copied by
// assignment everywhere else in this package, and a slice copied that way shares its
// entries: writing through the copy would move the caller's own settings, which for
// LiveOnly's probe would mean answering a question by changing it.
func holdAudioLevels(from settings.Settings, onto *settings.Settings) {
	onto.Publish.AudioSources = slices.Clone(onto.Publish.AudioSources)
	for i := range onto.Publish.AudioSources {
		if i >= len(from.Publish.AudioSources) {
			break
		}
		onto.Publish.AudioSources[i].Gain = from.Publish.AudioSources[i].Gain
		onto.Publish.AudioSources[i].Mute = from.Publish.AudioSources[i].Mute
	}
}

func init() {
	rules.Register(liveRules()...)
}

// liveRules is the table as rules, so the form's answer and the apply path's answer come
// off one statement. A live rule names no value and carries no reason: it grants the
// control a property rather than replacing it with a sentence (rules.Live).
func liveRules() []rules.Rule {
	out := make([]rules.Rule, 0, len(liveFields))
	for _, f := range liveFields {
		out = append(out, rules.Rule{When: f.when, Verdict: rules.Live, Field: f.key})
	}
	return out
}

// LiveFields is every settings field a pipeline built from s would take a new value for
// while it runs, in table order.
//
// The facts it evaluates against are the three the rows name and no more, which is what
// lets this package answer without the machine's own answers: the engine follows from the
// capture backend, and the codec and the mode are the draft's. A capture backend no
// publisher runs names no engine, which matches no row, so an unbuildable configuration is
// answered as not live rather than guessed at.
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

// LiveOnly reports whether next differs from running in nothing but what a running
// pipeline will take.
//
// It is asked rather than derived from a list of field names: the running settings' live
// values are put back onto the proposal, and if the two then render the same pipeline,
// everything that differs is something the socket can carry. A field no builder reads
// changes neither rendering and is not a reason to relaunch either, which is the same
// answer SamePipeline already gives.
//
// The live set is the running stream's and not the proposal's. What decides whether a
// change can be applied is what the child that is running will accept, and a proposal that
// moves onto a codec with a live bitrate does not make the pipeline that is already
// playing take one.
func LiveOnly(running, next settings.Settings) (bool, error) {
	probe := next
	for _, key := range LiveFields(running) {
		f, ok := liveFieldFor(key)
		assert.Assert(ok, "a live field is one the table declares", key)
		f.hold(running, &probe)
	}
	return SamePipeline(running, probe)
}

// liveFieldFor is the row a key names.
func liveFieldFor(key string) (liveField, bool) {
	for _, f := range liveFields {
		if f.key == key {
			return f, true
		}
	}
	return liveField{}, false
}
