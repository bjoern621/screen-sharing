package gpupath

import (
	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
)

// Both publish engines and the form read this table.
// A row naming an engine or family the app does not carry greys a control nothing can select.
func TestEveryPathNamesAKnownEngineAndFamily(t *testing.T) {
	for _, p := range Paths {
		if !contains(capabilities.Engines, p.Engine) {
			t.Errorf("%s/%s: engine %q is not one of %v", p.Capture, p.Family, p.Engine, capabilities.Engines)
		}
		if !contains(capabilities.Families, p.Family) {
			t.Errorf("%s/%s: family %q is not one of %v", p.Capture, p.Family, p.Family, capabilities.Families)
		}
		if p.Capture == "" {
			t.Errorf("%s/%s: a row names no capture backend", p.Engine, p.Family)
		}
		// The import is what the form shows in place of the greyed control,
		// and what a reader checks this machine against,
		// so a row without one says a path exists and nothing about what carries the frames.
		if p.Import == screensharev1.TextCode_TEXT_CODE_UNSPECIFIED {
			t.Errorf("%s/%s/%s: a row states no import", p.Engine, p.Capture, p.Family)
		}
	}
}

// A pair appearing twice makes For's answer depend on table order,
// and the two rows can disagree about what carries the frames.
func TestNoPairIsDeclaredTwice(t *testing.T) {
	seen := map[Path]bool{}
	for _, p := range Paths {
		key := Path{Engine: p.Engine, Capture: p.Capture, Family: p.Family}
		if seen[key] {
			t.Errorf("%s/%s/%s is declared twice", p.Engine, p.Capture, p.Family)
		}
		seen[key] = true
	}
}

// Every combination satisfies auto,
// which is what makes it the value a settings file with no frame memory is filled with.
// A pair it refused would turn an upgrade into a publish that stops starting.
func TestAutoResolvesForEveryPair(t *testing.T) {
	for _, engine := range capabilities.Engines {
		for _, family := range capabilities.Families {
			for _, capture := range []string{
				"portal", "ximagesrc", "x11grab", "kmsgrab", "ddagrab", "gdigrab",
				"avfoundation", "avfvideosrc", "d3d11screencapturesrc",
			} {
				memory, err := Resolve(engine, capture, family, MemoryAuto)
				if err != nil {
					t.Errorf("%s/%s/%s: auto must resolve, got %v", engine, capture, family, err)
					continue
				}
				// A row auto takes costs nothing visible.
				// Where the only device path lets the encoder pick the colour, auto is the round trip instead.
				p, path := For(engine, capture, family)
				want := MemorySystem
				if path && !p.Colour.TradesColour() {
					want = MemoryGpu
				}
				if memory != want {
					t.Errorf("%s/%s/%s: auto resolved to %s, want %s", engine, capture, family, memory, want)
				}
			}
		}
	}
}

// The device path is a demand, not a preference.
// A pair without one is refused,
// resolving to the copy handing back the round trip the setting exists to avoid.
func TestGpuIsRefusedForAPairWithoutAPath(t *testing.T) {
	memory, err := Resolve(capabilities.EngineGst, "ximagesrc", capabilities.FamilySoftware, MemoryGpu)
	if err == nil {
		t.Fatalf("a pair with no GPU path must refuse the gpu setting, got %q", memory)
	}
	// The message names the way out:
	// a settings file that skipped the form reaches this with no control to read.
	if !strings.Contains(err.Error(), MemorySystem) {
		t.Errorf("the refusal must name the memory that always works: %v", err)
	}
}

// Every row resolves under a device demand, or the table declares a path no run can ask for.
// The colour verdict decides which demand reaches it:
// gpu wants the colour too and gpu-encoder-color pays the colour for the device,
// so between them they reach every row there is.
func TestEveryPathResolvesUnderTheDemandThatFitsIt(t *testing.T) {
	for _, p := range Paths {
		want := MemoryGpu
		demand := MemoryGpu
		if p.Colour.TradesColour() {
			want = MemoryGpuEncoderColor
			demand = MemoryGpuEncoderColor
		}
		memory, err := Resolve(p.Engine, p.Capture, p.Family, demand)
		if err != nil {
			t.Errorf("%s/%s/%s: %v", p.Engine, p.Capture, p.Family, err)
			continue
		}
		if memory != want {
			t.Errorf("%s/%s/%s: %s resolved to %s, want %s", p.Engine, p.Capture, p.Family, demand, memory, want)
		}
	}
}

// The two kinds of row owe the user different things.
// An encoder-colour row is offered only because it states what it costs:
// the refusal under gpu quotes the cost,
// and the form greys the overridden fields with the signalled values in them,
// so a row missing either greys a control and leaves the reader guessing what replaced it.
// An exact row carries neither,
// the settings being the answer there and a second copy a fact that can drift from the form's.
func TestEveryPathStatesWhatItsColourCosts(t *testing.T) {
	for _, p := range Paths {
		switch p.Colour {
		case ColourEncoder:
			if p.Cost == screensharev1.TextCode_TEXT_CODE_UNSPECIFIED {
				t.Errorf("%s/%s/%s: an encoder-colour row states no cost", p.Engine, p.Capture, p.Family)
			}
			if p.Signalled.Matrix == "" || p.Signalled.Range == "" || p.Signalled.Chroma == "" {
				t.Errorf("%s/%s/%s: an encoder-colour row signals %+v, and all three are read",
					p.Engine, p.Capture, p.Family, p.Signalled)
			}
		case ColourExact:
			if p.Cost != screensharev1.TextCode_TEXT_CODE_UNSPECIFIED {
				t.Errorf("%s/%s/%s: an exact-colour row costs nothing, yet names %q",
					p.Engine, p.Capture, p.Family, p.Cost)
			}
			if p.Signalled != (Signalled{}) {
				t.Errorf("%s/%s/%s: an exact-colour row signals the settings' colour, yet restates %+v",
					p.Engine, p.Capture, p.Family, p.Signalled)
			}
		default:
			t.Errorf("%s/%s/%s: colour %q is neither %q nor %q",
				p.Engine, p.Capture, p.Family, p.Colour, ColourExact, ColourEncoder)
		}
	}
}

