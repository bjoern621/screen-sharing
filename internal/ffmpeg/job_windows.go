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

// The one Job Object every media child is put into, created on the first assignment and held for the
// rest of this process's life.
// A zero jobHandle means no job, which leaves App.shutdown as the only cleanup.
var (
	jobOnce   sync.Once
	jobHandle windows.Handle
)

// ensureJob creates the job on the first call and never again.
//
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE is the whole point of it: the job terminates every process
// still in it as soon as the last handle to the job closes.
// The only handle is this process's, so the children go when this process goes, by whatever means it
// went.
//
// A job that cannot be created or configured is an Umgebungsfehler the app runs past: every child
// starts, and a normal quit stops each one through App.shutdown.
// What is lost is the guarantee for the exits that never reach it, which is what the warning names
// rather than the API that refused.
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
			// Without the limit the job is a grouping that kills nothing, so the handle goes rather than
			// being kept and relied on.
			windows.CloseHandle(handle)
			logger.Warnf("job object took no kill-on-close limit, so an app killed outright can leave its children running: %v", err)
			return
		}

		// Never closed: closing this handle is what kills the children, so it has to outlive every one of
		// them.
		// Process exit closes it.
		jobHandle = handle
	})
}

// KillOnAppExit ties one already-started child to this process's lifetime, so the OS terminates it
// as soon as this process is gone however it went.
//
// App.shutdown stays the path that stops children on a normal quit, and is the faster and quieter
// one.
// This covers the exits that never reach it: a panic, the log.Fatalf behind logger.Errorf, a kill
// from Task Manager, the WebView2 side taking the process down.
// The window closes to the tray, so an app the user believes is gone is a realistic thing to kill
// from Task Manager, and a publish ffmpeg left behind by that keeps capturing the screen into the
// relay with no window left to stop it from.
//
// Children are assigned one at a time rather than by putting this process in the job and letting
// them inherit it, which would be less code and would also cover the short-lived capability probes.
// Inheritance would take in every process the app starts, and system.go's openInShell starts
// processes meant to outlive it: the file browser or editor opened on a run log would be killed the
// moment the user quits.
//
// Called after cmd.Start and before the goroutine that waits on the child.
// The assignment goes by pid, and the pid names only this child for as long as os/exec holds its
// process handle open, which is what stops Windows handing the number to something else.
// Putting a stranger into a job that kills its members on this process's exit is the failure that
// ordering rules out.
//
// A failed assignment is reported and survivable on ensureJob's reasoning: the child runs, only its
// crash guarantee is lost.
// Off Windows this is a no-op (job_other.go).
func KillOnAppExit(cmd *exec.Cmd) {
	assert.IsNotNil(cmd, "a child put into the job is a command")
	assert.IsNotNil(cmd.Process, "a child joins the job once it has been started")

	ensureJob()
	if jobHandle == 0 {
		return
	}

	// AssignProcessToJobObject needs both: the job accounts for the child's quotas and has to be able
	// to terminate it.
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
		// A child that exited between Start and here is refused like one the job cannot take, and has
		// nothing left to orphan.
		logger.Warnf("cannot put child %d into the job, so an app killed outright can leave it running: %v",
			cmd.Process.Pid, err)
	}
}
