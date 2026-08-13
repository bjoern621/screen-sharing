// Package screensrc names the GStreamer elements that read one of this machine's monitors, and the
// properties that single one out.
//
// Two consumers read the same rectangle: the publish pipeline's capture head, and the setup
// wizard's monitor preview.
// A preview cropped differently from the stream would be a picture that lies about what is shared,
// so the element and its monitor selection are written once here and read by both.
//
// Two tables, answering different questions.
// Head answers what reads the monitor a named element captures, keyed by the element, because a
// publish pipeline is rendered from settings alone and an ximagesrc line has to render the same on
// a machine running Windows.
// Session answers what reads a monitor here, keyed by the running session, because a preview is
// opened on the machine it is shown on with no capture backend selected to derive it from.
//
// A missing row is a fact the surface states.
// A Wayland session reaches its screens through the portal alone, which pops the compositor's
// picker and answers with whatever was chosen there rather than with the output asked for, and
// AVFoundation's screen source picks its own display.
// Neither can produce a picture of one named monitor, so neither has a row, and Session says so
// with the session named.
package screensrc

import (
	"fmt"
	"strconv"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/text"
)

// The elements this package builds heads for, spelled as GStreamer spells them.
// Both take a monitor index, and the constants keep the tables below and every caller on one
// spelling.
const (
	XImage = "ximagesrc"
	D3D11  = "d3d11screencapturesrc"
)

// heads builds one element's source fragment: the element itself, the pointer properties and the
// monitor selection, and nothing downstream of them.
// What follows differs per consumer, a publish head being paced and converted for an encoder and a
// preview head scaled for a window.
var heads = map[string]func(index int, pointer bool) []string{
	XImage: ximageHead,
	D3D11:  d3d11Head,
}

// sessionSource pairs a session with the element that reads its screens, in the shape
// publish.captureNeeds states platform applicability in: an operating system, plus the Linux
// display server where the operating system is not the whole answer.
type sessionSource struct {
	os      string
	display string
	element string
}

// sessions is what reads a single monitor, per session.
//
// Windows names no display server, having one.
// Linux names x11 and not wayland, which is the whole of why a Wayland session has no monitor
// preview: the portal is its only way to a screen and it answers with what the picker was told.
// macOS has no row at all, avfvideosrc choosing its own display, which is also why the monitor
// setting does not reach the publish pipeline there (docs/capture-architecture.md).
var sessions = []sessionSource{
	{os: "windows", element: D3D11},
	{os: "linux", display: "x11", element: XImage},
}

// One set of elements across both tables.
// A session naming an element with no head is one this package offers a preview for and then fails
// to build, which is an Entwicklungsfehler and fails at load.
func init() {
	for _, s := range sessions {
		_, ok := heads[s.element]
		assert.Assert(ok, "a session's screen source has a head to build", s.element, s.os)
		assert.Assert(s.os != "", "a screen source names the operating system it runs on", s.element)
	}
}

// Head is the fragment that reads the monitor at index through the named element, nil for an
// element this package builds no head for.
//
// The index is the display enumeration's, which is what PublishSettings.monitor carries.
// Where the enumeration holds no such output the head is built without a selection: X11 then
// captures the whole screen rather than a guessed rectangle, the answer x11grab gives, and Windows
// is handed the index whatever the enumeration knows, the index being that enumeration's own.
//
// pointer draws the mouse pointer into the frames.
// A publish passes what the settings hold, a preview passes true, a preview showing the screen as
// it is rather than as a stream would carry it.
func Head(element string, index int, pointer bool) []string {
	build, ok := heads[element]
	if !ok {
		return nil
	}

	head := build(index, pointer)
	assert.Assert(len(head) > 0 && head[0] == element,
		"a head leads with the element it was built for", element)
	return head
}

// Session is the element that reads a single monitor here, or the statement saying what this
// session has instead.
//
// Exactly one of the two comes back: a machine either has a way to show one screen or has a
// sentence about why it does not, and an element with a reason beside it would leave a caller
// deciding which half to believe.
func Session(p platform.Info) (string, *screensharev1.Text) {
	for _, s := range sessions {
		if s.os != p.OS {
			continue
		}
		if s.display != "" && s.display != p.Display {
			continue
		}
		return s.element, nil
	}

	return "", text.Of(screensharev1.TextCode_TEXT_CODE_NO_MONITOR_PREVIEW,
		text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_OS, p.OS),
		text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_DISPLAY, p.Display))
}

