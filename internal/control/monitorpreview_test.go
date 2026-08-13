package control

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/events"
	"bjoernblessin.de/screenshare/internal/wire"
)

// A start the backend refuses with a plain error is FAILED_PRECONDITION: the request is well formed,
// every integer being a monitor index somewhere, and what refuses it is a fact about this machine.
// That is the line docs/ipc-api.md draws between the two codes.
// The sentence stays the backend's, so the reason travels instead of being replaced.
func TestARefusedMonitorPreviewIsAPrecondition(t *testing.T) {
	server := New(&fakeBackend{err: errors.New("this session cannot read one monitor apart from another")},
		events.New(), "test")

	_, err := server.StartMonitorPreview(context.Background(),
		&screensharev1.StartMonitorPreviewRequest{Monitor: 2})
	if err == nil {
		t.Fatal("a backend that refused the screen answered success")
	}
	if got := status.Code(err); got != codes.FailedPrecondition {
		t.Errorf("code = %s, want %s", got, codes.FailedPrecondition)
	}

	message := status.Convert(err).Message()
	if !strings.Contains(message, "cannot read one monitor apart from another") {
		t.Errorf("refusal %q drops the backend's own reason", message)
	}
	if !strings.Contains(message, "2") {
		t.Errorf("refusal %q does not name the screen it is about", message)
	}
}

// An index no output is enumerated under is INVALID_ARGUMENT, which the contract states and which
// only a typed refusal can carry: both cases arrive as an error from one method, and a code read off
// the sentence would be the contract deriving itself from prose (refusal.go).
func TestAPreviewOfAScreenThatDoesNotExistIsARequestFault(t *testing.T) {
	server := New(&fakeBackend{err: Refuse("monitor 9 is not one of this machine's outputs")},
		events.New(), "test")

	_, err := server.StartMonitorPreview(context.Background(),
		&screensharev1.StartMonitorPreviewRequest{Monitor: 9})
	if err == nil {
		t.Fatal("a screen this machine does not have was previewed")
	}
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("code = %s, want %s", got, codes.InvalidArgument)
	}
	if message := status.Convert(err).Message(); !strings.Contains(message, "not one of this machine's outputs") {
		t.Errorf("refusal %q drops the backend's own reason", message)
	}
}

// The same line on the watch pair: a leg this build has no viewer for is the request naming
// something that does not exist, while a leg that cannot carry the stream's present format is the
// world not being ready.
func TestAWatchOverALegThisBuildHasNoViewerForIsARequestFault(t *testing.T) {
	viewer := &screensharev1.WatchKey{StreamName: "desk", Transport: "moq"}

	refused := New(&fakeBackend{err: Refuse("no viewer implements transport %q", "moq")}, events.New(), "test")
	_, err := refused.StartWatch(context.Background(), &screensharev1.StartWatchRequest{Viewer: viewer})
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("a leg with no viewer answered %s, want %s", got, codes.InvalidArgument)
	}

	unready := New(&fakeBackend{err: errors.New("desk is av1, which srt cannot carry")}, events.New(), "test")
	_, err = unready.StartWatch(context.Background(), &screensharev1.StartWatchRequest{Viewer: viewer})
	if got := status.Code(err); got != codes.FailedPrecondition {
		t.Errorf("a leg that cannot carry the format answered %s, want %s", got, codes.FailedPrecondition)
	}
}

// A stop names the state the caller wants, and for a screen nothing is reading that state already
// holds, so the call succeeds: the idempotency the whole contract is built on.
// The backend's error field is set to prove the stop never consults it.
func TestStoppingAScreenNobodyIsReadingSucceeds(t *testing.T) {
	server := New(&fakeBackend{err: errors.New("would refuse anything that asked")}, events.New(), "test")

	if _, err := server.StopMonitorPreview(context.Background(),
		&screensharev1.StopMonitorPreviewRequest{Monitor: 7}); err != nil {
		t.Errorf("stopping a screen nothing is reading was refused: %v", err)
	}
}

// The three subscription arms reach three different backend methods, and one that named no arm is
// refused rather than served the first.
// Two arms carry no key, so a discriminator read off a missing key would send an empty request to
// whichever method it fell through to.
func TestAFrameSubscriptionNamesOneOfTheThreePictures(t *testing.T) {
	for name, tc := range map[string]struct {
		subscribe *screensharev1.FrameSubscribe
		want      wire.FrameSourceKind
		named     bool
	}{
		"a relay decode carries the pair that identifies it": {
			subscribe: &screensharev1.FrameSubscribe{
				Source: &screensharev1.FrameSubscribe_Stream{
					Stream: &screensharev1.WatchKey{StreamName: "desk", Transport: "srt"},
				},
			},
			want:  wire.FrameSourceRelay,
			named: true,
		},
		"the publish preview carries nothing and needs nothing": {
			subscribe: &screensharev1.FrameSubscribe{
				Source: &screensharev1.FrameSubscribe_PublishPreview{
					PublishPreview: &screensharev1.PublishPreview{},
				},
			},
			want:  wire.FrameSourcePublishPreview,
			named: true,
		},
		"a screen carries the index its output is enumerated under": {
			subscribe: &screensharev1.FrameSubscribe{
				Source: &screensharev1.FrameSubscribe_MonitorPreview{
					MonitorPreview: &screensharev1.MonitorPreview{Monitor: 3},
				},
			},
			want:  wire.FrameSourceMonitorPreview,
			named: true,
		},
		"a subscription that filled no arm names none of them": {
			subscribe: &screensharev1.FrameSubscribe{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			source, named := wire.FrameSourceOf(tc.subscribe)

			if named != tc.named {
				t.Fatalf("named = %v, want %v", named, tc.named)
			}
			if !named {
				return
			}
			if source.Kind != tc.want {
				t.Errorf("kind = %v, want %v", source.Kind, tc.want)
			}
			if tc.want == wire.FrameSourceMonitorPreview && source.Monitor != 3 {
				t.Errorf("monitor = %d, want 3", source.Monitor)
			}
		})
	}
}
