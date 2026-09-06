//go:build !windows

package update

import (
	"os"
	"os/exec"
	"syscall"
)

// alive answers whether that process is still running.
// Signal zero delivers nothing and reports whether the process is there to deliver to.
// A process somebody else owns answers EPERM, which is a process that exists.
func alive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = process.Signal(syscall.Signal(0))
	return err == nil || err == syscall.EPERM
}

// detach puts the child in a session of its own,
// so the applier survives the process group this one is stopped with.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
