package wire

import (
	"slices"
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/transport"
)

// catalogInput is a machine the tests can describe: one monitor, a platform, and a
// probe that answered one engine and could not be asked about the other. The fixed
// tables are the process's own and are not part of it, which is the split
// CatalogInput exists to make.
func catalogInput() CatalogInput {
	return CatalogInput{
		Platform: platform.Info{OS: "linux", Display: "wayland"},
		Monitors: []display.Monitor{
			{Index: 0, Width: 2560, Height: 1440, Primary: true, RefreshHz: 144},
		},
		Encoders: encoders.Availability{
			Usable: map[string]map[string]bool{
				capabilities.EngineGst: {"libx264": false, "vp9enc": false},
			},
			Unprobed: map[string]string{
				capabilities.EngineFfmpeg: "ffmpeg not found on PATH",
			},
		},
	}
}

// engineSpelling reads a wire engine back to the name the Go tables use, so a test can
// compare a converted row against the table it came from without restating the mapping.
func engineSpelling(t *testing.T, e screensharev1.Engine) string {
	t.Helper()
	for name, v := range engines {
		if v == e {
			return name
		}
	}
	t.Fatalf("engine %v names no publish engine the capability table declares", e)
	return ""
}

// legSpelling is the same for a carriage row's leg.
func legSpelling(t *testing.T, l screensharev1.Leg) string {
	t.Helper()
	for name, v := range legs {
		if v == l {
			return name
		}
	}
	t.Fatalf("leg %v names neither of the two legs", l)
	return ""
}

// The catalog is the reference set a shell explains a decision from, so a codec the
// table declares and the message omits is a codec no shell can name. Every row crosses,
// including the ones marked unimplemented: such a row exists so the table can state why
// the codec is absent, and dropping it here would remove the explanation with it.
func TestEveryDeclaredCodecReachesTheMessage(t *testing.T) {
	got := Catalog(catalogInput()).GetCodecs()

	if len(got) != len(capabilities.Codecs) {
		t.Fatalf("catalog carries %d codecs, the table declares %d", len(got), len(capabilities.Codecs))
	}
	for i, want := range capabilities.Codecs {
		row := got[i]
		if row.GetName() != want.Name {
			t.Errorf("codec %d is %q, want %q: the order is the table's", i, row.GetName(), want.Name)
			continue
		}
		if row.GetFamily() != want.Family || row.GetFormat() != want.Format {
			t.Errorf("%s crosses as family %q format %q, want %q and %q",
				want.Name, row.GetFamily(), row.GetFormat(), want.Family, want.Format)
		}
		if row.GetImplemented() != want.Implemented {
			t.Errorf("%s crosses as implemented=%v, want %v", want.Name, row.GetImplemented(), want.Implemented)
		}
		if !slices.Equal(row.GetChromas(), want.Chromas) {
			t.Errorf("%s crosses with chromas %v, want %v: gaps narrow a row per engine, the wire does not",
				want.Name, row.GetChromas(), want.Chromas)
		}
		if len(row.GetGaps()) != len(want.Gaps) {
			t.Errorf("%s crosses with %d gaps, the table declares %d", want.Name, len(row.GetGaps()), len(want.Gaps))
		}
		// The two per-engine columns are one row per engine on the wire, so each is
		// looked up by the enum the row carries rather than by the name that keyed the
		// map. An engine the table bounds and the row omits reads as an absent limit,
		// which is exactly what the message means by one, so both halves are checked
		// for presence and not only for value.
		for engine, want_ := range want.CqMax {
			limit := engineLimitOf(row, engineEnum(engine))
			if limit == nil || limit.GetCqMax() != int32(want_) {
				t.Errorf("%s cq_max on %s crosses as %v, want %d", want.Name, engine, limit.GetCqMax(), want_)
			}
		}
		for engine, want_ := range want.BitrateLimitM {
			limit := engineLimitOf(row, engineEnum(engine))
			if limit == nil || limit.GetBitrateLimitMbps() != int32(want_) {
				t.Errorf("%s bitrate ceiling on %s crosses as %v, want %d", want.Name, engine, limit.GetBitrateLimitMbps(), want_)
			}
		}
	}
}

