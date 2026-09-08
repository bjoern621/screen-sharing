//go:build windows

package window

import (
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                    = windows.NewLazySystemDLL("user32.dll")
	dwmapi                    = windows.NewLazySystemDLL("dwmapi.dll")
	procEnumWindows           = user32.NewProc("EnumWindows")
	procIsWindowVisible       = user32.NewProc("IsWindowVisible")
	procGetWindowTextW        = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW  = user32.NewProc("GetWindowTextLengthW")
	procGetWindowRect         = user32.NewProc("GetWindowRect")
	procGetWindowLongPtrW     = user32.NewProc("GetWindowLongPtrW")
	procGetAncestor           = user32.NewProc("GetAncestor")
	procDwmGetWindowAttribute = dwmapi.NewProc("DwmGetWindowAttribute")
)

const (
	// GWL_EXSTYLE, and the extended style bit on a window kept out of the task switcher.
	gwlExStyle     = -20
	wsExToolWindow = 0x00000080

	// GA_ROOTOWNER, which walks to the window the owner chain ends at.
	// A window that is its own root owner is the one the task switcher lists,
	// so a dialog and its parent contribute one entry rather than two.
	gaRootOwner = 3

	// DWMWA_CLOAKED, non-zero on a window the shell composes but never shows:
	// a suspended packaged app, or one on another virtual desktop.
	// Such a window is visible by IsWindowVisible and holds no picture to capture.
	dwmwaCloaked = 14
)

type rect struct {
	left, top, right, bottom int32
}

// enumMu guards enumTarget across one enumeration.
// EnumWindows runs the callback on the calling thread before it returns,
// so the lock spans the call and no reading of one enumeration reaches another.
var (
	enumMu     sync.Mutex
	enumTarget []Window
)

// enumWindowProc collects one capturable window per call into enumTarget.
//
// Registered once for the process, as display's monitor callback is:
// the runtime keeps every callback for the life of the program and dedups on the closure handed in,
// so a closure built per call takes a slot of its own out of a table holding 2000.
var enumWindowProc = syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
	w, ok := describe(hwnd)
	if ok {
		enumTarget = append(enumTarget, w)
	}
	return 1 // non-zero continues the enumeration
})

// List enumerates the windows a capture can read, in EnumWindows' own order,
// which runs front to back in z-order.
// Enumeration failing answers an empty slice, which a caller reads as nothing to pick.
func List() []Window {
	enumMu.Lock()
	defer enumMu.Unlock()

	enumTarget = nil
	procEnumWindows.Call(enumWindowProc, 0)
	windowsOpen := enumTarget
	enumTarget = nil
	return windowsOpen
}

// describe answers what one window is, and false where it is not one a user would pick.
//
// The filters are the task switcher's, which is the list a reader recognises:
// a window that is shown, titled, sized, owned by nothing and not cloaked by the shell.
// Everything else on the desktop is a tool window, a message sink or a hidden host,
// and offering those would bury the four windows somebody meant among a hundred they did not.
func describe(hwnd uintptr) (Window, bool) {
	if visible, _, _ := procIsWindowVisible.Call(hwnd); visible == 0 {
		return Window{}, false
	}
	if root, _, _ := procGetAncestor.Call(hwnd, gaRootOwner); root != hwnd {
		return Window{}, false
	}
	// Through a variable: the index is negative and sign-extends into uintptr at run time,
	// where a constant conversion of it does not compile.
	index := int32(gwlExStyle)
	if style, _, _ := procGetWindowLongPtrW.Call(hwnd, uintptr(index)); style&wsExToolWindow != 0 {
		return Window{}, false
	}
	if cloaked(hwnd) {
		return Window{}, false
	}

	title := titleOf(hwnd)
	if title == "" {
		return Window{}, false
	}

	var r rect
	if ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ok == 0 {
		return Window{}, false
	}
	width, height := int(r.right-r.left), int(r.bottom-r.top)
	if width <= 0 || height <= 0 {
		return Window{}, false
	}

	return Window{
		Handle: uint64(hwnd),
		Title:  title,
		App:    appOf(hwnd),
		Width:  width,
		Height: height,
	}, true
}

// cloaked reports whether the shell composes the window without ever showing it.
// A call that fails answers false: the attribute is the filter's, and a window that cannot be
// asked about is kept rather than dropped.
func cloaked(hwnd uintptr) bool {
	var value uint32
	ret, _, _ := procDwmGetWindowAttribute.Call(hwnd, dwmwaCloaked,
		uintptr(unsafe.Pointer(&value)), unsafe.Sizeof(value))
	return ret == 0 && value != 0
}

// titleOf reads the window's title bar.
// The length is asked for first, GetWindowTextW writing at most the buffer it is handed.
func titleOf(hwnd uintptr) string {
	length, _, _ := procGetWindowTextLengthW.Call(hwnd)
	if length == 0 {
		return ""
	}
	buf := make([]uint16, length+1)
	written, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:written])
}
