package publish

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/cursor"
	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/portal"
	"bjoernblessin.de/screenshare/internal/screensrc"
	"bjoernblessin.de/screenshare/internal/settings"
)

// A GStreamer capture backend is the head of the publish pipeline: the elements producing raw
// frames, up to and including the conversion that pins the encoder input.
// The seam sits there because everything downstream of it is the same for all of them.
//
// The backends differ in more than an element name.
// The portal one performs a D-Bus handshake first and hands the child a descriptor; the rest open
// their source from the element alone and acquire nothing.
// gstCapture covers both shapes, so the engine below drives either without naming one.

// childFdBase is the descriptor number ExtraFiles[0] is inherited as.
// A backend that passes files gets the first slots, so the numbers it writes into its elements
// start here.
const childFdBase = 3

// The placeholders a displayed command carries where a run passes a value that exists only once the
// source is open.
const (
	fdPlaceholder   = "<portal-fd>"
	nodePlaceholder = "<portal-node>"
)

// gstCaptureOptions are the parts of the source chain that belong to the run rather than to the
// backend.
type gstCaptureOptions struct {
	// Memory is the resolved frame memory, read through gpupath.OnDevice to pin the source:
	// an import needs the source to negotiate device memory, which plain video/x-raw caps never ask
	// for.
	Memory string
	// InCaps pins memory, chroma and colorimetry on the encoder input.
	// Passed in rather than appended by the engine, because where the conversion sits differs per
	// backend: a damage-driven source converts once per damage frame, ahead of the element pacing the
	// output.
	InCaps string
	// Feature is the caps feature InCaps carries, empty for system memory.
	// A backend pinning caps of its own downstream of InCaps repeats it: a capsfilter naming no
	// feature pins system memory and breaks the negotiation InCaps won.
	Feature string
	// Convert turns captured frames into InCaps: videoconvert on the CPU, preceded by videoscale where
	// the run scales, or the encoder family's own post-processor on the GPU path.
	// Backends place it through convertInto rather than naming it, so what it holds stays gstgpu.go's
	// business.
	Convert []string
	// RateProbe counts the frames the source really produced, empty for a pipeline built without
	// instrumentation.
	// A backend places it at the last point where one buffer is one new picture, so it counts the rate
	// the screen changed at rather than the rate the encoder was paced to.
	RateProbe []string
}

// convertInto is the conversion chain and the capsfilter it converts into, with the links between
// them and the one attaching it to what came before.
//
// Every backend places the pair through here, so a chain that grows an element changes once.
// The links belong to the answer because the chain's length does: a caller writing its own "!"
// would have to know how many.
func (o gstCaptureOptions) convertInto() []string {
	assert.Assert(o.InCaps != "", "a conversion chain names the caps it converts into")

	out := make([]string, 0, len(o.Convert)+3)
	out = append(out, "!")
	out = append(out, o.Convert...)
	return append(out, "!", o.InCaps)
}

// rateCaps pins a framerate, in the memory the encoder input was negotiated in.
//
// Every capture chain pins its rate through here, wherever in the chain it does so.
// Plain video/x-raw means system memory and nothing else, so a rate capsfilter without the feature
// pins a device path's frames back into the round trip that path exists to avoid, and negotiation
// fails against a source offering device memory alone.
func (o gstCaptureOptions) rateCaps(fps int) string {
	// The pipeline builders refuse a rate at or below zero before this is reached (gstpipeline.go),
	// so one arriving here is a caller that skipped them rather than a stored setting nobody repaired.
	assert.Assert(fps > 0, "a pinned rate is a rate", fps)

	return "video/x-raw" + o.Feature + ",framerate=" + strconv.Itoa(fps) + "/1"
}

// gstCapture is one screen source the GStreamer publish engine can drive.
type gstCapture interface {
	// Name is the settings value and the key in captureBackends.
	Name() string
	// Describe returns the source elements for the rendered command, with a placeholder wherever Open
	// substitutes a value the handshake produces.
	Describe(s settings.Settings, opts gstCaptureOptions) []string
	// Open acquires the source and returns the source elements a run plays, the files the child
	// inherits from childFdBase up, and the teardown that runs when the child exits.
	// A backend acquiring nothing returns no files and a no-op teardown.
	Open(s settings.Settings, opts gstCaptureOptions) (elements []string, files []*os.File, closeFn func(), err error)
	// HoldsOneDevice refuses a machine where the frames this backend captures and the surfaces the
	// encoder reads cannot be one GPU's.
	// Asked only of a backend with a gpupath row, and only for a run resolved onto the GPU path.
	HoldsOneDevice() error
}

