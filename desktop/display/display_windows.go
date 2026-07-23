//go:build windows

package display

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procEnumDisplayMonitors = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW     = user32.NewProc("GetMonitorInfoW")
)

// monitorinfofPrimary is the MONITORINFO.dwFlags bit marking the primary output.
const monitorinfofPrimary = 0x1

type rect struct {
	left, top, right, bottom int32
}

// monitorInfo mirrors the Win32 MONITORINFO struct.
type monitorInfo struct {
	cbSize    uint32
	rcMonitor rect
	rcWork    rect
	dwFlags   uint32
}

// List enumerates the display monitors in EnumDisplayMonitors order, each with
// its pixel size. Returns an empty slice if enumeration fails; callers treat
// that as "resolution unknown".
func List() []Monitor {
	var monitors []Monitor

	callback := syscall.NewCallback(func(hMonitor, _, _, _ uintptr) uintptr {
		var mi monitorInfo
		mi.cbSize = uint32(unsafe.Sizeof(mi))
		ret, _, _ := procGetMonitorInfoW.Call(hMonitor, uintptr(unsafe.Pointer(&mi)))
		if ret != 0 {
			monitors = append(monitors, Monitor{
				Index:   len(monitors),
				Width:   int(mi.rcMonitor.right - mi.rcMonitor.left),
				Height:  int(mi.rcMonitor.bottom - mi.rcMonitor.top),
				Primary: mi.dwFlags&monitorinfofPrimary != 0,
			})
		}
		return 1 // non-zero: continue enumeration
	})

	procEnumDisplayMonitors.Call(0, 0, callback, 0)
	return monitors
}
