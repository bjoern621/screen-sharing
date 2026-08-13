// Package screensrc names the GStreamer elements that read one of this machine's monitors,
// and the properties that single one out.
//
// Two consumers need the same rectangle.
// The publish pipeline's capture head reads the screen the stream carries,
// and the setup wizard's monitor preview reads the screen it is offering; a preview cropped
// differently from the stream would be a picture that lies about what is shared,
// which is the one thing a preview may not do.
// So the element and its monitor selection are written here and read by both.
//
// Two tables, and they answer different questions.
// Head answers "what reads the monitor this element captures", keyed by the element itself,
// because a publish pipeline is rendered from settings alone and an ximagesrc line has to render
// the same on a machine running Windows.
// Session answers "what reads a monitor here", keyed by the running session,
// because a preview is opened on the machine it is shown on and there is no backend selected yet to
// derive it from.
//
// The absence of a row is a fact the surface states.
// A Wayland session reaches its screens through the portal alone, which pops the compositor's
// picker and answers with whatever the user chose rather than with the output that was asked for,
// and AVFoundation's screen source picks its own display.
// Neither can produce a picture of one named monitor, so neither has a row,
// and Session says so with the session named.
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

// The elements this package builds heads for, named as GStreamer names them.
// They are the two capture backends that take a monitor index, and the constants exist so the two
// tables below and the callers all spell one string once.
const (
	XImage = "ximagesrc"
	D3D11  = "d3d11screencapturesrc"
)

// heads builds one element's source fragment: the element, the properties that draw the pointer in,
// and the ones that select the monitor.
// Nothing downstream of it, since what follows differs per consumer: a publish head is paced and
// converted for an encoder, and a preview head is scaled for a window.
var heads = map[string]func(index int, pointer bool) []string{
	XImage: ximageHead,
	D3D11:  d3d11Head,
}

// sessionSource pairs one session with the element that reads its screens,
// in the shape publish.captureNeeds states platform applicability in: the operating system,
// and the Linux display server where the operating system is not the whole answer.
type sessionSource struct {
	os      string
	display string
	element string
}

// sessions is what reads a single monitor, per session.
//
// Windows names no display server because it has one.
// Linux names x11 and not wayland, which is the whole of why a Wayland session has no monitor
// preview: the portal is its only way to a screen, and the portal's answer is what the picker was
// told rather than what the caller asked for.
// macOS has no row at all, since avfvideosrc chooses its own display, which is also why the monitor
// setting does not reach the publish pipeline there (docs/capture-architecture.md).
var sessions = []sessionSource{
	{os: "windows", element: D3D11},
	{os: "linux", display: "x11", element: XImage},
}

// The two tables describe one set of elements, so a session naming an element with no head is a
// session this package would offer a preview for and then fail to build.
// That is an Entwicklungsfehler rather than a condition to survive, so it fails at load.
func init() {
	for _, s := range sessions {
		_, ok := heads[s.element]
		assert.Assert(ok, "a session's screen source has a head to build", s.element, s.os)
		assert.Assert(s.os != "", "a screen source names the operating system it runs on", s.element)
	}
}

// Head is the elements that read the monitor at index through the named element,
// and nil for an element this package builds no head for.
//
// The index is the one the display enumeration reports, which is the value PublishSettings.monitor
// carries.
// Where the enumeration holds no such output the head is built without a selection:
// on X11 that captures the whole screen rather than a guessed rectangle, which is the same answer
// x11grab gives, and on Windows the element is handed the index whatever the enumeration knows,
// because it is the enumeration's own.
//
// pointer says whether the head draws the mouse pointer into the frames.
// A publish passes what the settings hold; a preview passes true, because a preview shows the
// screen as it is rather than as a stream would carry it.
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

// Session is the element that reads a single monitor here, and the statement saying what this
// session has instead where there is none.
//
// Exactly one of the two is returned.
// An element with a reason beside it would be a caller having to decide which half to believe,
// and the whole point of the pair is that a machine either has a way to show one screen or has a
// sentence about why it does not.
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

// What a preview is paced and sized at, which is the whole of what separates it from the publish
// head above it.
//
// Both are bounds written for a wizard tile rather than for a stream.
// Five frames a second is what tells one screen from another and what a reader notices a window
// opening on; the size is a bound the scaler fixates inside, so a source smaller than it is left
// alone and a larger one is reduced with its aspect ratio kept.
// Scaling here rather than in the render chain is deliberate: the default chain on Linux writes no
// size bound at all, so a preview that did not reduce its own frames would upload whole desktops
// for a picture drawn at a fraction of one.
const (
	previewFps    = 5
	previewWidth  = 640
	previewHeight = 360
)

// PreviewSource is the launch fragment one monitor's preview is read through:
// the session's own screen element, paced and reduced to what a wizard tile draws.
//
// The frames it produces are pictures and not a bitstream, so a receiver built on it is opened raw
// and grows no decoder and no audio branch (receive.Stream.Raw).
//
// The error is an Umgebungsfehler and the one refusal this can make: a session with no element that
// reads a single output.
// Which sessions those are is sessions' business, and Session states the same fact as a code for
// the surface that has to write a sentence about it.
func PreviewSource(p platform.Info, index int) (string, error) {
	element, gap := Session(p)
	if gap != nil {
		return "", fmt.Errorf("this session cannot read one monitor apart from another")
	}

	// The pointer is drawn whatever the publish setting holds.
	// A preview answers what a screen looks like, so that a reader can tell one monitor from another,
	// and it is not a rendering of the stream: a preview with no pointer would show two identical
	// desktops where the pointer is the only thing telling them apart.
	head := Head(element, index, true)
	assert.Assert(len(head) > 0, "a session's screen source builds a head", element)

	parts := append(head,
		"!", fmt.Sprintf("video/x-raw,framerate=%d/1", previewFps),
		"!", "videoscale",
		"!", fmt.Sprintf("video/x-raw,width=[1,%d],height=[1,%d]", previewWidth, previewHeight),
	)
	return strings.Join(parts, " "), nil
}

// ximageHead reads the X screen and crops to the selected monitor where its geometry is known.
// Enumeration failing leaves no offset, so the whole X screen is captured instead of a guessed
// rectangle.
//
// The end coordinates are inclusive, so the last captured column is the offset plus the width minus
// one.
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
// The index reaches the element without a lookup, the way ddagrab's output_idx does:
// both name a monitor in the Windows enumeration the index already stands for.
func d3d11Head(index int, pointer bool) []string {
	return []string{D3D11, "show-cursor=" + boolProperty(pointer), "monitor-index=" + strconv.Itoa(index)}
}

// boolProperty is how a GStreamer element spells a boolean property value.
// One helper because the two heads set the same fact through differently named properties,
// and a literal typed at each site is one that can be spelled "1" at one of them.
func boolProperty(on bool) string {
	if on {
		return "true"
	}
	return "false"
}

// monitorAt is the enumerated output with this index, and false where the enumeration has no entry
// for it: a screen unplugged since the setting was stored, or a session whose outputs could not be
// read at all.
func monitorAt(index int) (display.Monitor, bool) {
	for _, m := range display.List() {
		if m.Index == index {
			return m, true
		}
	}
	return display.Monitor{}, false
}
