package receive

/*
#cgo pkg-config: gstreamer-1.0 gstreamer-gl-1.0 gstreamer-video-1.0 egl

#include <stdlib.h>
#include "share_linux.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/assert"
)

// The Go half of the Linux export leg. Everything about OpenGL and EGL is in
// share_linux.c; what is here is the pool's lifetime, the socket its descriptors are lent
// over, and the shape the rest of the package reads it in.

// errorBytes is how much room a C failure is given to say what happened. The sentences are
// one line naming an API and a reason, and a reason too long to fit would be a reason
// nobody wrote.
const errorBytes = 256

// The DRM formats a converted frame can arrive in, spelled here rather than pulled through
// cgo because they are the wire's business: the value has to be matched against what the
// contract carries, and a constant that only exists inside a preamble cannot be.
//
// A fourcc names the order the channels sit in memory, which is the reverse of the order it
// is written in: DRM_FORMAT_ABGR8888 is a red byte first, which is the layout the contract
// calls R8G8B8A8_UNORM.
const (
	drmFormatABGR8888 = 0x34324241 // 'AB24'
	drmFormatARGB8888 = 0x34325241 // 'AR24'
)

// glPlatformVariable is the environment variable GStreamer picks its GL platform with.
//
// It is written on this platform because the frame channel has one requirement of that
// choice: a descriptor is exported through EGL, and GLX has no export at all. GStreamer
// prefers GLX on an X11 display, so a session without a Wayland compositor would decode
// onto a context whose frames cannot be named to the shell - which the exporter reports
// honestly and which nothing downstream can fix.
//
// A value already in the environment is left alone. Steering a choice nobody made is a
// default; overwriting one somebody made is a substitution.
const glPlatformVariable = "GST_GL_PLATFORM"

func init() {
	if _, set := os.LookupEnv(glPlatformVariable); !set {
		os.Setenv(glPlatformVariable, "egl")
	}
}

// dmabufSharer is the Linux sharer: a pool of GL textures on the decoder's own context,
// each exported as a dmabuf descriptor.
type dmabufSharer struct {
	pool C.screenshare_share_pool
	// opened is whether pool holds textures. A zeroed pool is safe to close, but a close on
	// one that was never opened would still unref a context that is not there.
	opened bool
	// lent is where the consumer reads the descriptors from. Descriptors do not travel in a
	// message, so the pool announces a socket and this is what answers on it.
	lent *descriptorSocket
}

// newSharer is the platform's exporter. It holds nothing until a frame opens it, because
// what a pool has to match is the memory a frame turned out to be in and not the caps a
// chain asked for.
func newSharer() sharer { return &dmabufSharer{} }

// onSample runs one C export entry point over a sample and turns its answer into an error,
// which is the whole of what this file's two calls have in common: the fault buffer, the
// sample's lifetime and the "0 means it wrote a reason" convention.
//
// The lifetime is why it is one function rather than two call sites. The sample is a C
// object the Go wrapper owns and unrefs from a finalizer, and samplePointer yields a bare
// address the collector cannot see: without the KeepAlive the wrapper is dead from the
// moment the argument is evaluated, and a collection during the call frees the buffer whose
// texture is being allocated from or copied out of. Held here, no caller can forget it.
func onSample(sample *gst.Sample, run func(sample unsafe.Pointer, fault *C.char, size C.int) C.int) error {
	fault := make([]byte, errorBytes)
	ok := run(samplePointer(sample), (*C.char)(unsafe.Pointer(&fault[0])), C.int(len(fault)))
	runtime.KeepAlive(sample)

	if ok == 0 {
		return errors.New(reason(fault))
	}
	return nil
}

func (s *dmabufSharer) open(sample *gst.Sample, slots int) (Pool, error) {
	assert.Assert(slots > 0, "a pool is opened for at least one slot", slots)

	s.close()

	err := onSample(sample, func(frame unsafe.Pointer, fault *C.char, size C.int) C.int {
		return C.screenshare_share_open(frame, C.int(slots), &s.pool, fault, size)
	})
	if err != nil {
		return Pool{}, err
	}
	s.opened = true

	format, err := shareFormat(uint32(s.pool.fourcc))
	if err != nil {
		s.close()
		return Pool{}, err
	}

	count := int(s.pool.slot_count)
	assert.Assert(count == slots, "an opened pool holds the slots it was asked for", count, slots)

	descriptors := make([]int, 0, count)
	for i := range count {
		descriptors = append(descriptors, int(s.pool.fds[i]))
	}
	s.lent, err = lendDescriptors(descriptors)
	if err != nil {
		s.close()
		return Pool{}, err
	}

	pool := Pool{
		Kind:   HandleDMABufFD,
		Format: format,
		Width:  int(s.pool.width),
		Height: int(s.pool.height),
		// A dmabuf describes its own extent, so no size is stated. Its first row is the
		// picture's first row: the copy is a straight blit, and a GL texture GStreamer
		// filled holds the top row at row zero.
		TopLeftOrigin: true,
		FDSocket:      s.lent.path(),
		Modifier:      uint64(s.pool.modifier),
		Slots:         make([]Slot, 0, count),
	}
	for i := range count {
		pool.Slots = append(pool.Slots, Slot{
			Index: i,
			// The handle is zero on this kind. What names the memory is the descriptor the
			// socket hands over, and a number out of this process's table would name a file
			// of the consumer's rather than a frame.
			Planes: []Plane{{
				Offset: uint64(s.pool.offsets[i]),
				Stride: uint32(s.pool.strides[i]),
			}},
		})
	}
	return pool, nil
}

func (s *dmabufSharer) write(slot int, sample *gst.Sample) error {
	if !s.opened {
		return errors.New("no pool has been opened for these frames")
	}

	return onSample(sample, func(frame unsafe.Pointer, fault *C.char, size C.int) C.int {
		return C.screenshare_share_blit(&s.pool, C.int(slot), frame, fault, size)
	})
}

func (s *dmabufSharer) close() {
	if !s.opened {
		return
	}
	// The socket goes first. It hands out descriptors the close below invalidates, and a
	// consumer that connected in between would be lent a number naming nothing.
	if s.lent != nil {
		s.lent.close()
		s.lent = nil
	}
	C.screenshare_share_close(&s.pool)
	s.opened = false
}

// shareFormat is the contract's name for a DRM format.
//
// A format outside the two is a chain that stopped pinning RGBA at its last filter, and it
// is refused rather than passed on: a consumer told the wrong layout draws the channels
// swapped, which looks like a decoder fault and is not one.
func shareFormat(fourcc uint32) (ShareFormat, error) {
	switch fourcc {
	case drmFormatABGR8888:
		return ShareFormatRGBA, nil
	case drmFormatARGB8888:
		return ShareFormatBGRA, nil
	default:
		return "", fmt.Errorf("the frames arrived in %s, which is a pixel format no window here imports",
			fourccName(fourcc))
	}
}

// fourccName is a DRM format as the four characters it is written with, which is how every
// driver and every specification spells it.
func fourccName(fourcc uint32) string {
	name := []byte{byte(fourcc), byte(fourcc >> 8), byte(fourcc >> 16), byte(fourcc >> 24)}
	for _, b := range name {
		if b < 0x20 || b > 0x7e {
			return fmt.Sprintf("0x%08x", fourcc)
		}
	}
	return string(name)
}

// reason is the C string a failure wrote, as a Go one.
func reason(fault []byte) string {
	for i, b := range fault {
		if b == 0 {
			return string(fault[:i])
		}
	}
	return string(fault)
}
