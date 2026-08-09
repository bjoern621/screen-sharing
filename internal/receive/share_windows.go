package receive

/*
#cgo pkg-config: gstreamer-1.0 gstreamer-d3d11-1.0
#cgo LDFLAGS: -ldxguid -luuid

#include <stdlib.h>
#include "share_windows.h"
*/
import "C"

import (
	"errors"
	"runtime"
	"unsafe"

	"github.com/go-gst/go-gst/pkg/gst"
)

// The Go half of the Windows export leg. Everything about Direct3D 11 is in
// share_windows.c; what is here is the pool's lifetime and the shape the rest of the
// package reads it in.

// errorBytes is how much room a C failure is given to say what happened. The
// sentences are one line naming an API and an HRESULT, and a reason too long to fit
// would be a reason nobody wrote.
const errorBytes = 256

// dxgiFormatRGBA and dxgiFormatBGRA are the two DXGI formats a converted frame can
// arrive in, spelled here rather than pulled through cgo because they are the wire's
// business: the value has to be matched against what the contract carries, and a
// constant that only exists inside a preamble cannot be.
const (
	dxgiFormatRGBA = 28 // DXGI_FORMAT_R8G8B8A8_UNORM
	dxgiFormatBGRA = 87 // DXGI_FORMAT_B8G8R8A8_UNORM
)

// d3d11Sharer is the Windows sharer: a pool of DXGI shared textures on the decoder's
// own device.
type d3d11Sharer struct {
	pool C.screenshare_share_pool
	// opened is whether pool holds textures. A zeroed pool is safe to close, but a
	// close on one that was never opened would still unref a device that is not there.
	opened bool
}

// newSharer is the platform's exporter. It holds nothing until a frame opens it,
// because what a pool has to match is the memory a frame turned out to be in and not
// the caps a chain asked for.
func newSharer() sharer { return &d3d11Sharer{} }

// onSample runs one C export entry point over a sample and turns its answer into an
// error, which is the whole of what this file's two calls have in common: the fault
// buffer, the sample's lifetime and the "0 means it wrote a reason" convention.
//
// The lifetime is why it is one function rather than two call sites. The sample is a C
// object the Go wrapper owns and unrefs from a finalizer, and samplePointer yields a bare
// address the collector cannot see: without the KeepAlive the wrapper is dead from the
// moment the argument is evaluated, and a collection during the call frees the buffer
// whose texture is being allocated from or blitted out of. Held here, no caller can
// forget it.
func onSample(sample *gst.Sample, run func(sample unsafe.Pointer, fault *C.char, size C.int) C.int) error {
	fault := make([]byte, errorBytes)
	ok := run(samplePointer(sample), (*C.char)(unsafe.Pointer(&fault[0])), C.int(len(fault)))
	runtime.KeepAlive(sample)

	if ok == 0 {
		return errors.New(reason(fault))
	}
	return nil
}

func (s *d3d11Sharer) open(sample *gst.Sample, slots int) (Pool, error) {
	s.close()

	err := onSample(sample, func(frame unsafe.Pointer, fault *C.char, size C.int) C.int {
		return C.screenshare_share_open(frame, C.int(slots), &s.pool, fault, size)
	})
	if err != nil {
		return Pool{}, err
	}
	s.opened = true

	format, err := shareFormat(uint32(s.pool.format))
	if err != nil {
		s.close()
		return Pool{}, err
	}

	pool := Pool{
		Kind:   HandleD3D11GlobalShared,
		Format: format,
		Width:  int(s.pool.width),
		Height: int(s.pool.height),
		// A DXGI texture describes its own extent, and its rows run downward. Neither
		// figure is this side's to state, so neither is stated.
		TopLeftOrigin: true,
		ProducerKey:   C.SCREENSHARE_PRODUCER_KEY,
		ConsumerKey:   C.SCREENSHARE_CONSUMER_KEY,
		Slots:         make([]Slot, 0, int(s.pool.slots)),
	}
	for i := range int(s.pool.slots) {
		pool.Slots = append(pool.Slots, Slot{Index: i, Handle: uint64(s.pool.handles[i])})
	}
	return pool, nil
}

func (s *d3d11Sharer) write(slot int, sample *gst.Sample) error {
	if !s.opened {
		return errors.New("no pool has been opened for these frames")
	}

	return onSample(sample, func(frame unsafe.Pointer, fault *C.char, size C.int) C.int {
		return C.screenshare_share_blit(&s.pool, C.int(slot), frame, fault, size)
	})
}

func (s *d3d11Sharer) close() {
	if !s.opened {
		return
	}
	C.screenshare_share_close(&s.pool)
	s.opened = false
}

// shareFormat is the contract's name for a DXGI format.
//
// A format outside the two is a chain that stopped pinning RGBA at its last filter,
// and it is refused rather than passed on: a consumer told the wrong layout draws the
// channels swapped, which looks like a decoder fault and is not one.
func shareFormat(format uint32) (ShareFormat, error) {
	switch format {
	case dxgiFormatRGBA:
		return ShareFormatRGBA, nil
	case dxgiFormatBGRA:
		return ShareFormatBGRA, nil
	default:
		return "", errors.New("the frames arrived in a pixel format no window here imports")
	}
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
