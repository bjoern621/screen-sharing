//go:build !linux

package gstrun

import "github.com/go-gst/go-gst/pkg/gst"

// cursorAt answers false on a platform with no PipeWire capture.
// Present rather than absent, so the pointer source table compiles everywhere and which captures
// answer a position stays a table rather than a build tag at every call site.
func cursorAt(*gst.Buffer) (int, int, bool) { return 0, 0, false }
