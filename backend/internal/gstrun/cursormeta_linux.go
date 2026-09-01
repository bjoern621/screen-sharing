//go:build linux

package gstrun

/*
#cgo pkg-config: gstreamer-1.0 gstreamer-video-1.0
#include <gst/gst.h>
#include <gst/video/gstvideometa.h>

// Position of the region of interest pipewiresrc names "cursor", FALSE where the frame carries none.
//
// Iterated rather than fetched by id:
// the meta is found by what it is named, and its id is whatever the element assigned.
static gboolean screenshare_cursor_at(GstBuffer *buffer, guint *x, guint *y) {
  gpointer state = NULL;
  GstMeta *meta;
  GQuark cursor = g_quark_from_static_string("cursor");

  while ((meta = gst_buffer_iterate_meta_filtered(buffer, &state,
             GST_VIDEO_REGION_OF_INTEREST_META_API_TYPE))) {
    GstVideoRegionOfInterestMeta *roi = (GstVideoRegionOfInterestMeta *) meta;
    if (roi->roi_type == cursor) {
      *x = roi->x;
      *y = roi->y;
      return TRUE;
    }
  }
  return FALSE;
}
*/
import "C"

import (
	"unsafe"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/assert"
)

// cursorAt is where the pointer was over the frame, in the captured picture's own pixels,
// and false for a frame carrying no cursor.
//
// The portal's metadata cursor mode sends the position beside each frame rather than drawing it,
// and pipewiresrc hands it on as a region of interest named "cursor".
// A pointer that is not over the captured region has no cursor in the frame, which is the pointer
// having left rather than a frame that failed to say.
//
// The binding exposes the meta's own fields through no accessor,
// so the struct is read where it is defined.
func cursorAt(buffer *gst.Buffer) (int, int, bool) {
	assert.IsNotNil(buffer, "a cursor position is read off a frame")

	var x, y C.guint
	native := (*C.GstBuffer)(unsafe.Pointer(gst.UnsafeBufferToGlibNone(buffer)))
	if C.screenshare_cursor_at(native, &x, &y) == 0 {
		return 0, 0, false
	}
	return int(x), int(y), true
}

var _ = unsafe.Pointer(nil)
