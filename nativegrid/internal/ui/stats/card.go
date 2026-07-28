package stats

import (
	"strings"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare-nativegrid/internal/player"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/widgets"
)

// The card's geometry: wide enough for a codec description, and the margins that
// leave the corner label chip above and the control bar below it clear on a tile
// tall enough for the whole card.
const (
	cardWidth  = 296
	valueChars = 28
	topGap     = 44
	bottomGap  = 44

	// unknown stands in for a figure the pipeline has not learned yet, on a row
	// that keeps its place while it is missing.
	unknown = "…"

	// RefreshMillis is how often the card wants a fresh poll. The rate rows are
	// deltas across it, so the interval is the card's own business.
	RefreshMillis = 1000
)

// Card is the stats overlay: the fixed rows of the blocks table, then a block per
// element the player reports counters for.
type Card struct {
	root   *gtk.Box
	boxes  []*gtk.Box // one per block, for the visible rule
	lines  [][]*line  // one per block, parallel to its rows
	groups *gtk.Box
	// groupLines are the transport blocks, and groupSig the shape they were built
	// for, so the widgets are rebuilt only when that shape changes.
	groupLines [][]*line
	groupSig   string
}

// NewCard builds the overlay from the blocks table: top-left under the chip,
// monospace white-on-black rows.
func NewCard() *Card {
	c := &Card{groups: gtk.NewBox(gtk.OrientationVertical, 0)}

	rows := gtk.NewBox(gtk.OrientationVertical, 0)
	for i, b := range blocks {
		// The first block of the card carries no rule; every later one is separated
		// from the one above.
		box := newBlock(rows, b.title, b.tip, i > 0)
		lines := make([]*line, 0, len(b.rows))
		for _, r := range b.rows {
			lines = append(lines, newLine(box, r.key, r.tip, r.hides))
		}
		c.boxes = append(c.boxes, box)
		c.lines = append(c.lines, lines)
	}
	rows.Append(c.groups)

	scroll := gtk.NewScrolledWindow()
	scroll.SetChild(rows)
	scroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroll.SetPropagateNaturalHeight(true)

	card := gtk.NewBox(gtk.OrientationVertical, 0)
	card.AddCSSClass("tile-stats")
	card.SetVAlign(gtk.AlignStart)
	card.SetSizeRequest(cardWidth, -1)
	card.Append(scroll)

	// The card sits in a box that spans the tile's height, so a tile too short for
	// the whole card allocates the card less than it asked for and the scroller
	// inside gives up the difference. Only the card is styled; the box around it is
	// there for the clamp.
	c.root = gtk.NewBox(gtk.OrientationVertical, 0)
	c.root.SetHAlign(gtk.AlignStart)
	c.root.SetVAlign(gtk.AlignFill)
	c.root.SetMarginStart(8)
	c.root.SetMarginTop(topGap)
	c.root.SetMarginBottom(bottomGap)
	c.root.SetVisible(false)
	c.root.Append(card)
	return c
}

// Widget is the card's outer box, an overlay child of the tile.
func (c *Card) Widget() gtk.Widgetter { return c.root }

func (c *Card) SetVisible(visible bool) { c.root.SetVisible(visible) }

// Update writes one poll into the rows. Only values are written: the widgets are
// the table's shape, which does not depend on the poll.
func (c *Card) Update(v View) {
	assert.Assert(len(c.lines) == len(blocks), "a card holds a line set per block", len(c.lines), len(blocks))

	for bi, b := range blocks {
		if b.visible != nil {
			c.boxes[bi].SetVisible(b.visible(v))
		}
		for ri, r := range b.rows {
			c.lines[bi][ri].set(r.value(v))
		}
	}
	c.syncGroups(v.Stats.Groups)
}

// syncGroups renders the transport blocks the player reports. Their shape settles
// while the pipeline sets up, so the widgets are rebuilt only when it changes and
// every poll after that just writes values.
func (c *Card) syncGroups(groups []player.StatGroup) {
	if sig := groupSignature(groups); sig != c.groupSig {
		widgets.ClearBox(c.groups)
		c.groupLines = nil
		for _, g := range groups {
			box := newBlock(c.groups, g.Name, g.Tip, true)
			lines := make([]*line, 0, len(g.Rows))
			for _, r := range g.Rows {
				lines = append(lines, newLine(box, r.Label, r.Tip, false))
			}
			c.groupLines = append(c.groupLines, lines)
		}
		c.groupSig = sig
	}
	for gi, g := range groups {
		for ri, r := range g.Rows {
			c.groupLines[gi][ri].set(r.Value)
		}
	}
}

// groupSignature is the shape of the reported groups, their names and labels but
// not their values, so it changes only when the widgets have to.
func groupSignature(groups []player.StatGroup) string {
	var b strings.Builder
	for _, g := range groups {
		b.WriteString(g.Name)
		for _, r := range g.Rows {
			b.WriteByte('/')
			b.WriteString(r.Label)
		}
		b.WriteByte(';')
	}
	return b.String()
}
