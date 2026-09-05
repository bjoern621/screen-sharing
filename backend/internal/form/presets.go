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

// The built-in presets: a promise about the picture, and the search that finds a configuration
// for it on this machine (docs/presets.md).
//
// A preset is not a stored set of field values.
// Which encoder, pixel format and capture backend deliver a promise differs per machine:
// an NVIDIA desktop on X11 codes lossless planar RGB, the same desktop on Wayland reaches 4:4:4
// and no RGB, and a machine with no GPU encoder reaches neither at 60 fps.
// A table of values could only be right where it was written, so the table below states the goal
// and presetResolve walks the same capability tables the rest of this package derives
// from until something reaches it.
//
// A preset crosses as a key and its verdict.
// What it is called and what it delivers are written where the layout is (docs/ipc-api.md).

// presetRung is one step of a preset's quality ladder: the picture it asks for,
// and whether an encoder that comes with a device is the only one allowed to serve it.
//
// The device restriction is what lets a ladder put a lesser picture above a better one on the CPU.
// A rung whose preset needs no such trade takes whichever encoder reaches it.
type presetRung struct {
	chroma   string
	onDevice bool
	// fps overrides the base's frame rate on this rung, zero keeping it.
	// How a ladder trades motion away where the encoders left to it cannot keep up.
	fps int
	// maxHeight closes this rung to machines whose encode is taller, zero keeping it open.
	// The height is the picture the encoder is fed (presetEncodeHeight),
	// so a rung can admit a software encode at desktop sizes and refuse it at display-wall ones.
	maxHeight int
}

// presetRange is an inclusive bound on one numeric axis of a claim.
// A nil range is an axis the claim leaves alone, which is the preset promising nothing about it.
type presetRange struct {
	min, max int
}

// The open-ended bounds, which are the shapes the claims below need.
// A range with both ends stated is written as a literal.
//
// The unstated end is the widest value the axis can hold rather than a flag,
// so both operations on a claim read min and max without asking first whether each is there.
func presetAtLeast(min int) *presetRange { return &presetRange{min: min, max: presetUnbounded} }
func presetAtMost(max int) *presetRange  { return &presetRange{min: -presetUnbounded, max: max} }

// presetUnbounded stands in for an end a claim does not state.
// Every axis here is a frame rate, a B-frame count or a millisecond window,
// so a bound this far out is one no settings object is outside of.
const presetUnbounded = 1 << 30

// presetClaim is the region of the settings space one preset stands for:
// every settings object inside it delivers what the preset promises,
// and every object outside it does not.
//
// presetHolds reads it, walking the axis tables below rather than a condition written per preset,
// and asks it of the settings a search reached:
// a candidate the repair walked out of the claim is one that stopped delivering the promise.
// Which preset is selected is the settings' own statement (settings.Publish.Preset),
// so two claims may overlap: balanced and gaming meet at 60 fps VBR,
// and the stored key says which of them the user asked for.
//
// An axis the claim leaves out is unconstrained.
type presetClaim struct {
	// modes, chromas and colorRanges are the rate-control modes, pixel formats
	// and quantization ranges the promise survives, empty where the promise is about none of them.
	modes       []string
	chromas     []string
	colorRanges []string

	fps     *presetRange
	bframes *presetRange
	// srtLatencyMs bounds this leg's SRT retransmit window.
	// A viewer pays the watch hop's window as well, but that one belongs to the machine doing
	// the watching and a preset carries no viewer settings,
	// so the claim is about the half a preset can speak for.
	srtLatencyMs *presetRange
}

