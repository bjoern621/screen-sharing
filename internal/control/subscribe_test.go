package control

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/events"
)

// fakeStream stands in for the client's half of a server-streaming call.
//
// grpc.ServerStream is embedded and left nil: the two methods Subscribe uses are Context and Send,
// and a call to any other would be a call this test wants to hear about as a panic rather than to
// have quietly satisfied.
// The buffer on sent is what keeps Send from blocking after a test has stopped reading,
// so a test that fails still lets the call under test return instead of deadlocking on its way out.
type fakeStream struct {
	grpc.ServerStream

	ctx  context.Context
	sent chan *screensharev1.Event
}

func newFakeStream(ctx context.Context) *fakeStream {
	return &fakeStream{ctx: ctx, sent: make(chan *screensharev1.Event, 1024)}
}

func (f *fakeStream) Context() context.Context { return f.ctx }

func (f *fakeStream) Send(event *screensharev1.Event) error {
	f.sent <- event
	return nil
}

// unknownKind is an EventKind value no build declares, which is what a shell generated against a
// later minor version would send.
// It is a number rather than a misspelt name because the kinds are an enum now:
// the mistake a shell can still make is naming a kind that exists somewhere and not here.
const unknownKind = screensharev1.EventKind(9999)

// TestAnUnknownEventKindIsRefusedRatherThanIgnored: a kind this build has none of would otherwise
// leave the shell holding an open stream that never delivers, which reads as a backend where
// nothing is happening rather than as a name that was got wrong.
// The refusal has to name the kind, because the request may have carried several and only one of
// them is the mistake.
func TestAnUnknownEventKindIsRefusedRatherThanIgnored(t *testing.T) {
	server := New(&fakeBackend{}, events.New(), "test")

	err := server.Subscribe(
		&screensharev1.SubscribeRequest{Kinds: []screensharev1.EventKind{screensharev1.EventKind_EVENT_KIND_PUBLISH_STATE, unknownKind}},
		newFakeStream(context.Background()))
	if err == nil {
		t.Fatal("a kind this build has none of was accepted, want it refused")
	}
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("code = %s, want %s", got, codes.InvalidArgument)
	}
	if message := status.Convert(err).Message(); !strings.Contains(message, strconv.Itoa(int(unknownKind))) {
		t.Errorf("refusal %q does not name the kind that was wrong", message)
	}
}

// TestANarrowedSubscriptionReceivesOnlyItsKinds: narrowing exists for a surface that wants the
// state changes without the per-second statistics, so a filter that leaked would hand that surface
// exactly the traffic it asked not to have.
func TestANarrowedSubscriptionReceivesOnlyItsKinds(t *testing.T) {
	broker := events.New()
	server := New(&fakeBackend{}, broker, "test")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := newFakeStream(ctx)

	go func() {
		// What this call returns is the next test's subject; here it only has to be open.
		_ = server.Subscribe(&screensharev1.SubscribeRequest{Kinds: []screensharev1.EventKind{screensharev1.EventKind_EVENT_KIND_VIEWER_STATE}}, out)
	}()

	wanted := &screensharev1.Event{
		Payload: &screensharev1.Event_ViewerState{ViewerState: &screensharev1.ViewerState{}},
	}
	unwanted := &screensharev1.Event{
		Payload: &screensharev1.Event_PublishStats{PublishStats: &screensharev1.PublishStats{}},
	}

	// The subscription opens on another goroutine and the broker offers no signal for when it has been
	// registered, so both kinds are published until something arrives rather than once behind a sleep.
	// Publishing both every round is what makes the arrival conclusive: a filter that let the wrong
	// kind through would have delivered it by the round the right one is delivered on.
	var received *screensharev1.Event
	for attempt := 0; attempt < 200 && received == nil; attempt++ {
		broker.Publish(unwanted)
		broker.Publish(wanted)

		select {
		case event := <-out.sent:
			received = event
		case <-time.After(10 * time.Millisecond):
		}
	}
	if received == nil {
		t.Fatal("a narrowed subscription received nothing, want the kind it asked for")
	}

	for {
		if kind := events.KindOf(received); kind != screensharev1.EventKind_EVENT_KIND_VIEWER_STATE {
			t.Fatalf("received a %s event, want only %s", kind, screensharev1.EventKind_EVENT_KIND_VIEWER_STATE)
		}
		select {
		case received = <-out.sent:
		default:
			return
		}
	}
}

// TestASubscriptionEndsWithTheClientThatOpenedIt: the broker sends on a channel it holds until the
// cancel removes it, so a call that outlived its client would leave a subscriber nothing reads and
// every publish still has to walk past.
// The call returning is the observable half of that release - the only way out of the loop is
// through the deferred cancel - and it is what a shell closing its window has to produce.
func TestASubscriptionEndsWithTheClientThatOpenedIt(t *testing.T) {
	server := New(&fakeBackend{}, events.New(), "test")

	ctx, cancel := context.WithCancel(context.Background())
	out := newFakeStream(ctx)

	ended := make(chan error, 1)
	go func() {
		ended <- server.Subscribe(&screensharev1.SubscribeRequest{}, out)
	}()

	cancel()

	select {
	case err := <-ended:
		if err != nil {
			t.Errorf("a client that went away ended the call with %v, want no failure", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the call did not return after its client's context ended, so the subscription is still held")
	}
}
