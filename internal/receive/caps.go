package receive

/*
#cgo pkg-config: gstreamer-video-1.0
#include <stdlib.h>
#include <gst/video/video.h>

// pixel_shape reports the bit depth per component and the chroma subsampling
// factors of a raw video format name, e.g. "Y444_10LE". A factor of 1 means
// full chroma resolution on that axis. Returns 0 for a name this GStreamer
// build does not know.
static int pixel_shape(const char *name, int *depth, int *h_sub, int *v_sub) {
	GstVideoFormat fmt = gst_video_format_from_string(name);
	if (fmt == GST_VIDEO_FORMAT_UNKNOWN) {
		return 0;
	}
	const GstVideoFormatInfo *info = gst_video_format_get_info(fmt);
	*depth = GST_VIDEO_FORMAT_INFO_DEPTH(info, 0);
	*h_sub = 1 << GST_VIDEO_FORMAT_INFO_W_SUB(info, 1);
	*v_sub = 1 << GST_VIDEO_FORMAT_INFO_H_SUB(info, 1);
	return 1;
}
*/
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/go-gst/go-gst/pkg/gst"
	"github.com/go-gst/go-gst/pkg/gstpbutils"
)

// padCaps returns the caps negotiated on one of an element's static pads, and nil
// while the pad or the negotiation is missing.
func padCaps(e gst.Element, name string) *gst.Caps {
	pad := e.GetStaticPad(name)
	if pad == nil {
		return nil
	}
	caps := pad.GetCurrentCaps()
	if caps == nil || caps.GetSize() == 0 {
		return nil
	}
	return caps
}

// codecDescription is GStreamer's own human name for caps, e.g. "H.265 (Main
// 4:4:4 profile)". It falls back to the media type, so a codec pbutils has no
// description for still names itself.
func codecDescription(caps *gst.Caps) string {
	if d := gstpbutils.PbUtilsGetCodecDescription(caps); d != "" {
		return d
	}
	return caps.GetStructure(0).GetName()
}

// pixelShape describes a raw video format the way a viewer cares about it: bit
// depth per component and chroma subsampling. The numbers come from GStreamer's
// own format table, so a format this build knows needs no entry here.
func pixelShape(format string) (depth int, subsampling string) {
	if format == "" {
		return 0, ""
	}
	name := C.CString(format)
	defer C.free(unsafe.Pointer(name))
	var d, h, v C.int
	if C.pixel_shape(name, &d, &h, &v) == 0 {
		return 0, ""
	}
	return int(d), subsamplingLabel(int(h), int(v))
}

// subsamplingLabel renders chroma subsampling factors in J:a:b notation: four
// luma samples wide, a chroma samples across them, and b the second row's
// chroma, which vertical subsampling drops to zero.
func subsamplingLabel(h, v int) string {
	if h <= 0 || v <= 0 || 4%h != 0 {
		return ""
	}
	a := 4 / h
	b := a
	if v > 1 {
		b = 0
	}
	return fmt.Sprintf("4:%d:%d", a, b)
}