// The axes a claim carves the settings space on, one entry per axis: where the value is read from,
// and which part of the claim bounds it.
//
// presetHolds walks these tables, so an axis added to presetClaim cannot be missed by it.
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
	// base writes the fields every candidate carries: the rate-control recipe, the frame rate
	// and this leg's retransmit window.
	// That part is the preset's identity rather than something to search for.
	//
	// It writes rather than replaces, so a field the preset promises nothing about keeps the value
	// the settings hold: a bitrate target means nothing to a lossless encode,
	// and zeroing it would spend the user's number on a field the preset never reads.
	base func(settings.Publish) settings.Publish
	// cq51 is the quantizer target on the anchor scale of 51 points,
	// rescaled to each candidate codec's own.
	// Zero on a preset whose mode targets no quantizer.
	cq51 int
	// formats narrows the search to these bitstreams, empty searching every codec.
	// For a preset whose promise is reach rather than efficiency:
	// balanced pins H.264 because every transport carries it and every viewer decodes it,
	// where the efficiency order would spend that compatibility on a better codec.
	formats []string
	// rungs is the quality ladder, best first.
	// The search takes the highest rung an encoder here reaches,
	// so the order is where the preset states what it gives up first.
	rungs []presetRung
}

// presetTable is every built-in preset, in the order a surface offers them.
//
// Every key is named once, which the init below holds this table to,
// so the key the settings follow addresses at most one row.
var presetTable = []preset{
	{
		key: settings.PresetBalanced,
		claim: presetClaim{
			// A bounded rate is the promise, whichever control delivers it:
			// the GStreamer software elements run no constrained VBR (gstNoRateCeiling),
			// so a claim on vbr alone would strand the preset on every machine
			// whose only encode is one of them.
			modes: []string{capabilities.ModeVbr, capabilities.ModeCbr, capabilities.ModeAbr},
			fps:   &presetRange{min: 30, max: 60},
		},
		// H.264 alone: the one bitstream every transport row carries and every viewer decodes,
		// which is the reach a first stream is judged by.
		formats: []string{"h264"},
		base: func(p settings.Publish) settings.Publish {
			p.Mode = capabilities.ModeVbr
			p.Fps = 60
			p.Gop = 0
			p.Bframes = 0
			p.BitrateM = balancedBitrate(p)
			// Written beside the target rather than left to the draft:
			// the VA elements express the burst as a share of the target
			// and refuse a ceiling far above it.
			p.MaxrateM = p.BitrateM + p.BitrateM/2
			p.VbvMs = 0
			p.SrtPublishLatencyMs = settings.SrtRelayFloorMs
			return p
		},
		// One picture, three admissions: silicon first at full motion,
		// then a CPU where the desktop is small enough for one to keep up,
		// then a CPU at half the motion.
		// Quarter-resolution chroma throughout, the cheapest encode and the one every encoder codes.
		rungs: []presetRung{
			{chroma: "yuv420p", onDevice: true},
			{chroma: "yuv420p", maxHeight: 1440},
			{chroma: "yuv420p", fps: 30},
		},
	},
	{
		key: "lossless",
		claim: presetClaim{
			modes:   []string{capabilities.ModeLossless},
			chromas: []string{"gbrp", "yuv444p"},
			// The one preset whose promise reaches the quantization range.
			// Coding the desktop into the narrower studio swing throws away code values before the encoder
			// sees them, so a bit-exact encode of a range-converted
			// picture is not what this preset promises.
			// The others say nothing about the range and leave the settings' own where it is.
			colorRanges: []string{capabilities.ColorRangeFull},
		},
		base: func(p settings.Publish) settings.Publish {
			p.Mode = capabilities.ModeLossless
			p.ColorRange = capabilities.ColorRangeFull
			p.Fps = 60
			p.Gop = 0
			p.Bframes = 0
			// The relay's own floor, the smallest window the hop runs at:
			// a smaller figure here would be raised on the wire.
			p.SrtPublishLatencyMs = settings.SrtRelayFloorMs
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
			p.SrtPublishLatencyMs = settings.SrtRelayFloorMs
			return p
		},
		// Quarter-resolution chroma is the cheapest encode and the one every encoder here codes,
		// so the frame rate holds up on motion.
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
		// and 30 fps of it is within reach of a CPU encoder,
		// so this rung takes whichever encoder codes it.
		// Quarter-resolution chroma still carries full-resolution luma,
		// which is most of what makes text legible,
		// so it is the rung below rather than a reason to be unreachable.
		rungs: []presetRung{{chroma: "yuv444p"}, {chroma: "yuv420p"}},
	},
}

