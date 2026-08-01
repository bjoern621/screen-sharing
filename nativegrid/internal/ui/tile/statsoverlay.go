package tile

import (
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gsk/v4"

	"bjoernblessin.de/screenshare-nativegrid/internal/ui/stats"
)

// toggleStats opens or closes the stats overlay. Opening starts the refresh and
// takes the first poll right away, so the card is never blank; closing lets the
// refresh stop itself at its next tick.
func (t *Tile) toggleStats() {
	t.statsOpen = !t.statsOpen
	// A closed card's timeout lives until its next tick, so a reopen inside that
	// second would leave two refreshes on one card, and every further fast toggle
	// another. The count retires each one at the tick it wakes on, the way the model
	// drops the reports of a player it has replaced.
	t.statsGen++
	t.apply()

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
	v := t.poller.Poll(t.stream, t.current.Stats())
	// The renderer is the tile's to report for the same reason the render size is:
	// it is a property of the window the picture hangs in, which no pipeline can see.
	v.Renderer = t.rendererName()
	t.statsCard.Update(v)
	return true
}

// rendererName is the GSK renderer drawing the picture, by its type name. It is the
// last link in the render path: what the pipeline hands over as a GL texture is
// downloaded again by a renderer that does not draw GL textures.
//
// A widget that is not in a window has no renderer, and one that is has it for as
// long as it stays realized, so it is read per poll rather than kept.
func (t *Tile) rendererName() string {
	native := t.picture.Native()
	if native == nil {
		return ""
	}
	renderer := native.Renderer()
	if renderer == nil {
		return ""
	}
	return gsk.BaseRenderer(renderer).Type().Name()
}
