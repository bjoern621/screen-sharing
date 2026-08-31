package form

import (
	"slices"
	"sort"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/text"
	"bjoernblessin.de/screenshare/internal/wire"
)

// The built-in presets: a promise about the picture, and the search that finds a configuration for
// it on this machine (docs/presets.md).
//
// A preset is not a stored set of field values.
// Which encoder, pixel format and capture backend deliver a promise differs per machine: an NVIDIA
// desktop on X11 codes lossless planar RGB, the same desktop on Wayland reaches 4:4:4 and no RGB,
// and a machine with no GPU encoder reaches neither at 60 fps.
// A table of values could only be right where it was written, so the table below states the goal
// and presetResolve walks the same capability tables the rest of this package derives from until
// something reaches it.
//
// A preset crosses as a key and its verdict.
// What it is called and what it delivers are written where the layout is (docs/ipc-api.md).

// presetRung is one step of a preset's quality ladder: the picture it asks for, and whether an
// encoder that comes with a device is the only one allowed to serve it.
//
// The device restriction is what lets a ladder put a lesser picture above a better one on the CPU.
// A rung whose preset needs no such trade takes whichever encoder reaches it.
type presetRung struct {
	chroma   string
	onDevice bool
}

// presetRange is an inclusive bound on one numeric axis of a claim.
// A nil range is an axis the claim leaves alone, which is the preset promising nothing about it.
type presetRange struct {
	min, max int
}

// The open-ended bounds, which are the shapes the claims below need.
// A range with both ends stated is written as a literal.
//
// The unstated end is the widest value the axis can hold rather than a flag, so both operations on
// a claim read min and max without asking first whether each is there.
func presetAtLeast(min int) *presetRange { return &presetRange{min: min, max: presetUnbounded} }
func presetAtMost(max int) *presetRange  { return &presetRange{min: -presetUnbounded, max: max} }

// presetUnbounded stands in for an end a claim does not state.
// Every axis here is a frame rate, a B-frame count or a millisecond window, so a bound this far out
// is one no settings object is outside of.
const presetUnbounded = 1 << 30

// presetClaim is the region of the settings space one preset stands for: every settings object
// inside it delivers what the preset promises, and every object outside it does not.
//
// Two operations read a claim, both walking the axis tables below rather than a condition written
// per preset.
// presetHolds decides whether a preset is still the selected one after a field changed.
// presetOverlaps decides whether two presets could both describe one settings object, which is the
// question the table is held to at load: a surface has one selection to show, so intersecting
// claims are a defect rather than a case to render.
//
// An axis the claim leaves out is unconstrained.
type presetClaim struct {
	// modes, chromas and colorRanges are the rate-control modes, pixel formats and quantization
	// ranges the promise survives, empty where the promise is about none of them.
	modes       []string
	chromas     []string
	colorRanges []string

	fps     *presetRange
	bframes *presetRange
	// srtLatencyMs bounds this leg's SRT retransmit window.
	// A viewer pays the watch hop's window as well, but that one belongs to the machine doing the
	// watching and a preset carries no viewer settings, so the claim is about the half a preset can
	// speak for.
	srtLatencyMs *presetRange
}

// The axes a claim carves the settings space on, one entry per axis: where the value is read from,
// and which part of the claim bounds it.
//
// Both operations below walk these tables, so an axis added to presetClaim cannot be honoured by
// one and missed by the other.
var presetEnumAxes = []struct {
	of      func(settings.Publish) string
	allowed func(presetClaim) []string
}{
	{of: func(p settings.Publish) string { return p.Mode }, allowed: func(c presetClaim) []string { return c.modes }},
	{of: func(p settings.Publish) string { return p.Chroma }, allowed: func(c presetClaim) []string { return c.chromas }},
	{
		of:      func(p settings.Publish) string { return p.ColorRange },
		allowed: func(c presetClaim) []string { return c.colorRanges },
	},
}

var presetRangeAxes = []struct {
	of    func(settings.Publish) int
	bound func(presetClaim) *presetRange
}{
	{of: func(p settings.Publish) int { return p.Fps }, bound: func(c presetClaim) *presetRange { return c.fps }},
	{of: func(p settings.Publish) int { return p.Bframes }, bound: func(c presetClaim) *presetRange { return c.bframes }},
	{
		of:    func(p settings.Publish) int { return p.SrtPublishLatencyMs },
		bound: func(c presetClaim) *presetRange { return c.srtLatencyMs },
	},
}

