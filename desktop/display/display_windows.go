//go:build windows

package display

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                   = windows.NewLazySystemDLL("user32.dll")
	procEnumDisplayMonitors  = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW      = user32.NewProc("GetMonitorInfoW")
	procEnumDisplaySettingsW = user32.NewProc("EnumDisplaySettingsW")
)

// monitorinfofPrimary is the MONITORINFO.dwFlags bit marking the primary output.
const monitorinfofPrimary = 0x1

// enumCurrentSettings (ENUM_CURRENT_SETTINGS, -1 as DWORD) asks EnumDisplaySettingsW
// for the mode the display is running now rather than a mode-table entry.
const enumCurrentSettings = 0xFFFFFFFF

const (
	cchDeviceName = 32
	cchFormName   = 32
)

type rect struct {
	left, top, right, bottom int32
}

// monitorInfoEx mirrors the Win32 MONITORINFOEX struct. The trailing szDevice
// names the display, which EnumDisplaySettingsW needs to read its current mode.
type monitorInfoEx struct {
	cbSize    uint32
	rcMonitor rect
	rcWork    rect
	dwFlags   uint32
	szDevice  [cchDeviceName]uint16
}

// devModeW mirrors the Win32 DEVMODEW struct for the display union variant. Field
// order and widths must match exactly so dmDisplayFrequency lands at the offset
// Windows fills.
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

// refreshHz reads the current refresh rate of the named display. Windows reports
// 0 or 1 for "hardware default"; both map to 0 (unknown).
func refreshHz(device *uint16) int {
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

// List enumerates the display monitors in EnumDisplayMonitors order, each with
// its pixel size and current refresh rate. Returns an empty slice if enumeration
// fails; callers treat that as "resolution unknown".
func List() []Monitor {
	var monitors []Monitor

	callback := syscall.NewCallback(func(hMonitor, _, _, _ uintptr) uintptr {
		var mi monitorInfoEx
		mi.cbSize = uint32(unsafe.Sizeof(mi))
		ret, _, _ := procGetMonitorInfoW.Call(hMonitor, uintptr(unsafe.Pointer(&mi)))
		if ret != 0 {
			monitors = append(monitors, Monitor{
				Index:     len(monitors),
				Width:     int(mi.rcMonitor.right - mi.rcMonitor.left),
				Height:    int(mi.rcMonitor.bottom - mi.rcMonitor.top),
				Primary:   mi.dwFlags&monitorinfofPrimary != 0,
				RefreshHz: refreshHz(&mi.szDevice[0]),
			})
		}
		return 1 // non-zero: continue enumeration
	})

	procEnumDisplayMonitors.Call(0, 0, callback, 0)
	return monitors
}
