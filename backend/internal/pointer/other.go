//go:build !linux

package pointer

// NewX11 answers false on a platform with no X server.
//
// Present rather than absent, so the child asking for a pointer source compiles everywhere and the
// platform's answer is a value rather than a build tag at every call site.
// Which backends serve the metadata cursor mode is the capture table's answer, and none of these
// platforms' backends does.
func NewX11() (Reader, bool) { return nil, false }
