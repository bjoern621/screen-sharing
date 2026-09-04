package main

import (
	"io"
	"log"
	"os"
	"runtime/debug"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/ffmpeg"
)

// ownLogTag names this process's run log, beside the ones its children write:
// backend-<stamp>.log under ffmpeg.LogDir.
const ownLogTag = "backend"

// openLog routes this process's log into a run log of its own.
//
// The shell starts the backend with no console (BackendProcess.cs),
// so on an install stderr alone reaches nobody.
// Stderr keeps every line as well, for a console run and a supervisor's journal.
// The file comes first in the writer: a console that is not there fails the write,
// and io.MultiWriter stops at the first failure.
//
// A crash lands in the same file (debug.SetCrashOutput).
// An assert ends the process with the stack that led there, which is what a bug report needs,
// and the runtime writes it to stderr and nowhere else.
//
// Nothing here stops the backend.
// A directory that cannot be made leaves the log on stderr, with a line saying so.
func openLog() {
	file, path, err := ffmpeg.OpenOwnLog(ownLogTag)
	if err != nil {
		logger.Warnf("logging to stderr alone: %v", err)
		return
	}
	log.SetOutput(io.MultiWriter(file, os.Stderr))
	if err := debug.SetCrashOutput(file, debug.CrashOptions{}); err != nil {
		logger.Warnf("a crash reaches stderr alone: %v", err)
	}
	logger.Infof("logging to %s", path)
}