// engineLimitOf finds one engine's row of numeric ceilings, nil where the codec is
// bounded by nothing on that engine.
func engineLimitOf(row *screensharev1.VideoCodec, engine screensharev1.Engine) *screensharev1.EngineLimit {
	for _, limit := range row.GetLimits() {
		if limit.GetEngine() == engine {
			return limit
		}
	}
	return nil
}

// A capture backend the registry runs and the catalog omits is a screen source no shell
// can offer, so every row crosses with the engine that runs it and the publish legs that
// engine carries. Those two are read through the registry rather than restated, which is
// what keeps the catalog and the publish path naming the same set.
func TestEveryCaptureBackendReachesTheMessage(t *testing.T) {
	got := Catalog(catalogInput()).GetCaptures()
	want := publish.Captures()

	if len(got) != len(want) {
		t.Fatalf("catalog carries %d capture backends, the registry holds %d", len(got), len(want))
	}
	for i, name := range want {
		row := got[i]
		if row.GetName() != name {
			t.Errorf("capture %d is %q, want %q: the order is publish.Captures's", i, row.GetName(), name)
			continue
		}
		engine, err := publish.EngineFor(name)
		if err != nil {
			t.Fatalf("%s has no publisher: %v", name, err)
		}
		if spelled := engineSpelling(t, row.GetEngine()); spelled != engine {
			t.Errorf("%s crosses on engine %q, the registry runs it on %q", name, spelled, engine)
		}
		transports, err := publish.TransportsFor(name)
		if err != nil {
			t.Fatalf("%s has no publisher: %v", name, err)
		}
		if !slices.Equal(row.GetTransports(), transports) {
			t.Errorf("%s crosses carrying %v, its engine carries %v", name, row.GetTransports(), transports)
		}
	}
}

// One row per (transport, leg, engine) that exists, and none for a leg an engine cannot
// serialize. A row for an absent leg would offer a shell a combination the registry has
// no code to build, and a missing row would hide a leg that works.
func TestEveryStatedCarriageReachesTheMessage(t *testing.T) {
	type row struct{ name, leg, engine string }

	want := map[row]transport.Carriage{}
	for _, name := range transport.Names() {
		f, ok := transport.FormatsOf(name)
		if !ok {
			t.Fatalf("transport %q is listed and not registered", name)
		}
		for engine, c := range f.Publish {
			want[row{name, legPublish, engine}] = c
		}
		for engine, c := range f.Watch {
			want[row{name, legWatch, engine}] = c
		}
	}

	got := Catalog(catalogInput()).GetCarriage()
	if len(got) != len(want) {
		t.Fatalf("catalog carries %d carriage rows, the transports state %d", len(got), len(want))
	}
	for _, r := range got {
		key := row{r.GetName(), legSpelling(t, r.GetLeg()), engineSpelling(t, r.GetEngine())}
		c, ok := want[key]
		if !ok {
			t.Errorf("catalog carries %v, which no transport states", key)
			continue
		}
		if !slices.Equal(r.GetVideo(), c.Video) || !slices.Equal(r.GetAudio(), c.Audio) {
			t.Errorf("%v crosses carrying video %v audio %v, the transport states %v and %v",
				key, r.GetVideo(), r.GetAudio(), c.Video, c.Audio)
		}
	}
}

// A gap and the control it greys are the same identifier on both sides of the wire, and
// the two sides spell one option differently: the capability table names it by the Go
// JSON tag settings.Stream carries, a form control by the proto field name. The
// normalization happens once, in this package, so a shell that receives a gap can grey
// the matching control with no mapping of its own.
func TestAGapNamesTheControlItGreys(t *testing.T) {
	declared := 0
	for _, c := range capabilities.Codecs {
		for _, g := range c.Gaps {
			if g.Option == capabilities.OptionColorRange {
				declared++
			}
		}
	}
	if declared == 0 {
		t.Fatal("the capability table declares no colour-range gap, so this test would pass by finding nothing")
	}

	catalog := Catalog(catalogInput())
	normalized := 0
	for _, c := range catalog.GetCodecs() {
		for _, g := range c.GetGaps() {
			if g.GetOption() == capabilities.OptionColorRange {
				t.Errorf("%s carries a gap on %q, which is the Go tag and not a control any shell binds by",
					c.GetName(), g.GetOption())
			}
			if g.GetOption() == "color_range" {
				normalized++
			}
		}
	}
	if normalized != declared {
		t.Errorf("%d colour-range gaps declared, %d arrived as color_range", declared, normalized)
	}

	// The list and the gaps have to agree, or a shell would hold an option name no gap
	// ever points at.
	if !slices.Contains(catalog.GetCapabilityOptions(), "color_range") {
		t.Errorf("capability_options = %v, want the same spelling the gaps arrive in",
			catalog.GetCapabilityOptions())
	}
	if slices.Contains(catalog.GetCapabilityOptions(), capabilities.OptionColorRange) {
		t.Errorf("capability_options = %v, want no Go-tag spelling among them",
			catalog.GetCapabilityOptions())
	}
}