// The table is held to what the stored selection stands on:
// the settings name their preset by key, so a key written twice would make one name
// address two promises, and no surface could say which one the user asked for.
func init() {
	for i, a := range presetTable {
		assert.Assert(a.key != "", "every preset is named", i)
		assert.Assert(len(a.rungs) > 0, "every preset states which pictures it would accept", a.key)
		assert.IsNotNil(a.base, "every preset states what its candidates carry", a.key)

		for _, b := range presetTable[i+1:] {
			assert.Assert(a.key != b.key, "every preset key names one promise", a.key)
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

// resolvePresets is every built-in preset against these settings:
// the settings applying it would produce here, or the reason nothing here reaches it,
// and whether the settings follow it.
//
// It runs on the repaired settings,
// so rejecting a repaired candidate is sound rather than merely strict.
// A draft still holding a stranded value would have every candidate moved by the repair,
// and every preset would then be unreachable for a fault none of them has (Resolve repairs first,
// and everything after it describes what that reached).
func resolvePresets(d Deps, s settings.Settings) []*screensharev1.BuiltinPreset {
	out := make([]*screensharev1.BuiltinPreset, 0, len(presetTable))
	selected := 0

	for _, p := range presetTable {
		// Selected is the settings' own statement rather than a reading off the values:
		// two claims may overlap, and the key says which promise the user asked for.
		entry := &screensharev1.BuiltinPreset{Key: p.key, Selected: s.Publish.Preset == p.key}
		if entry.GetSelected() {
			selected++
		}

		if reached, ok := presetResolve(d, p, s); ok {
			entry.Settings = wire.PublishSettings(reached.Publish)
		} else {
			// The transport is the one dimension the search leaves alone,
			// being how viewers are reached rather than a property of the picture,
			// so a leg whose formats rule out every candidate
			// is what the reason names for the user to act on.
			entry.Reason = text.Of(screensharev1.TextCode_TEXT_CODE_PRESET_UNREACHABLE,
				text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_PRESET, p.key),
				text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_TRANSPORT, s.Publish.Transport))
		}

		assert.Assert((entry.GetSettings() != nil) != (entry.GetReason() != nil),
			"a preset either resolves to settings or says why it does not", p.key)
		out = append(out, entry)
	}

	assert.Assert(len(out) == len(presetTable), "an entry per preset", len(out), len(presetTable))
	assert.Assert(selected <= 1, "settings follow at most one preset", selected)
	return out
}

// presetResolve is the settings applying this preset produces here,
// and false where nothing this machine runs delivers its promise.
//
// Rungs are walked in order, each rung against every codec and each codec against every capture
// backend, and the first candidate the repair leaves standing is the answer.
// Rung above codec above capture backend is what makes the ladder the preset's statement
// of what it gives up: the search changes encoder, then capture backend,
// to stay on a rung it can still reach.
//
// A candidate whose own rung, codec or capture backend the repair walked is a different
// configuration under the same name, so it is rejected and the next one tried (presetKeeps).
// Nothing is approximated: such a near-miss would be a configuration the user did not ask
// for wearing the name of one they did.
//
// A candidate is taken only where it delivers the claim as well,
// so a base contradicting the preset's own promise leaves the preset unreachable rather
// than applying something a surface would immediately stop marking as selected.
//
// Applying twice equals applying once: what this returns is itself the candidate the next search
// reaches first, the rung, codec and capture backend
// that produced it being the ones it tries first.
func presetResolve(d Deps, p preset, s settings.Settings) (settings.Settings, bool) {
	captures := presetCaptures(d, s)

	for _, rung := range p.rungs {
		if rung.maxHeight > 0 && presetEncodeHeight(d, s) > rung.maxHeight {
			continue
		}
		for _, codec := range presetCodecs(p, rung) {
			for _, capture := range captures {
				candidate := s
				candidate.Publish = p.base(s.Publish)
				// The find follows the preset that produced it,
				// so applying it is what selects the preset,
				// and a start on it resolves the same promise again.
				candidate.Publish.Preset = p.key
				candidate.Publish.Chroma = rung.chroma
				if rung.fps > 0 {
					candidate.Publish.Fps = rung.fps
				}
				candidate.Publish.Format, candidate.Publish.Encoder = codec.Format, codec.Encoder()
				candidate.Publish.Capture = capture
				// The ladder steps are the codec's own identifiers, so they are written here rather
				// than by the base above: a base naming one would carry it onto every other candidate,
				// where the repair moves it and the candidate is rejected for having been repaired.
				// What each mode is worth running at is the row's answer,
				// the same one a fresh installation gets.
				candidate.Publish.Effort, candidate.Publish.Tune =
					settings.LadderSteps(codec.Name, candidate.Publish.Mode)
				if p.cq51 > 0 {
					// The quantizer scale is the codec's on the engine that drives it,
					// and the capture backend fixes that engine,
					// so the target is placed inside this loop rather than above it.
					candidate.Publish.Cq = presetCq(p.cq51, codec.Name, optionEngineOf(candidate))
				}

				if presetStrands(d, candidate) {
					continue
				}

				reached, repaired := Repair(d, candidate)
				if !presetKeeps(repaired) || !presetHolds(reached.Publish, p.claim) {
					continue
				}
				// A preset states a configuration this machine encodes,
				// so one the elements cannot express is not a find.
				// The claim and the repair answer for the settings against each other;
				// what no table states is the pair a single element cannot hold,
				// a VBR ceiling more than twice its target being the case that exists.
				// The encoder alone, not a rendered command: a command carries the transport too,
				// and a group key that will not read back would take away a preset that says nothing about how
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
// A walk on one of the axes the search wrote disqualifies it,
// those being the candidate's own answer.
// So does a walk outside the publish group: a preset carries publish settings
// and nothing else (docs/presets.md), so a configuration reachable only by moving how this machine
// watches is one the preset cannot deliver.
//
// Every other field arrived holding what the settings held rather than something the preset chose,
// so the repair's answer for it is the machine's own.
// A quantization range the running encoder signals nothing for is walked there,
// and a preset that promised nothing about the range is still itself afterwards.
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

// presetStrands reports whether one of the axes the search wrote cannot run as this candidate
// holds it: the entry is ruled out on this machine, by the probe, an engine gap
// or the leg's carriage.
//
// The option's own state is asked rather than whether the repair would walk it.
// The walk keeps a value whose every alternative is also ruled out,
// so a machine whose probe refused a whole format would hand the search a candidate
// that dies at launch, wearing the name of a preset that promised it runs.
// The repair's answer is narrower on exactly that candidate, and only there:
// a ruled-out value it would walk is ruled out here too.
//
// It also spares the rest of the walk, a fixed point over every field several rounds deep,
// for a candidate whose own encoder is already gone.
// The ladder is a few hundred candidates long, and a machine that reaches no preset would otherwise
// pay for a repair per candidate on every keystroke.
func presetStrands(d Deps, candidate settings.Settings) bool {
	// One evaluation for every field asked about: the candidate does not move between them,
	// and the search runs this over a few hundred candidates on every keystroke.
	av := availabilityOf(d, candidate)

	for _, key := range presetSearchedKeys {
		f := presetField(key)
		if enabled, _ := optionStateOf(av, f.key, optionValue(f.value(candidate)), noEntry); !enabled {
			return true
		}
	}
	return false
}

// presetSearchedKeys are the axes the search itself writes: the rung's pixel format,
// the encode it is tried on, both halves of it, and the capture backend under it.
//
// A field the base writes is left out of this list.
// The base writes only what the claim speaks for, so a walk off one of those is a walk
// out of the claim, which presetHolds catches on the repaired settings,
// and a walk inside it is a value the promise covers: a preset that names four rate-control modes
// is still itself on the second of them.
// Gating here on such a field would reject a candidate the promise accepts,
// and a machine gapped on the base's first choice would fall past every encoder it has.
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

// The balanced target's shape: a share of the measured line, held inside a band.
//
// The share leaves room on the line for the audio track, retransmits
// and whatever else the machine sends.
// The ceiling is where more H.264 stops showing on desktop content,
// and the floor is the least a readable 1080p60 picture takes.
// The unmeasured figure fits the modest end of home uplinks,
// since the stated line is a guess until a measurement stands behind it
// (settings.Publish.UplinkMeasuredUnix).
const (
	balancedUplinkSharePct     = 70
	balancedBitrateCeilingM    = 40
	balancedBitrateFloorM      = 3
	balancedBitrateUnmeasuredM = 8
)

// balancedBitrate is the target the balanced preset writes, in Mbit/s.
func balancedBitrate(p settings.Publish) int {
	if p.UplinkMeasuredUnix == 0 {
		return balancedBitrateUnmeasuredM
	}
	return min(max(p.UplinkMbps*balancedUplinkSharePct/100, balancedBitrateFloorM), balancedBitrateCeilingM)
}

// presetEncodeHeight is the height of the picture a candidate's encoder is fed:
// the scaled output where one is set, the selected monitor's where this machine lists it,
// and the tallest monitor otherwise, any of them being what a capture could hand over.
// Zero where nothing states a height, which closes no rung:
// an unlisted display greys nothing, as an unprobed engine does (Deps).
func presetEncodeHeight(d Deps, s settings.Settings) int {
	if size, scaled, err := s.Publish.OutputSize(); err == nil && scaled {
		return size.Height
	}

	tallest := 0
	for _, m := range d.Monitors {
		if m.Index == s.Publish.Monitor {
			return m.Height
		}
		tallest = max(tallest, m.Height)
	}
	return tallest
}

// presetCodecs is the codecs a preset tries at one rung, best first:
// the encoders that come with a device before the ones that come with a build,
// and coding efficiency inside each half, in opposite directions.
//
// An encoder on fixed-function silicon leaves the machine free to run whatever is being captured,
// the aim of every preset here, so one is taken wherever it reaches the rung.
//
// Efficiency orders each half, a format spending fewer bits by searching more for them.
// On dedicated silicon that search costs nothing, so the most efficient format wins;
// on a CPU the frame rate pays for it, so the cheapest wins and a desktop encode never lands
// on the slowest encoder in the table.
// Codecs that tie keep the capability table's order.
//
// What this machine has is not filtered here.
// A codec no encoder was found for is greyed by availability and the repair walks the candidate
// off it, which is the same verdict arrived at by the one rule that owns it.
func presetCodecs(p preset, rung presetRung) []capabilities.Codec {
	type candidate struct {
		row    capabilities.Codec
		device bool
		// rank orders one half.
		// The bits a format spends run one way on silicon and the other on a CPU,
		// so the sign is what states which half this row
		// is in rather than a second comparison in the sort.
		rank float64
	}

	var ranked []candidate
	for _, c := range capabilities.Codecs {
		if !c.Implemented {
			continue
		}
		if len(p.formats) > 0 && !slices.Contains(p.formats, c.Format) {
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
// A configuration reachable without changing the backend is therefore the one taken,
// and the compositor's picker is not raised for a preset that had no need of it.
// The rest are the backends this platform runs that need no privilege granted first:
// one behind a privilege stays selectable by hand and is never picked on the user's behalf,
// the failure it produces being a capture that dies at launch for a reason no form mentioned.
func presetCaptures(d Deps, s settings.Settings) []string {
	out := []string{s.Publish.Capture}
	for _, capture := range publish.AutoCaptures(d.Platform) {
		if capture != s.Publish.Capture {
			out = append(out, capture)
		}
	}
	return out
}

// presetByKey is the row the settings name, and false for the detached draft.
// An unknown key reads as detached rather than asserting:
// the settings file is the user's to edit, so a key from some other build is an Umgebungsfehler,
// and the form shows no selection with every verdict beside it.
func presetByKey(key string) (preset, bool) {
	if key == "" {
		return preset{}, false
	}
	for _, p := range presetTable {
		if p.key == key {
			return p, true
		}
	}
	return preset{}, false
}

// presetOwnedKeys are the fields a followed preset decides:
// what a base writes, what the search varies and the ladder steps beside them.
// A write to one detaches the draft, the user taking the value into their own hands,
// and a write anywhere else keeps it following, the preset promising nothing about the field.
//
// One list for every preset rather than a column per row.
// The union costs a detach on a field one preset leaves standing,
// where a per-row list would cost a value silently overwritten on the next resolve
// wherever the two fell out of step, and the base is a function no table can read.
var presetOwnedKeys = []string{
	KeyFormat, KeyEncoder, KeyChroma, KeyCapture,
	KeyMode, KeyFps, KeyGop, KeyBframes, KeyColorRange,
	KeyBitrateM, KeyMaxrateM, KeyVbvMs, KeyCq, KeyEffort, KeyTune,
	KeySrtPublishLatencyMs,
}

// presetTransports is the walk a followed draft's relaunch takes when a publish leg spends
// its retry attempts: SRT first for its retransmit window,
// RTSP behind it interleaving over the one TCP connection the session already made,
// which crosses a path that blocks UDP.
// One list for every built-in, the walk trading reach for delay the same way on each,
// and RTSP's carriage covering every format SRT's does.
//
// The search never reads it: a preset resolves on the leg the settings hold (docs/presets.md),
// and the walk belongs to the relaunch alone (app, publishEnded).
var presetTransports = []string{"srt", "rtsp"}

// NextTransport is the leg a followed draft's relaunch tries after its own,
// and false where the walk ends: the draft follows no built-in,
// its leg is outside the walk, or it stands on the last rung.
// A detached draft never walks, the transport being the user's own word.
func NextTransport(draft settings.Settings) (string, bool) {
	if !Followed(draft) {
		return "", false
	}
	i := slices.Index(presetTransports, draft.Publish.Transport)
	if i < 0 || i+1 >= len(presetTransports) {
		return "", false
	}
	return presetTransports[i+1], true
}

// Followed reports whether the draft follows a built-in preset this build carries.
// An unknown key reads as detached, the reading presetByKey gives every caller,
// so a start and a form answer a stored stranger the same way.
func Followed(draft settings.Settings) bool {
	_, ok := presetByKey(draft.Publish.Preset)
	return ok
}

// ResolveBuiltin is the configuration the preset the draft follows produces on this machine,
// and false where the draft follows none or nothing here reaches its promise.
//
// What a start runs when the settings follow a preset (control.StartPublish):
// the draft seeds the same search the form ran, against the machine as it stands at the press,
// so an encoder that left since the form was drawn is one the stream never asks for.
func ResolveBuiltin(d Deps, draft settings.Settings) (settings.Settings, bool) {
	p, ok := presetByKey(draft.Publish.Preset)
	if !ok {
		return settings.Settings{}, false
	}
	s, _ := Repair(d, draft)
	return presetResolve(d, p, s)
}

// presetCq places a preset's quantizer target on the scale the named codec counts on,
// against the anchor scale of 51 points the target is stated on.
//
// The scale is the running engine's, the two engines setting different properties
// and one counting further than the other.
// A codec whose scale this engine declares none for keeps the target where it stands,
// there being no ratio to convert it by, which is the answer the bitrate prediction gives
// the same question (estimate.go).
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
