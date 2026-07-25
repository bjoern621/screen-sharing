package tile

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/screenshare-nativegrid/internal/ui/theme"
)

// The sizes the pages are drawn at: the skeleton spinner, the chip marker, and the
// failure badge, each the web tile's.
const (
	spinnerSize = 22
	markerSize  = 16
	badgeSize   = 20

	// pageSpacing is the gap between the stacked parts of a page, the web tile's
	// space-y-3.
	pageSpacing = 12
)

// buildSkeleton is the loading page: the spinner over the stream's name.
func (t *Tile) buildSkeleton() {
	spinner := theme.NewSpinner(spinnerSize)
	spinner.Widget().AddCSSClass("tile-spinner")

	name := gtk.NewLabel(t.stream.Name)
	name.AddCSSClass("tile-overlay-strong")
	name.AddCSSClass("heading")

	sub := gtk.NewLabel("waiting for the first frame")
	sub.AddCSSClass("tile-overlay-dim")
	sub.AddCSSClass("caption")

	t.skeleton = gtk.NewBox(gtk.OrientationVertical, pageSpacing)
	t.skeleton.SetHAlign(gtk.AlignCenter)
	t.skeleton.SetVAlign(gtk.AlignCenter)
	t.skeleton.Append(spinner.Widget())
	t.skeleton.Append(name)
	t.skeleton.Append(sub)
}

// buildChip is the corner label: the stream's name, and the web tile's volume-off
// icon beside it while the tile is muted.
func (t *Tile) buildChip() {
	t.chipMute = theme.FixedImage("volume-off", markerSize, theme.OnTile)
	t.chipMute.SetVisible(false)

	t.chip = gtk.NewBox(gtk.OrientationHorizontal, 6)
	t.chip.AddCSSClass("tile-label")
	t.chip.Append(gtk.NewLabel(t.stream.Name))
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

	t.failureLabel = gtk.NewLabel("")
	t.failureLabel.AddCSSClass("tile-error")
	t.failureLabel.AddCSSClass("caption")
	t.failureLabel.SetWrap(true)
	t.failureLabel.SetMaxWidthChars(40)
	t.failureLabel.SetJustify(gtk.JustifyCenter)

	retryBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
	retryBox.Append(t.icons.Image("refresh", markerSize, theme.Foreground))
	retryBox.Append(gtk.NewLabel("retry"))
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
