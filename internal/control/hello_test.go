package control

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/encoderate"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/events"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// fakeBackend is a backend with no window, no encoder and no relay.
//
// It is the property Backend was made an interface for (server.go): the contract can be
// served in front of something a test can reach entirely. Every read answers with a
// field and every effect answers with err, which is enough for the tests in this
// package and is why it is written by hand instead of generated - what these tests need
// from a backend is one field at a time.
type fakeBackend struct {
	publish wire.PublishSnapshot
	// err is what every effect answers with, so a test that wants a refusal sets one
	// thing rather than one thing per method.
	err error
}

func (f *fakeBackend) Settings() settings.Settings                    { return settings.Settings{} }
func (f *fakeBackend) StoreNotice() *screensharev1.Text               { return nil }
func (f *fakeBackend) Monitors() []display.Monitor                    { return nil }
func (f *fakeBackend) Platform() platform.Info                        { return platform.Info{} }
func (f *fakeBackend) Encoders(context.Context) encoders.Availability { return encoders.Availability{} }
func (f *fakeBackend) CachedEncoders() encoders.Availability          { return encoders.Availability{} }
func (f *fakeBackend) PublishState() wire.PublishSnapshot             { return f.publish }
func (f *fakeBackend) RelayStatus() relay.Status                      { return relay.Status{} }
func (f *fakeBackend) Watching() []wire.WatchKey                      { return nil }
func (f *fakeBackend) ReceiveState() []wire.ReceiveStream             { return nil }
func (f *fakeBackend) TestStreamsRunning() int                        { return 0 }
func (f *fakeBackend) MaxTestStreams() int                            { return 9 }
func (f *fakeBackend) MeasureUplink(context.Context) (float64, error) { return 0, f.err }
func (f *fakeBackend) MeasureEncodeRate(context.Context, settings.Settings) (encoderate.Rate, error) {
	return encoderate.Rate{}, f.err
}

func (f *fakeBackend) SaveSettings(settings.Settings) error  { return f.err }
func (f *fakeBackend) StartPublish(settings.Settings) error  { return f.err }
func (f *fakeBackend) ApplyToStream(settings.Settings) error { return f.err }
func (f *fakeBackend) StopPublish()                          {}
func (f *fakeBackend) StartWatch(wire.WatchKey) error        { return f.err }
func (f *fakeBackend) StopWatch(wire.WatchKey)               {}
func (f *fakeBackend) StartReceive(wire.WatchKey) error      { return nil }
func (f *fakeBackend) StopReceive(wire.WatchKey)             {}

// SubscribeFrames and SubscribePreviewFrames refuse, which is what a backend with no
// pipeline behind it has to answer: there is no decode here to draw from and nothing is
// publishing, and a fake stream of handles would be a fake naming GPU memory that does
// not exist.
func (f *fakeBackend) SubscribeFrames(wire.WatchKey) (FrameStream, error) {
	return nil, errors.New("nothing is decoding")
}

func (f *fakeBackend) SubscribePreviewFrames() (FrameStream, error) {
	return nil, errors.New("nothing is publishing with a local preview")
}

func (f *fakeBackend) StartTestStreams(int) error { return f.err }
func (f *fakeBackend) StopTestStreams()           {}
func (f *fakeBackend) ForgetPortalConsent() error { return f.err }
func (f *fakeBackend) OpenLog(string) error       { return f.err }
func (f *fakeBackend) OpenLogsFolder() error      { return f.err }

// TestAMismatchedMajorNamesBothVersions: the handshake is the last call a backend and a
// shell that disagree about the contract can both still understand, so the refusal has
// to carry the one thing neither of them can work out afterwards - which major each is
// on. A refusal that named only its own would leave the user reading "this backend is
// on 1" with no way to know what their shell wanted.
func TestAMismatchedMajorNamesBothVersions(t *testing.T) {
	server := New(&fakeBackend{}, events.New(), "test")

	_, err := server.Hello(context.Background(), &screensharev1.HelloRequest{
		Client:        "avalonia",
		ProtocolMajor: ProtocolMajor + 1,
	})
	if err == nil {
		t.Fatal("a major this build does not implement was accepted, want it refused")
	}
	if got := status.Code(err); got != codes.FailedPrecondition {
		t.Errorf("code = %s, want %s", got, codes.FailedPrecondition)
	}

	message := status.Convert(err).Message()
	for _, version := range []int{ProtocolMajor, ProtocolMajor + 1} {
		if !strings.Contains(message, strconv.Itoa(version)) {
			t.Errorf("refusal %q does not name major %d", message, version)
		}
	}
}

// TestAMatchingMajorIsAnsweredWithThisBuildsNumbers: the answer is what a shell reports
// in a bug report and what it compares its own minor against, so all three fields have
// to be this build's rather than an echo of what was asked.
func TestAMatchingMajorIsAnsweredWithThisBuildsNumbers(t *testing.T) {
	const version = "v9.9.9-test"
	server := New(&fakeBackend{}, events.New(), version)

	answer, err := server.Hello(context.Background(), &screensharev1.HelloRequest{
		Client:        "wails",
		ProtocolMajor: ProtocolMajor,
	})
	if err != nil {
		t.Fatalf("the major this build implements was refused with %v, want it accepted", err)
	}
	if answer.GetProtocolMajor() != ProtocolMajor {
		t.Errorf("major = %d, want %d", answer.GetProtocolMajor(), ProtocolMajor)
	}
	if answer.GetProtocolMinor() != ProtocolMinor {
		t.Errorf("minor = %d, want %d", answer.GetProtocolMinor(), ProtocolMinor)
	}
	if answer.GetBackendVersion() != version {
		t.Errorf("backend version = %q, want %q", answer.GetBackendVersion(), version)
	}
}

// TestAShellThatNamesNoMajorIsRefused: the field exists so the version is settled
// explicitly, so a request that left it unset has settled nothing. Answering one would
// let a shell that never looked at the contract version reach every other method.
func TestAShellThatNamesNoMajorIsRefused(t *testing.T) {
	server := New(&fakeBackend{}, events.New(), "test")

	_, err := server.Hello(context.Background(), &screensharev1.HelloRequest{Client: "cli"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("code = %s, want %s", status.Code(err), codes.FailedPrecondition)
	}
}
