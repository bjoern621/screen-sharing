//go:build webkit2_41

package main

// WebKitGTK gates RTCPeerConnection behind WebKitSettings:enable-webrtc, which
// defaults off, and Wails sets neither it nor its prerequisite
// enable-media-stream. Without both, WHEP tiles fail with "Can't find variable:
// RTCPeerConnection".
//
// The switch only has an effect on a webkitgtk built with ENABLE_WEB_RTC; the
// nixpkgs default build leaves the bindings out entirely (see
// viewer-architecture.md).

/*
#cgo pkg-config: gtk+-3.0 webkit2gtk-4.1

#include <gtk/gtk.h>
#include <webkit2/webkit2.h>

// Wails keeps its WebKitWebView private, so the widget is found by walking the
// GTK toplevels.
static WebKitWebView *find_web_view(GtkWidget *widget)
{
	if (WEBKIT_IS_WEB_VIEW(widget)) {
		return WEBKIT_WEB_VIEW(widget);
	}
	if (!GTK_IS_CONTAINER(widget)) {
		return NULL;
	}
	GList *children = gtk_container_get_children(GTK_CONTAINER(widget));
	WebKitWebView *found = NULL;
	for (GList *child = children; child != NULL && found == NULL; child = child->next) {
		found = find_web_view(GTK_WIDGET(child->data));
	}
	g_list_free(children);
	return found;
}

static gboolean enable_webrtc(gpointer data)
{
	GList *toplevels = gtk_window_list_toplevels();
	WebKitWebView *view = NULL;
	for (GList *top = toplevels; top != NULL && view == NULL; top = top->next) {
		view = find_web_view(GTK_WIDGET(top->data));
	}
	g_list_free(toplevels);
	if (view == NULL) {
		return G_SOURCE_CONTINUE;
	}

	WebKitSettings *settings = webkit_web_view_get_settings(view);
	webkit_settings_set_enable_media_stream(settings, TRUE);
	webkit_settings_set_enable_webrtc(settings, TRUE);

	// A document keeps the constructors it was created with, so a page that
	// beat this callback needs reloading. Wails normally loads its URL after
	// the callback has run, leaving the URI unset and the reload skipped.
	if (webkit_web_view_get_uri(view) != NULL) {
		webkit_web_view_reload(view);
	}
	return G_SOURCE_REMOVE;
}

// The webview lives on the GTK main thread; attaching a source is the
// thread-safe way in from Go's startup goroutine.
static void schedule_enable_webrtc(void)
{
	g_timeout_add(50, enable_webrtc, NULL);
}
*/
import "C"

func enableWebviewWebRTC() {
	C.schedule_enable_webrtc()
}
