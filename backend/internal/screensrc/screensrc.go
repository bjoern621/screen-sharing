// Package screensrc names the GStreamer elements that read this machine's own screen,
// and the properties pointing one at a monitor, a window or a rectangle.
//
// Two consumers read the same rectangle:
// the publish pipeline's capture head, and the setup wizard's monitor preview.
// A preview cropped differently from the stream would be a picture that lies about what is shared,
// so the element and its selection are written once here and read by both.
//
// Two tables, answering different questions.
// heads answers what reads a target through a named element, keyed by the element (head.go):
// a publish pipeline is rendered from settings alone,
// and an ximagesrc line has to render the same on a machine running Windows.
// sessions answers what reads a monitor here, keyed by the running session:
// a preview is opened on the machine it is shown on,
// with no capture backend selected to derive it from.
//
// A missing row is a fact the surface states.
// A Wayland session reaches its screens through the portal alone,
// which pops the compositor's picker and answers with whatever was chosen there,
// and AVFoundation's screen source picks its own display.
// Neither can produce a picture of one named monitor, so neither has a row,
// and Session says so with the session named.
package screensrc

import (
	"fmt"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/share"
	"bjoernblessin.de/screenshare/internal/text"
)

// The elements this package builds heads for, spelled as GStreamer spells them.
// The constants keep the tables and every caller on one spelling.
const (
	XImage = "ximagesrc"
	D3D11  = "d3d11screencapturesrc"
)

// sessionSource pairs a session with the element that reads its screens,
// in the shape publish.captureNeeds states platform applicability in:
// an operating system, and the Linux display server where that is not the whole answer.
type sessionSource struct {
	os      string
	display string
	element string
}

// sessions is what reads a single monitor, per session.
//
// Windows leaves display blank: one windowing system, nothing to distinguish.
// Linux names x11 and not wayland, the whole of why a Wayland session has no monitor preview:
// the portal is its only way to a screen and it answers with what the picker was told.
// macOS has no row at all, avfvideosrc choosing its own display,
// which is also why the monitor setting does not reach the publish pipeline there
// (docs/capture-architecture.md).
var sessions = []sessionSource{
	{os: "windows", element: D3D11},
	{os: "linux", display: "x11", element: XImage},
}

// One set of elements across both tables.
// A session naming an element with no head offers a preview and then fails to build it,
// an Entwicklungsfehler, so it fails at load.
func init() {
	for _, s := range sessions {
		_, ok := heads[s.element]
		assert.Assert(ok, "a session's screen source has a head to build", s.element, s.os)
		assert.Assert(s.os != "", "a screen source names the operating system it runs on", s.element)
	}
}

// Session is the element that reads a single monitor here,
// or the statement saying what this session has instead.
//
// Exactly one of the two comes back:
// a machine either has a way to show one screen or a statement about why it does not,
// and an element with a reason beside it would leave a caller deciding which half to believe.
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
// The size is a bound the scaler fixates inside,
// so a smaller source is left alone and a larger one is reduced with its aspect ratio kept.
// The scaling sits here rather than in the render chain:
// the default chain on Linux writes no size bound at all,
// and a preview that did not reduce its own frames would upload whole desktops for a tile.
const (
	previewFps    = 5
	previewWidth  = 640
	previewHeight = 360
)

// PreviewSource is the launch fragment one monitor's preview is read through:
// the session's own screen element, paced and reduced to what a wizard tile draws.
//
// It produces pictures rather than a bitstream,
// so a receiver built on it is opened raw and grows no decoder and no audio branch
// (receive.Stream.Raw).
//
// The error is an Umgebungsfehler: a session with no element that reads a single output.
// Which sessions those are is sessions' business,
// and Session states the same fact as a code for the surface that writes a sentence about it.
func PreviewSource(p platform.Info, index int) (string, error) {
	element, gap := Session(p)
	if gap != nil {
		return "", fmt.Errorf("this session cannot read one monitor apart from another")
	}

	head, err := Head(element, share.MonitorTarget(index))
	assert.Assert(err == nil, "a monitor head builds on every session that has one", element, index)
	assert.Assert(len(head) > 0, "a session's screen source builds a head", element)

	parts := append(head,
		"!", fmt.Sprintf("video/x-raw,framerate=%d/1", previewFps),
		"!", "videoscale",
		"!", fmt.Sprintf("video/x-raw,width=[1,%d],height=[1,%d]", previewWidth, previewHeight),
	)
	return strings.Join(parts, " "), nil
}
