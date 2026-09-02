package receive

import (
	"fmt"
	"net"
	"testing"

	"github.com/go-gst/go-gst/pkg/gst"
	"github.com/go-gst/go-gst/pkg/gstapp"
	"golang.org/x/sys/unix"
)

// The Linux leg's own checks: a decoded frame is copied into a slot, and the slot's descriptor
// crosses the socket the pool announces and names a buffer big enough to hold the picture.
//
// The picture itself is not checked.
// Reading it back means importing the descriptor into a second EGL display, which is C, and
// a test file cannot use cgo; the side that does import it is the shell, and a tile drawing noise
// is what says so.
//
// Every check skips where the machine has no GL to decode onto, which is a headless build host
// rather than a failure: the leg is about a GPU, and a machine without one is a machine whose
// viewer is the native player.

// The size the test frames are decoded at.
// Small: what is under test is the export and not the scaler.
const (
	probeWidth  = 320
	probeHeight = 240
)

func TestExportedPoolLendsADescriptorPerSlot(t *testing.T) {
	sample, stop := glSample(t)
	defer stop()

	shared := newSharer()
	defer shared.close()

	pool, err := shared.open(sample, slotCount)
	if err != nil {
		t.Skipf("this machine exports no dmabuf from its GL frames: %v", err)
	}

	if pool.Kind != HandleDMABufFD {
		t.Errorf("the pool lends %q handles, and this leg lends %q", pool.Kind, HandleDMABufFD)
	}
	if pool.Format != ShareFormatRGBA && pool.Format != ShareFormatBGRA {
		t.Errorf("the pool announces %q, which is neither RGBA nor BGRA", pool.Format)
	}
	if pool.Width != probeWidth || pool.Height != probeHeight {
		t.Errorf("the pool is %dx%d and the frames are %dx%d", pool.Width, pool.Height,
			probeWidth, probeHeight)
	}
	if !pool.TopLeftOrigin {
		t.Error("the pool states a bottom-left origin, and a GL texture GStreamer filled holds the top row first")
	}
	if pool.FDSocket == "" {
		t.Fatal("the pool names no socket, and a descriptor cannot travel in a message")
	}
	if len(pool.Slots) != slotCount {
		t.Fatalf("the pool lends %d slots and was opened for %d", len(pool.Slots), slotCount)
	}
	for _, slot := range pool.Slots {
		if slot.Handle != 0 {
			t.Errorf("slot %d carries a handle, and this kind's descriptors travel over the socket",
				slot.Index)
		}
		if len(slot.Planes) != 1 {
			t.Fatalf("slot %d describes %d planes, and a converted frame is one", slot.Index,
				len(slot.Planes))
		}
		if slot.Planes[0].Stride < probeWidth*4 {
			t.Errorf("slot %d strides %d bytes a row, which is under a %d-pixel row",
				slot.Index, slot.Planes[0].Stride, probeWidth)
		}
	}

	if err := shared.write(0, sample); err != nil {
		t.Fatalf("the frame could not be copied into slot 0: %v", err)
	}

	descriptors := readDescriptors(t, pool.FDSocket, len(pool.Slots))
	defer func() {
		for _, fd := range descriptors {
			unix.Close(fd)
		}
	}()

	for i, fd := range descriptors {
		// A dmabuf answers its own extent, so a size check says the descriptor names
		// the picture rather than any file this process happened to hold.
		size, err := unix.Seek(fd, 0, unix.SEEK_END)
		if err != nil {
			t.Fatalf("slot %d's descriptor names nothing that can be sized: %v", i, err)
		}
		want := int64(pool.Slots[i].Planes[0].Stride) * int64(pool.Height)
		if size < want {
			t.Errorf("slot %d's descriptor holds %d bytes, and the picture needs %d", i, size, want)
		}
	}
}