// portalCapture is the xdg-desktop-portal ScreenCast backend: the compositor's own picker chooses
// what is shared and the frames arrive over PipeWire.
//
// hold carries the granted consent from one child to the next, so the picker is answered once per
// stream rather than once per launch (portal.Hold).
// The registry supplies it, and Describe needs none: rendering a command acquires nothing.
type portalCapture struct{ hold *portal.Hold }

func (portalCapture) Name() string { return "portal" }

func (portalCapture) Describe(s settings.Settings, opts gstCaptureOptions) []string {
	return portalElements(s, fdPlaceholder, nodePlaceholder, opts)
}

func (p portalCapture) Open(s settings.Settings, opts gstCaptureOptions) ([]string, []*os.File, func(), error) {
	assert.IsNotNil(p.hold, "a portal capture opens against the hold the registry gave it")

	session, err := p.hold.Session(portal.Options{
		// Both kinds are offered and the picker answers which one is shared, rather than a setting here:
		// the compositor owns the choice and is the only side that knows which windows exist.
		Types:        portal.SourceMonitor | portal.SourceWindow,
		Cursor:       portalCursor(s.Publish.Cursor),
		RestoreToken: settings.PortalToken(),
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("portal ScreenCast: %w", err)
	}
	// The token is the compositor's receipt for this consent, stored off the session that produced it
	// and reused by the next stream, which keeps the picker from popping on every publish where the
	// compositor persists consents at all.
	// A store that fails costs a picker and nothing else, so it is warned about rather than failing
	// the publish it was acquired for: this stream runs, and the next one asks again.
	if err := settings.SavePortalToken(session.Restore); err != nil {
		logger.Warnf("portal consent not stored, the next capture will ask again: %v", err)
	}

	remote, err := session.Remote()
	if err != nil {
		// The consent stands and the connection to it does not, which is a session this app can no longer
		// hand a child.
		// Dropping it sends the relaunch to a session that can carry one instead of to the same failure.
		p.hold.Release()
		return nil, nil, nil, fmt.Errorf("portal ScreenCast: %w", err)
	}
	elements := portalElements(s, strconv.Itoa(childFdBase), strconv.FormatUint(uint64(session.NodeID), 10), opts)
	// The remote alone, since the session belongs to the stream rather than to this child.
	return elements, []*os.File{remote}, func() { remote.Close() }, nil
}

// Release drops the held consent, so the next capture pops the compositor's picker.
func (p portalCapture) Release() {
	assert.IsNotNil(p.hold, "a portal capture releases the hold the registry gave it")

	p.hold.Release()
}

// HoldsOneDevice refuses a machine with more than one render node.
//
// The portal names no device: the compositor renders where it renders, and the PipeWire node
// carries frames without saying which GPU allocated them.
// The va elements bind to the first VA device the plugin finds.
// The two are one GPU exactly when the machine has one, so a machine with several is refused with
// them named rather than importing frames across a device boundary and failing in negotiation.
func (portalCapture) HoldsOneDevice() error {
	nodes := renderNodes()
	if len(nodes) == 1 {
		return nil
	}
	if len(nodes) == 0 {
		return fmt.Errorf("no render node under /dev/dri: this machine has no GPU the va elements can import the portal's frames into")
	}
	return gpupath.Undetermined("the portal does not name the GPU it captures on", nodes)
}

// renderNodes lists the /dev/dri render nodes: the unprivileged half of each DRM device, and what a
// VA driver opens.
// Lexical order, which is the order the va plugin enumerates devices in.
func renderNodes() []string {
	nodes, err := filepath.Glob("/dev/dri/renderD[0-9]*")
	if err != nil {
		return nil
	}
	return nodes
}

// portalElements is the portal source, converted to the configured chroma and paced to the
// configured framerate.
//
// Portal capture is damage-driven: the compositor sends a frame only when the screen changes, and
// the PipeWire graph clock stops ticking while the captured node idles.
// Feeding the encoder straight from pipewiresrc therefore fails twice.
// A static screen starves the encoder, so no keyframe reaches the relay and viewers see black.
// The first frame after an idle spell is stamped far ahead of the frozen clock, so a syncing sink
// waits on a clock that no longer advances, the SRT peer times out, and the relay drops the stream
// while the pipeline keeps running.
// pipewiresrc's keepalive-time property covers the first failure alone and still forwards the
// portal's timestamps, so it dies the same way on the first damage frame.
//
// imagefreeze breaks both dependencies: allow-replace swaps in the newest damage frame, is-live
// repeats it at the capsfilter framerate, and the output carries imagefreeze's own monotonic
// timestamps, so the portal's clock domain never reaches the encoder.
// provide-clock=false keeps the freezing PipeWire clock from being elected pipeline clock, and the
// system clock paces imagefreeze instead.
//
// The single-slot leaky queue drops all but the newest frame when a damage burst outruns the
// conversion, which sits before imagefreeze so a conversion runs once per damage frame rather than
// once per output frame.
//
// The rate probe sits on imagefreeze's input, the last point where one buffer is one new picture.
// Everything downstream runs at the configured framerate whatever the screen does, so a counter
// there would report the target back as a measurement.
// The probe counts the rate the shared screen changed at, which is the figure a viewer sees and the
// one a starved capture shows up in.
//
// On the GPU path the source is pinned to DMABuf.
// pipewiresrc negotiates the compositor's dmabuf export only when the caps ask for it, and unpinned
// it settles on the MemPtr buffers PipeWire copies into shared memory, the round trip the path
// exists to avoid.
// Pinning also makes a compositor that exports no dmabuf fail in negotiation rather than quietly
// deliver copies.
func portalElements(s settings.Settings, fd, node string, opts gstCaptureOptions) []string {
	// Each is either the placeholder a rendered command shows or the value the handshake produced,
	// so an empty one is a caller that built neither.
	assert.Assert(fd != "", "a portal source names the descriptor it reads frames on")
	assert.Assert(node != "", "a portal source names the node the compositor gave it")

	elements := []string{"pipewiresrc", "fd=" + fd, "path=" + node, "provide-clock=false"}
	if gpupath.OnDevice(opts.Memory) {
		elements = append(elements, "!", "video/x-raw(memory:DMABuf)")
	}
	elements = append(elements, "!", "queue", "max-size-buffers=1", "leaky=downstream")
	elements = append(elements, opts.convertInto()...)
	if len(opts.RateProbe) > 0 {
		elements = append(elements, "!")
		elements = append(elements, opts.RateProbe...)
	}
	return append(elements,
		"!", "imagefreeze", "is-live=true", "allow-replace=true",
		"!", opts.rateCaps(s.Publish.Fps),
	)
}

// ximageCapture is the X11 backend: ximagesrc reads the X screen over the shared memory extension,
// the GStreamer counterpart of the x11grab grabber.
//
// It paces itself, so no imagefreeze is involved: use-damage=false reads the screen once per frame
// period whether or not anything changed, which is the constant input the relay needs.
// The frames carry ximagesrc's own timestamps off the pipeline clock, so no foreign clock domain
// has to be escaped either.
//
// The element reads into a shared-memory segment and hands on system memory, which is why it
// carries no gpupath row: there is no device frame for an encoder to import, whatever it can read.
type ximageCapture struct{}

func (ximageCapture) Name() string { return "ximagesrc" }

func (x ximageCapture) Describe(s settings.Settings, opts gstCaptureOptions) []string {
	return x.elements(s, opts)
}

func (x ximageCapture) Open(s settings.Settings, opts gstCaptureOptions) ([]string, []*os.File, func(), error) {
	if os.Getenv("DISPLAY") == "" {
		return nil, nil, nil, fmt.Errorf("ximagesrc capture needs an X display: DISPLAY is unset")
	}
	// An index no output answers to is refused, as x11grab refuses it (internal/ffmpeg, x11grabArgs).
	// The crop comes off the enumeration, and a head built without one captures the whole X screen: a
	// machine that cannot measure its outputs has that as its only honest answer, but a settings file
	// naming a monitor that was unplugged does not, and publishing every remaining screen to whoever is
	// watching is the wrong way to be wrong about it.
	// The form keeps such a selection on the list on purpose, so this is the leg that has to say no
	// (internal/form, optionMonitors).
	if _, ok := display.At(s.Publish.Monitor); !ok {
		return nil, nil, nil, fmt.Errorf("monitor %d is not one of this machine's outputs", s.Publish.Monitor)
	}
	return x.elements(s, opts), nil, func() {}, nil
}

func (ximageCapture) HoldsOneDevice() error {
	assert.Never("the X11 backend has no GPU path, so no run holds its devices against one")
	return nil
}

// elements reads the head from screensrc, where the crop to the selected monitor is written.
// The wizard's monitor preview builds from the same table, so the picture a screen is offered by is
// the rectangle this stream carries (internal/screensrc).
//
// The rate probe sits at the end of the chain, as it does on every backend.
// Nothing here repeats a frame, so it counts what ximagesrc read off the screen: the configured
// framerate while the source keeps up, and less where it does not.
func (x ximageCapture) elements(s settings.Settings, opts gstCaptureOptions) []string {
	src := screensrc.Head(x.Name(), s.Publish.Monitor, s.Publish.Cursor == cursor.Embedded)
	src = append(src, "!", opts.rateCaps(s.Publish.Fps))
	src = append(src, opts.convertInto()...)
	if len(opts.RateProbe) > 0 {
		src = append(src, "!")
		src = append(src, opts.RateProbe...)
	}
	return src
}

// avfCapture is the macOS backend: avfvideosrc reads a screen through AVFoundation, the GStreamer
// counterpart of the avfoundation grabber.
//
// capture-screen turns the element from a camera source into a screen one, and
// capture-screen-cursor draws the pointer into the frames, which is -capture_cursor on the ffmpeg
// side.
// Which screen it reads is the element's own device choice, so the monitor setting does not reach
// it.
//
// The chain is ximageCapture's.
// AVFoundation paces a screen input by a frame duration rather than by damage, so the framerate
// capsfilter on the source is what the encoder is fed at and no imagefreeze is involved.
// That follows from the element's properties rather than from a run, macOS being unavailable on the
// development machine.
//
// It carries no gpupath row.
// A row needs its engine half in gstGpuMemories, which names the va family, a Linux interface, and
// the nvcodec one, which reaches the device through Direct3D 11.
// Neither holds anything a macOS pair could claim, and a row missing its half is what the pipeline
// builder asserts on.
type avfCapture struct{}

func (avfCapture) Name() string { return "avfvideosrc" }

func (a avfCapture) Describe(s settings.Settings, opts gstCaptureOptions) []string {
	return a.elements(s, opts)
}

func (a avfCapture) Open(s settings.Settings, opts gstCaptureOptions) ([]string, []*os.File, func(), error) {
	return a.elements(s, opts), nil, func() {}, nil
}

func (avfCapture) HoldsOneDevice() error {
	assert.Never("the macOS backend has no GPU path, so no run holds its devices against one")
	return nil
}

// elements places the rate probe at the end of the chain, where nothing has repeated a frame yet,
// so it counts what AVFoundation delivered.
func (avfCapture) elements(s settings.Settings, opts gstCaptureOptions) []string {
	src := []string{"avfvideosrc", "capture-screen=true",
		"capture-screen-cursor=" + gstBool(s.Publish.Cursor == cursor.Embedded),
		"!", opts.rateCaps(s.Publish.Fps),
	}
	src = append(src, opts.convertInto()...)
	if len(opts.RateProbe) > 0 {
		src = append(src, "!")
		src = append(src, opts.RateProbe...)
	}
	return src
}

// d3d11Capture is the Windows backend: d3d11screencapturesrc reads a monitor through Desktop
// Duplication, the GStreamer counterpart of the ddagrab grabber.
//
// The element and its monitor selection come from screensrc, so the wizard's preview of a screen
// and the stream of that screen are one output.
//
// The chain is ximageCapture's: the framerate capsfilter on the source paces the encoder and
// nothing downstream repeats a frame, so no imagefreeze is involved.
// The element runs on Windows alone, so that follows from its properties rather than from a run
// here.
type d3d11Capture struct{}

func (d3d11Capture) Name() string { return "d3d11screencapturesrc" }

func (d d3d11Capture) Describe(s settings.Settings, opts gstCaptureOptions) []string {
	return d.elements(s, opts)
}

func (d d3d11Capture) Open(s settings.Settings, opts gstCaptureOptions) ([]string, []*os.File, func(), error) {
	return d.elements(s, opts), nil, func() {}, nil
}

// HoldsOneDevice refuses nothing: the condition holds on every machine this backend runs on.
//
// It is why the ddagrab rows need no check either.
// The auto-GPU encoder element takes its adapter from the frames it is handed rather than opening a
// device of its own, so the encode runs on the GPU that allocated the Desktop Duplication texture
// whatever else the machine carries.
// A second card is then a monitor on that card, and capturing it moves the encode along with it.
func (d3d11Capture) HoldsOneDevice() error {
	return nil
}

// elements places the rate probe at the end of the chain, where nothing has repeated a frame yet,
// so it counts what Desktop Duplication delivered.
//
// The rate capsfilter carries the encoder input's memory feature, which the source already
// produces: Desktop Duplication hands out a Direct3D 11 texture, and that is the memory the nvcodec
// device path converts and encodes in (gstGpuMemories).
// Pinning it keeps the frames on the device, plain video/x-raw between the two being system memory
// and nothing else.
func (d d3d11Capture) elements(s settings.Settings, opts gstCaptureOptions) []string {
	src := screensrc.Head(d.Name(), s.Publish.Monitor, s.Publish.Cursor == cursor.Embedded)
	src = append(src, "!", opts.rateCaps(s.Publish.Fps))
	src = append(src, opts.convertInto()...)
	if len(opts.RateProbe) > 0 {
		src = append(src, "!")
		src = append(src, opts.RateProbe...)
	}
	return src
}