// A gap naming no engine binds on every one: the format or the library has no such
// capability, rather than one builder failing to reach it. It crosses as ENGINE_ANY and
// never as ENGINE_UNSPECIFIED, so that the strongest claim the message can make is a
// value somebody chose rather than the value a dropped field leaves behind.
func TestAGapOnEveryEngineNamesEveryEngine(t *testing.T) {
	declared := map[string]int{}
	for _, c := range capabilities.Codecs {
		for _, g := range c.Gaps {
			if g.Engine == "" {
				declared[c.Name]++
			}
		}
	}

	for _, c := range Catalog(catalogInput()).GetCodecs() {
		any := 0
		for _, g := range c.GetGaps() {
			if g.GetEngine() == screensharev1.Engine_ENGINE_UNSPECIFIED {
				t.Errorf("%s carries a gap on ENGINE_UNSPECIFIED, which reads as a field nobody set", c.GetName())
			}
			if g.GetEngine() == screensharev1.Engine_ENGINE_ANY {
				any++
			}
		}
		if any != declared[c.GetName()] {
			t.Errorf("%s crosses with %d engine-wide gaps, the table declares %d",
				c.GetName(), any, declared[c.GetName()])
		}
	}
}

// engineProbeOf finds one engine's probe row, nil where the probe has not reached that
// engine at all.
func engineProbeOf(got *screensharev1.EncoderAvailability, engine screensharev1.Engine) *screensharev1.EngineProbe {
	for _, row := range got.GetEngines() {
		if row.GetEngine() == engine {
			return row
		}
	}
	return nil
}

// An engine that could not be probed is not an engine with nothing usable, and a form
// must not present it as one: the first is a missing ffmpeg or a missing GStreamer
// registry, which no choice of codec repairs, and the second is a machine without the
// hardware, which another codec may well reach.
//
// The two are the arms of one oneof rather than two parallel maps, so a row that stated
// both is no longer a value this test has to rule out - it is a value that cannot be
// built. What is left to check is that each engine took the arm its probe result calls
// for.
func TestAnUnprobedEngineIsNotAnEmptyOne(t *testing.T) {
	in := catalogInput()
	got := Catalog(in).GetEncoders()

	unprobed := engineProbeOf(got, screensharev1.Engine_ENGINE_FFMPEG)
	if unprobed == nil {
		t.Fatal("an unprobed engine crosses with no row at all, which reads as an engine nothing has asked about")
	}
	if unprobed.GetUnprobed() == nil {
		t.Error("an unprobed engine crosses without the reason it could not be asked")
	}
	if unprobed.GetProbed() != nil {
		t.Error("an unprobed engine crosses with verdicts, which would read as a machine that has no encoders")
	}

	probed := engineProbeOf(got, screensharev1.Engine_ENGINE_GSTREAMER)
	if probed == nil || probed.GetProbed() == nil {
		t.Fatal("a probed engine crosses without its verdicts")
	}
	usable := probed.GetProbed().GetUsable()
	if len(usable) != len(in.Encoders.Usable[capabilities.EngineGst]) {
		t.Errorf("a probed engine crosses with %d verdicts, the probe returned %d",
			len(usable), len(in.Encoders.Usable[capabilities.EngineGst]))
	}
	for codec, want := range in.Encoders.Usable[capabilities.EngineGst] {
		if got, ok := usable[codec]; !ok || got != want {
			t.Errorf("%s crosses as usable=%v (present=%v), the probe said %v", codec, got, ok, want)
		}
	}
	if reason := probed.GetUnprobed(); reason != nil {
		t.Errorf("a probed engine carries the reason %v, which is what the other arm is for", reason)
	}
}

