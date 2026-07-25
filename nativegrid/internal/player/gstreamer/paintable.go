package gstreamer

/*
#cgo pkg-config: gobject-2.0
#include <stdlib.h>
#include <glib-object.h>

// grab_object_property returns a new reference to an object-typed property.
static gpointer grab_object_property(gpointer object, const char *name) {
	gpointer out = NULL;
	g_object_get(object, name, &out, NULL);
	return out;
}
*/
import "C"

import (
	"unsafe"

	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/go-gst/go-glib/pkg/gobject/v2"
	"github.com/go-gst/go-gst/pkg/gst"
)

// paintableOf bridges the two binding worlds: go-gst wraps the sink element,
// gotk4 must wrap the paintable the GtkPicture draws. Both wrap the same C
// GObject, so the property crosses as a raw pointer. g_object_get returns a new
// reference, which AssumeOwnership hands to gotk4's lifetime management; the
// paintable itself is never wrapped by go-glib, so the two runtimes never manage
// the same object.
func paintableOf(sink gst.Element) *gdk.Paintable {
	obj := gobject.UnsafeObjectToGlibNone(sink)
	name := C.CString("paintable")
	defer C.free(unsafe.Pointer(name))
	ptr := C.grab_object_property(C.gpointer(obj), name)
	return &gdk.Paintable{Object: coreglib.AssumeOwnership(unsafe.Pointer(ptr))}
}