// preset is one entry of the table: what it promises, what every candidate for it carries, and the
// ladder of pictures it would accept.
type preset struct {
	// key is the identifier the whole app names this preset by, the shell's own label included
	// (api/proto/screenshare/v1/form.proto, BuiltinPreset).
	key   string
	claim presetClaim
	// base writes the fields every candidate carries: the rate-control recipe, the frame rate and
	// this leg's retransmit window.
	// That part is the preset's identity rather than something to search for.
	//
	// It writes rather than replaces, so a field the preset promises nothing about keeps the value
	// the settings hold: a bitrate target means nothing to a lossless encode, and zeroing it would
	// spend the user's number on a field the preset never reads.
	base func(settings.Publish) settings.Publish
	// cq51 is the quantizer target on the anchor scale of 51 points, rescaled to each candidate
	// codec's own.
	// Zero on a preset whose mode targets no quantizer.
	cq51 int
	// rungs is the quality ladder, best first.
	// The search takes the highest rung an encoder here reaches, so the order is where the preset
	// states what it gives up first.
	rungs []presetRung
}

// presetTable is every built-in preset, in the order a surface offers them.
//
// Every claim is disjoint from every other, which the init below holds this table to, so at most
// one of them describes one settings object.
var presetTable = []preset{
	{
		key: "lossless",
		claim: presetClaim{
			modes:   []string{capabilities.ModeLossless},
			chromas: []string{"gbrp", "yuv444p"},
			// The one preset whose promise reaches the quantization range.
			// Coding the desktop into the narrower studio swing throws away code values before the
			// encoder sees them, so a bit-exact encode of a range-converted picture is not what this
			// preset promises.
			// The others say nothing about the range and leave the settings' own where it is.
			colorRanges: []string{capabilities.ColorRangeFull},
		},
		base: func(p settings.Publish) settings.Publish {
			p.Mode = capabilities.ModeLossless
			p.ColorRange = capabilities.ColorRangeFull
			p.Fps = 60
			p.Gop = 0
			p.Bframes = 0
			// Loss on a LAN is near zero, so the window absorbs scheduling jitter rather than a WAN's
			// retransmits.
			p.SrtPublishLatencyMs = 50
			return p
		},
		// Planar RGB is the desktop's own format and reaches the encoder without a colour conversion,
		// and 4:4:4 carries the same detail after one.
		//
		// The CPU rungs run the other way round.
		// A software encoder codes lossless 4:4:4 an order of magnitude faster than it codes lossless
		// RGB, and an encode that cannot keep up with the screen delivers neither format,
		// so the exact format is what this ladder gives up last rather than first.
		rungs: []presetRung{
			{chroma: "gbrp", onDevice: true},
			{chroma: "yuv444p", onDevice: true},
			{chroma: "yuv444p"},
			{chroma: "gbrp"},
		},
	},
	{
		key: "gaming",
		claim: presetClaim{
			modes:        []string{capabilities.ModeCbr, capabilities.ModeVbr, capabilities.ModeAbr, capabilities.ModeCrf},
			fps:          presetAtLeast(60),
			bframes:      presetAtMost(0),
			srtLatencyMs: presetAtMost(250),
		},
		base: func(p settings.Publish) settings.Publish {
			p.Mode = capabilities.ModeCbr
			p.Fps = 60
			p.Gop = 0
			p.Bframes = 0
			p.BitrateM = 40
			// Around six frames of rate buffer at 60 fps: room to carry the target across a scene change,
			// short enough that the buffer adds no delay a player would show.
			p.VbvMs = 100
			p.SrtPublishLatencyMs = 100
			return p
		},
		// Quarter-resolution chroma is the cheapest encode and the one every encoder here codes,
		// which is what holds the frame rate up on motion.
		rungs: []presetRung{{chroma: "yuv420p"}},
	},
	{
		key: "readability",
		claim: presetClaim{
			modes: []string{capabilities.ModeCrf},
			fps:   presetAtMost(30),
		},
		base: func(p settings.Publish) settings.Publish {
			p.Mode = capabilities.ModeCrf
			p.Fps = 30
			p.Gop = 0
			p.Bframes = 2
			p.SrtPublishLatencyMs = 300
			return p
		},
		cq51: 18,
		// Full-resolution chroma keeps the edges of coloured glyphs where they are, and 30 fps of it is
		// within reach of a CPU encoder, so this rung takes whichever encoder codes it.
		// Quarter-resolution chroma still carries full-resolution luma, which is most of what makes
		// text legible, so it is the rung below rather than a reason to be unreachable.
		rungs: []presetRung{{chroma: "yuv444p"}, {chroma: "yuv420p"}},
	},
}

