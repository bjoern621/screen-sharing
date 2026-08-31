package receive

import (
	"unsafe"

	"github.com/go-gst/go-gst/pkg/gst"
)

// The export side of the receive package: how a decoded frame becomes a handle the shell's
// compositor can import, and the bookkeeping that says whose turn it is to touch the memory
// behind it (docs/viewer-architecture.md, "The frame channel").
//
// Nothing here reads a pixel.
// A frame is decoded into GPU memory, copied on the GPU into a slot of a pool this side owns,
// and named to the consumer, the bytes staying on the device from the decoder to the window.
// That copy is the price of the pool, and it buys what a bare decoder buffer cannot give:
// a lifetime this side controls, so a consumer that died cannot pin the decoder's pool,
// and a fixed set of handles the consumer imports once instead of per frame.
//
// The API-specific half is one file per platform, each the only file naming its graphics API, and
// a platform whose leg is not built refuses rather than falling back (share_other.go).

// slotCount is how many buffers a pool lends one consumer:
// one the consumer is drawing, one it has been handed and not drawn, and one this side is writing
// into.
//
// Two would serialize the producer against the compositor's frame time,
// every write waiting for the draw of the frame before it.
// Four buys nothing but latency, a consumer two frames behind being one that will be three behind.
const slotCount = 3

// HandleKind names what a slot's handle is, in the contract's own identifiers.
// The consumer matches it against the handle types its compositor imports,
// a list that differs between two graphics backends on one operating system.
type HandleKind string

const (
	// HandleD3D11GlobalShared is a DXGI global shared handle, valid in every process
	// on the machine without being duplicated into one.
	HandleD3D11GlobalShared HandleKind = "d3d11-global-shared"
	// HandleDMABufFD is a Linux dmabuf descriptor, which travels over a socket rather
	// than in a message (Pool.FDSocket).
	HandleDMABufFD HandleKind = "dmabuf-fd"
)

// ShareFormat is a lent slot's pixel layout: what the chain produced and not what the stream
// carried, every converting render chain ending in RGBA.
type ShareFormat string

const (
	ShareFormatRGBA ShareFormat = "R8G8B8A8_UNORM"
	ShareFormatBGRA ShareFormat = "B8G8R8A8_UNORM"
)

// Plane places one plane inside a slot's allocation, for a handle kind that does not describe
// its own layout.
type Plane struct {
	Offset uint64
	Stride uint32
}

type Slot struct {
	Index int
	// Handle names the memory in the consumer's process, and is zero for a kind whose
	// descriptors travel over a socket instead.
	Handle uint64
	Planes []Plane
}

// Pool is lent memory as the consumer has to be told about it, and the whole of what crosses
// about the frames themselves.
//
// Every figure is read off what the pipeline negotiated rather than what was asked for:
// the size is the scaler's output and not the size the tile asked to be drawn at,
// and the format is what the chain's converter produced.
type Pool struct {
	// Generation counts one subscription's pools, from one.
	// Stamped by the subscription: the platform's exporter allocates memory and knows
	// nothing about who is being lent it.
	Generation    uint64
	Kind          HandleKind
	Format        ShareFormat
	Width, Height int
	// MemorySize is the allocation in bytes where the import needs one, and zero where
	// the handle describes its own extent.
	MemorySize uint64
	// TopLeftOrigin is whether row zero is the picture's top row.
	TopLeftOrigin bool
	// ProducerKey and ConsumerKey are the keyed-mutex protocol where a handle kind
	// carries one: each side acquires with its own key and releases with the other's.
	// Both zero on a kind synchronized some other way.
	ProducerKey, ConsumerKey uint32
	// FDSocket is where a descriptor kind's descriptors are read from, one per slot
	// in index order, and Modifier the tiling the frames carry.
	// Empty and zero on a kind that is neither.
	FDSocket string
	Modifier uint64
	Slots    []Slot
}

// sharer is one platform's export machinery for one consumer.
//
// Opens from a frame rather than from caps: a chain can negotiate device caps and still hand
// over a buffer the exporter cannot name, and the buffer is the only place that shows.
//
// No method is safe to call from two goroutines at once.
// One subscription owns one sharer and drives it from the sink's streaming thread, the only
// thread a sample exists on.
type sharer interface {
	// open prepares a pool for frames like this one, replacing anything it held.
	// Fails where the frames are in a memory this platform cannot export, naming
	// the memory, the fix being another render chain rather than a retry.
	open(sample *gst.Sample, slots int) (Pool, error)
	// write copies one frame into one slot, taking and releasing whatever the pool's
	// synchronization is.
	// The slot is free by contract: the caller lends one out again only after
	// the consumer handed it back.
	write(slot int, sample *gst.Sample) error
	// close frees the pool.
	// Every handle the consumer holds names nothing afterwards, so it runs when
	// the subscription ends and never while one lives.
	close()
}

// samplePointer is the C GstSample behind a sample, for the platform files to hand to their own
// graphics API, written once so this package makes one unsafe conversion.
//
// Yields an address and not a reference: the collector cannot see it,
// so the Go wrapper is dead from the moment the call's arguments are evaluated.
// Every caller keeps the sample alive across the C call it hands the pointer to.
func samplePointer(sample *gst.Sample) unsafe.Pointer {
	return gst.UnsafeSampleToGlibNone(sample)
}
