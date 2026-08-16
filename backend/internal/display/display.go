// Package display enumerates the machine's monitors, so a capture source can be picked as an output
// with a resolution beside it rather than typed as a bare index.
package display

import (
	"sync"
	"time"
)

// recentFor is how long one enumeration answers for the machine.
// Long enough that a slider dragged across its range costs one enumeration rather than one per
// step, short enough that a screen plugged in is offered by the time the reader looks for it.
const recentFor = 2 * time.Second

var (
	recentMu   sync.Mutex
	recent     []Monitor
	recentTime time.Time
)

// Recent is List, enumerated again only once the last answer is older than recentFor.
//
// Every enumerator here is a child process, and the form resolves on every keystroke, so a resolve
// reading through pays a fork per character typed and per step of a dragged slider.
// The capture path reads List: what a pipeline crops to is the machine as it stands, where a form
// offers the outputs as they were within the window above.
func Recent() []Monitor {
	recentMu.Lock()
	defer recentMu.Unlock()

	if recentTime.IsZero() || time.Since(recentTime) > recentFor {
		recent = List()
		recentTime = time.Now()
	}
	return recent
}

// Monitor is one enumerated output.
// Index is its position in the enumeration, which is what the capture setting stores and what an
// index-selecting capture backend hands its API.
// Zero Width and Height are an unresolved size rather than an empty screen (see List).
type Monitor struct {
	Index  int `json:"index"`
	Width  int `json:"width"`
	Height int `json:"height"`
	// Top-left corner in the virtual desktop, in pixels, with the platform's own origin.
	// Crop-based capture (x11grab, ximagesrc) starts its rectangle there; ddagrab and
	// d3d11screencapturesrc select by Index and never read it.
	OffsetX int  `json:"offsetX"`
	OffsetY int  `json:"offsetY"`
	Primary bool `json:"primary"`
	// Active mode's refresh rate, in Hz.
	// Zero is unknown: a platform that does not report one, or an output whose mode line did not
	// parse.
	RefreshHz int `json:"refreshHz"`
}

// At is the enumerated output carrying this capture index, and false where the enumeration carries
// none: a screen unplugged since the setting was stored, or a session whose outputs could not be
// read at all.
//
// Both are Umgebungsfehler and what to do about them is the caller's: x11grab refuses the capture,
// ximagesrc falls back to the whole X screen.
// The absence is reported rather than resolved into one of the two.
func At(index int) (Monitor, bool) {
	for _, m := range List() {
		if m.Index == index {
			return m, true
		}
	}
	return Monitor{}, false
}
