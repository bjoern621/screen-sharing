//go:build !linux

package pointer

// NewX11 is the X11 reader on a platform that has no X server.
//
// It answers false rather than being absent, so the child that asks for a pointer source
// compiles everywhere and the platform's answer is a value rather than a build tag at every
// call site. Which backends serve the metadata cursor mode is the capture table's answer, and
// no backend on these platforms does.
func NewX11() (Reader, bool) { return nil, false }
