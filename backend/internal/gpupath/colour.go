package gpupath

import "bjoernblessin.de/go-utils/util/assert"

// Colour says who converts the captured frames to the encoder's layout on a device path,
// and so whether the colour the settings name is the colour the stream carries.
//
// A pair having a device path and what that path does to the colour are two independent facts.
// A filter running on the device is told the matrix, range and chroma,
// and hands the encoder frames already in them.
// Where no such filter links the two ends,
// the encoder takes the captured surface as it is and converts it on its own terms:
// the round trip is still avoided, and the form's colour fields do not reach the stream.
//
// Both values are device paths, so this is no quality ranking and a row of either kind ships.
// The axis decides which of them a memory setting may resolve to on its own:
// trading the colour away is something the user asks for,
// never something a resolution does behind a setting that said "GPU".
type Colour string

const (
	// ColourExact is a device-side conversion told the settings' colour:
	// the encoder is handed frames in the requested matrix, range and chroma,
	// and the stream signals what the form shows.
	// The only verdict a default may resolve to, costing the user nothing they can see.
	ColourExact Colour = "exact"
	// ColourEncoder is a device path with no conversion on it.
	// Nothing hands the captured surface to the encoder in another layout,
	// so the encoder converts and signals its own colour and the settings' colour fields are discarded.
	// Such a row carries the Cost code and the Signalled values,
	// "there is a device path" alone offering the round trip's absence while hiding what pays for it.
	ColourEncoder Colour = "encoder"
)

// Signalled is the colour a ColourEncoder path's stream carries, in the settings' own vocabulary.
//
// A measured fact about the encoder,
// recorded so the round-trip test asserts the table's claim against a decoded stream
// rather than against a value restated beside it.
// A path that honours the settings fails there first and is promoted to ColourExact,
// the direction the assertion points: the table may understate what a path does, never overstate it.
//
// The form reads the same values,
// so a field the encoder overrides shows what the run produces instead of the number last typed.
//
// A ColourExact row carries none of it.
// The settings are the answer there, and a second copy is one more place the fact can drift.
type Signalled struct {
	// Matrix is the colour matrix the encoder signals, spelled as ffmpeg spells one: "bt470bg".
	Matrix string `json:"matrix"`
	// Range is the signalled range as the colour range setting spells it: "pc" or "tv".
	Range string `json:"range"`
	// Chroma is the layout the encoder converts to, as the chroma setting spells it: "yuv420p".
	Chroma string `json:"chroma"`
}

// TradesColour reports whether taking this path gives up the colour the settings name.
// The one place a Colour value is dispatched on.
// A further verdict stops here rather than being read as one of the others inside Resolve.
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
