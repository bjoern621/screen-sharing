package tile

import (
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"

	"bjoernblessin.de/screenshare-nativegrid/internal/ui/stats"
)

// toggleStats opens or closes the stats overlay. Opening starts the refresh and
// takes the first poll right away, so the card is never blank; closing lets the
// refresh stop itself at its next tick.
func (t *Tile) toggleStats() {
	t.statsOpen = !t.statsOpen
	t.statsCard.SetVisible(t.statsOpen)
	t.statsToggle.SetOn(t.statsOpen)
	// A closed card's timeout lives until its next tick, so a reopen inside that
	// second would leave two refreshes on one card, and every further fast toggle
	// another. The count retires each one at the tick it wakes on, the way the model
	// drops the reports of a player it has replaced.
	t.statsGen++
	if !t.statsOpen {
		return
	}
	// The counters kept running while the card was closed, so the first poll after
	// opening reports no rates rather than an average over the gap.
	t.poller.Reset()
	t.refreshStats()
	gen := t.statsGen
	coreglib.TimeoutAdd(stats.RefreshMillis, func() bool { return gen == t.statsGen && t.refreshStats() })
}

// refreshStats writes one poll into the card and reports whether the refresh should
// keep running. It stops on close, on a tile with no player, and when the tile left
// the widget tree, which is how the timeout of an unwatched tile ends.
func (t *Tile) refreshStats() bool {
	if !t.statsOpen || t.current == nil || t.root.Parent() == nil {
		return false
	}
	t.statsCard.Update(t.poller.Poll(t.stream, t.current.Stats()))
	return true
}
