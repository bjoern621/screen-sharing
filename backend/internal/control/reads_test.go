package control

import (
	"bjoernblessin.de/screenshare/internal/pointer"
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/events"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/portal"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// probedBackend is a machine whose encoder probe has been taken:
// one family it cannot run, one it can.
// Everything else is fakeBackend's.
//
// The platform is stated because it decides which capture backends are reachable,
// and a backend with no operating system names no publish engine:
// every codec would then stay ungreyed for want of an engine to grey it on,
// and the test would pass for the wrong reason.
type probedBackend struct {
	fakeBackend
	// probed is what the probe found, left zero by the test that wants an unprobed machine.
	probed encoders.Availability
	// probes counts the runs of the probe itself.
	probes int
}

func (p *probedBackend) Platform() platform.Info               { return platform.Info{OS: "windows"} }
func (p *probedBackend) Device() capabilities.Device           { return capabilities.Device{} }
func (p *probedBackend) CachedEncoders() encoders.Availability { return p.probed }
func (p *probedBackend) AudioDevices() []platform.AudioDevice  { return nil }
func (p *probedBackend) PortalCapabilities() portal.Capabilities {
	return portal.Capabilities{}
}
func (p *probedBackend) Pointer() (pointer.Spot, bool) { return pointer.Spot{}, false }

func (p *probedBackend) StreamPointer(wire.StreamRef) (pointer.Spot, bool) {
	return pointer.Spot{}, false
}

// Encoders counts its calls, how a test says a read did not probe.
// The split exists because reading the catalog may not start seconds of work,
// and a count is the only way to observe that it did not.
func (p *probedBackend) Encoders(context.Context) encoders.Availability {
	p.probes++
	return p.probed
}

// qsvMissing is what the probe finds on a machine with no Intel GPU:
// the two Quick Sync encoders would not run, the NVIDIA one did.
// The zero Availability is that machine before the probe,
// a different fact rather than a weaker version of this one.
var qsvMissing = encoders.Availability{Usable: map[string]map[string]bool{
	capabilities.EngineFfmpeg: {"h264_qsv": false, "hevc_qsv": false, "hevc_nvenc": true},
}}

// encoderOption finds one entry of the encoder control in a resolved form,
// so a test reads the contract's own shape instead of form's internals.
//
// Which row that entry is about follows the format the draft holds, the pair being what addresses one:
// the defaults publish HEVC, so "qsv" here is the Quick Sync HEVC encoder.
func encoderOption(t *testing.T, form *screensharev1.Form, encoder string) *screensharev1.FieldOption {
	t.Helper()

	for _, group := range form.GetGroups() {
		for _, field := range group.GetFields() {
			if field.GetKey() != "publish.encoder" {
				continue
			}
			for _, option := range field.GetOptions() {
				if option.GetValue() == encoder {
					return option
				}
			}
		}
	}

	t.Fatalf("the resolved form offers no encoder %q", encoder)
	return nil
}

// The boundary between the probe and the screen, and the one place both halves show at once.
//
// The probe knows an Intel encoder is absent, the form says so,
// and ResolveForm is where the first reaches the second.
// It reads CachedEncoders rather than probing, so this is also the difference the read is written
// around: before a probe the same call greys nothing,
// and a shell that never asked for one goes on offering an encoder that fails at launch.
//
// The entry stays in the list, which is the treatment the rule names.
// A general concept the machine blocks is taught by a greyed option and its sentence
// (docs/field-availability.md, "The rule").
func TestResolveFormGreysAnEncoderTheProbeCouldNotRun(t *testing.T) {
	server := New(&probedBackend{probed: qsvMissing}, events.New(), "test")

	form, err := server.ResolveForm(context.Background(), &screensharev1.ResolveFormRequest{})
	if err != nil {
		t.Fatalf("resolving a form answered %v, want an answer", err)
	}

	option := encoderOption(t, form.GetForm(), "qsv")
	if option.GetEnabled() {
		t.Fatal("the Quick Sync encoder is offered on a machine whose probe could not run it")
	}
	// The statement names the family whose device is missing, and the surface writes the sentence.
	// How "no Intel Quick Sync encoder on this computer" reads belongs there.
	// A reason greying the codec without naming the family would leave the reader nothing to act on.
	if option.GetReason().GetCode() != screensharev1.TextCode_TEXT_CODE_PROBE_NO_DEVICE {
		t.Errorf("the Quick Sync encoder greys with %v, want the probe's no-device verdict", option.GetReason().GetCode())
	}
	if got := textArgID(option.GetReason(), screensharev1.TextArgName_TEXT_ARG_NAME_FAMILY); got != "qsv" {
		t.Errorf("the Quick Sync encoder greys naming family %q, which is not the hardware that is missing", got)
	}
}

// The other half, what makes the greying above mean something.
//
// An engine with no verdicts is a machine nothing has asked about and not one with nothing usable,
// so the form withholds no codec under a fact nobody established.
// Hence the contract having a shell ask for the probe and read again:
// the difference between these two answers is the whole of what a probe buys,
// and it reaches the screen only where somebody asks.
func TestResolveFormGreysNothingBeforeTheProbeHasRun(t *testing.T) {
	server := New(&probedBackend{}, events.New(), "test")

	form, err := server.ResolveForm(context.Background(), &screensharev1.ResolveFormRequest{})
	if err != nil {
		t.Fatalf("resolving a form answered %v, want an answer", err)
	}

	if option := encoderOption(t, form.GetForm(), "qsv"); !option.GetEnabled() {
		t.Errorf("the Quick Sync encoder is greyed on an unprobed machine with %q", option.GetReason())
	}
}

// The verdict a shell greys the apply on, answered where sameness is decided (publish.SamePipeline):
// a shell comparing fields would fall behind the builders the first time one read a field it did not name.
//
// The stream is built from what the empty draft repairs to on this machine,
// the settings a start would run being the ones a resolve answers with.
func TestResolveFormReportsADraftTheRunningStreamWasBuiltFrom(t *testing.T) {
	backend := &probedBackend{}
	server := New(backend, events.New(), "test")

	idle, err := server.ResolveForm(context.Background(), &screensharev1.ResolveFormRequest{})
	if err != nil {
		t.Fatalf("resolving a form answered %v, want an answer", err)
	}
	if idle.GetForm().GetInForce() {
		t.Fatal("a draft is reported in force with nothing publishing")
	}

	running := idle.GetForm().GetSettings()
	backend.publish = wire.PublishSnapshot{Live: &wire.LiveSnapshot{Settings: wire.ToSettings(running)}}

	same, err := server.ResolveForm(context.Background(), &screensharev1.ResolveFormRequest{Settings: running})
	if err != nil {
		t.Fatalf("resolving the running settings answered %v, want an answer", err)
	}
	if !same.GetForm().GetInForce() {
		t.Error("the settings the stream was built from are not reported in force")
	}

	moved := proto.Clone(running).(*screensharev1.Settings)
	moved.Publish.Fps = running.GetPublish().GetFps() + 1
	changed, err := server.ResolveForm(context.Background(), &screensharev1.ResolveFormRequest{Settings: moved})
	if err != nil {
		t.Fatalf("resolving a moved draft answered %v, want an answer", err)
	}
	if changed.GetForm().GetInForce() {
		t.Error("a draft at another frame rate is reported in force")
	}
}

// The read hands a shell something to draw and changes nothing,
// which lets a mount call it without putting seconds in front of the first paint.
// A flag that started the probe would let one shell's read replace the result a different shell's
// next resolve answers from.
func TestGetCatalogNeverProbes(t *testing.T) {
	backend := &probedBackend{probed: qsvMissing}
	server := New(backend, events.New(), "test")

	got, err := server.GetCatalog(context.Background(), &screensharev1.GetCatalogRequest{})
	if err != nil {
		t.Fatalf("reading the catalog answered %v, want an answer", err)
	}
	if backend.probes != 0 {
		t.Errorf("reading the catalog ran the probe %d times, want none", backend.probes)
	}

	// The catalog carries the probe's shape whether or not one has been taken.
	// Dropped, it would leave a shell unable to tell an unprobed machine from one with nothing usable.
	if got.GetCatalog().GetEncoders() == nil {
		t.Error("the catalog carries no encoder availability")
	}
}

// The probe replaces what every later resolve is answered from,
// so without the event a shell that never asked for one would watch its form start greying codecs
// with nothing having told it why.
// The event carries the whole catalog:
// a shell holding one has nothing to merge a half-state into.
func TestProbeEncodersAnnouncesWhatItFound(t *testing.T) {
	backend := &probedBackend{probed: qsvMissing}
	broker := events.New()
	server := New(backend, broker, "test")

	feed, cancel, err := broker.Subscribe(nil)
	if err != nil {
		t.Fatalf("subscribing answered %v, want a feed", err)
	}
	defer cancel()

	if _, err := server.ProbeEncoders(context.Background(), &screensharev1.ProbeEncodersRequest{}); err != nil {
		t.Fatalf("probing answered %v, want an answer", err)
	}
	if backend.probes != 1 {
		t.Errorf("probing ran the probe %d times, want once", backend.probes)
	}

	select {
	case event := <-feed:
		catalog := event.GetCatalog()
		if catalog == nil {
			t.Fatalf("the probe announced a %s event, want the catalog", events.KindOf(event))
		}
		if catalog.GetEncoders() == nil {
			t.Error("the announced catalog carries no encoder availability")
		}
	default:
		t.Fatal("the probe announced nothing, so a shell that did not ask never learns what it found")
	}
}

// textArgID reads one identifier out of a statement, and the empty string where it carries none.
// A statement names what it is about by argument name,
// so a test asks for one the way a surface does.
func textArgID(t *screensharev1.Text, name screensharev1.TextArgName) string {
	for _, arg := range t.GetArgs() {
		if arg.GetName() == name {
			return arg.GetId()
		}
	}
	return ""
}

// Discord mode's membership and link ride on no wire message (internal/wire, RelaySettings),
// so the draft a resolve answers with carries neither.
// Read back bare, the settings a stream was built from name a pipeline nothing is running,
// and a shell lights the apply on a stream already publishing exactly them.
func TestResolveFormReportsADiscordStreamInForce(t *testing.T) {
	backend := &probedBackend{}
	server := New(backend, events.New(), "test")

	idle, err := server.ResolveForm(context.Background(), &screensharev1.ResolveFormRequest{})
	if err != nil {
		t.Fatalf("resolving a form answered %v, want an answer", err)
	}
	draft := idle.GetForm().GetSettings()
	draft.Relay.DiscordMode = true

	resolved, err := server.ResolveForm(context.Background(), &screensharev1.ResolveFormRequest{Settings: draft})
	if err != nil {
		t.Fatalf("resolving a Discord draft answered %v, want an answer", err)
	}
	// What a start runs, the brokered facts injected the way the backend injects them
	// (internal/app, StartPublish).
	running := backend.Brokered(wire.ToSettings(resolved.GetForm().GetSettings()))
	backend.publish = wire.PublishSnapshot{Live: &wire.LiveSnapshot{Settings: running}}

	same, err := server.ResolveForm(context.Background(), &screensharev1.ResolveFormRequest{Settings: draft})
	if err != nil {
		t.Fatalf("resolving the running settings answered %v, want an answer", err)
	}
	if !same.GetForm().GetInForce() {
		t.Error("the settings the Discord stream was built from are not reported in force")
	}
}

// Brokered is Discord mode's injection: the link and the group the manager derives for the current
// voice channel, which the settings a shell sends carry on neither direction (internal/app, withBrokered).
func (p *probedBackend) Brokered(s settings.Settings) settings.Settings {
	if !s.Relay.DiscordMode {
		return s
	}
	s.Relay.DiscordLink = "link-secret"
	s.Relay = s.Relay.WithBrokeredGroup("PREFIX/", "passphrase", "Bob")
	return s
}
