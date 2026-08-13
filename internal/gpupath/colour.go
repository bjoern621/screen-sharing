package gpupath

import "bjoernblessin.de/go-utils/util/assert"

// Colour says who converts the captured frames to the encoder's layout on a GPU path,
// and therefore whether the colour the settings name is the colour the stream carries.
//
// A pair having a GPU path is one fact; what that path does to the colour is a second one,
// and the two are independent.
// Where a filter runs on the device it is told the matrix, range and chroma and hands the encoder
// frames already in them.
// Where no such filter links the two ends, the encoder reads the captured surface as it is and
// converts it on its own terms: the run still avoids the round trip, and the colour fields on the
// form no longer reach the stream.
//
// Both values are real device paths, so this is not a quality ranking, and a row of either kind is
// worth shipping.
// It is the axis that decides which of them a memory setting may resolve to on its own:
// trading the colour away is something the user asks for, never something a resolution does quietly
// behind a setting that only said "GPU".
type Colour string

const (
	// ColourExact is a device-side conversion that takes the settings' colour and states it,
	// so the encoder is handed frames in the requested matrix, range and chroma and the stream signals
	// what the form shows.
	// It is the only verdict a default may resolve to, because it costs the user nothing they can see.
	ColourExact Colour = "exact"
	// ColourEncoder is a device path with no conversion on it: nothing here can hand the captured
	// surface to the encoder in another layout, so the encoder converts and signals its own colour and
	// the settings' colour fields are discarded.
	// Such a row carries the Cost sentence and the Signalled values, because "there is a GPU path" on
	// its own would offer the round trip's absence while hiding what pays for it.
	ColourEncoder Colour = "encoder"
)

// Signalled is the colour a ColourEncoder path's stream actually carries, in the same vocabulary
// the settings use.
//
// It is a measured fact about the encoder, recorded so the round-trip test asserts the table's
// claim against a decoded stream rather than against a value restated beside it.
// A path that begins honouring the settings fails here first and is then promoted to ColourExact,
// which is the direction the assertion has to point: the table may understate what a path does and
// must never overstate it.
//
// The form reads the same values, so a field the encoder overrides shows what the run will produce
// instead of keeping the number the user last typed into it.
//
// A ColourExact row leaves it empty.
// There the settings are the answer already, and a second copy of them here would be one more place
// the same fact could drift.
type Signalled struct {
	// Matrix is the colour matrix the encoder signals, named as ffmpeg names one, such as "bt470bg".
	Matrix string `json:"matrix"`
	// Range is the signalled range, "pc" or "tv", as the colour range setting names it.
	Range string `json:"range"`
	// Chroma is the layout the encoder converts to, as the chroma setting names it, such as "yuv420p".
	Chroma string `json:"chroma"`
}

// TradesColour reports whether taking this path means giving up the colour the settings name.
// It is the one place a Colour value is dispatched on, so a third verdict added to the enum stops
// here rather than being read as one of the two by an equality check somewhere in Resolve.
func (c Colour) TradesColour() bool {
	switch c {
	case ColourExact:
		return false
	case ColourEncoder:
		return true
	default:
		assert.Never("a GPU path states one of the two colour verdicts", string(c))
		return false
	}
}