// The table is held to the one property a surface cannot render its way out of: two presets whose
// claims intersect would both describe one settings object, and there is one selection to show.
//
// The claims are written to part on an axis, the rate-control mode telling lossless from the rest
// and the frame rate telling those apart, so a claim widened past its neighbour fails here rather
// than at a surface left to pick one of two right answers.
func init() {
	for i, a := range presetTable {
		assert.Assert(a.key != "", "every preset is named", i)
		assert.Assert(len(a.rungs) > 0, "every preset states which pictures it would accept", a.key)
		assert.IsNotNil(a.base, "every preset states what its candidates carry", a.key)

		for _, b := range presetTable[i+1:] {
			assert.Assert(!presetOverlaps(a.claim, b.claim),
				"preset promises are pairwise disjoint", a.key, b.key)
		}
	}
}

// presetHolds reports whether these settings deliver everything the claim covers.
func presetHolds(p settings.Publish, c presetClaim) bool {
	for _, axis := range presetEnumAxes {
		allowed := axis.allowed(c)
		if len(allowed) > 0 && !slices.Contains(allowed, axis.of(p)) {
			return false
		}
	}
	for _, axis := range presetRangeAxes {
		bound := axis.bound(c)
		if bound == nil {
			continue
		}
		if v := axis.of(p); v < bound.min || v > bound.max {
			return false
		}
	}
	return true
}

// presetOverlaps reports whether some settings object lies in both claims.
// One separating axis is enough for two regions to miss each other, so a pair that shares every
// axis overlaps.
func presetOverlaps(a, b presetClaim) bool {
	for _, axis := range presetEnumAxes {
		x, y := axis.allowed(a), axis.allowed(b)
		if len(x) == 0 || len(y) == 0 {
			continue
		}
		if !slices.ContainsFunc(x, func(v string) bool { return slices.Contains(y, v) }) {
			return false
		}
	}
	for _, axis := range presetRangeAxes {
		x, y := axis.bound(a), axis.bound(b)
		if x == nil || y == nil {
			continue
		}
		if max(x.min, y.min) > min(x.max, y.max) {
			return false
		}
	}
	return true
}

// resolvePresets is every built-in preset against these settings: the settings applying it would
// produce here, or the reason nothing here reaches it, and whether the settings already deliver it.
//
// It runs on the repaired settings, which is what makes rejecting a repaired candidate sound rather
// than merely strict.
// A draft still holding a stranded value would have every candidate moved by the repair, and every
// preset would then be unreachable for a fault none of them has (Resolve repairs first, and
// everything after it describes what that reached).
func resolvePresets(d Deps, s settings.Settings) []*screensharev1.BuiltinPreset {
	out := make([]*screensharev1.BuiltinPreset, 0, len(presetTable))
	selected := 0

	for _, p := range presetTable {
		entry := &screensharev1.BuiltinPreset{Key: p.key, Selected: presetHolds(s.Publish, p.claim)}
		if entry.GetSelected() {
			selected++
		}

		if reached, ok := presetResolve(d, p, s); ok {
			entry.Settings = wire.PublishSettings(reached.Publish)
		} else {
			// The transport is the one dimension the search leaves alone, being how viewers are reached
			// rather than a property of the picture, so a leg whose formats rule out every candidate is
			// what the reason names for the user to act on.
			entry.Reason = text.Of(screensharev1.TextCode_TEXT_CODE_PRESET_UNREACHABLE,
				text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_PRESET, p.key),
				text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_TRANSPORT, s.Publish.Transport))
		}

		assert.Assert((entry.GetSettings() != nil) != (entry.GetReason() != nil),
			"a preset either resolves to settings or says why it does not", p.key)
		out = append(out, entry)
	}

	assert.Assert(len(out) == len(presetTable), "an entry per preset", len(out), len(presetTable))
	assert.Assert(selected <= 1, "settings deliver at most one preset's promise", selected)
	return out
}

