package ffmpeg

import (
	"os/exec"
	"syscall"
)

// createNoWindow is CREATE_NO_WINDOW: the child runs without allocating a
// console, so no black window flashes up beside the encoder.
const createNoWindow = 0x08000000

// setHidden suppresses the child's console window. It must not be used for
// ffplay, whose video window is a normal top-level window and would vanish too.
func setHidden(cmd *exec.Cmd, hide bool) {
	if !hide {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
