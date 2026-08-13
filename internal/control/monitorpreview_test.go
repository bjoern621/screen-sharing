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

// A start the backend refuses is FAILED_PRECONDITION and never INVALID_ARGUMENT:
// the request is well formed - every integer is a monitor index somewhere - and what refuses it is
// a fact about this machine, which is the distinction docs/ipc-api.md draws between the two codes.
// The sentence is the backend's own, so the reason travels rather than being replaced.
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

// A stop is not refused for a screen nothing is reading, which is the idempotence the whole
// contract holds to: a stop names the state the caller wants and that state already holds.
// The backend's error field is set here to prove the stop does not consult it.
func TestStoppingAScreenNobodyIsReadingSucceeds(t *testing.T) {
	server := New(&fakeBackend{err: errors.New("would refuse anything that asked")}, events.New(), "test")

	if _, err := server.StopMonitorPreview(context.Background(),
		&screensharev1.StopMonitorPreviewRequest{Monitor: 7}); err != nil {
		t.Errorf("stopping a screen nothing is reading was refused: %v", err)
	}
}

// The three subscription arms are dispatched to three different backend methods,
// and a subscription that named none of them is refused rather than served the first arm.
// Two arms carry no key at all, so a discriminator read off a missing key would send an empty
// request to whichever method it defaulted to.
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
