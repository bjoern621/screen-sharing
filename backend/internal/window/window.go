// Package window enumerates the machine's windows,
// so a capture source can be picked as an application with a title beside it
// rather than typed as a bare handle.
//
// A window is what one process put on screen and what another process may read.
// Which sessions answer that question at all is sessions below,
// and a session with no row enumerates nothing and says so.
package window

import (
	"sync"
	"time"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/text"
)

// recentFor is how long one enumeration answers for the machine.
// Long enough that a form resolving per keystroke costs one enumeration rather than one per
// character, short enough that a window opened while the picker is up is offered by the time
// the reader looks for it.
const recentFor = time.Second

var (
	recentMu   sync.Mutex
	recent     []Window
	recentTime time.Time
)

// Window is one enumerated window.
//
// Handle is what the platform addresses it by and what PublishSettings.share_window stores,
// unsigned because both platform handles are pointer-width and neither is ever compared for order.
// Zero Width and Height are a window whose size could not be read.
type Window struct {
	Handle uint64 `json:"handle"`
	Title  string `json:"title"`
	// Executable behind the window, without its directory or extension: "chrome", "code".
	// Empty where the process could not be read, which a caller shows by title alone.
	App    string `json:"app"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// Recent is List, enumerated again only once the last answer is older than recentFor.
//
// The capture path reads List: a pipeline opens the window as it stands,
// where a form offers the windows as they were within recentFor.
func Recent() []Window {
	recentMu.Lock()
	defer recentMu.Unlock()

	if recentTime.IsZero() || time.Since(recentTime) > recentFor {
		recent = List()
		recentTime = time.Now()
	}
	return recent
}

// At is the enumerated window carrying this handle,
// and false where the enumeration carries none:
// one closed since the setting was stored, or a session that enumerates nothing.
//
// Both are Umgebungsfehler, and what to do about them is the caller's:
// the form greys the stored handle and the capture refuses to open it.
func At(handle uint64) (Window, bool) {
	for _, w := range List() {
		if w.Handle == handle {
			return w, true
		}
	}
	return Window{}, false
}

// enumerators is the sessions that answer what windows are open,
// in the shape screensrc states platform applicability in:
// an operating system, and the Linux display server where that is not the whole answer.
//
// Windows leaves the display blank, having one windowing system.
// Linux and macOS have no row.
// A Wayland compositor exposes no window list to a client at all,
// X11 needs a connection this app opens for nothing else,
// and macOS reads the list behind a privacy grant the app would have to be given first.
// A screen is reachable on all three, so a session with no row here still shares.
var enumerators = []struct {
	os      string
	display string
}{
	{os: "windows"},
}

// Enumerates reports whether this session can list its windows.
func Enumerates(p platform.Info) bool {
	for _, e := range enumerators {
		if e.os != p.OS {
			continue
		}
		if e.display != "" && e.display != p.Display {
			continue
		}
		return true
	}
	return false
}

// Statement is why this session lists no windows, and nil where it lists them.
//
// One statement for the machine rather than one per window:
// what is missing belongs to the session, and every window open on it is out of reach the same way.
func Statement(p platform.Info) *screensharev1.Text {
	if Enumerates(p) {
		return nil
	}
	return text.Of(screensharev1.TextCode_TEXT_CODE_NO_WINDOW_ENUMERATION,
		text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_OS, p.OS),
		text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_DISPLAY, p.Display))
}
