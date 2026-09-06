package events

import (
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// KindOf is a second copy of the contract's payload list, and a shell subscribing to a subset
// never receives a payload it does not name.
//
// So a payload added to Event without an arm here is a state announced to nobody,
// and this walks the oneof off the descriptor rather than listing the payloads again:
// a list would be a third copy, drifting alongside the second.
//
// Defect locked out: an event the broker cannot name, which asserts inside Publish
// on the first announcement rather than in a build.
func TestEveryPayloadHasAKind(t *testing.T) {
	event := &screensharev1.Event{}
	message := event.ProtoReflect()

	oneofs := message.Descriptor().Oneofs()
	if oneofs.Len() == 0 {
		t.Fatal("Event carries no payload oneof, so this test asserts nothing")
	}

	payload := oneofs.Get(0)
	for i := range payload.Fields().Len() {
		field := payload.Fields().Get(i)

		one := message.New()
		one.Set(field, one.NewField(field))

		kind := KindOf(one.Interface().(*screensharev1.Event))
		if kind == screensharev1.EventKind_EVENT_KIND_UNSPECIFIED {
			t.Errorf("payload %s has no kind, so a filtered subscription never receives it", field.Name())
			continue
		}
		if !Known(kind) {
			t.Errorf("payload %s names %s, which Kinds does not list, so Publish refuses it", field.Name(), kind)
		}
	}
}