// presetResolve is the settings applying this preset produces here, and false where nothing this
// machine runs delivers its promise.
//
// Rungs are walked in order, each rung against every codec and each codec against every capture
// backend, and the first candidate the repair leaves standing is the answer.
// Rung above codec above capture backend is what makes the ladder the preset's statement of what it
// gives up: the search changes encoder, then capture backend, to stay on a rung it can still reach.
//
// A candidate whose own rung, codec or capture backend the repair walked is a different
// configuration under the same name, so it is rejected and the next one tried (presetKeeps).
// Nothing is approximated: such a near-miss would be a configuration the user did not ask for
// wearing the name of one they did.
//
// A candidate is taken only where it delivers the claim as well, so a base contradicting the
// preset's own promise leaves the preset unreachable rather than applying something a surface would
// immediately stop marking as selected.
//
// Applying twice equals applying once: what this returns is itself the candidate the next search
// reaches first, the rung, codec and capture backend that produced it being the ones it tries
// first.
func presetResolve(d Deps, p preset, s settings.Settings) (settings.Settings, bool) {
	captures := presetCaptures(d, s)

	for _, rung := range p.rungs {
		for _, codec := range presetCodecs(rung) {
			for _, capture := range captures {
				candidate := s
				candidate.Publish = p.base(s.Publish)
				candidate.Publish.Chroma = rung.chroma
				candidate.Publish.Format, candidate.Publish.Encoder = codec.Format, codec.Encoder()
				candidate.Publish.Capture = capture
				// The ladder steps are the codec's own identifiers, so they are written here rather than by
				// the base above: a base naming one would carry it onto every other candidate, where the
				// repair moves it and the candidate is rejected for having been repaired.
				// What each mode is worth running at is the row's answer, the same one a fresh installation
				// gets.
				candidate.Publish.Effort, candidate.Publish.Tune =
					settings.LadderSteps(codec.Name, candidate.Publish.Mode)
				if p.cq51 > 0 {
					// The quantizer scale is the codec's on the engine that drives it, and the capture backend
					// fixes that engine, so the target is placed inside this loop rather than above it.
					candidate.Publish.Cq = presetCq(p.cq51, codec.Name, optionEngineOf(candidate))
				}

				if presetStrands(d, candidate) {
					continue
				}

				reached, repaired := Repair(d, candidate)
				if !presetKeeps(repaired) || !presetHolds(reached.Publish, p.claim) {
					continue
				}
				// A preset states a configuration this machine encodes, so one the elements cannot express
				// is not a find.
				// The claim and the repair answer for the settings against each other; what no table
				// states is the pair a single element cannot hold, a VBR ceiling more than twice its
				// target being the case that exists.
				// The encoder alone, not a rendered command: a command carries the transport too, and a
				// group key that will not read back would take away a preset that says nothing about how
				// the stream is carried (publish.EncoderRefusal).
				if err := publish.EncoderRefusal(reached); err != nil {
					continue
				}
				return reached, true
			}
		}
	}
	return settings.Settings{}, false
}

// presetKeeps reports whether the repair left this candidate the one the search asked for.
//
// A walk on one of the axes the search wrote disqualifies it, those being the candidate's own
// answer.
// So does a walk outside the publish group: a preset carries publish settings and nothing else
// (docs/presets.md), so a configuration reachable only by moving how this machine watches is one the
// preset cannot deliver.
//
// Every other field arrived holding what the settings held rather than something the preset chose,
// so the repair's answer for it is the machine's own.
// A quantization range the running encoder signals nothing for is walked there, and a preset that
// promised nothing about the range is still itself afterwards.
// What the promise does cover is presetHolds' question, asked of the repaired settings.
func presetKeeps(repaired []string) bool {
	for _, key := range repaired {
		if slices.Contains(presetSearchedKeys, keyTemplate(key)) {
			return false
		}
		if !strings.HasPrefix(key, settingsGroupPublish+keySeparator) {
			return false
		}
	}
	return true
}

// presetStrands reports whether the repair would already walk one of the axes this candidate names
// off the value the search put there, which is what makes it a rejected candidate.
//
// It decides nothing the repair does not: it asks legalOption, the function the walk itself asks,
// about the fields the search sets.
// What it saves is the rest of the walk, a fixed point over every field in the table several rounds
// deep, for a candidate whose own encoder or pixel format is already gone.
// The ladder is a few hundred candidates long, and a machine that reaches no preset would otherwise
// pay for a repair per candidate on every keystroke.
//
// legalOption rather than optionState is what keeps the two in step.
// An entry the form greys is not always one the repair moves: a field whose every entry is greyed
// keeps the value it has, and a candidate dropped for that would report a preset unreachable for an
// axis the repair would have left alone.
func presetStrands(d Deps, candidate settings.Settings) bool {
	// One evaluation for every field asked about: the candidate does not move between them, and the
	// search runs this over a few hundred candidates on every keystroke.
	av := availabilityOf(d, candidate)

	for _, key := range presetSearchedKeys {
		f := presetField(key)
		if _, walked := legalOption(av, d, candidate, f, optionValue(f.value(candidate)), noEntry); walked {
			return true
		}
	}
	return false
}

