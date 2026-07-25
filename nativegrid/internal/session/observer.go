package session

import "bjoernblessin.de/go-utils/util/assert"

// ChangeKind is what happened to the model.
type ChangeKind int

const (
	// StreamAdded reports a stream that joined the model, at Change.Index. A
	// stream never leaves: indexes stay valid for the process lifetime because
	// widgets and player callbacks hold on to them.
	StreamAdded ChangeKind = iota
	// StateChanged reports the watch state of Change.Index.
	StateChanged
	// AudioReady reports that Change.Index turned out to carry audio.
	AudioReady
	// RosterChanged reports that the streams the relay lists changed, which moves
	// rows in and out of sight without touching any watch state. It carries no
	// index: presence changed across the model.
	RosterChanged
	// OrderChanged reports a new display order. It carries no index.
	OrderChanged
)

func (k ChangeKind) String() string {
	switch k {
	case StreamAdded:
		return "stream added"
	case StateChanged:
		return "state changed"
	case AudioReady:
		return "audio ready"
	case RosterChanged:
		return "roster changed"
	case OrderChanged:
		return "order changed"
	}
	assert.Never("unexpected change kind", int(k))
	return ""
}

// Change is one model change a view redraws for. Index is the stream it is about,
// and noStream for a change that is about the whole model.
type Change struct {
	Kind  ChangeKind
	Index int
}

// noStream is Change.Index for a change that names no single stream.
const noStream = -1

// Observer is a view of the model. Changed runs on the UI loop, synchronously
// inside the call that changed the model, so an observer reads the model back
// rather than being handed the new values.
//
// A view handles the kinds it draws and ignores the rest.
type Observer interface {
	Changed(c Change)
}

// ObserverFunc adapts a function to Observer.
type ObserverFunc func(c Change)

func (f ObserverFunc) Changed(c Change) { f(c) }
