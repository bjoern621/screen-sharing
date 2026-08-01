package gpupath

import (
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/capabilities"
)

// The pair table is read by both publish engines and by the form, so a row naming an
// engine or family the rest of the app does not carry would grey a control against a
// combination nothing can select.
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
		// The import is what the form shows in place of the greyed control and what a
		// reader checks the machine against, so a row without one states that a path
		// exists and nothing about what carries it.
		if p.Import == "" {
			t.Errorf("%s/%s/%s: a row states no import", p.Engine, p.Capture, p.Family)
		}
	}
}

// A pair appearing twice would make For's answer depend on table order, and the two
// rows could then disagree about what carries the frames.
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

// Auto is the value every combination satisfies, which is what makes it the default a
// settings file with no frame memory is filled with. A pair it refused would turn an
// upgrade into a publish that no longer starts.
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
				// A row auto can take costs nothing visible. Where the only device path
				// lets the encoder pick the colour, auto is the round trip.
				p, path := For(engine, capture, family)
				want := MemorySystem
				if path && !p.Colour.tradesColour() {
					want = MemoryGpu
				}
				if memory != want {
					t.Errorf("%s/%s/%s: auto resolved to %s, want %s", engine, capture, family, memory, want)
				}
			}
		}
	}
}

// The direct path is a demand, not a preference: a pair without one is refused so the
// run says what it cannot do, where resolving to the copy would hand back the round
// trip the setting exists to avoid.
func TestGpuIsRefusedForAPairWithoutAPath(t *testing.T) {
	memory, err := Resolve(capabilities.EngineGst, "ximagesrc", capabilities.FamilySoftware, MemoryGpu)
	if err == nil {
		t.Fatalf("a pair with no GPU path must refuse the gpu setting, got %q", memory)
	}
	// The message has to name the way out, since the form's repair moves to auto and a
	// settings file that skipped the form reaches this with no control to read.
	if !strings.Contains(err.Error(), MemorySystem) {
		t.Errorf("the refusal must name the memory that always works: %v", err)
	}
}

// Every row resolves under a demand for the device, or the table declares a path no run
// can ask for. Which demand reaches it is the colour verdict's business: gpu is the one
// that also wants the colour, and gpu-encoder-color the one that pays for the device with
// it, so between them they reach every row there is.
func TestEveryPathResolvesUnderTheDemandThatFitsIt(t *testing.T) {
	for _, p := range Paths {
		want := MemoryGpu
		demand := MemoryGpu
		if p.Colour.tradesColour() {
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

// The colour verdict decides what a setting may resolve to on its own, and the two kinds
// of row owe the user different things. An encoder-colour row is offered only because
// what it costs is stated: the refusal under gpu quotes the cost, and the form greys the
// fields the run overrides with the signalled values in them, so a row missing either
// greys a control and leaves the reader to guess what replaced it. An exact row carries
// neither, because there the settings are the answer and a second copy of them here is a
// fact that can drift from the one the form holds.
func TestEveryPathStatesWhatItsColourCosts(t *testing.T) {
	for _, p := range Paths {
		switch p.Colour {
		case ColourEncoder:
			if p.Cost == "" {
				t.Errorf("%s/%s/%s: an encoder-colour row states no cost", p.Engine, p.Capture, p.Family)
			}
			if p.Signalled.Matrix == "" || p.Signalled.Range == "" || p.Signalled.Chroma == "" {
				t.Errorf("%s/%s/%s: an encoder-colour row signals %+v, and all three are read",
					p.Engine, p.Capture, p.Family, p.Signalled)
			}
		case ColourExact:
			if p.Cost != "" {
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

// Auto is the value a settings file with no frame memory is filled with, so it may never
// answer with the memory that trades the colour away. That is a choice, and a default is
// not where a choice gets made.
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

// The encoder-colour column of the resolution table, held against a row of that kind
// rather than against the reasoning that produced it. Every shipping row converts on the
// device today, so without a row installed here these four answers are unreached code
// until the first such path lands, and the phase that lands it would be the phase that
// finds out what they do.
func TestAnEncoderColourRowResolvesByWhatEachDemandWillPay(t *testing.T) {
	const cost = "the encoder reads the captured texture and picks the colour itself"
	row := Path{
		Engine: capabilities.EngineGst, Capture: "testsrc", Family: capabilities.FamilySoftware,
		Import: "the test row imports nothing", Colour: ColourEncoder, Cost: cost,
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

	// The demand for the device and the colour is refused, and the refusal has to leave
	// the user somewhere to go: what the path takes, the value that takes it knowingly,
	// and the value that keeps the colour instead.
	_, err := Resolve(row.Engine, row.Capture, row.Family, MemoryGpu)
	if err == nil {
		t.Fatalf("a row that trades the colour must refuse the gpu setting")
	}
	for _, want := range []string{cost, MemoryGpuEncoderColor, MemorySystem} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal must name %q: %v", want, err)
		}
	}
}

// System memory is the path every pair has, so it never depends on the table.
func TestSystemResolvesWhateverThePairIs(t *testing.T) {
	for _, p := range Paths {
		memory, err := Resolve(p.Engine, p.Capture, p.Family, MemorySystem)
		if err != nil || memory != MemorySystem {
			t.Errorf("%s/%s/%s: got %q, %v", p.Engine, p.Capture, p.Family, memory, err)
		}
	}
}

// A value outside the table is refused rather than read as auto. It decides whether
// every frame makes a round trip through system memory, so substituting one runs a
// pipeline the form does not show.
func TestAnUnknownMemoryIsRefused(t *testing.T) {
	for _, bad := range []string{"", "GPU", "device", "zerocopy"} {
		if _, err := Resolve(capabilities.EngineGst, "portal", capabilities.FamilyVaapi, bad); err == nil {
			t.Errorf("frame memory %q must be refused, not read as auto", bad)
		}
	}
}

// Both device refusals have to name the memory that gets the user publishing again,
// since neither is something the machine can be talked out of.
func TestTheDeviceRefusalsNameTheWayOut(t *testing.T) {
	for _, err := range []error{
		Mismatch("/dev/dri/renderD128", "/dev/dri/renderD129"),
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

// Both device values keep the frames on the GPU: they differ in who converts, not in
// where the frames are. A capture chain reading only MemoryGpu would build the round trip
// for the other one, which is the whole cost the setting was picked to avoid.
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
