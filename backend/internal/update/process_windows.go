package update

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// alive answers whether that process is still running.
//
// A handle that opens and a wait that times out is a process still there.
// A pid nothing answers for has exited, and its handle cannot be opened at all.
func alive(pid int) bool {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	event, err := windows.WaitForSingleObject(handle, 0)
	return err == nil && event == uint32(windows.WAIT_TIMEOUT)
}

// detachedProcess is Windows' DETACHED_PROCESS: the child inherits no console,
// so it outlives the one this process was started from.
const detachedProcess = 0x00000008

// detach frees the child from this process's console and process group,
// so the applier survives the app it is replacing.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: detachedProcess | windows.CREATE_NEW_PROCESS_GROUP,
	}
}
