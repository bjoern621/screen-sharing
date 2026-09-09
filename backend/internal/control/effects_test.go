package control

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/events"
	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/wire"
)

// A start naming the pipeline already publishing succeeds and starts nothing,
// in Discord mode as in any other.
// Membership is brokered rather than sent (internal/wire, RelaySettings),
// so a draft read straight off the request names a pipeline that is not the one running,
// and the repeat a shell makes after a lost answer would be refused as a second stream.
func TestStartPublishRepeatsADiscordStream(t *testing.T) {
	backend := &probedBackend{}
	server := New(backend, events.New(), "test")

	idle, err := server.ResolveForm(context.Background(), &screensharev1.ResolveFormRequest{})
	if err != nil {
		t.Fatalf("resolving a form answered %v, want an answer", err)
	}
	draft := idle.GetForm().GetSettings()
	draft.Relay.GroupSource = group.SourceDiscord

	running := backend.Brokered(wire.ToSettings(draft))
	backend.publish = wire.PublishSnapshot{Live: &wire.LiveSnapshot{Settings: running}}

	if _, err := server.StartPublish(context.Background(), &screensharev1.StartPublishRequest{Settings: draft}); err != nil {
		t.Errorf("starting the stream already publishing answered %v, want the repeat to succeed", err)
	}
}

// The name the service stored the report under is what a reader quotes,
// so it crosses the contract rather than being logged where the press cannot see it.
func TestSendReportAnswersTheStoredName(t *testing.T) {
	backend := &probedBackend{}
	backend.reportID = "7f3a91c2"
	server := New(backend, events.New(), "test")

	sent, err := server.SendReport(context.Background(), &screensharev1.SendReportRequest{})
	if err != nil {
		t.Fatalf("sending a report answered %v, want the send to succeed", err)
	}
	if got := sent.GetReportId(); got != backend.reportID {
		t.Errorf("report id = %q, want %q", got, backend.reportID)
	}
}

// A report goes to the group service of the deployment in use,
// and settings naming no relay are a moment that is wrong rather than a request that is
// (docs/ipc-api.md).
func TestSendReportRefusesWithNoRelay(t *testing.T) {
	backend := &probedBackend{}
	backend.err = errors.New("the settings name no relay")
	server := New(backend, events.New(), "test")

	if _, err := server.SendReport(context.Background(), &screensharev1.SendReportRequest{}); err == nil {
		t.Fatal("sending a report to no relay succeeded, want a refusal")
	} else if got := status.Code(err); got != codes.FailedPrecondition {
		t.Errorf("code = %s, want %s", got, codes.FailedPrecondition)
	}
}
