package publish

import (
	"fmt"
	"os"
	"strconv"

	"bjoernblessin.de/screenshare/display"
	"bjoernblessin.de/screenshare/portal"
	"bjoernblessin.de/screenshare/settings"
)

// A GStreamer capture backend is the head of the publish pipeline: the elements
// that produce raw frames, up to and including the conversion that pins the
// encoder input. Everything after that point is the same for all of them, which
// is why the seam sits there.
//
// The two backends differ in more than an element name. The portal one performs
// a D-Bus handshake first and hands the child a descriptor; the X11 one opens
// its display itself and needs nothing acquired. gstCapture is the contract that
// covers both, so the engine below runs either without naming one.

// childFdBase is the descriptor number ExtraFiles[0] is inherited as. A backend
// that passes files gets the first ones, so the descriptors it writes into its
// elements start here.
const childFdBase = 3

// The placeholders the displayed command carries where a run passes values that
// exist only after the source is opened.
const (
	fdPlaceholder   = "<portal-fd>"
	nodePlaceholder = "<portal-node>"
)

// gstCaptureOptions are the parts of the source chain that belong to the run
// rather than to the backend.
type gstCaptureOptions struct {
	// InCaps is the capsfilter pinning chroma and colorimetry. It is passed in
	// rather than appended by the engine because where the conversion sits differs
	// per backend: a damage-driven source converts once per damage frame, ahead of
	// the element that paces the output.
	InCaps string
	// RateProbe counts the frames the source really produced, empty for a
	// pipeline built without instrumentation. A backend places it at the last
	// point where one buffer is one new picture, so what it counts is the rate the
	// screen changed at and not the rate the encoder was paced to.
	RateProbe []string
}

// gstCapture is one screen source the GStreamer publish engine can drive.
type gstCapture interface {
	// Name is the settings value and the key in captureBackends.
	Name() string
	// Describe returns the source elements for the rendered command, with a
	// placeholder wherever Open substitutes a value the handshake produces.
	Describe(s settings.Stream, opts gstCaptureOptions) []string
	// Open acquires the source and returns the elements a run uses, the files the
	// child inherits as descriptors childFdBase and up, and the teardown that
	// runs when the child exits. A backend that acquires nothing returns no files
	// and a no-op teardown.
	Open(s settings.Stream, opts gstCaptureOptions) (elements []string, files []*os.File, closeFn func(), err error)
}

// portalCapture is the xdg-desktop-portal ScreenCast backend: the compositor's
// own picker chooses what is shared, and the frames arrive over PipeWire.
type portalCapture struct{}

func (portalCapture) Name() string { return "portal" }

func (portalCapture) Describe(s settings.Stream, opts gstCaptureOptions) []string {
	return portalElements(s, fdPlaceholder, nodePlaceholder, opts)
}

func (p portalCapture) Open(s settings.Stream, opts gstCaptureOptions) ([]string, []*os.File, func(), error) {
	session, err := portal.Open(portal.Options{})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("portal ScreenCast: %w", err)
	}
	elements := portalElements(s, strconv.Itoa(childFdBase), strconv.FormatUint(uint64(session.NodeID), 10), opts)
	return elements, []*os.File{session.Fd}, session.Close, nil
}