// The relay re-serves a stream on the listeners whose protocol has a mapping for its
// format and on no others, so the per-format lists narrow the watch roster rather than
// repeating it. Repeating it would put a viewer in front of a stream its protocol
// cannot carry, and the failure would read as a broken stream.
func TestWatchTransportsNarrowPerFormat(t *testing.T) {
	catalog := Catalog(catalogInput())
	all := catalog.GetWatchTransports()
	if len(all) == 0 {
		t.Fatal("no transport has a viewer watch form, so there is nothing to narrow")
	}

	byFormat := catalog.GetWatchTransportsByFormat()
	if len(byFormat) != len(capabilities.Formats()) {
		t.Fatalf("the catalog narrows %d formats, implemented codecs produce %d",
			len(byFormat), len(capabilities.Formats()))
	}

	narrowed := 0
	for format, list := range byFormat {
		for _, name := range list.GetTransports() {
			if !slices.Contains(all, name) {
				t.Errorf("%s may be watched over %s, which is not a watch transport at all", format, name)
			}
			if !transport.CanWatchFormat(name, capabilities.EngineFfmpeg, format) {
				t.Errorf("%s may be watched over %s, which carries no such format", format, name)
			}
		}
		if len(list.GetTransports()) < len(all) {
			narrowed++
		}
	}
	if narrowed == 0 {
		t.Errorf("every format is offered the whole watch roster %v, so the map states nothing the list does not", all)
	}
}

// The second-track sources cross as what this machine serves, which is the field's whole
// claim: a shell reading them is reading what the platform has, and a source it has not
// is not something the machine offers under a flag nobody can see.
//
// The reason a source is out of reach is deliberately absent here and arrives on the
// form's option instead, so a machine serving fewer sources is a shorter list here and
// the same list greyed there. Both are read off platform.AudioSources, which is what
// keeps the two from disagreeing about which sources exist.
func TestAudioSourcesAreTheOnesThisMachineServes(t *testing.T) {
	for _, p := range []platform.Info{
		{OS: "linux", Display: "wayland"}, {OS: "windows"}, {OS: "darwin"}, {OS: "plan9"},
	} {
		in := catalogInput()
		in.Platform = p

		var want []string
		for _, s := range platform.AudioSources(p) {
			if s.Available {
				want = append(want, s.ID)
			}
		}

		got := Catalog(in).GetAudioSources()
		if !slices.Equal(got, want) {
			t.Errorf("%s crosses carrying %v, the platform table serves %v", p.OS, got, want)
		}
		if !slices.Contains(got, platform.AudioSourceNone) {
			t.Errorf("%s crosses without the absent source, which every platform serves", p.OS)
		}
	}
}

// The one source difference the table states, read back off the wire: a Linux session
// serves what the machine plays and the other two have nothing either publish engine can
// open, so the field is one entry longer there. A catalog that carried the same list
// everywhere would be the hardcoded slice this table replaced.
func TestAudioSourcesDifferByPlatform(t *testing.T) {
	linux := catalogInput()
	linux.Platform = platform.Info{OS: "linux", Display: "wayland"}
	windows := catalogInput()
	windows.Platform = platform.Info{OS: "windows"}

	served := Catalog(linux).GetAudioSources()
	unserved := Catalog(windows).GetAudioSources()
	if !slices.Contains(served, platform.AudioSourceDesktop) {
		t.Errorf("a Linux catalog carries %v, which does not offer desktop audio", served)
	}
	if slices.Contains(unserved, platform.AudioSourceDesktop) {
		t.Errorf("a Windows catalog carries %v, offering a source neither engine can open there", unserved)
	}
}

// The tables are the process's and a message is the caller's, so a slice that crossed
// has to be the caller's to hold. Handing out the table's own backing array would let a
// consumer that edits a message edit the domain model with it.
func TestAMessageHoldsNoTablesBackingArray(t *testing.T) {
	catalog := Catalog(catalogInput())

	for i, c := range catalog.GetCodecs() {
		want := capabilities.Codecs[i]
		if len(c.GetChromas()) == 0 {
			continue
		}
		c.Chromas[0] = "edited"
		if want.Chromas[0] == "edited" {
			t.Fatalf("editing %s's chromas on the message edited the capability table", want.Name)
		}
	}
}
