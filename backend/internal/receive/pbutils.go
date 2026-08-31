package receive

/*
#cgo pkg-config: gstreamer-pbutils-1.0
#include <gst/pbutils/pbutils.h>
*/
import "C"

import (
	"runtime"
	"unsafe"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/assert"
)

// The two pbutils calls this package makes, bound here rather than taken from go-gst's
// gstpbutils package.
//
// That package registers its GObject types from a package-level variable block, so importing it
// runs gst_encoding_profile_get_type() during Go's package initialization, ahead of gst_init().
// The type then registers GValue functions into tables gst_init has not built: GLib fails
// assertions on stderr and the registrations are dropped.
//
// Two calls are a smaller thing to own than an init order nothing in this package can reach, so
// initialization stays with initGStreamer (receive.go), on the first pipeline and after
// the plugin path is set.

// initPbUtils loads the tables pbUtilsCodecDescription reads, and is idempotent.
func initPbUtils() { C.gst_pb_utils_init() }

// pbUtilsCodecDescription is GStreamer's own name for caps, e.g. "H.265 (Main 4:4:4 profile)".
// Empty for caps pbutils has no name for.
func pbUtilsCodecDescription(caps *gst.Caps) string {
	assert.IsNotNil(caps, "a codec description is asked of caps that exist")

	described := C.gst_pb_utils_get_codec_description((*C.GstCaps)(gst.UnsafeCapsToGlibNone(caps)))
	runtime.KeepAlive(caps)
	if described == nil {
		return ""
	}
	// pbutils allocated the description for this caller.
	defer C.g_free(C.gpointer(described))

	return C.GoString((*C.char)(unsafe.Pointer(described)))
}
