package session

import "bjoernblessin.de/go-utils/util/assert"

// State is a stream's watch state, the same vocabulary as the web grid
// (docs/design-language.md, "Status language").
type State int

const (
	Idle    State = iota // not watched, no tile
	Loading              // player started, no frame yet
	Live                 // frames on the surface
	Failed               // player error, the tile shows the message
	// Reconnecting is the window between a pipeline ending and the retry the
	// model schedules for it.
	// The tile stays open and keeps its last frame, because the stream is
	// expected back.
	Reconnecting
)

func (s State) String() string {
	switch s {
	case Idle:
		return "idle"
	case Loading:
		return "loading"
	case Live:
		return "live"
	case Failed:
		return "failed"
	case Reconnecting:
		return "reconnecting"
	}
	assert.Never("unexpected watch state", int(s))
	return ""
}

// Watched reports whether the state is one a tile is open for, which is every
// state but Idle.
func (s State) Watched() bool { return s != Idle }