// portalElements is the portal source, converted to the configured chroma and
// paced to the configured framerate.
//
// Portal capture is damage-driven: the compositor sends a frame only when the
// screen changes, and the PipeWire graph clock stops ticking while the captured
// node idles. Feeding the encoder straight from pipewiresrc therefore fails twice.
// On a static screen the encoder starves, so no keyframes reach the relay and
// viewers see black. And the first frame after an idle spell is stamped far ahead
// of the frozen clock, so a syncing sink waits on a clock that no longer advances,
// the SRT peer times out, and the relay drops the stream while the pipeline keeps
// running. (pipewiresrc's keepalive-time property covers only the first failure
// and still forwards the portal's timestamps, so it dies the same way on the first
// damage frame.)
//
// imagefreeze breaks both dependencies: allow-replace swaps in the newest damage
// frame, is-live repeats it at the capsfilter framerate, and the output carries
// imagefreeze's own monotonic timestamps, so the portal's clock domain never
// reaches the encoder. provide-clock=false keeps the freezing PipeWire clock from
// being elected pipeline clock; the system clock paces imagefreeze instead.
//
// The single-slot leaky queue keeps only the newest frame when a damage burst
// outruns videoconvert, which sits before imagefreeze so conversion runs once per
// damage frame, not once per output frame.
//
// The rate probe sits on imagefreeze's input, the last point where one buffer is
// one new picture. Everything downstream of imagefreeze runs at the configured
// framerate whatever the screen does, so a counter there would report the target
// back to the user as if it were a measurement. What the probe counts instead is
// the rate the shared screen actually changed at, which is the figure a viewer
// sees and the one a starved capture shows up in.
func portalElements(s settings.Stream, fd, node string, opts gstCaptureOptions) []string {
	elements := []string{
		"pipewiresrc", "fd=" + fd, "path=" + node, "provide-clock=false",
		"!", "queue", "max-size-buffers=1", "leaky=downstream",
		"!", "videoconvert",
		"!", opts.InCaps,
	}
	if len(opts.RateProbe) > 0 {
		elements = append(elements, "!")
		elements = append(elements, opts.RateProbe...)
	}
	return append(elements,
		"!", "imagefreeze", "is-live=true", "allow-replace=true",
		"!", "video/x-raw,framerate="+strconv.Itoa(s.Fps)+"/1",
	)
}

// ximageCapture is the X11 backend: ximagesrc reads the X screen over the shared
// memory extension, the GStreamer counterpart of the x11grab grabber.
//
// It paces itself, so no imagefreeze is involved: use-damage=false makes the
// element read the screen once per frame period whether or not anything changed,
// which is what gives the encoder the constant input the relay needs. The frames
// carry ximagesrc's own timestamps off the pipeline clock, so there is no foreign
// clock domain to escape either.
type ximageCapture struct{}

func (ximageCapture) Name() string { return "ximagesrc" }

func (x ximageCapture) Describe(s settings.Stream, opts gstCaptureOptions) []string {
	return x.elements(s, opts)
}

func (x ximageCapture) Open(s settings.Stream, opts gstCaptureOptions) ([]string, []*os.File, func(), error) {
	if os.Getenv("DISPLAY") == "" {
		return nil, nil, nil, fmt.Errorf("ximagesrc capture needs an X display: DISPLAY is unset")
	}
	return x.elements(s, opts), nil, func() {}, nil
}

// elements crops to the selected monitor when its geometry is known, the same
// rule x11grab follows: enumeration failing leaves no offset, so the whole X
// screen is captured instead of a guessed rectangle.
//
// ximagesrc's end coordinates are inclusive, so the last captured column is
// offset plus width minus one.
//
// The rate probe sits at the end of the chain, where it does for every backend.
// Nothing here repeats a frame, so what it counts is what ximagesrc read off the
// screen: the figure matches the configured framerate while the source keeps up,
// and falls below it when it does not.
func (ximageCapture) elements(s settings.Stream, opts gstCaptureOptions) []string {
	src := []string{"ximagesrc", "use-damage=false", "show-pointer=true"}
	if m, ok := monitorByIndex(s.Monitor); ok && m.Width > 0 && m.Height > 0 {
		src = append(src,
			"startx="+strconv.Itoa(m.OffsetX),
			"starty="+strconv.Itoa(m.OffsetY),
			"endx="+strconv.Itoa(m.OffsetX+m.Width-1),
			"endy="+strconv.Itoa(m.OffsetY+m.Height-1),
		)
	}
	src = append(src,
		"!", "video/x-raw,framerate="+strconv.Itoa(s.Fps)+"/1",
		"!", "videoconvert",
		"!", opts.InCaps,
	)
	if len(opts.RateProbe) > 0 {
		src = append(src, "!")
		src = append(src, opts.RateProbe...)
	}
	return src
}

// monitorByIndex returns the enumerated monitor with the settings index, and
// false when enumeration has no entry for it.
func monitorByIndex(idx int) (display.Monitor, bool) {
	for _, m := range display.List() {
		if m.Index == idx {
			return m, true
		}
	}
	return display.Monitor{}, false
}
