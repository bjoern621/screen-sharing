package share

// Target is what a capture is pointed at: one kind, and the field that kind names.
//
// The three fields are read one at a time, by Kind.
// One struct rather than a target type per kind, so a caller passes what the settings hold
// and each capture backend answers for the kinds it serves.
type Target struct {
	// Kind is one of Kinds.
	Kind string
	// Monitor is the display enumeration's index, read for Monitor.
	Monitor int
	// Window is the platform handle, read for Window.
	Window uint64
	// Region is the rectangle in virtual-desktop pixels, read for Region.
	Region Rect
	// Pointer draws the mouse pointer into the frames.
	// A publish passes what the settings hold and a preview passes true,
	// a preview showing the screen as it is rather than as a stream would carry it.
	Pointer bool
}

// MonitorTarget is one whole output, with the pointer drawn.
// What a preview reads, having no share kind to derive one from.
func MonitorTarget(index int) Target {
	return Target{Kind: Monitor, Monitor: index, Pointer: true}
}
