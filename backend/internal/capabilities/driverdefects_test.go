package capabilities

import (
	"strings"
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/rules"
)

// Every defect names a driver, a gappable option and a value that option has, so a row that would
// bind on nothing, or take away a value no control offers, fails here rather than at a resolve.
func TestEveryDriverDefectIsWellFormed(t *testing.T) {
	for _, c := range Codecs {
		for _, d := range c.DriverDefects {
			if d.Driver == "" {
				t.Errorf("%s declares a defect naming no driver, which would bind on every machine", c.Name)
			}
			if _, ok := optionAxes[d.Option]; !ok {
				t.Errorf("%s declares a defect on %q, which is no gappable option", c.Name, d.Option)
			}
			if d.Reason == screensharev1.TextCode_TEXT_CODE_UNSPECIFIED {
				t.Errorf("%s declares a defect on %s with no reason, so a shell would grey it silently",
					c.Name, d.Option)
			}
			// A defect withholds what the encoder does implement. One that names a value already gapped
			// says nothing, and one that names a value outside the option's set binds on nothing.
			if d.Option == OptionMode && !contains(Modes, d.Value) {
				t.Errorf("%s declares a defect on mode %q, which is no rate-control mode", c.Name, d.Value)
			}
			gappedEverywhere := true
			for _, engine := range Engines {
				if _, gapped := c.OptionGap(engine, d.Option, d.Value); !gapped {
					gappedEverywhere = false
				}
			}
			if gappedEverywhere {
				t.Errorf("%s gaps %s=%s on every engine already, so the defect withholds nothing",
					c.Name, d.Option, d.Value)
			}
		}
	}
}

// A defect binds where its driver is the one installed, and nowhere else.
// The machine that named no driver is the case that decides the shape: it keeps every value the
// encoder declares, rather than being withheld on a guess.
func TestADriverDefectBindsOnItsOwnDriverAlone(t *testing.T) {
	c := mustGet(t, "av1_vaapi")

	cases := []struct {
		name   string
		device Device
		want   bool
	}{
		{name: "the driver carrying it", device: Device{Driver: "radeonsi"}, want: true},
		{name: "another driver", device: Device{Driver: "iHD"}, want: false},
		{name: "no driver named", device: Device{}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, engine := range Engines {
				if got := c.WithheldByDriver(tc.device, engine, OptionMode, ModeCbr); got != tc.want {
					t.Errorf("on %s: withheld=%v, want %v", engine, got, tc.want)
				}
			}
		})
	}
}

// A defect narrowed to models binds on those and not on the driver's other adapters.
func TestADriverDefectNarrowsToTheModelsItNames(t *testing.T) {
	const driver, carried, spared = "testdriver", "Carrying Adapter", "Other Adapter"

	c := Codec{Name: "test", DriverDefects: []DriverDefect{{
		Driver: driver,
		Models: []string{carried},
		Option: OptionMode,
		Value:  ModeCbr,
		Reason: screensharev1.TextCode_TEXT_CODE_DRIVER_DEFECT_WITHHOLDS_OPTION,
	}}}

	for _, tc := range []struct {
		model string
		want  bool
	}{{carried, true}, {spared, false}, {"", false}} {
		facts := deviceCodecFacts(c.Name, EngineGst, ModeCbr, Device{Driver: driver, Model: tc.model})
		v := rules.EvaluateRules(facts, c.driverDefectRules())
		if got := !v.ValueEnabled(rules.AxisMode, ModeCbr); got != tc.want {
			t.Errorf("model %q: withheld=%v, want %v", tc.model, got, tc.want)
		}
	}
}

// A defect naming the release it is fixed in stops binding there.
// A machine whose release went unread reads zero and keeps the defect, which is what stops an
// unnamed version from lifting a restriction nobody established was lifted.
func TestADriverDefectLiftsAtTheReleaseThatFixesIt(t *testing.T) {
	const driver = "testdriver"
	fixedIn := 26_002_000

	c := Codec{Name: "test", DriverDefects: []DriverDefect{{
		Driver:  driver,
		Option:  OptionMode,
		Value:   ModeCbr,
		FixedIn: fixedIn,
		Reason:  screensharev1.TextCode_TEXT_CODE_DRIVER_DEFECT_WITHHOLDS_OPTION,
	}}}

	for _, tc := range []struct {
		name    string
		version int
		want    bool
	}{
		{"a release below the fix", 26_001_006, true},
		{"the release below the fix", fixedIn - 1, true},
		{"the release carrying the fix", fixedIn, false},
		{"a release above the fix", 27_000_000, false},
		{"a release nobody read", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := deviceCodecFacts(c.Name, EngineGst, ModeCbr, Device{Driver: driver, Version: tc.version})
			v := rules.EvaluateRules(facts, c.driverDefectRules())
			if got := !v.ValueEnabled(rules.AxisMode, ModeCbr); got != tc.want {
				t.Errorf("version %d: withheld=%v, want %v", tc.version, got, tc.want)
			}
		})
	}
}

// A publish on the driver carrying the defect is refused, on both engines, and the refusal names the
// driver rather than the encoder.
//
// The wording is what is asserted alongside the refusal: the encoder does implement the value, so
// the phrase for a capability it lacks would send a reader to change engines, and the other engine
// drives the same driver.
func TestValidateRefusesADriverDefectAndNamesTheDriver(t *testing.T) {
	device := Device{Driver: "radeonsi", Model: "AMD Radeon 780M Graphics", Version: 26_001_006}
	opts := options("yuv420p", ModeCbr, ColorRangeLimited)

	for _, engine := range Engines {
		err := Validate(engine, "av1_vaapi", opts, 0, 40, 0, device)
		if err == nil {
			t.Fatalf("on %s: a rate control this driver miscodes must be refused", engine)
		}
		if !strings.Contains(err.Error(), "radeonsi") {
			t.Errorf("on %s: the refusal names no driver: %v", engine, err)
		}
		if strings.Contains(err.Error(), "only on") {
			t.Errorf("on %s: the refusal points at the other engine, which drives the same driver: %v", engine, err)
		}
	}

	// The same settings on a machine running another driver reach the encoder.
	if err := Validate(EngineGst, "av1_vaapi", opts, 0, 40, 0, Device{Driver: "iHD"}); err != nil {
		t.Errorf("another driver carries no such defect: %v", err)
	}
}

// A defect is not a gap, and the engine-scoped lookups keep answering what the encoder implements.
// A builder enumerating an element's rate controls has to get the same answer on every machine, or a
// pipeline would be built differently depending on which card is installed.
func TestADriverDefectIsNoGap(t *testing.T) {
	c := mustGet(t, "av1_vaapi")

	for _, engine := range Engines {
		if _, gapped := c.OptionGap(engine, OptionMode, ModeCbr); gapped {
			t.Errorf("on %s: the encoder implements cbr, so the table must carry no gap on it", engine)
		}
		if !Reaches(c.Name, engine, OptionMode, ModeCbr) {
			t.Errorf("on %s: cbr reaches this encoder, whatever driver is installed", engine)
		}
	}
}
