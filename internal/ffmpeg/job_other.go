//go:build !windows

package ffmpeg

import "os/exec"

// KillOnAppExit does nothing off Windows, the Job Object it uses there (job_windows.go) having no
// equivalent: App.shutdown is the only thing that stops the children, and an app killed outright
// leaves them running.
//
// prctl(PR_SET_PDEATHSIG) is the nearest Linux mechanism and is deliberately unused.
// It fires when the thread that spawned the child exits rather than the process, and the Go runtime
// retires threads whenever it likes, so it would kill live streams at moments nothing here chose.
func KillOnAppExit(cmd *exec.Cmd) {}
