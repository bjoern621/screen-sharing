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
// That package registers its GObject types from a package-level variable block, so
// importing it runs gst_encoding_profile_get_type() during Go's package initialization,
// which is before any code of this process runs and therefore before gst_init(). The
// type registers GValue functions into tables gst_init has not built yet: GLib reports
// three failed assertions on stderr, and the registrations are dropped.
//
// Two calls are a smaller thing to own than an init order nothing in this package can
// reach. Initialization stays where initGStreamer puts it (receive.go), with the first
// pipeline and after the plugin path is set.

// initPbUtils loads the tables pbUtilsCodecDescription reads. It is the one call that
// has to happen before the other, and it is idempotent.
func initPbUtils() { C.gst_pb_utils_init() }

// pbUtilsCodecDescription is GStreamer's own human name for caps, e.g. "H.265 (Main
// 4:4:4 profile)", and empty for caps pbutils has no name for.
func pbUtilsCodecDescription(caps *gst.Caps) string {
	assert.IsNotNil(caps, "a codec description is asked of caps that exist")

	described := C.gst_pb_utils_get_codec_description((*C.GstCaps)(gst.UnsafeCapsToGlibNone(caps)))
	runtime.KeepAlive(caps)
	if described == nil {
		return ""
	}
	// The description is a copy pbutils allocated for this caller.
	defer C.g_free(C.gpointer(described))

	return C.GoString((*C.char)(unsafe.Pointer(described)))
}
