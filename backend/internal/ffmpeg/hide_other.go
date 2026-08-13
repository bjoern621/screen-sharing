//go:build !windows

package ffmpeg

import "os/exec"

// setHidden does nothing off Windows, where a child allocates no console window of its own.
func setHidden(cmd *exec.Cmd, hide bool) {}
