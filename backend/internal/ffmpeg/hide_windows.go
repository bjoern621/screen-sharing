package ffmpeg

import (
	"os/exec"
	"syscall"
)

// createNoWindow is Windows' CREATE_NO_WINDOW: the child allocates no console, so no black window
// flashes up beside the encoder.
const createNoWindow = 0x08000000

// setHidden suppresses the child's console window.
// Never for ffplay: its video window is an ordinary top-level window and would go with it.
func setHidden(cmd *exec.Cmd, hide bool) {
	if !hide {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
