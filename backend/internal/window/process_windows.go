//go:build windows

package window

import (
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")

// maxPath is the buffer QueryFullProcessImageNameW writes a path into.
// Long paths reach beyond it, and a truncated read leaves the app unnamed,
// which the surface shows by title alone.
const maxPath = 32768

// appOf is the executable behind the window, without its directory or its extension:
// "chrome", "code".
//
// Empty wherever the process cannot be read.
// A window of a process running at a higher integrity level than this one refuses the open,
// and that is an ordinary state on a desktop rather than a failure to report:
// the window is still capturable and still has a title.
func appOf(hwnd uintptr) string {
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return ""
	}

	proc, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(proc)

	buf := make([]uint16, maxPath)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(proc, 0, &buf[0], &size); err != nil {
		return ""
	}

	base := filepath.Base(windows.UTF16ToString(buf[:size]))
	return strings.TrimSuffix(base, filepath.Ext(base))
}
