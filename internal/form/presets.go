package form

import (
	"slices"
	"sort"

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
// Which encoder, pixel format and capture backend deliver a promise differs per machine - an NVIDIA
// desktop on X11 codes lossless planar RGB, the same desktop on Wayland reaches 4:4:4 and no RGB,
// a machine with no GPU encoder reaches neither at 60 fps - so a table of values could only be
// right where it was written.
// The table below therefore states the goal, and presetResolve walks the same capability tables the
// rest of this package derives from until something reaches it.
//
// This is where the Wails frontend's util/presets.ts and util/presetSearch.ts went,
// for the reason every other rule moved here: three shells held three copies of it
// (docs/ipc-api.md).
// What stayed on the surface is every word - a preset crosses as a key and its verdict,
// and what it is called and what it delivers are written where the layout is.

// presetRung is one step of a preset's quality ladder: the picture it asks for,
// and whether an encoder that comes with a device is the only one allowed to serve it.
//
// The device restriction is what lets a ladder put a lesser picture above a better one on the CPU.
// Where a preset's own trade does not need that, the rung takes whichever encoder reaches it.
type presetRung struct {
	chroma   string
	onDevice bool
}

// presetRange is an inclusive bound on one numeric axis of a claim.
// A nil range is an axis the claim leaves alone, which is the preset making no promise about it.
type presetRange struct {
	min, max int
}

// The two open-ended bounds, which are the only shapes the claims below need.
// A range with both ends stated would be written as a literal.
//
// The unstated end is the widest value the axis can hold rather than a flag,
// so both operations on a claim read min and max without asking first whether each is there.
func presetAtLeast(min int) *presetRange { return &presetRange{min: min, max: presetUnbounded} }
func presetAtMost(max int) *presetRange  { return &presetRange{min: -presetUnbounded, max: max} }

// presetUnbounded stands in for an end a claim does not state.
// Every axis here is a frame rate, a B-frame count or a millisecond window,
// so a bound this far out is one no settings object can be outside of.
const presetUnbounded = 1 << 30

// presetClaim is the region of the settings space one preset stands for: every settings object
// inside it delivers what the preset promises, and every object outside it does not.
//
// Two operations read a claim, and both walk the axis tables below rather than a condition written
// per preset.
// presetHolds decides whether a preset is still the selected one after a field changed.
// presetOverlaps decides whether two presets could ever both describe one settings object,
// which is the question the table is held to at load: a surface has one selection to show,
// so two claims that intersect are a defect rather than a case to render.
//
// An axis the claim leaves out is unconstrained.
type presetClaim struct {
	// modes, chromas and colorRanges are the rate-control modes, pixel formats and quantization ranges
	// the promise survives, empty where the promise is about none of them.
	modes       []string
	chromas     []string
	colorRanges []string

	fps     *presetRange
	bframes *presetRange
	// srtLatencyMs bounds this leg's SRT retransmit window.
	// The watch hop holds a window of its own and a viewer pays both, but that one belongs to the
	// machine doing the watching and a preset carries no viewer settings, so the claim is about the
	// half a preset can speak for.
	srtLatencyMs *presetRange
}

// The axes a claim carves the settings space on, one entry per axis: where the value comes from,
// and which part of the claim bounds it.
//
// Both operations below read these tables, so an axis added to presetClaim cannot be honoured by
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

// preset is one entry of the table: what it promises, what every candidate for it carries,
// and the ladder of pictures it would accept.
type preset struct {
	// key is the identifier the whole app names this preset by, the shell's own label included
	// (api/proto/screenshare/v1/form.proto, BuiltinPreset).
	key   string
	claim presetClaim
	// base writes the fields every candidate carries: the rate-control recipe,
	// the frame rate and this leg's retransmit window.
	// That part is the preset's identity rather than something to search for.
	//
	// It writes rather than replaces, so a field the preset makes no promise about keeps the value the
	// settings already hold - a bitrate target means nothing to a lossless encode,
	// and zeroing it would spend the user's number on a field the preset never reads.
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
// Every claim is disjoint from every other, which presetInit holds this table to,
// so at most one of them ever describes one settings object.
var presetTable = []preset{
	{
		key: "lossless",
		claim: presetClaim{
			modes:   []string{capabilities.ModeLossless},
			chromas: []string{"gbrp", "yuv444p"},
			// The one preset whose promise reaches the quantization range.
			// Coding the desktop into the narrower studio swing throws away code values before the encoder
			// ever sees them, so a bit-exact encode of a range-converted picture is not the promise this
			// preset makes.
			// The other two say nothing about the range and leave the settings' own where it is.
			colorRanges: []string{capabilities.ColorRangeFull},
		},
		base: func(p settings.Publish) settings.Publish {
			p.Mode = capabilities.ModeLossless
			p.ColorRange = capabilities.ColorRangeFull
			p.Fps = 60
			p.Gop = 0
			p.Bframes = 0
			// Loss on a LAN is near zero, so the window only has to absorb scheduling jitter rather than a
			// WAN's retransmits.
			p.SrtPublishLatencyMs = 60
			return p
		},
		// Planar RGB is the desktop's own format and reaches the encoder without a colour conversion;
		// 4:4:4 carries the same detail after one.
		//
		// The two CPU rungs run the other way round.
		// A software encoder codes lossless 4:4:4 an order of magnitude faster than it codes lossless
		// RGB, and an encode that cannot keep up with the screen delivers neither format,
		// so the exact one is what this ladder gives up last rather than first.
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
			// Around six frames of rate buffer at 60 fps: room for the encoder to carry the target across a
			// scene change, short enough that the buffer itself adds no delay a player would show.
			p.VbvMs = 100
			p.SrtPublishLatencyMs = 100
			return p
		},
		// Quarter-resolution chroma is the cheapest encode and the one every encoder here codes,
		// which is what keeps the frame rate up on motion.
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
		// Full-resolution chroma keeps the edges of coloured glyphs where they are,
		// and 30 fps of it is within reach of a CPU encoder, so this rung takes whichever encoder codes
		// it.
		// Quarter-resolution chroma still carries full-resolution luma, which is most of what makes text
		// legible, so it is the rung below rather than a reason to be unavailable.
		rungs: []presetRung{{chroma: "yuv444p"}, {chroma: "yuv420p"}},
	},
}

// presetInit holds the table to the one property a surface cannot render its way out of:
// two presets whose claims intersect would both describe one settings object,
// and there is one selection to show.
//
// The claims are written to part on an axis - the rate-control mode tells lossless from the other
// two, and the frame rate tells those two apart - and this is where a claim widened past its
// neighbour fails, rather than at a surface left to pick one of two right answers.
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

// presetHolds reports whether these settings deliver what the claim covers.
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
// Two regions miss each other as soon as one axis separates them, so a pair that shares every axis
// overlaps.
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

// resolvePresets is every built-in preset against these settings: what applying it would produce
// here, why nothing here reaches it, and whether the settings already deliver it.
//
// The settings are the repaired ones, which is what makes the search sound rather than merely
// convenient.
// A candidate is kept only if the repair leaves it untouched, so a draft that still held a stranded
// value would have every candidate rejected for a fault none of the presets has (Resolve repairs
// first, and everything after describes what it reached).
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
			// The transport is named because it is the one dimension the search leaves alone:
			// it is how viewers are reached rather than a property of the picture, so a preset never moves
			// it, and a leg whose formats rule out every candidate is the thing the user can act on.
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
// The ladder is walked rung by rung, each rung against every codec and each codec against every
// capture backend, and the first candidate that survives the repair intact is the answer.
// Rung above codec above capture backend is what makes the ladder the preset's statement of what it
// gives up: the search changes encoder, and then capture backend, to stay on a rung it can still
// reach.
//
// A candidate the repair had to touch is a different configuration under the same name,
// so it is rejected and the next one tried.
// Nothing is approximated: a repaired near-miss would be a configuration the user did not ask for
// wearing the name of one they did.
//
// A candidate is taken only when it also delivers the claim, so a base that contradicts the
// preset's own promise leaves the preset unreachable rather than applying something a surface would
// immediately stop marking as selected.
//
// Applying twice equals applying once: the settings this returns are themselves the candidate the
// next search reaches first, since the rung, codec and capture backend that produced them are the
// ones it tries first.
func presetResolve(d Deps, p preset, s settings.Settings) (settings.Settings, bool) {
	captures := presetCaptures(d, s)

	for _, rung := range p.rungs {
		for _, codec := range presetCodecs(rung) {
			for _, capture := range captures {
				candidate := s
				candidate.Publish = p.base(s.Publish)
				candidate.Publish.Chroma = rung.chroma
				candidate.Publish.Codec = codec
				candidate.Publish.Capture = capture
				// The two ladder steps are the codec's own, so they are placed inside this loop rather than
				// written by the base above: a step is the encoder's identifier, and a base naming one would
				// carry it onto every other candidate, where the repair moves it and the candidate is rejected
				// for having been repaired.
				// What each mode is worth running at is the row's, which is the same answer a fresh
				// installation gets.
				candidate.Publish.Effort, candidate.Publish.Tune =
					settings.LadderSteps(codec, candidate.Publish.Mode)
				if p.cq51 > 0 {
					// The quantizer scale is the codec's on the engine that will drive it, and the capture backend
					// fixes that engine, so the target is placed inside this loop rather than above it.
					candidate.Publish.Cq = presetCq(p.cq51, codec, optionEngineOf(candidate))
				}

				if presetStrands(d, candidate) {
					continue
				}

				reached, repaired := Repair(d, candidate)
				if len(repaired) == 0 && presetHolds(reached.Publish, p.claim) {
					return reached, true
				}
			}
		}
	}
	return settings.Settings{}, false
}

// presetStrands reports whether the repair would already walk one of the five fields this candidate
// names off the value the search put there, which is what makes the candidate a rejected one.
//
// It decides nothing the repair does not decide: it asks legalOption, the same function the walk
// asks, about the fields the search sets.
// What it saves is the rest of the walk - a fixed point over every field in the table,
// several rounds deep - for a candidate whose own encoder or pixel format is already gone.
// A machine that reaches a preset pays for one repair; one that reaches none used to pay for a
// repair per candidate, and the ladder is a few hundred candidates long.
//
// Asking legalOption rather than optionState is what keeps the two in step.
// An entry the form greys is not always one the repair moves: a field whose every entry is greyed
// keeps the value it has, which is the case a colour range on planar RGB is in,
// and a candidate dropped for that would be a preset reported unreachable for a field the repair
// would have left alone.
func presetStrands(d Deps, candidate settings.Settings) bool {
	for _, key := range presetSearchedKeys {
		f := presetField(key)
		if _, walked := legalOption(d, candidate, f, optionValue(f.value(candidate)), noEntry); walked {
			return true
		}
	}
	return false
}

// presetSearchedKeys are the fields a candidate names: the three the search varies and the two the
// base writes that a codec can be gapped on.
// A field the base writes that no gap can take away needs no gate, since nothing would walk it.
var presetSearchedKeys = []string{KeyCapture, KeyCodec, KeyChroma, KeyMode, KeyColorRange}

// presetField is one row of the field table, by key.
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
// Efficiency then orders each half, because a format spends fewer bits by searching more for them.
// On dedicated silicon that search costs nothing, so the most efficient format wins;
// on a CPU it is the frame rate that pays for it, so the cheapest one wins and the ladder does not
// hand a desktop encode to the slowest encoder in the table.
// Codecs that tie keep the capability table's order.
//
// What this machine actually has is not filtered here.
// A codec no encoder was found for is greyed by availability, and the repair then walks the
// candidate off it, which is the same verdict arrived at by the one rule that owns it.
func presetCodecs(rung presetRung) []string {
	type candidate struct {
		name   string
		device bool
		// rank orders one half.
		// The bits a format spends run one way on silicon and the other way on a CPU,
		// so the sign is what states which half this row is in rather than a second comparison in the
		// sort.
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
		ranked = append(ranked, candidate{name: c.Name, device: device, rank: bits})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if a.device != b.device {
			return a.device
		}
		return a.rank < b.rank
	})

	out := make([]string, 0, len(ranked))
	for _, c := range ranked {
		out = append(out, c.name)
	}
	return out
}