// The pool owns the socket, so a subscription that ended leaves nothing for a consumer
// to connect to.
func TestClosingAPoolTakesItsSocketWithIt(t *testing.T) {
	sample, stop := glSample(t)
	defer stop()

	shared := newSharer()
	pool, err := shared.open(sample, slotCount)
	if err != nil {
		t.Skipf("this machine exports no dmabuf from its GL frames: %v", err)
	}

	shared.close()

	if conn, err := net.Dial("unix", pool.FDSocket); err == nil {
		conn.Close()
		t.Error("the socket of a closed pool still answers")
	}
}

// The renegotiation path: a subscription opens a pool per size, and the second brings a socket
// and descriptors of its own rather than adding to the first.
func TestReopeningAPoolReplacesTheOneBeforeIt(t *testing.T) {
	sample, stop := glSample(t)
	defer stop()

	shared := newSharer()
	defer shared.close()

	first, err := shared.open(sample, slotCount)
	if err != nil {
		t.Skipf("this machine exports no dmabuf from its GL frames: %v", err)
	}
	second, err := shared.open(sample, slotCount)
	if err != nil {
		t.Fatalf("a pool could not be reopened: %v", err)
	}

	if first.FDSocket == second.FDSocket {
		t.Error("the second pool lends its descriptors over the first pool's socket")
	}
	if conn, err := net.Dial("unix", first.FDSocket); err == nil {
		conn.Close()
		t.Error("the socket of the replaced pool still answers")
	}
}

// glSample decodes one test frame onto the GPU and hands it over with what ends the pipeline.
//
// The launch line is the GL chain's own, written out rather than built through resolve: what
// is under test is the export and not the table, and a resolve would fall back on a machine missing
// an element instead of saying so.
func glSample(t *testing.T) (*gst.Sample, func()) {
	t.Helper()
	initGStreamer()

	el, err := gst.ParseLaunch(fmt.Sprintf(
		"videotestsrc num-buffers=1 ! video/x-raw,width=%d,height=%d ! "+
			"glupload ! glcolorconvert ! glcolorscale ! "+
			"video/x-raw(memory:GLMemory),format=RGBA,colorimetry=sRGB ! appsink name=sink",
		probeWidth, probeHeight))
	if err != nil {
		t.Skipf("this machine builds no GL pipeline: %v", err)
	}
	pipeline := el.(gst.Pipeline)
	pipeline.SetState(gst.StatePlaying)

	sink, ok := pipeline.GetByName("sink").(gstapp.AppSink)
	if !ok {
		pipeline.SetState(gst.StateNull)
		t.Skip("the GL pipeline grew no sink")
	}
	sample := sink.PullSample()
	if sample == nil {
		pipeline.SetState(gst.StateNull)
		t.Skip("the GL pipeline decoded no frame, so this machine has no GL to export from")
	}
	return sample, func() {
		gst.UnsafeSampleUnref(sample)
		pipeline.SetState(gst.StateNull)
	}
}

// readDescriptors is the consumer's side of the socket: one descriptor per slot, in index order,
// each in a message whose payload is that slot's number.
func readDescriptors(t *testing.T, path string, slots int) []int {
	t.Helper()

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("the socket the pool announced does not answer: %v", err)
	}
	defer conn.Close()

	descriptors := make([]int, 0, slots)
	for i := range slots {
		payload := make([]byte, 1)
		oob := make([]byte, unix.CmsgSpace(4))
		n, oobn, _, _, err := conn.(*net.UnixConn).ReadMsgUnix(payload, oob)
		if err != nil {
			t.Fatalf("slot %d's descriptor did not arrive: %v", i, err)
		}
		if n != 1 || int(payload[0]) != i {
			t.Fatalf("the message for slot %d names slot %d", i, payload[0])
		}

		messages, err := unix.ParseSocketControlMessage(oob[:oobn])
		if err != nil || len(messages) != 1 {
			t.Fatalf("slot %d's message carries no rights: %v", i, err)
		}
		fds, err := unix.ParseUnixRights(&messages[0])
		if err != nil || len(fds) != 1 {
			t.Fatalf("slot %d's message carries %d descriptors: %v", i, len(fds), err)
		}
		descriptors = append(descriptors, fds[0])
	}
	return descriptors
}