// Auto fills a settings file that names no frame memory,
// so it may never answer with the memory that trades the colour away.
// That is a choice, and a default does not make choices.
func TestAutoNeverResolvesToTheEncoderColourMemory(t *testing.T) {
	for _, p := range Paths {
		memory, err := Resolve(p.Engine, p.Capture, p.Family, MemoryAuto)
		if err != nil {
			t.Errorf("%s/%s/%s: auto must resolve, got %v", p.Engine, p.Capture, p.Family, err)
			continue
		}
		if memory == MemoryGpuEncoderColor {
			t.Errorf("%s/%s/%s: auto resolved to %s", p.Engine, p.Capture, p.Family, memory)
		}
	}
}

// The encoder-colour column of the resolution table,
// held against a row of that kind and not against the reasoning behind it.
// The row is installed for the test and taken out again,
// so the four answers stay covered whichever rows the shipping table carries.
func TestAnEncoderColourRowResolvesByWhatEachDemandWillPay(t *testing.T) {
	row := Path{
		Engine: capabilities.EngineGst, Capture: "testsrc", Family: capabilities.FamilySoftware,
		Import:    screensharev1.TextCode_TEXT_CODE_IMPORT_FFMPEG_DDAGRAB_NVENC,
		Colour:    ColourEncoder,
		Cost:      screensharev1.TextCode_TEXT_CODE_COST_ENCODER_SIGNALS_ITS_OWN_COLOUR,
		Signalled: Signalled{Matrix: "bt470bg", Range: "tv", Chroma: "yuv420p"},
	}
	shipped := Paths
	Paths = append(append([]Path{}, Paths...), row)
	t.Cleanup(func() { Paths = shipped })

	for memory, want := range map[string]string{
		MemoryAuto:            MemorySystem,
		MemoryGpuEncoderColor: MemoryGpuEncoderColor,
		MemorySystem:          MemorySystem,
	} {
		got, err := Resolve(row.Engine, row.Capture, row.Family, memory)
		if err != nil || got != want {
			t.Errorf("%s resolved to %q, %v, want %s", memory, got, err, want)
		}
	}

	// The demand for the device and the colour is refused,
	// and the refusal leaves the user somewhere to go:
	// the value that takes the cost knowingly, and the value that keeps the colour instead.
	// What the path takes is the row's Cost statement,
	// shown by the form on the greyed control rather than inside this string.
	_, err := Resolve(row.Engine, row.Capture, row.Family, MemoryGpu)
	if err == nil {
		t.Fatalf("a row that trades the colour must refuse the gpu setting")
	}
	for _, want := range []string{MemoryGpuEncoderColor, MemorySystem} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal must name %q: %v", want, err)
		}
	}
}

// System memory is the path every pair has, so the table never decides it.
func TestSystemResolvesWhateverThePairIs(t *testing.T) {
	for _, p := range Paths {
		memory, err := Resolve(p.Engine, p.Capture, p.Family, MemorySystem)
		if err != nil || memory != MemorySystem {
			t.Errorf("%s/%s/%s: got %q, %v", p.Engine, p.Capture, p.Family, memory, err)
		}
	}
}

// A value outside Memories is refused rather than read as auto.
// The setting decides whether every frame makes a round trip through system memory,
// so substituting one runs a pipeline the form does not show.
func TestAnUnknownMemoryIsRefused(t *testing.T) {
	for _, bad := range []string{"", "GPU", "device", "zerocopy"} {
		if _, err := Resolve(capabilities.EngineGst, "portal", capabilities.FamilyVaapi, bad); err == nil {
			t.Errorf("frame memory %q must be refused, not read as auto", bad)
		}
	}
}

// A device refusal names the memory that gets the user publishing again,
// the machine's GPU count not being something it can be talked out of.
func TestTheDeviceRefusalsNameTheWayOut(t *testing.T) {
	for _, err := range []error{
		Undetermined("the portal does not name the GPU it captures on",
			[]string{"/dev/dri/renderD128", "/dev/dri/renderD129"}),
	} {
		if !strings.Contains(err.Error(), MemorySystem) {
			t.Errorf("a device refusal must name the memory that always works: %v", err)
		}
		if !strings.Contains(err.Error(), "renderD129") {
			t.Errorf("a device refusal must name the devices it read: %v", err)
		}
	}
}

// Both device values keep the frames on the GPU,
// differing in who converts and not in where the frames are.
// A capture chain reading MemoryGpu alone would build the round trip for the other one,
// the cost the setting was picked to avoid.
func TestOnDeviceCoversBothDeviceMemories(t *testing.T) {
	for memory, want := range map[string]bool{
		MemoryGpu:             true,
		MemoryGpuEncoderColor: true,
		MemorySystem:          false,
	} {
		if got := OnDevice(memory); got != want {
			t.Errorf("OnDevice(%q) = %v, want %v", memory, got, want)
		}
	}
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
