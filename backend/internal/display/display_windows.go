//go:build windows

package display

import (
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"bjoernblessin.de/go-utils/util/assert"
)

var (
	user32                   = windows.NewLazySystemDLL("user32.dll")
	procEnumDisplayMonitors  = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW      = user32.NewProc("GetMonitorInfoW")
	procEnumDisplaySettingsW = user32.NewProc("EnumDisplaySettingsW")
)

// monitorinfofPrimary is MONITORINFOF_PRIMARY, the MONITORINFO.dwFlags bit on the primary output.
const monitorinfofPrimary = 0x1

// enumCurrentSettings is ENUM_CURRENT_SETTINGS, -1 as a DWORD.
// EnumDisplaySettingsW then reports the mode the display runs on,
// rather than an entry from its mode table.
const enumCurrentSettings = 0xFFFFFFFF

const (
	cchDeviceName = 32
	cchFormName   = 32
)

type rect struct {
	left, top, right, bottom int32
}

// monitorInfoEx mirrors Win32 MONITORINFOEX.
// The trailing szDevice is the display name EnumDisplaySettingsW reads a mode off.
type monitorInfoEx struct {
	cbSize    uint32
	rcMonitor rect
	rcWork    rect
	dwFlags   uint32
	szDevice  [cchDeviceName]uint16
}

// devModeW mirrors Win32 DEVMODEW in its display-union variant.
// Field order and widths match it exactly,
// or dmDisplayFrequency is read off an offset Windows never wrote.
type devModeW struct {
	dmDeviceName         [cchDeviceName]uint16
	dmSpecVersion        uint16
	dmDriverVersion      uint16
	dmSize               uint16
	dmDriverExtra        uint16
	dmFields             uint32
	dmPositionX          int32
	dmPositionY          int32
	dmDisplayOrientation uint32
	dmDisplayFixedOutput uint32
	dmColor              int16
	dmDuplex             int16
	dmYResolution        int16
	dmTTOption           int16
	dmCollate            int16
	dmFormName           [cchFormName]uint16
	dmLogPixels          uint16
	dmBitsPerPel         uint32
	dmPelsWidth          uint32
	dmPelsHeight         uint32
	dmDisplayFlags       uint32
	dmDisplayFrequency   uint32
	dmICMMethod          uint32
	dmICMIntent          uint32
	dmMediaType          uint32
	dmDitherType         uint32
	dmReserved1          uint32
	dmReserved2          uint32
	dmPanningWidth       uint32
	dmPanningHeight      uint32
}

// refreshHz is the active refresh rate of the named display, in Hz.
// Windows spells "hardware default" as 0 or 1, and both become zero, the unknown sentinel.
func refreshHz(device *uint16) int {
	assert.IsNotNil(device, "a refresh rate is read off a named display")

	var dm devModeW
	dm.dmSize = uint16(unsafe.Sizeof(dm))
	ret, _, _ := procEnumDisplaySettingsW.Call(
		uintptr(unsafe.Pointer(device)),
		uintptr(enumCurrentSettings),
		uintptr(unsafe.Pointer(&dm)),
	)
	if ret == 0 || dm.dmDisplayFrequency <= 1 {
		return 0
	}
	return int(dm.dmDisplayFrequency)
}

// enumMu guards enumTarget across one enumeration.
// EnumDisplayMonitors runs the callback on the calling thread before it returns,
// so the lock spans the call and no reading of one enumeration reaches another.
var (
	enumMu     sync.Mutex
	enumTarget []Monitor
)

// enumMonitorProc collects one output per call into enumTarget.
//
// Registered once for the process, and it has to be:
// the runtime keeps every callback for the life of the program and dedups on the closure handed in,
// so a closure built per call matches none before it and takes a slot of its own.
// The table holds 2000 and passing it is a runtime throw rather than an error,
// which a form resolve reading the monitor list on every keystroke reaches well inside one session.
// Writing through a package variable leaves nothing per call to capture.
var enumMonitorProc = syscall.NewCallback(func(hMonitor, _, _, _ uintptr) uintptr {
	var mi monitorInfoEx
	mi.cbSize = uint32(unsafe.Sizeof(mi))
	ret, _, _ := procGetMonitorInfoW.Call(hMonitor, uintptr(unsafe.Pointer(&mi)))
	if ret != 0 {
		enumTarget = append(enumTarget, Monitor{
			Index:     len(enumTarget),
			Width:     int(mi.rcMonitor.right - mi.rcMonitor.left),
			Height:    int(mi.rcMonitor.bottom - mi.rcMonitor.top),
			OffsetX:   int(mi.rcMonitor.left),
			OffsetY:   int(mi.rcMonitor.top),
			Primary:   mi.dwFlags&monitorinfofPrimary != 0,
			RefreshHz: refreshHz(&mi.szDevice[0]),
		})
	}
	return 1 // non-zero continues the enumeration
})

// List enumerates the monitors through EnumDisplayMonitors, whose order Index counts in.
// The offsets are rcMonitor.
// Origin is the primary output's top-left corner,
// so an output placed left of or above it carries negative ones.
// Enumeration failing answers an empty slice, which a caller reads as an unknown resolution.
func List() []Monitor {
	enumMu.Lock()
	defer enumMu.Unlock()

	enumTarget = nil
	procEnumDisplayMonitors.Call(0, 0, enumMonitorProc, 0)
	monitors := enumTarget
	enumTarget = nil

	for i, m := range monitors {
		assert.Assert(m.Index == i, "an enumerated output is indexed in enumeration order", m.Index, i)
	}
	return monitors
}
