package control

import (
	"context"
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/events"
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
	draft.Relay.DiscordMode = true

	running := backend.Brokered(wire.ToSettings(draft))
	backend.publish = wire.PublishSnapshot{Live: &wire.LiveSnapshot{Settings: running}}

	if _, err := server.StartPublish(context.Background(), &screensharev1.StartPublishRequest{Settings: draft}); err != nil {
		t.Errorf("starting the stream already publishing answered %v, want the repeat to succeed", err)
	}
}
