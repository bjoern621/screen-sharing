//go:build !windows

package receive

import (
	"errors"
	"runtime"

	"github.com/go-gst/go-gst/pkg/gst"
)

// The export leg of a platform whose handle type is not built yet.
//
// It refuses rather than falling back to a download. A tile fed from system memory
// would be a tile that works and costs gigabytes a second, and the whole reason the
// frame channel exists is that this is the copy nobody can afford
// (docs/viewer-architecture.md, "The frame channel"). A refusal names the platform,
// and the viewer it points at is the native player, which needs no frame channel at
// all.
//
// Linux is the next leg: dmabuf out of the receive pipeline, the descriptors over a
// socket, and eglCreateImageKHR in the shell. macOS is last and is IOSurface.

type unbuiltSharer struct{}

func newSharer() sharer { return unbuiltSharer{} }

func (unbuiltSharer) open(*gst.Sample, int) (Pool, error) {
	return Pool{}, errors.New("frames do not reach a window on " + runtime.GOOS + " yet, so watch this stream in a player")
}

func (unbuiltSharer) write(int, *gst.Sample) error {
	return errors.New("no pool has been opened for these frames")
}

func (unbuiltSharer) close() {}
