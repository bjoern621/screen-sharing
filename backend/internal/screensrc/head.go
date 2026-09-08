package screensrc

import (
	"fmt"
	"strconv"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/share"
)


// heads builds one element's source fragment:
// the element itself, the pointer properties and the selection, and nothing downstream.
// What follows differs per consumer,
// a publish head being paced and converted for an encoder and a preview head scaled for a window.
var heads = map[string]func(share.Target) ([]string, error){
	XImage: ximageHead,
	D3D11:  d3d11Head,
}

// Head is the fragment that reads the target through the named element,
// nil for an element this package builds no head for.
//
// The error is an Umgebungsfehler, and every one of them is a stored setting the machine has
// moved out from under: a rectangle no single output holds, or a kind this element cannot read.
// The form greys both before a reader can pick them,
// so what reaches here is a hand-edited file or a screen unplugged since the pick.
func Head(element string, t share.Target) ([]string, error) {
	assert.Assert(share.Known(t.Kind), "a head is built for a kind the settings carry", t.Kind)

	build, ok := heads[element]
	if !ok {
		return nil, nil
	}

	head, err := build(t)
	if err != nil {
		return nil, err
	}
	assert.Assert(len(head) > 0 && head[0] == element,
		"a head leads with the element it was built for", element)
	return head, nil
}

// ximageHead reads the X screen, cropped to what the target names.
//
// One mechanism for two kinds: the X screen is one picture, a monitor is a rectangle of it,
// and a region is a rectangle of it measured in the same pixels.
// A window is the element's other input, read by XID with no crop at all.
//
// An enumeration reporting no geometry for the selected monitor leaves the crop off,
// capturing the whole X screen rather than a guessed rectangle.
// An enumeration with no geometry is a machine that cannot measure its outputs,
// and the whole screen is the honest answer to it.
// An index no output answers to is a settings file naming a monitor that was unplugged.
// It is refused before a head is ever built, where the capture answers with an error
// (publish/gstcapture.go, ximageCapture.Open; internal/ffmpeg, x11grabArgs).
func ximageHead(t share.Target) ([]string, error) {
	head := []string{XImage, "use-damage=false", "show-pointer=" + boolProperty(t.Pointer)}

	switch t.Kind {
	case share.Window:
		return append(head, "xid="+strconv.FormatUint(t.Window, 10)), nil
	case share.Region:
		return append(head, cropProperties(t.Region.X, t.Region.Y, t.Region.Width, t.Region.Height)...), nil
	}

	m, ok := display.At(t.Monitor)
	if !ok || m.Width <= 0 || m.Height <= 0 {
		return head, nil
	}
	return append(head, cropProperties(m.OffsetX, m.OffsetY, m.Width, m.Height)...), nil
}

// cropProperties is ximagesrc's rectangle, in the X screen's own pixels.
// endx and endy are inclusive: the last captured column is the offset plus the width minus one.
func cropProperties(x, y, width, height int) []string {
	assert.Assert(width > 0 && height > 0, "a crop has a size", width, height)

	return []string{
		"startx=" + strconv.Itoa(x),
		"starty=" + strconv.Itoa(y),
		"endx=" + strconv.Itoa(x+width-1),
		"endy=" + strconv.Itoa(y+height-1),
	}
}

// d3d11Head reads one output through Desktop Duplication, or one window through Windows Graphics
// Capture.
//
// The monitor index reaches the element without a lookup, as ddagrab's output_idx does:
// both name a monitor in the Windows enumeration the index already stands for.
//
// A region is that same output plus a crop into it,
// the crop properties counting from the output's own top-left corner where the setting counts
// from the desktop's.
// A rectangle no single output holds is refused:
// Desktop Duplication hands out one output at a time,
// and cropping the wrong one would publish a picture nobody drew.
//
// A window needs the Graphics Capture path, Desktop Duplication reading outputs alone,
// so the capture API is named with the handle rather than left at its default.
func d3d11Head(t share.Target) ([]string, error) {
	head := []string{D3D11, "show-cursor=" + boolProperty(t.Pointer)}

	switch t.Kind {
	case share.Window:
		return append(head, "capture-api=wgc", "window-handle="+strconv.FormatUint(t.Window, 10)), nil
	case share.Region:
		r := t.Region
		m, ok := display.Containing(r.X, r.Y, r.Width, r.Height)
		if !ok {
			return nil, fmt.Errorf("the rectangle %s is not inside one of this machine's outputs", r)
		}
		local := r.Offset(m.OffsetX, m.OffsetY)
		return append(head,
			"monitor-index="+strconv.Itoa(m.Index),
			"crop-x="+strconv.Itoa(local.X),
			"crop-y="+strconv.Itoa(local.Y),
			"crop-width="+strconv.Itoa(local.Width),
			"crop-height="+strconv.Itoa(local.Height),
		), nil
	}
	return append(head, "monitor-index="+strconv.Itoa(t.Monitor)), nil
}

// boolProperty is how a GStreamer element spells a boolean property value: "true" or "false".
// One helper: the two heads carry the same fact through differently named properties,
// and a literal typed per site is one that can be spelled "1" at one of them.
func boolProperty(on bool) string {
	if on {
		return "true"
	}
	return "false"
}
