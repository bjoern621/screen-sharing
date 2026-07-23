package ffmpeg

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32         = windows.NewLazySystemDLL("user32.dll")
	procFindWindow = user32.NewProc("FindWindowW")
)

// WindowExists reports whether a top-level window with the exact title exists.
// The zero class-name argument matches any class, so only the title matters.
func WindowExists(title string) bool {
	ptr, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return false
	}
	hwnd, _, _ := procFindWindow.Call(0, uintptr(unsafe.Pointer(ptr)))
	return hwnd != 0
}
