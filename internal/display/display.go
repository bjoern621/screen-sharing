// Package display enumerates the machine's monitors so the capture-source
// dropdown can offer one entry per physical output (with its resolution) instead
// of a raw number field the user has to guess at.
package display

// Monitor describes one display output: its capture index, pixel dimensions,
// position in the virtual desktop, current refresh rate and whether it is the
// primary monitor.
type Monitor struct {
	Index  int `json:"index"`
	Width  int `json:"width"`
	Height int `json:"height"`
	// OffsetX and OffsetY place the monitor's top-left corner in the virtual
	// desktop. x11grab crops the X screen at this origin; ddagrab selects the
	// output by Index and ignores them.
	OffsetX int  `json:"offsetX"`
	OffsetY int  `json:"offsetY"`
	Primary bool `json:"primary"`
	// RefreshHz is the active mode's refresh rate. 0 means unknown: a platform
	// that cannot report it, or an enumeration that failed for this output.
	RefreshHz int `json:"refreshHz"`
}

// At is the enumerated output carrying this capture index, and false where the
// enumeration has none: a screen unplugged since the setting was stored, or a
// session whose outputs could not be read at all.
//
// What a caller does about that differs and is the caller's - x11grab refuses the
// capture and ximagesrc falls back to the whole X screen - so this reports the
// absence rather than deciding on one.
func At(index int) (Monitor, bool) {
	for _, m := range List() {
		if m.Index == index {
			return m, true
		}
	}
	return Monitor{}, false
}