// presetSearchedKeys are the axes the search itself writes: the rung's pixel format, the encode it
// is tried on, both halves of it, and the capture backend under it.
//
// A field the base writes is left out of this list.
// The base writes only what the claim speaks for, so a walk off one of those is a walk out of the
// claim, which presetHolds catches on the repaired settings, and a walk inside it is a value the
// promise covers: a preset that names four rate-control modes is still itself on the second of them.
// Gating here on such a field would reject a candidate the promise accepts, and a machine gapped on
// the base's first choice would fall past every encoder it has.
var presetSearchedKeys = []string{KeyCapture, KeyFormat, KeyEncoder, KeyChroma}

func presetField(key string) *field {
	for i := range fieldTable {
		if fieldTable[i].key == key {
			return &fieldTable[i]
		}
	}
	assert.Never("a searched field is a row of the field table", key)
	return nil
}

// presetCodecs is the codecs a preset tries at one rung, best first: the encoders that come with a
// device before the ones that come with a build, and coding efficiency inside each half,
// in opposite directions.
//
// An encoder on fixed-function silicon leaves the machine free to run whatever is being captured,
// which is what every preset here is for, so one is taken wherever it reaches the rung.
//
// Efficiency orders each half, a format spending fewer bits by searching more for them.
// On dedicated silicon that search costs nothing, so the most efficient format wins; on a CPU the
// frame rate pays for it, so the cheapest wins and a desktop encode never lands on the slowest
// encoder in the table.
// Codecs that tie keep the capability table's order.
//
// What this machine has is not filtered here.
// A codec no encoder was found for is greyed by availability and the repair walks the candidate off
// it, which is the same verdict arrived at by the one rule that owns it.
func presetCodecs(rung presetRung) []capabilities.Codec {
	type candidate struct {
		row    capabilities.Codec
		device bool
		// rank orders one half.
		// The bits a format spends run one way on silicon and the other on a CPU, so the sign is what
		// states which half this row is in rather than a second comparison in the sort.
		rank float64
	}

	var ranked []candidate
	for _, c := range capabilities.Codecs {
		if !c.Implemented {
			continue
		}
		device := presetOnDevice(c)
		if rung.onDevice && !device {
			continue
		}
		bits := estimateEfficiency[c.Format]
		if !device {
			bits = -bits
		}
		ranked = append(ranked, candidate{row: c, device: device, rank: bits})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if a.device != b.device {
			return a.device
		}
		return a.rank < b.rank
	})

	out := make([]capabilities.Codec, 0, len(ranked))
	for _, c := range ranked {
		out = append(out, c.row)
	}
	return out
}

// presetOnDevice reports whether this codec's encoders come with a device rather than with a build.
// The answer is availabilityFamilies' column, read rather than restated here.
func presetOnDevice(c capabilities.Codec) bool {
	family, ok := availabilityFamilies[c.Family]
	assert.Assert(ok, "every encoder family states where its encoders come from", c.Family)
	return family.needsDevice
}

// presetCaptures is the capture backends a preset tries, the selected one first.
//
// A configuration reachable without changing the backend is therefore the one taken, and the
// compositor's picker is not raised for a preset that had no need of it.
// The rest are the backends this platform runs that need no privilege granted first: one behind a
// privilege stays selectable by hand and is never picked on the user's behalf, the failure it
// produces being a capture that dies at launch for a reason no form mentioned.
func presetCaptures(d Deps, s settings.Settings) []string {
	out := []string{s.Publish.Capture}
	for _, capture := range publish.AutoCaptures(d.Platform) {
		if capture != s.Publish.Capture {
			out = append(out, capture)
		}
	}
	return out
}

// presetCq places a preset's quantizer target on the scale the named codec counts on, against the
// anchor scale of 51 points the target is stated on.
//
// The scale is the running engine's, the two engines setting different properties and one counting
// further than the other.
// A codec whose scale this engine declares none for keeps the target where it stands, there being
// no ratio to convert it by, which is the answer the bitrate prediction gives the same question
// (estimate.go).
func presetCq(cq51 int, codec, engine string) int {
	c, ok := capabilities.Get(codec)
	if !ok {
		return cq51
	}
	scale := c.CqMaxOn(engine)
	if scale <= 0 {
		return cq51
	}
	return cq51 * scale / estimateAnchorCqMax
}
