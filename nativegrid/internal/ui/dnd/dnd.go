// Package dnd reorders streams by drag and drop, with the live preview the web
// grid has: while a drag is over another stream's widget the dragged stream
// re-slots there, so the others make room as the pointer moves.
//
// One controller serves every surface that shows the same order, so a drag started
// on a tile previews and commits in the sidebar too, and the other way round.
package dnd

import (
	"slices"
	"strconv"

	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/graphene"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// noDrag is Controller.dragging outside a drag.
const noDrag = -1

// Model is the ordering a drag rearranges.
type Model interface {
	// Order is the display order as stream indexes.
	Order() []int
	// Move re-slots one stream at another's position.
	Move(from, to int)
	// SetOrder replaces the order, which is how a cancelled drag falls back.
	SetOrder(order []int)
}

// Controller is the drag state.
type Controller struct {
	model Model
	// dragging is the stream index in flight, -1 outside a drag. The preview skips
	// the dragged stream when hit-testing.
	dragging int
	// startOrder is the order the current drag started from; a cancelled drag falls
	// back to it, undoing the preview.
	startOrder []int
}

func New(m Model) *Controller {
	assert.IsNotNil(m, "a drag rearranges a model")

	return &Controller{model: m, dragging: noDrag}
}

// AttachSource makes w a handle for dragging stream i to another place in the
// order.
//
// The drag icon is a snapshot of w held where the pointer picked it up; w itself
// stays where it is, dimmed to a placeholder that the preview moves around.
func (c *Controller) AttachSource(w gtk.Widgetter, i int) {
	base := gtk.BaseWidget(w)
	src := gtk.NewDragSource()
	src.SetActions(gdk.ActionMove)

	var hotX, hotY int
	src.ConnectPrepare(func(x, y float64) *gdk.ContentProvider {
		hotX, hotY = int(x), int(y)
		src.SetIcon(gtk.NewWidgetPaintable(w).CurrentImage(), hotX, hotY)
		return gdk.NewContentProviderForValue(coreglib.NewValue(strconv.Itoa(i)))
	})
	src.ConnectDragBegin(func(drag gdk.Dragger) {
		c.dragging = i
		c.startOrder = c.model.Order()
		base.AddCSSClass("dragging")
		logger.Tracef("drag started on stream %d", i)
		// Wayland applies a drag hotspot on an icon surface commit; one set before
		// the surface maps is lost and the icon hangs off the pointer by its
		// top-left corner. Re-setting it from idle lands after the map.
		d := gdk.BaseDrag(drag)
		d.SetHotspot(hotX, hotY)
		coreglib.IdleAdd(func() { d.SetHotspot(hotX, hotY) })
	})
	src.ConnectDragCancel(func(_ gdk.Dragger, _ gdk.DragCancelReason) bool {
		c.model.SetOrder(c.fallbackOrder())
		logger.Debugf("drag cancelled, order restored")
		return false
	})
	src.ConnectDragEnd(func(_ gdk.Dragger, _ bool) {
		c.dragging = noDrag
		base.RemoveCSSClass("dragging")
	})
	base.AddController(src)
}

// AttachTarget makes container the one drop target over the stream widgets it
// holds and drives the preview. The drop commits whatever the preview shows;
// nothing is read from the drop value.
//
// widgetAt maps a stream to its widget in this container and returns nil for a
// stream the container does not show, which the hit test skips. stale reports that
// a relayout of the container is due, in which case the bounds a hit test would
// read are the ones that relayout is about to change.
//
// One target on the stable container, instead of one per widget, because a drop
// target that gets reparented mid-drop kills the operation.
func (c *Controller) AttachTarget(container gtk.Widgetter, widgetAt func(i int) gtk.Widgetter, stale func() bool) {
	assert.IsNotNil(widgetAt, "a drop target maps streams to its own widgets")
	assert.IsNotNil(stale, "a drop target knows when its layout is about to change")

	over := func(x, y float64) gdk.DragAction {
		if c.dragging == noDrag {
			return 0
		}
		// Hit-testing against bounds a pending relayout will change would bounce the
		// order back and forth.
		if !stale() {
			c.previewAt(container, widgetAt, x, y)
		}
		return gdk.ActionMove
	}
	dst := gtk.NewDropTarget(coreglib.TypeString, gdk.ActionMove)
	dst.ConnectEnter(over)
	dst.ConnectMotion(over)
	dst.ConnectDrop(func(_ *coreglib.Value, _, _ float64) bool {
		return c.dragging != noDrag
	})
	gtk.BaseWidget(container).AddController(dst)
}

// previewAt re-slots the dragged stream at whichever widget the pointer is over.
// Each re-slot puts the dragged stream's placeholder under the pointer, which keeps
// the preview stable until the pointer crosses into the next widget.
func (c *Controller) previewAt(container gtk.Widgetter, widgetAt func(i int) gtk.Widgetter, x, y float64) {
	pt := graphene.NewPointAlloc().Init(float32(x), float32(y))
	for _, i := range c.model.Order() {
		w := widgetAt(i)
		if i == c.dragging || w == nil {
			continue
		}
		b, ok := gtk.BaseWidget(w).ComputeBounds(container)
		if ok && b.ContainsPoint(pt) {
			c.model.Move(c.dragging, i)
			return
		}
	}
}

// fallbackOrder is the order a cancelled drag returns to. A roster push may have
// added streams mid-drag, so they are carried over: the order the drag started from
// does not know them, and the model's own order must stay a permutation.
func (c *Controller) fallbackOrder() []int {
	restored := slices.Clone(c.startOrder)
	for _, i := range c.model.Order() {
		if !slices.Contains(restored, i) {
			restored = append(restored, i)
		}
	}
	return restored
}
