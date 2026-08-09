// Package events is the one place a change to the running state is announced.
//
// Every producer in the backend publishes here and every surface in front of it
// subscribes here, which is what keeps a window that pressed a button and a window
// that did not from being told different things (docs/ipc-api.md, "Events").
//
// The events are the contract's own messages rather than a shape of this package's
// own. A second state vocabulary between the producer and the wire is the drift the
// contract exists to remove, so a producer builds a screensharev1.Event and this
// package only decides who receives it and in what order.
//
// Two rules of the contract are structural here. Every event carries a whole state,
// never a delta, so a dropped event costs a subscriber nothing but the interval it
// was stale for. And a subscriber that acted still waits for the event, so an effect
// never answers with the state it produced.
package events

import (
	"strconv"
	"sync"

	"google.golang.org/protobuf/proto"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// Kinds lists every event kind, in the order events.proto declares them.
//
// They are the contract's own enum rather than strings this package spells. A kind a
// subscriber names used to be the oneof's field name typed into a request, which put
// the set in three places that could each miss a value and made a misspelling a
// subscription that waits forever.
var Kinds = []screensharev1.EventKind{
	screensharev1.EventKind_EVENT_KIND_PUBLISH_STATE,
	screensharev1.EventKind_EVENT_KIND_PUBLISH_STATS,
	screensharev1.EventKind_EVENT_KIND_PUBLISH_EXIT,
	screensharev1.EventKind_EVENT_KIND_RELAY_STATUS,
	screensharev1.EventKind_EVENT_KIND_VIEWER_STATE,
	screensharev1.EventKind_EVENT_KIND_VIEWER_EXIT,
	screensharev1.EventKind_EVENT_KIND_TEST_STREAM_STATE,
	screensharev1.EventKind_EVENT_KIND_TEST_STREAM_EXIT,
	screensharev1.EventKind_EVENT_KIND_CATALOG,
	screensharev1.EventKind_EVENT_KIND_SETTINGS_CHANGED,
	screensharev1.EventKind_EVENT_KIND_RECEIVE_STATE,
	screensharev1.EventKind_EVENT_KIND_RECEIVE_EXIT,
}

// queueDepth is how many events one subscriber may fall behind by before the
// broker starts dropping the oldest of them.
//
// A depth exists at all because the per-second statistics are the high-rate kind and
// a subscriber that stalls must not stall the encoder that is publishing them. The
// oldest is what goes, because every event is a whole state: the newest is the one
// worth having, and the sequence number is what says a gap happened.
const queueDepth = 64

// Broker fans one event out to every subscriber.
type Broker struct {
	mu sync.Mutex
	// subscribers is keyed by an id handed out at subscribe time, so a cancel removes
	// exactly the one it belongs to and never a same-shaped neighbour.
	subscribers map[uint64]*subscriber
	nextID      uint64
}

// subscriber is one open Subscribe call.
type subscriber struct {
	// kinds is the filter, nil for a subscriber that asked for everything.
	kinds map[screensharev1.EventKind]bool
	// out carries the events. It is buffered to queueDepth and is closed by the
	// cancel function alone, so a send never races a close.
	out chan *screensharev1.Event
	// sequence counts the events this subscription was numbered for, from one.
	//
	// It is per subscriber and not per broker, and that is what makes the number mean
	// what the contract says it means. A shared counter skipped a number for every
	// event a filter dropped, so a subscriber that named kinds - the whole reason the
	// filter exists - saw a permanent gap it could not tell from falling behind.
	sequence uint64
	// dropped counts what this subscriber fell behind by, for the log.
	dropped uint64
}

// New returns a broker with no subscribers.
func New() *Broker {
	return &Broker{subscribers: map[uint64]*subscriber{}}
}

// Known reports whether kind names an event kind this build carries.
func Known(kind screensharev1.EventKind) bool {
	for _, k := range Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// Subscribe opens a stream of events, narrowed to kinds where any are named and
// carrying every kind where none are.
//
// The caller must call cancel when it is done, whether the call ended by return or
// by the client going away: the subscriber holds a channel the broker sends on until
// it is removed. Calling cancel twice is safe, which is what lets it be deferred and
// also called on an error path.
//
// An unknown kind is an error rather than an empty stream, on the contract's rule
// that a shell asking for a kind this build does not have should learn it now.
// EVENT_KIND_UNSPECIFIED is among the unknown ones: a request that failed to say what
// it wanted is not a request for all of it.
func (b *Broker) Subscribe(kinds []screensharev1.EventKind) (<-chan *screensharev1.Event, func(), error) {
	var filter map[screensharev1.EventKind]bool
	if len(kinds) > 0 {
		filter = make(map[screensharev1.EventKind]bool, len(kinds))
		for _, kind := range kinds {
			if !Known(kind) {
				return nil, nil, &UnknownKindError{Kind: kind}
			}
			filter[kind] = true
		}
	}

	sub := &subscriber{kinds: filter, out: make(chan *screensharev1.Event, queueDepth)}

	b.mu.Lock()
	b.nextID++
	id := b.nextID
	b.subscribers[id] = sub
	b.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subscribers, id)
			b.mu.Unlock()
			// The subscriber is out of the map before the channel closes, so no publish
			// can still be holding it, and the receiver's range ends.
			close(sub.out)
		})
	}

	return sub.out, cancel, nil
}

