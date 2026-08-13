//go:build windows

package ffmpeg

import (
	"os/exec"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// jobHandle is the one Job Object every media child is put into, created on the first assignment
// and held for the rest of this process's life.
// Zero means the job is unavailable, which leaves the shutdown path as the only cleanup.
var (
	jobOnce   sync.Once
	jobHandle windows.Handle
)

// ensureJob creates the job on the first call and never again.
//
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE is the whole point of it: the job terminates every process
// still in it as soon as the last handle to the job closes.
// The only handle is the one this process holds, so the children go when this process goes,
// by whatever means it went.
//
// A job that cannot be created or configured is an Umgebungsfehler the app runs past:
// every child still starts, and a normal quit still stops each one through App.shutdown.
// What is lost is the guarantee for the exits that never reach it, so the warning says that rather
// than naming the API that refused.
func ensureJob() {
	jobOnce.Do(func() {
		handle, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			logger.Warnf("no job object, so an app killed outright can leave its children running: %v", err)
			return
		}

		limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
			BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
				LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
			},
		}
		_, err = windows.SetInformationJobObject(handle, windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)))
		if err != nil {
			// Without the limit the job is a grouping that kills nothing, so the handle is dropped rather
			// than kept and relied on.
			windows.CloseHandle(handle)
			logger.Warnf("job object took no kill-on-close limit, so an app killed outright can leave its children running: %v", err)
			return
		}

		// Deliberately never closed: closing this handle is exactly what kills the children,
		// so it has to outlive every one of them.
		// Process exit closes it.
		jobHandle = handle
	})
}

// KillOnAppExit ties one already-started child to this process's lifetime,
// so the OS terminates it as soon as this process is gone however it went.
//
// App.shutdown stays the path that stops children on a normal quit, and is the faster and quieter
// one.
// This covers the exits that never reach it: a panic, the log.Fatalf behind logger.Errorf,
// a kill from Task Manager, the WebView2 side taking the process down.
// The window is closable to the tray, so an app the user believes is gone is a realistic thing to
// kill from Task Manager, and a publish ffmpeg left behind by that keeps capturing the screen into
// the relay with no window left to stop it from.
//
// Children are assigned one at a time rather than by putting this process in the job and letting
// them inherit it, which would be less code and would also cover the short-lived capability probes.
// Inheritance would take in every process the app starts, and system.go's openInShell starts
// processes that are meant to outlive it: the file browser or editor opened on a run log would be
// killed the moment the user quits.
//
// It must be called after cmd.Start and before the goroutine that waits on the child.
// The assignment goes by pid, and the pid names only this child for as long as os/exec holds its
// process handle open, which is what stops Windows from handing the number to something else.
// Putting a stranger into a job that kills its members when this process exits is the failure that
// ordering rules out.
//
// A failed assignment is reported and survivable on the same reasoning as ensureJob:
// the child runs, only its crash guarantee is lost.
// Off Windows this is a no-op (job_other.go).
func KillOnAppExit(cmd *exec.Cmd) {
	assert.IsNotNil(cmd, "a child put into the job is a command")
	assert.IsNotNil(cmd.Process, "a child joins the job once it has been started")

	ensureJob()
	if jobHandle == 0 {
		return
	}

	// The two rights AssignProcessToJobObject needs: the job accounts for the child's quotas and has
	// to be able to terminate it.
	process, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		logger.Warnf("cannot open child %d to put it into the job, so an app killed outright can leave it running: %v",
			cmd.Process.Pid, err)
		return
	}
	defer windows.CloseHandle(process)

	err = windows.AssignProcessToJobObject(jobHandle, process)
	if err != nil {
		// A child that exited between Start and here is refused the same way as one the job cannot take,
		// and has nothing left to orphan.
		logger.Warnf("cannot put child %d into the job, so an app killed outright can leave it running: %v",
			cmd.Process.Pid, err)
	}
}
