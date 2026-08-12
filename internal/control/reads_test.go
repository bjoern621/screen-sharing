package control

import (
	"context"
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/events"
	"bjoernblessin.de/screenshare/internal/platform"
)

// probedBackend is a machine whose encoder probe has been taken: one family it cannot run
// and one it can. Everything else is fakeBackend's.
//
// The platform is stated because it is what decides which capture backends are reachable,
// and a backend with no operating system names no publish engine - which would leave every
// codec ungreyed for want of an engine to grey it on, and the test passing for the wrong
// reason.
type probedBackend struct {
	fakeBackend
	// probed is what the probe found, and is left zero by the test that wants an unprobed
	// machine.
	probed encoders.Availability
	// probes counts how many times the probe was actually run.
	probes int
}

func (p *probedBackend) Platform() platform.Info               { return platform.Info{OS: "windows"} }
func (p *probedBackend) CachedEncoders() encoders.Availability { return p.probed }
func (p *probedBackend) AudioDevices() []platform.AudioDevice  { return nil }

// Encoders counts its calls, which is what lets a test say that a read did not probe.
// The contract's whole reason for splitting the two is that reading the catalog must
// not start seconds of work, and a count is the only way to observe that it did not.
func (p *probedBackend) Encoders(context.Context) encoders.Availability {
	p.probes++
	return p.probed
}

// qsvMissing is what the probe found on a machine with no Intel GPU: the two Quick Sync
// encoders would not run and the NVIDIA one did. The zero Availability beside it is the same
// machine before the probe, which is a different fact and not a weaker version of this one.
var qsvMissing = encoders.Availability{Usable: map[string]map[string]bool{
	capabilities.EngineFfmpeg: {"h264_qsv": false, "hevc_qsv": false, "hevc_nvenc": true},
}}

// codecOption finds one entry of the codec control in a resolved form, so the test reads the
// contract's own shape rather than form's internals.
func codecOption(t *testing.T, form *screensharev1.Form, codec string) *screensharev1.FieldOption {
	t.Helper()

	for _, group := range form.GetGroups() {
		for _, field := range group.GetFields() {
			if field.GetKey() != "publish.codec" {
				continue
			}
			for _, option := range field.GetOptions() {
				if option.GetValue() == codec {
					return option
				}
			}
		}
	}

	t.Fatalf("the resolved form offers no codec %q", codec)
	return nil
}

// TestResolveFormGreysAnEncoderTheProbeCouldNotRun is the seam between the probe and the
// screen, and it is the one place both halves are visible at once.
//
// The probe is what knows an Intel encoder is not on this machine; the form is what says so;
// and ResolveForm is where the first reaches the second. It reads CachedEncoders rather than
// probing, so this is also the difference the read is written around: before the probe the
// same call greys nothing, and a shell that never asked for one would go on offering an
// encoder that fails at launch.
//
// The entry stays in the list, which is the treatment rather than an accident. A general
// concept the machine blocks is taught by a greyed option and its sentence, not by a dropdown
// that is quietly one item shorter (docs/field-availability.md, "The rule").
func TestResolveFormGreysAnEncoderTheProbeCouldNotRun(t *testing.T) {
	server := New(&probedBackend{probed: qsvMissing}, events.New(), "test")

	form, err := server.ResolveForm(context.Background(), &screensharev1.ResolveFormRequest{})
	if err != nil {
		t.Fatalf("resolving a form answered %v, want an answer", err)
	}

	option := codecOption(t, form.GetForm(), "h264_qsv")
	if option.GetEnabled() {
		t.Fatal("h264_qsv is offered on a machine whose probe could not run it")
	}
	// The statement names the family whose device is missing, not a sentence about it:
	// what "no Intel Quick Sync encoder on this machine" reads as is the surface's, and
	// what is true is this. A reason that greyed the codec without naming the family
	// would leave the reader with nothing to act on either way.
	if option.GetReason().GetCode() != screensharev1.TextCode_TEXT_CODE_PROBE_NO_DEVICE {
		t.Errorf("h264_qsv greys with %v, want the probe's no-device verdict", option.GetReason().GetCode())
	}
	if got := textArgID(option.GetReason(), screensharev1.TextArgName_TEXT_ARG_NAME_FAMILY); got != "qsv" {
		t.Errorf("h264_qsv greys naming family %q, which is not the hardware that is missing", got)
	}
}

// TestResolveFormGreysNothingBeforeTheProbeHasRun states the other half, because it is what
// makes the greying above mean something.
//
// An engine with no verdicts is a machine nothing has been asked about, not one with nothing
// usable, so the form withholds no codec under a fact nobody established. That is why the
// contract has a shell ask for the probe and read again: the difference between these two
// answers is the whole of what the probe buys, and it only reaches the screen if somebody
// asks.
func TestResolveFormGreysNothingBeforeTheProbeHasRun(t *testing.T) {
	server := New(&probedBackend{}, events.New(), "test")

	form, err := server.ResolveForm(context.Background(), &screensharev1.ResolveFormRequest{})
	if err != nil {
		t.Fatalf("resolving a form answered %v, want an answer", err)
	}

	if option := codecOption(t, form.GetForm(), "h264_qsv"); !option.GetEnabled() {
		t.Errorf("h264_qsv is greyed on an unprobed machine with %q", option.GetReason())
	}
}

// TestGetCatalogNeverProbes: the read hands a shell something to draw and changes
// nothing, which is what lets it be called on a mount without putting seconds in front
// of the first paint. It used to take a flag that started the probe, and the flag was
// the defect - one shell's read replaced the result a different shell's next resolve
// answered from.
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

	// The catalog still carries the probe's shape, whether or not one has been taken: a
	// catalog that dropped it would leave a shell unable to tell an unprobed machine
	// from one with nothing usable.
	if got.GetCatalog().GetEncoders() == nil {
		t.Error("the catalog carries no encoder availability")
	}
}

// TestProbeEncodersAnnouncesWhatItFound: the probe replaces what every later resolve is
// answered from, so a shell that never asked for one would otherwise watch its form
// start greying codecs with nothing having told it why. The event is how the other
// shells find out, and it carries the whole catalog because a shell holding one has
// nothing to merge a half-state into.
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

// textArgID reads one identifier out of a statement, and the empty string where it
// carries none. A statement names what it is about by argument name rather than by
// position, so a test asserting on one asks for it the same way a surface does.
func textArgID(t *screensharev1.Text, name screensharev1.TextArgName) string {
	for _, arg := range t.GetArgs() {
		if arg.GetName() == name {
			return arg.GetId()
		}
	}
	return ""
}
