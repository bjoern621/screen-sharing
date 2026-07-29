package publish

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/display"
	"bjoernblessin.de/screenshare/gpupath"
	"bjoernblessin.de/screenshare/portal"
	"bjoernblessin.de/screenshare/settings"
)

// A GStreamer capture backend is the head of the publish pipeline: the elements
// that produce raw frames, up to and including the conversion that pins the
// encoder input. Everything after that point is the same for all of them, which
// is why the seam sits there.
//
// The backends differ in more than an element name. The portal one performs a
// D-Bus handshake first and hands the child a descriptor; the rest open their
// source from the element alone and acquire nothing. gstCapture is the contract
// that covers both shapes, so the engine below runs either without naming one.

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
	// Memory is the resolved frame memory, gpupath.MemoryGpu or MemorySystem. A
	// backend reads it to pin its source: an import needs the source to negotiate
	// device memory, which plain video/x-raw caps would never ask for.
	Memory string
	// InCaps is the capsfilter pinning memory, chroma and colorimetry on the encoder
	// input. It is passed in rather than appended by the engine because where the
	// conversion sits differs per backend: a damage-driven source converts once per
	// damage frame, ahead of the element that paces the output.
	InCaps string
	// Feature is the caps feature InCaps carries, empty for system memory. A backend
	// that pins caps of its own downstream of InCaps repeats it, since a capsfilter
	// naming no feature pins system memory and would break the negotiation InCaps won.
	Feature string
	// Convert is the element converting captured frames into InCaps: videoconvert on
	// the CPU, or the encoder family's own post-processor on the GPU path.
	Convert string
	// RateProbe counts the frames the source really produced, empty for a
	// pipeline built without instrumentation. A backend places it at the last
	// point where one buffer is one new picture, so what it counts is the rate the
	// screen changed at and not the rate the encoder was paced to.
	RateProbe []string
}

// rateCaps returns the capsfilter pinning the output rate, in the memory the encoder
// input was negotiated in.
func (o gstCaptureOptions) rateCaps(fps int) string {
	return "video/x-raw" + o.Feature + ",framerate=" + strconv.Itoa(fps) + "/1"
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
	// HoldsOneDevice refuses a machine on which the frames this backend captures and
	// the surfaces the encoder reads cannot be the same GPU's. Only a backend with a
	// gpupath row is asked, and only for a run resolved onto the GPU path.
	HoldsOneDevice() error
}

// portalCapture is the xdg-desktop-portal ScreenCast backend: the compositor's
// own picker chooses what is shared, and the frames arrive over PipeWire.
type portalCapture struct{}

func (portalCapture) Name() string { return "portal" }

func (portalCapture) Describe(s settings.Stream, opts gstCaptureOptions) []string {
	return portalElements(s, fdPlaceholder, nodePlaceholder, opts)
}