// What a preview is paced and sized at, the whole of what separates it from a publish head.
//
// Both are bounds for a wizard tile rather than for a stream.
// Five frames a second tells one screen from another and shows a window opening.
// The size is a bound the scaler fixates inside, so a smaller source is left alone and a larger one
// is reduced with its aspect ratio kept.
// The scaling sits here rather than in the render chain because the default chain on Linux writes
// no size bound at all, and a preview that did not reduce its own frames would upload whole
// desktops for a picture drawn at a fraction of one.
const (
	previewFps    = 5
	previewWidth  = 640
	previewHeight = 360
)

// PreviewSource is the launch fragment one monitor's preview is read through: the session's own
// screen element, paced and reduced to what a wizard tile draws.
//
// It produces pictures and not a bitstream, so a receiver built on it is opened raw and grows no
// decoder and no audio branch (receive.Stream.Raw).
//
// The error is an Umgebungsfehler and the one refusal made here: a session with no element that
// reads a single output.
// Which sessions those are is sessions' business, and Session states the same fact as a code for
// the surface that writes a sentence about it.
func PreviewSource(p platform.Info, index int) (string, error) {
	element, gap := Session(p)
	if gap != nil {
		return "", fmt.Errorf("this session cannot read one monitor apart from another")
	}

	// The pointer is drawn whatever the publish setting holds.
	// A preview answers what a screen looks like rather than what the stream would carry: two desktops
	// alike but for the pointer are two pictures a reader cannot tell apart without it.
	head := Head(element, index, true)
	assert.Assert(len(head) > 0, "a session's screen source builds a head", element)

	parts := append(head,
		"!", fmt.Sprintf("video/x-raw,framerate=%d/1", previewFps),
		"!", "videoscale",
		"!", fmt.Sprintf("video/x-raw,width=[1,%d],height=[1,%d]", previewWidth, previewHeight),
	)
	return strings.Join(parts, " "), nil
}

// ximageHead reads the X screen, cropped to the selected monitor where its geometry is known.
// An enumeration that reports no geometry leaves the crop off, capturing the whole X screen rather
// than a guessed rectangle.
//
// endx and endy are inclusive: the last captured column is the offset plus the width minus one.
func ximageHead(index int, pointer bool) []string {
	head := []string{XImage, "use-damage=false", "show-pointer=" + boolProperty(pointer)}
	m, ok := monitorAt(index)
	if !ok || m.Width <= 0 || m.Height <= 0 {
		return head
	}
	return append(head,
		"startx="+strconv.Itoa(m.OffsetX),
		"starty="+strconv.Itoa(m.OffsetY),
		"endx="+strconv.Itoa(m.OffsetX+m.Width-1),
		"endy="+strconv.Itoa(m.OffsetY+m.Height-1),
	)
}

// d3d11Head reads one output through Desktop Duplication.
// The index reaches the element without a lookup, as ddagrab's output_idx does: both name a monitor
// in the Windows enumeration the index already stands for.
func d3d11Head(index int, pointer bool) []string {
	return []string{D3D11, "show-cursor=" + boolProperty(pointer), "monitor-index=" + strconv.Itoa(index)}
}

// boolProperty is how a GStreamer element spells a boolean property value: "true" or "false".
// One helper, because the two heads carry the same fact through differently named properties and a
// literal typed per site is one that can be spelled "1" at one of them.
func boolProperty(on bool) string {
	if on {
		return "true"
	}
	return "false"
}

// monitorAt is the enumerated output carrying this index, false where the enumeration carries none:
// a screen unplugged since the setting was stored, or a session whose outputs could not be read.
func monitorAt(index int) (display.Monitor, bool) {
	for _, m := range display.List() {
		if m.Index == index {
			return m, true
		}
	}
	return display.Monitor{}, false
}
