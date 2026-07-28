package session

import (
	"slices"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
)

// sendWatchSet states which streams have a tile open, sorted by name.
//
// The app acts on the set rather than on the transitions, so a report that
// repeats the last one carries nothing and is dropped.
// It is coalesced like the layout write: applying a roster moves several streams
// at once, and the app has no use for the sets in between.
func (s *Session) sendWatchSet() {
	watching := make([]string, 0, len(s.entries))
	for i := range s.entries {
		if s.entries[i].state.Watched() {
			watching = append(watching, s.entries[i].stream.Name)
		}
	}
	slices.Sort(watching)
	if slices.Equal(watching, s.reported) {
		return
	}
	s.reported = watching
	logger.Debugf("watch set reported: %v", watching)
	s.report(roster.Status{Watching: watching})
}