// Publish hands one event to every subscriber that asked for its kind, each with its
// own copy carrying its own sequence number.
//
// It never blocks. A subscriber whose queue is full loses its oldest queued event to
// make room, because every event is a whole state and the newest is the one worth
// keeping. The number is spent before the enqueue rather than after it, so a dropped
// event leaves the gap that is the whole point of numbering them.
//
// Each subscriber gets a clone. The alternative is one message every subscriber's
// stream writes its own number into, which is a data race on a value that has to
// differ per reader.
func (b *Broker) Publish(e *screensharev1.Event) {
	assert.IsNotNil(e, "an announced change is an event")
	kind := KindOf(e)
	assert.Assert(Known(kind), "an announced change names a declared kind", kind.String())

	b.mu.Lock()
	defer b.mu.Unlock()

	for _, sub := range b.subscribers {
		if sub.kinds != nil && !sub.kinds[kind] {
			continue
		}

		sub.sequence++
		event := proto.Clone(e).(*screensharev1.Event)
		event.Sequence = sub.sequence

		select {
		case sub.out <- event:
		default:
			// Full: drop the oldest to make room for this one. The receive cannot block,
			// since this is the only sender and the buffer is known full.
			select {
			case <-sub.out:
				sub.dropped++
			default:
			}
			select {
			case sub.out <- event:
			default:
				sub.dropped++
			}
		}
	}
}

// KindOf names the payload an event carries.
func KindOf(e *screensharev1.Event) screensharev1.EventKind {
	assert.IsNotNil(e, "an event kind belongs to an event")

	switch e.GetPayload().(type) {
	case *screensharev1.Event_PublishState:
		return screensharev1.EventKind_EVENT_KIND_PUBLISH_STATE
	case *screensharev1.Event_PublishStats:
		return screensharev1.EventKind_EVENT_KIND_PUBLISH_STATS
	case *screensharev1.Event_PublishExit:
		return screensharev1.EventKind_EVENT_KIND_PUBLISH_EXIT
	case *screensharev1.Event_RelayStatus:
		return screensharev1.EventKind_EVENT_KIND_RELAY_STATUS
	case *screensharev1.Event_ViewerState:
		return screensharev1.EventKind_EVENT_KIND_VIEWER_STATE
	case *screensharev1.Event_ViewerExit:
		return screensharev1.EventKind_EVENT_KIND_VIEWER_EXIT
	case *screensharev1.Event_TestStreamState:
		return screensharev1.EventKind_EVENT_KIND_TEST_STREAM_STATE
	case *screensharev1.Event_TestStreamExit:
		return screensharev1.EventKind_EVENT_KIND_TEST_STREAM_EXIT
	case *screensharev1.Event_Catalog:
		return screensharev1.EventKind_EVENT_KIND_CATALOG
	case *screensharev1.Event_SettingsChanged:
		return screensharev1.EventKind_EVENT_KIND_SETTINGS_CHANGED
	case *screensharev1.Event_ReceiveState:
		return screensharev1.EventKind_EVENT_KIND_RECEIVE_STATE
	case *screensharev1.Event_ReceiveExit:
		return screensharev1.EventKind_EVENT_KIND_RECEIVE_EXIT
	default:
		assert.Never("unexpected event payload", e.GetPayload())
		return screensharev1.EventKind_EVENT_KIND_UNSPECIFIED
	}
}

// KindNames lists the declared kinds as their enum names, for the sentence that
// refuses an unknown one.
func KindNames() []string {
	out := make([]string, 0, len(Kinds))
	for _, k := range Kinds {
		out = append(out, k.String())
	}
	return out
}

// UnknownKindError is a subscriber naming an event kind this build has none of. It
// is an environment condition rather than a bug in this code: the value came off the
// wire from a shell that may have been built against a different minor version.
type UnknownKindError struct {
	Kind screensharev1.EventKind
}

func (e *UnknownKindError) Error() string {
	name := e.Kind.String()
	if name == "" {
		name = strconv.Itoa(int(e.Kind))
	}
	return "no event kind named " + name
}
