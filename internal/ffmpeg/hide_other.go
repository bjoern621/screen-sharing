//go:build !windows

package ffmpeg

import "os/exec"

// setHidden is a no-op off Windows: there is no separate console window to hide.
func setHidden(cmd *exec.Cmd, hide bool) {}
