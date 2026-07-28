package tile

import "bjoernblessin.de/go-utils/util/assert"

// stripWidth and stripHeight size the film-strip tiles under a spotlit stream: the
// web grid's h-28 strip (112px) at 16:9.
const (
	stripWidth  = 199
	stripHeight = 112
)

// Shape is how a container sizes a tile.
type Shape int

const (
	ShapeFill  Shape = iota // fills its cell: a grid tile, and the spotlit one
	ShapeStrip              // a fixed thumbnail in the spotlight's film strip
)

// sizing is what a shape asks GTK for: a size request in logical pixels, and whether the
// tile takes the space its container offers beyond it.
// A request of -1 is GTK's "no request", which leaves a filling tile at the size of its
// cell.
type sizing struct {
	width, height int
	expand        bool
}

// sizingByShape is the geometry of every shape.
// The size a shape works out to is GTK's to allocate, so it reaches the player from the
// heartbeat that reads it back off the picture (picture.go) rather than from here.
var sizingByShape = map[Shape]sizing{
	ShapeFill:  {width: -1, height: -1, expand: true},
	ShapeStrip: {width: stripWidth, height: stripHeight},
}

// sizingOf is the geometry of one shape.
// A shape with no entry fails here rather than leaving the tile at whatever size the
// layout before it asked for.
func sizingOf(s Shape) sizing {
	z, ok := sizingByShape[s]
	assert.Assert(ok, "a tile sizes for the shape it is in", int(s))
	return z
}

// SetShape sizes the tile for the layout it is in.
func (t *Tile) SetShape(s Shape) {
	_, ok := sizingByShape[s]
	assert.Assert(ok, "a tile takes a shape it can size for", int(s))

	t.shape = s
	t.apply()
}

// SetSpotlit styles the tile for its place in the spotlight layout: the upgraded ring
// on the spotlit tile, and the spot button flipping between maximize and minimize like
// the web tile's.
func (t *Tile) SetSpotlit(on bool) {
	t.spotlit = on
	t.apply()
}