// presetOnDevice reports whether this codec's encoders come with a device rather than with a build.
// It is availabilityFamilies' own column, read rather than restated.
func presetOnDevice(c capabilities.Codec) bool {
	family, ok := availabilityFamilies[c.Family]
	assert.Assert(ok, "every encoder family states where its encoders come from", c.Family)
	return family.needsDevice
}

// presetCaptures is the capture backends a preset tries, the selected one first.
//
// A configuration reachable without changing the backend is therefore the one taken,
// and the compositor's picker is not raised for a preset that had no need of it.
// The rest are the ones this platform runs that need no privilege granted first:
// a backend behind one stays selectable by hand and is never picked on the user's behalf,
// because the failure it produces is a capture that dies at launch for a reason no form mentioned.
func presetCaptures(d Deps, s settings.Settings) []string {
	out := []string{s.Publish.Capture}
	for _, capture := range publish.AutoCaptures(d.Platform) {
		if capture != s.Publish.Capture {
			out = append(out, capture)
		}
	}
	return out
}

// presetCq places a preset's quantizer target on the scale the named codec counts on,
// against the anchor scale of 51 points the target is stated on.
//
// The scale is the running engine's, since the two engines set different properties and one may
// count further than the other.
// A codec whose scale this engine declares none for keeps the target where it stands,
// there being no ratio to convert it by - which is the same answer the bitrate prediction gives the
// same question (estimate.go).
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
