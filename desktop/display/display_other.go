//go:build !windows

package display

// List returns a single placeholder monitor on non-Windows platforms. Per-output
// enumeration with dimensions is a DXGI (Windows) capability; on Linux the
// monitor field is disabled by the dependency rules and the width/height of 0
// tells the UI the resolution is unknown.
func List() []Monitor {
	return []Monitor{{Index: 0, Width: 0, Height: 0, Primary: true}}
}
