//go:build !windows && !linux

package receive

import (
	"errors"
	"runtime"

	"github.com/go-gst/go-gst/pkg/gst"
)

// The export leg of a platform whose handle type is not built.
//
// Refuses rather than falling back to a download.
// A tile fed from system memory would work and cost gigabytes a second,
// the copy the frame channel exists to avoid (docs/viewer-architecture.md, "The frame channel").
// The refusal names the platform,
// and the viewer it points at is the native player, which needs no frame channel at all.
//
// macOS is the leg that is left, and it is IOSurface out of VideoToolbox.

type unbuiltSharer struct{}

func newSharer() sharer { return unbuiltSharer{} }

func (unbuiltSharer) open(*gst.Sample, int) (Pool, error) {
	return Pool{}, errors.New("frames reach no window on " + runtime.GOOS + ", so watch this stream in a player")
}

func (unbuiltSharer) write(int, *gst.Sample) error {
	return errors.New("no pool has been opened for these frames")
}

func (unbuiltSharer) close() {}