func (p portalCapture) Open(s settings.Stream, opts gstCaptureOptions) ([]string, []*os.File, func(), error) {
	session, err := portal.Open(portal.Options{
		// Both source kinds are offered, and which one is shared is the picker's
		// answer rather than a setting here: the compositor owns the choice, and it is
		// the only side that knows which windows exist.
		Types:        portal.SourceMonitor | portal.SourceWindow,
		RestoreToken: settings.PortalToken(),
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("portal ScreenCast: %w", err)
	}
	// The token is the compositor's receipt for this consent, so it is stored on the
	// session that produced it and reused by the next one, which is what keeps the
	// picker from popping on every publish. A store that fails costs a picker and
	// nothing else, so it is reported rather than failing the publish it was acquired
	// for: the stream the user asked for is running, and the next one asks again.
	if err := settings.SavePortalToken(session.Restore); err != nil {
		logger.Warnf("portal consent not stored, the next capture will ask again: %v", err)
	}
	elements := portalElements(s, strconv.Itoa(childFdBase), strconv.FormatUint(uint64(session.NodeID), 10), opts)
	return elements, []*os.File{session.Fd}, session.Close, nil
}

// HoldsOneDevice refuses a machine with more than one render node.
//
// The portal names no device: the compositor renders where it renders, and the
// PipeWire node carries frames without saying which GPU allocated them. The va
// elements bind to the first VA device the plugin finds. The two are the same GPU
// exactly when the machine has one, so that is the condition this holds, and a
// machine with several is refused with them named instead of importing frames across
// a device boundary and failing in negotiation.
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

// renderNodes lists the /dev/dri render nodes, the unprivileged half of each DRM
// device and what a VA driver opens. Order is lexical, which is the order the va
// plugin enumerates devices in.
func renderNodes() []string {
	nodes, err := filepath.Glob("/dev/dri/renderD[0-9]*")
	if err != nil {
		return nil
	}
	return nodes
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
// outruns the conversion, which sits before imagefreeze so conversion runs once per
// damage frame, not once per output frame.
//
// The rate probe sits on imagefreeze's input, the last point where one buffer is
// one new picture. Everything downstream of imagefreeze runs at the configured
// framerate whatever the screen does, so a counter there would report the target
// back to the user as if it were a measurement. What the probe counts instead is
// the rate the shared screen actually changed at, which is the figure a viewer
// sees and the one a starved capture shows up in.
//
// On the GPU path the source is pinned to DMABuf. pipewiresrc negotiates the
// compositor's dmabuf export only when the caps ask for it, and unpinned it settles
// on the MemPtr buffers PipeWire copies into shared memory, which is the round trip
// the path exists to avoid. Pinning it also makes a compositor that exports no
// dmabuf fail in negotiation rather than quietly deliver copies.
func portalElements(s settings.Stream, fd, node string, opts gstCaptureOptions) []string {
	elements := []string{"pipewiresrc", "fd=" + fd, "path=" + node, "provide-clock=false"}
	if opts.Memory == gpupath.MemoryGpu {
		elements = append(elements, "!", "video/x-raw(memory:DMABuf)")
	}
	elements = append(elements,
		"!", "queue", "max-size-buffers=1", "leaky=downstream",
		"!", opts.Convert,
		"!", opts.InCaps,
	)
	if len(opts.RateProbe) > 0 {
		elements = append(elements, "!")
		elements = append(elements, opts.RateProbe...)
	}
	return append(elements,
		"!", "imagefreeze", "is-live=true", "allow-replace=true",
		"!", opts.rateCaps(s.Fps),
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
//
// The element reads the screen into a shared-memory segment and hands on system
// memory, which is why it carries no gpupath row: there is no device frame for an
// encoder to import, whatever it can read.
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

func (ximageCapture) HoldsOneDevice() error {
	assert.Never("the X11 backend has no GPU path, so no run holds its devices against one")
	return nil
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
		"!", opts.Convert,
		"!", opts.InCaps,
	)
	if len(opts.RateProbe) > 0 {
		src = append(src, "!")
		src = append(src, opts.RateProbe...)
	}
	return src
}

// avfCapture is the macOS backend: avfvideosrc reads a screen through
// AVFoundation, the GStreamer counterpart of the avfoundation grabber.
//
// capture-screen turns the element from a camera source into a screen one, and
// capture-screen-cursor draws the pointer into the frames, which is what
// -capture_cursor does on the ffmpeg side. Which screen it reads is the element's
// own device choice, so the monitor setting does not reach it.
//
// The chain is ximageCapture's. AVFoundation paces a screen input by a frame
// duration rather than by damage, so the framerate capsfilter on the source is
// what the encoder is fed at and no imagefreeze is involved. That follows from
// the element's properties rather than from a run, since macOS is not available
// on the development machine.
//
// It carries no gpupath row. A row needs its engine half in gstGpuMemories, which
// names the va family alone and so holds nothing a macOS pair could claim, and a
// row without its half is what the pipeline builder asserts on.
type avfCapture struct{}

func (avfCapture) Name() string { return "avfvideosrc" }

func (a avfCapture) Describe(s settings.Stream, opts gstCaptureOptions) []string {
	return a.elements(s, opts)
}

func (a avfCapture) Open(s settings.Stream, opts gstCaptureOptions) ([]string, []*os.File, func(), error) {
	return a.elements(s, opts), nil, func() {}, nil
}

func (avfCapture) HoldsOneDevice() error {
	assert.Never("the macOS backend has no GPU path, so no run holds its devices against one")
	return nil
}

// elements places the rate probe at the end of the chain, where nothing has
// repeated a frame yet, so what it counts is what AVFoundation delivered.
func (avfCapture) elements(s settings.Stream, opts gstCaptureOptions) []string {
	src := []string{"avfvideosrc", "capture-screen=true", "capture-screen-cursor=true",
		"!", "video/x-raw,framerate=" + strconv.Itoa(s.Fps) + "/1",
		"!", opts.Convert,
		"!", opts.InCaps,
	}
	if len(opts.RateProbe) > 0 {
		src = append(src, "!")
		src = append(src, opts.RateProbe...)
	}
	return src
}

// d3d11Capture is the Windows backend: d3d11screencapturesrc reads a monitor
// through Desktop Duplication, the GStreamer counterpart of the ddagrab grabber.
//
// monitor-index selects the output and show-cursor draws the pointer into the
// frames. The settings index reaches the element without a lookup, the way
// ddagrab's output_idx does: both name a monitor in the Windows enumeration the
// index already stands for.
//
// The chain is ximageCapture's: the framerate capsfilter on the source is what
// paces the encoder and nothing downstream repeats a frame, so no imagefreeze is
// involved. The element runs on Windows only, so that follows from its properties
// rather than from a run here.
//
// It carries no gpupath row. Desktop Duplication hands out a Direct3D texture,
// but gstGpuMemories names the va family alone, so no encoder family this source
// could pair with has an engine half, and a row without its half is what the
// pipeline builder asserts on.
type d3d11Capture struct{}

func (d3d11Capture) Name() string { return "d3d11screencapturesrc" }

func (d d3d11Capture) Describe(s settings.Stream, opts gstCaptureOptions) []string {
	return d.elements(s, opts)
}

func (d d3d11Capture) Open(s settings.Stream, opts gstCaptureOptions) ([]string, []*os.File, func(), error) {
	return d.elements(s, opts), nil, func() {}, nil
}

func (d3d11Capture) HoldsOneDevice() error {
	assert.Never("the Windows Direct3D backend has no GPU path, so no run holds its devices against one")
	return nil
}

// elements places the rate probe at the end of the chain, where nothing has
// repeated a frame yet, so what it counts is what Desktop Duplication delivered.
func (d3d11Capture) elements(s settings.Stream, opts gstCaptureOptions) []string {
	src := []string{"d3d11screencapturesrc", "show-cursor=true", "monitor-index=" + strconv.Itoa(s.Monitor),
		"!", "video/x-raw,framerate=" + strconv.Itoa(s.Fps) + "/1",
		"!", opts.Convert,
		"!", opts.InCaps,
	}
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
