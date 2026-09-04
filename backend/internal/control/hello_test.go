package control

import (
	"bjoernblessin.de/screenshare/internal/pointer"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/encoderate"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/events"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/portal"
	"bjoernblessin.de/screenshare/internal/reach"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// fakeBackend is a backend with no window, no encoder and no relay.
//
// The property Backend was made an interface for (server.go):
// the contract serves in front of something a test reaches entirely.
// Written by hand rather than generated,
// what these tests need from a backend being one field at a time.
type fakeBackend struct {
	publish wire.PublishSnapshot
	// settings is what the backend holds, for the reads and for the refusals decided off them.
	settings settings.Settings
	// members is the group this machine shares, as the presence loop last read it.
	members wire.MembersSnapshot
	// legs is what a relay check answers, which no err field can stand in for:
	// every leg comes back with a verdict of its own,
	// and a relay that answers nothing is still a response.
	legs []reach.Result
	// err is what every effect answers with,
	// so a test wanting a refusal sets one field rather than one per method.
	err error
}

func (f *fakeBackend) Settings() settings.Settings                    { return f.settings }
func (f *fakeBackend) StoreNotice() *screensharev1.Text               { return nil }
func (f *fakeBackend) Monitors() []display.Monitor                    { return nil }
func (f *fakeBackend) Platform() platform.Info                        { return platform.Info{} }
func (f *fakeBackend) Device() capabilities.Device                    { return capabilities.Device{} }
func (f *fakeBackend) Encoders(context.Context) encoders.Availability { return encoders.Availability{} }
func (f *fakeBackend) CachedEncoders() encoders.Availability          { return encoders.Availability{} }
func (f *fakeBackend) AudioDevices() []platform.AudioDevice           { return nil }
func (f *fakeBackend) PortalCapabilities() portal.Capabilities        { return portal.Capabilities{} }
func (f *fakeBackend) Pointer() (pointer.Spot, bool)                  { return pointer.Spot{}, false }

func (f *fakeBackend) StreamPointer(wire.StreamRef) (pointer.Spot, bool) {
	return pointer.Spot{}, false
}
func (f *fakeBackend) PublishState() wire.PublishSnapshot             { return f.publish }
func (f *fakeBackend) RelayStatus() relay.Status                      { return relay.Status{} }
func (f *fakeBackend) Watching() []wire.StreamRef                     { return nil }
func (f *fakeBackend) ReceiveState() []wire.ReceiveStream             { return nil }
func (f *fakeBackend) AudioLevels() []wire.AudioLevel                 { return nil }
func (f *fakeBackend) MembersState() wire.MembersSnapshot             { return f.members }
func (f *fakeBackend) Brokered(s settings.Settings) settings.Settings { return s }
func (f *fakeBackend) DiscordState() wire.DiscordSnapshot             { return wire.DiscordSnapshot{} }
func (f *fakeBackend) MaxTestStreams() int                            { return 9 }
func (f *fakeBackend) MeasureUplink(context.Context) (float64, error) { return 0, f.err }
func (f *fakeBackend) MeasureEncodeRate(context.Context, settings.Settings) (encoderate.Rate, error) {
	return encoderate.Rate{}, f.err
}

func (f *fakeBackend) CheckRelay(context.Context, settings.Settings) []reach.Result {
	return f.legs
}

func (f *fakeBackend) SaveSettings(settings.Settings) error    { return f.err }
func (f *fakeBackend) StartPublish(settings.Settings) error    { return f.err }
func (f *fakeBackend) ApplyToStream(settings.Settings) error   { return f.err }
func (f *fakeBackend) StopPublish()                            {}
func (f *fakeBackend) StartWatch(wire.StreamRef) error         { return f.err }
func (f *fakeBackend) StopWatch(wire.StreamRef)                {}
func (f *fakeBackend) StartReceive(wire.StreamRef, bool) error { return nil }
func (f *fakeBackend) StopReceive(wire.StreamRef)              {}

func (f *fakeBackend) SetReceiveAudio(wire.StreamRef, float64, bool) error { return f.err }

// The frame subscriptions refuse, what a backend with no pipeline behind it has to answer:
// nothing is decoding, publishing or previewing here,
// and a fake stream of handles would name GPU memory that does not exist.
func (f *fakeBackend) SubscribeFrames(wire.StreamRef) (FrameStream, error) {
	return nil, errors.New("nothing is decoding")
}

func (f *fakeBackend) SubscribePreviewFrames() (FrameStream, error) {
	return nil, errors.New("nothing is publishing with a local preview")
}

func (f *fakeBackend) SubscribeMonitorFrames(monitor int) (FrameStream, error) {
	return nil, fmt.Errorf("nothing is previewing monitor %d", monitor)
}

func (f *fakeBackend) StartMonitorPreview(int) error { return f.err }

func (f *fakeBackend) StopMonitorPreview(int) {}

func (f *fakeBackend) MonitorPreviewState() []wire.PreviewedMonitor { return nil }

func (f *fakeBackend) TestStreamState() (int, []wire.TestStreamSlot) { return 0, nil }

func (f *fakeBackend) StartTestStreams(int) error { return f.err }
func (f *fakeBackend) StopTestStreams()           {}
func (f *fakeBackend) ForgetPortalConsent() error { return f.err }
func (f *fakeBackend) OpenLog(string) error       { return f.err }
func (f *fakeBackend) LinkDiscord(context.Context, settings.Relay) error { return nil }
func (f *fakeBackend) CreateGroup(settings.Relay) (string, string, error) {
	return "", "", f.err
}

func (f *fakeBackend) OpenLogsFolder() error              { return f.err }
func (f *fakeBackend) OpenInBrowser(wire.StreamRef) error { return f.err }

// The handshake is the last call two sides that disagree about the contract can both understand,
// so its refusal carries the one thing neither works out afterwards: which major each is on.
// A refusal naming only the backend's leaves a reader with no way to learn what the shell wanted.
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

// The answer is what a shell puts in a bug report and what it compares its own minor against,
// so every field is this build's rather than an echo of what was asked.
func TestAMatchingMajorIsAnsweredWithThisBuildsNumbers(t *testing.T) {
	const version = "v9.9.9-test"
	server := New(&fakeBackend{}, events.New(), version)

	answer, err := server.Hello(context.Background(), &screensharev1.HelloRequest{
		Client:        "avalonia",
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

// The field exists so the version is settled explicitly,
// and a request that left it unset settled nothing.
// Answering one would let a shell that never looked at the contract version reach every other method.
func TestAShellThatNamesNoMajorIsRefused(t *testing.T) {
	server := New(&fakeBackend{}, events.New(), "test")

	_, err := server.Hello(context.Background(), &screensharev1.HelloRequest{Client: "cli"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("code = %s, want %s", status.Code(err), codes.FailedPrecondition)
	}
}
