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
