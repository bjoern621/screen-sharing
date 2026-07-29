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
				_, path := For(engine, capture, family)
				want := MemorySystem
				if path {
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

// Every row resolves under the demand, or the table declares a path no run can ask for.
func TestGpuResolvesForEveryPath(t *testing.T) {
	for _, p := range Paths {
		memory, err := Resolve(p.Engine, p.Capture, p.Family, MemoryGpu)
		if err != nil {
			t.Errorf("%s/%s/%s: %v", p.Engine, p.Capture, p.Family, err)
			continue
		}
		if memory != MemoryGpu {
			t.Errorf("%s/%s/%s: resolved to %s, want %s", p.Engine, p.Capture, p.Family, memory, MemoryGpu)
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

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
