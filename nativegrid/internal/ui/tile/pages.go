package tile

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"bjoernblessin.de/screenshare-nativegrid/internal/ui/theme"
)

// The sizes the pages are drawn at: the waiting spinner, a marker beside the chip's
// label, and the badge of a page that reports a problem, each the web tile's.
const (
	spinnerSize = 22
	markerSize  = 16
	badgeSize   = 20

	// pageSpacing is the gap between the stacked parts of a page, the web tile's
	// space-y-3.
	pageSpacing = 12

	// messageChars is how wide a page lets a sentence run before it wraps.
	messageChars = 40
)

// stallTip explains a frozen picture. Both the corner marker and the page carry it,
// since either one can be what the pointer finds first.
const stallTip = "No frames are arriving. The picture is the last one that did, and the connection to the relay is still open."

// pageLabel is one line of a page's prose, in the color the page draws it in.
//
// It ellipsizes because a widget's size request is a minimum: a label that insisted
// on the width of its text would hold the tile open around it, and a tile shrunk to
// the film strip is a third the width of one sentence.
func pageLabel(text, color string) *gtk.Label {
	l := gtk.NewLabel(text)
	l.AddCSSClass(color)
	l.SetEllipsize(pango.EllipsizeEnd)
	l.SetMaxWidthChars(messageChars)
	return l
}

// waitPage is the centered page of a tile with nothing to draw yet: the spinner over
// the stream's name and the one thing the tile waits for. It names the stream, which
// is why the corner chip stays out of the states that show it.
func (t *Tile) waitPage(waitingFor string) *gtk.Box {
	spinner := theme.NewSpinner(spinnerSize)
	spinner.Widget().AddCSSClass("tile-spinner")

	name := pageLabel(t.stream.Name, "tile-overlay-strong")
	name.AddCSSClass("heading")

	sub := pageLabel(waitingFor, "tile-overlay-dim")
	sub.AddCSSClass("caption")

	page := gtk.NewBox(gtk.OrientationVertical, pageSpacing)
	page.SetHAlign(gtk.AlignCenter)
	page.SetVAlign(gtk.AlignCenter)
	page.Append(spinner.Widget())
	page.Append(name)
	page.Append(sub)
	return page
}

// buildSkeleton is the loading page, the one page a tile opens on.
func (t *Tile) buildSkeleton() {
	t.skeleton = t.waitPage("Waiting for the first frame")
}

// buildReconnect is the page of a stream whose pipeline ended with a retry pending.
// It is the waiting treatment rather than the failure page's badge and retry button,
// because the retry is the model's and there is nothing to ask of the viewer.
func (t *Tile) buildReconnect() {
	t.reconnect = t.waitPage("The stream dropped, reconnecting")

	// The pipeline's own last words, so a stream that keeps dropping says why while
	// the retries are still running rather than once they are spent.
	t.reconnectLabel = pageLabel("", "tile-overlay-dim")
	t.reconnectLabel.AddCSSClass("caption")
	t.reconnectLabel.SetWrap(true)
	t.reconnectLabel.SetJustify(gtk.JustifyCenter)

	t.reconnect.Append(t.reconnectLabel)
	t.reconnect.SetVisible(false)
}

// buildStall is the frozen-picture page. A stall takes the overlay's white and no
// retry button: the pipeline is running, so there is nothing to restart and the
// frames can come back on their own.
func (t *Tile) buildStall() {
	icon := theme.FixedImage("alert-triangle", badgeSize, theme.OnTile)

	label := gtk.NewLabel("No frames arriving")
	label.AddCSSClass("tile-overlay-strong")
	label.AddCSSClass("caption")

	t.stall = gtk.NewBox(gtk.OrientationVertical, pageSpacing)
	t.stall.SetHAlign(gtk.AlignCenter)
	t.stall.SetVAlign(gtk.AlignCenter)
	t.stall.SetTooltipText(stallTip)
	t.stall.Append(icon)
	t.stall.Append(label)
	t.stall.SetVisible(false)
}

// buildChip is the corner label: the stream's name, the stall marker, and the web
// tile's volume-off icon while the tile is muted.
func (t *Tile) buildChip() {
	t.chipStall = theme.FixedImage("alert-triangle", markerSize, theme.OnTile)
	t.chipStall.SetTooltipText(stallTip)
	t.chipStall.SetVisible(false)

	t.chipMute = theme.FixedImage("volume-off", markerSize, theme.OnTile)
	t.chipMute.SetTooltipText("This tile is muted. The volume control in its bar plays the audio again.")
	t.chipMute.SetVisible(false)

	t.chip = gtk.NewBox(gtk.OrientationHorizontal, 6)
	t.chip.AddCSSClass("tile-label")
	// The chip prints the stream's name, and names the watch leg it arrives over on
	// hover. A roster that left the transport out gets no tooltip rather than a
	// sentence with a hole in it.
	if t.stream.Transport != "" {
		t.chip.SetTooltipText("Watching this stream over " + t.stream.Transport +
			", the watch leg (relay to viewer). It is independent of the protocol the stream was published over.")
	}
	t.chip.Append(gtk.NewLabel(t.stream.Name))
	t.chip.Append(t.chipStall)
	t.chip.Append(t.chipMute)
	t.chip.SetHAlign(gtk.AlignStart)
	t.chip.SetVAlign(gtk.AlignStart)
	t.chip.SetMarginTop(8)
	t.chip.SetMarginStart(8)
	t.chip.SetVisible(false)
}

// buildFailure is the error page: the badge, the pipeline's message, and a retry
// that restarts the player.
func (t *Tile) buildFailure() {
	icon := theme.FixedImage("plug-connected-x", badgeSize, theme.DestructiveOnTile)
	icon.SetHExpand(true)
	icon.SetVExpand(true)

	badge := gtk.NewBox(gtk.OrientationVertical, 0)
	badge.AddCSSClass("error-icon")
	badge.SetHAlign(gtk.AlignCenter)
	badge.Append(icon)

	t.failureLabel = pageLabel("", "tile-error")
	t.failureLabel.AddCSSClass("caption")
	t.failureLabel.SetWrap(true)
	t.failureLabel.SetJustify(gtk.JustifyCenter)

	retryBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
	retryBox.Append(t.icons.Image("refresh", markerSize, theme.Foreground))
	retryBox.Append(gtk.NewLabel("Retry"))
	retry := gtk.NewButton()
	retry.SetChild(retryBox)
	retry.SetHAlign(gtk.AlignCenter)
	retry.ConnectClicked(t.hooks.Retry)

	t.failure = gtk.NewBox(gtk.OrientationVertical, pageSpacing)
	t.failure.SetHAlign(gtk.AlignCenter)
	t.failure.SetVAlign(gtk.AlignCenter)
	t.failure.Append(badge)
	t.failure.Append(t.failureLabel)
	t.failure.Append(retry)
	t.failure.SetVisible(false)
}
