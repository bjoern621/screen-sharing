package sidebar

import (
	"fmt"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/screenshare-nativegrid/internal/ui/renderpick"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/theme"
)

// The header's render-chain control.
const (
	renderIconSize = 16
	renderMargin   = 12
)

// renderTip describes the control the roster carries no text for: the chain is this
// window's own choice, and what a chain is at all belongs to the decode backend.
const renderTip = "Render chain the tiles draw through unless a tile was given one of its own: what scales and converts the decoded frames on the way to the screen, where it does that, and what it says about the colour they arrive in. " +
	"Moving it restarts the tiles that follow it, since a chain is fixed when the pipeline is built."

// build wraps the list in the scroller, the header the window shows it in, and
// the app bar under both.
func (v *View) build() gtk.Widgetter {
	scroll := gtk.NewScrolledWindow()
	scroll.SetChild(v.list)
	scroll.SetVExpand(true)

	header := adw.NewHeaderBar()
	header.SetTitleWidget(v.title)
	// The render chain is the window's and no row's, so it sits in the header beside
	// the heading rather than on a stream.
	header.PackEnd(v.buildRender())

	view := adw.NewToolbarView()
	view.AddTopBar(header)
	view.SetContent(scroll)
	view.AddBottomBar(v.app.Widget())
	return view
}

// buildRender is the header's render-chain control: a button whose popover holds the
// picker of the window's default. Only build calls it, and what the picker shows is
// drawRender's.
//
// The pick applies at once, unlike the watch-leg popover's Apply. Nothing is being
// composed here: one chain is one change, and the tiles restart on it whenever it
// lands.
//
// The popover closes with the pick, because the picker is the whole of what it holds:
// there is nothing left in it to do once a chain was chosen, and a list that stays
// open reads as a choice that has not been taken yet.
func (v *View) buildRender() *gtk.MenuButton {
	popover := gtk.NewPopover()
	v.render = renderpick.New(false, v.dispatch, func(name string) {
		v.sess.SetDefaultRenderChain(name)
		popover.Popdown()
	})

	body := gtk.NewBox(gtk.OrientationVertical, legSpacing)
	body.SetMarginTop(renderMargin)
	body.SetMarginBottom(renderMargin)
	body.SetMarginStart(renderMargin)
	body.SetMarginEnd(renderMargin)
	body.Append(legRow("Render through", renderTip, v.render.Widget()))

	popover.SetChild(body)

	v.renderButton = gtk.NewMenuButton()
	v.renderButton.AddCSSClass("flat")
	v.renderButton.SetChild(v.icons.Image("video", renderIconSize, theme.Muted))
	v.renderButton.SetTooltipText(renderTip)
	v.renderButton.SetPopover(popover)
	return v.renderButton
}

// drawRender puts the window's default render chain on the header's control.
//
// What the backend offers is read back on every pass rather than kept: which chains
// this machine can run is the backend's answer, and a copy here would be a second
// version of it. A backend offering nothing leaves the control hidden, since there is
// then nothing to choose and no default to name.
func (v *View) drawRender() {
	chains := v.sess.Chains()
	v.renderButton.SetVisible(len(chains) > 0)
	if len(chains) == 0 {
		v.render.Draw(renderpick.Choice{})
		return
	}
	v.render.Draw(renderpick.Choice{Chains: chains, Chosen: v.sess.DefaultRenderChain()})
}

// drawTitle states the open-tile count beside the heading, the way the web roster's
// badge does. An empty subtitle collapses, so a grid with nothing open shows the
// heading alone, like the web shows no badge.
func (v *View) drawTitle() {
	n := v.sess.Watching()
	if n == 0 {
		v.title.SetSubtitle("")
		return
	}
	v.title.SetSubtitle(fmt.Sprintf("%d watching", n))
}
