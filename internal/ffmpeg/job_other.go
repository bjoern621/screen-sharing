//go:build !windows

package ffmpeg

import "os/exec"

// KillOnAppExit is a no-op off Windows, where the Job Object it uses there (job_windows.go) has no
// equivalent: App.shutdown is the only thing that stops the children, and an app killed outright
// leaves them running.
//
// prctl(PR_SET_PDEATHSIG) is the nearest Linux mechanism and is deliberately not used.
// It fires when the thread that spawned the child exits, not the process, and the Go runtime
// retires threads whenever it likes, so it would kill live streams at moments nothing in this code
// chose.
func KillOnAppExit(cmd *exec.Cmd) {}
