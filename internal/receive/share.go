package receive

import (
	"unsafe"

	"github.com/go-gst/go-gst/pkg/gst"
)

// The export side of the receive package: how a decoded frame becomes a handle the
// shell's compositor can import, and the bookkeeping that says whose turn it is to
// touch the memory behind it (docs/viewer-architecture.md, "The frame channel").
//
// Nothing here reads a pixel. A frame is decoded into GPU memory, copied on the GPU
// into a slot of a pool this side owns, and named to the consumer; the bytes stay on
// the device from the decoder to the window. That copy is the price of the pool, and
// it buys the two things a bare decoder buffer cannot give: a lifetime this side
// controls, so a consumer that died cannot pin the decoder's pool, and a fixed set of
// handles the consumer imports once instead of per frame.
//
// The platform files hold the whole of the API-specific half. share_windows.go
// exports a DXGI shared texture, share_other.go refuses on a platform whose leg is
// not built, and each is the only file that names its graphics API.

// slotCount is how many buffers a pool lends one consumer.
//
// Three: one the consumer is drawing, one it has been handed and not drawn yet, and
// one this side is writing into. Two would serialize the producer against the
// compositor's frame time - every write would wait for the draw of the frame before
// it - and four buys nothing but latency, since a consumer that is behind by two
// frames is a consumer that will be behind by three.
const slotCount = 3

// HandleKind names what a slot's handle is, in the identifiers the contract carries.
// It is what the consumer matches against the handle types its own compositor
// imports, which is a list that differs between two graphics backends on one
// operating system.
type HandleKind string

const (
	// HandleD3D11GlobalShared is a DXGI global shared handle, valid in every process
	// on the machine without being duplicated into one.
	HandleD3D11GlobalShared HandleKind = "d3d11-global-shared"
	// HandleDMABufFD is a Linux dmabuf file descriptor, which travels over a socket
	// rather than in a message.
	HandleDMABufFD HandleKind = "dmabuf-fd"
)

// ShareFormat is the pixel layout of a lent slot. Every render chain that converts
// ends in RGBA, so this is what the chain produced and not what the stream carried.
type ShareFormat string

const (
	ShareFormatRGBA ShareFormat = "R8G8B8A8_UNORM"
	ShareFormatBGRA ShareFormat = "B8G8R8A8_UNORM"
)

// Plane is one plane's placement inside a slot's allocation, for a handle kind that
// does not describe its own layout.
type Plane struct {
	Offset uint64
	Stride uint32
}

// Slot is one lent buffer of a pool.
type Slot struct {
	Index int
	// Handle names the memory in the consumer's process, and is zero for a handle
	// kind whose descriptors travel over a socket instead.
	Handle uint64
	Planes []Plane
}

// Pool is lent memory, as the consumer has to be told about it.
//
// It is the whole of what crosses about the frames themselves. Everything in it is
// read off what the pipeline negotiated rather than asked for: the size is the
// scaler's output and not the size the tile asked to be drawn at, and the format is
// what the chain's converter produced.
type Pool struct {
	// Generation counts the pools of one subscription, from one. It is stamped by the
	// subscription rather than by the platform's exporter, which allocates memory and
	// knows nothing about who is being lent it.
	Generation    uint64
	Kind          HandleKind
	Format        ShareFormat
	Width, Height int
	// MemorySize is the allocation's size in bytes where the import needs one, and
	// zero where the handle describes its own extent.
	MemorySize uint64
	// TopLeftOrigin is whether row zero is the top row.
	TopLeftOrigin bool
	// ProducerKey and ConsumerKey are the keyed-mutex protocol for a handle kind that
	// carries one: each side acquires with its own key and releases with the other's.
	// Both zero on a kind synchronized some other way.
	ProducerKey, ConsumerKey uint32
	// FDSocket is where a descriptor handle kind's descriptors are read from, and
	// Modifier the tiling the frames carry. Both empty on a kind that is neither.
	FDSocket string
	Modifier uint64
	Slots    []Slot
}

// sharer is one platform's export machinery for one consumer.
//
// It is opened from a frame rather than from caps, because what has to be matched is
// the memory the decoder actually produced - a chain can negotiate device caps and
// still hand over a buffer the exporter cannot name, and the buffer is the only place
// that is visible.
//
// None of its methods is safe to call from two goroutines at once. One subscription
// owns one sharer and drives it from the sink's streaming thread, which is the only
// thread a sample exists on.
type sharer interface {
	// open prepares a pool for frames like this one, replacing anything it held. It
	// fails where the frames are in a memory this platform cannot export, and the
	// failure names the memory, because the fix is a render chain rather than a retry.
	open(sample *gst.Sample, slots int) (Pool, error)
	// write copies one frame into one slot, taking and releasing whatever the pool's
	// synchronization is. The slot is free by contract: the caller lends it only after
	// the consumer handed it back.
	write(slot int, sample *gst.Sample) error
	// close frees the pool. Every handle the consumer holds names nothing afterwards,
	// which is why it runs when the subscription ends and never while one lives.
	close()
}

// samplePointer is the C GstSample behind a sample, for the platform files to pass
// into their own graphics API. It is here rather than in each of them so the one
// unsafe conversion this package makes is written once.
//
// What it yields is an address and not a reference: the collector cannot see it, so
// the Go wrapper is dead the moment the call's arguments are evaluated. Every caller
// therefore keeps the sample alive across the C call it hands the pointer to.
func samplePointer(sample *gst.Sample) unsafe.Pointer {
	return gst.UnsafeSampleToGlibNone(sample)
}
