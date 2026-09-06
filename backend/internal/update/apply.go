package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The applier: the one run that replaces the app's own files.
//
// A process of its own, from a copy of this binary outside the tree it is replacing,
// started by the run that is about to exit.
// Nothing it touches is open by anything: it waits for the app to be gone,
// puts the files in place, starts the app again and deletes what it worked from.
//
// It writes to stderr and to nothing else.
// The app it was started by is gone by then, so no log of that run is still open,
// and what it says lands in the console a developer started it from.

// Subcommand runs the applier instead of the app, on the executable both share.
const Subcommand = "update-apply"

// StageFlag names the staging directory, WaitFlag the process to outlive.
const (
	StageFlag = "--stage="
	WaitFlag  = "--wait-pid="
)

// waitLimit bounds how long the applier waits for the app to close.
// A window asking to save on the way out is what the wait is for,
// and past this the install is abandoned with the running app untouched.
const waitLimit = 60 * time.Second

// waitStep is how often the wait looks.
const waitStep = 200 * time.Millisecond

// Main runs the applier and answers the process's exit status.
//
// Every failure leaves the running install exactly as it was:
// the staged files stay where they are, and the next start finds the release still staged.
func Main(args []string) int {
	stage, pid, err := applierArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	pending, staged := ReadPending(stage)
	if !staged {
		fmt.Fprintf(os.Stderr, "nothing is staged in %s\n", stage)
		return 1
	}

	if pid > 0 && !closed(pid) {
		fmt.Fprintf(os.Stderr, "process %d is still running after %s, so nothing was installed\n", pid, waitLimit)
		return 1
	}

	if err := install(pending); err != nil {
		fmt.Fprintf(os.Stderr, "installing %s: %v\n", pending.Tag, err)
		return 1
	}

	if err := relaunch(pending.Launch); err != nil {
		// The files are in place, so the install landed and only the start did not.
		// Reported rather than undone: the app the reader opens next is the new one.
		fmt.Fprintf(os.Stderr, "starting %s: %v\n", pending.Launch, err)
	}

	// The staging directory holds this binary, so what is left of it goes when this run does.
	if err := ClearStage(stage); err != nil {
		fmt.Fprintf(os.Stderr, "clearing %s: %v\n", stage, err)
	}
	return 0
}

// applierArgs reads the two flags the applier takes.
func applierArgs(args []string) (stage string, pid int, err error) {
	for _, arg := range args {
		if dir, found := strings.CutPrefix(arg, StageFlag); found {
			stage = dir
			continue
		}
		if raw, found := strings.CutPrefix(arg, WaitFlag); found {
			pid, err = strconv.Atoi(raw)
			if err != nil {
				return "", 0, fmt.Errorf("%s takes a process id: %w", WaitFlag, err)
			}
		}
	}
	if stage == "" {
		return "", 0, fmt.Errorf("%s names the staging directory", StageFlag)
	}
	return stage, pid, nil
}

// closed waits out the app, and answers false where it is still there at the limit.
func closed(pid int) bool {
	deadline := time.Now().Add(waitLimit)
	for time.Now().Before(deadline) {
		if !alive(pid) {
			// The other half of the app closes on its own once this one is gone,
			// and on Windows a file it still holds open is a file that cannot be replaced.
			time.Sleep(2 * time.Second)
			return true
		}
		time.Sleep(waitStep)
	}
	return false
}

// install puts the staged release in place, by the method its channel declared.
func install(pending Pending) error {
	switch pending.Method {
	case MethodRun:
		return run(pending.File)
	case MethodSwap:
		return Swap(pending.File, pending.Target)
	default:
		return fmt.Errorf("a staged release names no way to install it")
	}
}

// run executes the downloaded installer and waits for it.
//
// Silent, because the reader already agreed to this in the app,
// and without a restart of its own: starting the app again is this applier's last step.
func run(path string) error {
	command := exec.Command(path, "/VERYSILENT", "/NORESTART", "/SUPPRESSMSGBOXES", "/NOCANCEL")
	command.Stdout, command.Stderr = os.Stderr, os.Stderr

	if err := command.Run(); err != nil {
		return err
	}
	return nil
}

// relaunch starts the installed app and leaves it running.
func relaunch(launch string) error {
	if _, err := os.Stat(launch); err != nil {
		return err
	}

	command := exec.Command(launch)
	command.Dir = filepath.Dir(launch)
	detach(command)

	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}
