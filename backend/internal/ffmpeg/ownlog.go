package ffmpeg

import (
	"os"
	"path/filepath"
	"sync"

	"bjoernblessin.de/go-utils/util/assert"
)

// own is the run log this process writes itself, nil until OpenOwnLog.
//
// The one owner of that fact.
// The prune reads it and leaves the file where it is: it is the log a reader wants,
// and Windows refuses to remove a file a process holds open,
// which would stop every prune after it at that file.
var own struct {
	sync.Mutex
	file *os.File
}

// OwnLogName is the file name of this process's own log, empty where it has none.
// The crash scan reads it to leave the running log out of the candidates (internal/report).
func OwnLogName() string { return ownName() }

// ownName is the file name of this process's own log, empty where it has none.
func ownName() string {
	own.Lock()
	defer own.Unlock()
	if own.file == nil {
		return ""
	}
	return filepath.Base(own.file.Name())
}

// openOwnLog opens the log this process writes itself, once:
// a second call answers the file the first one opened.
// Named and pruned as a child's, so the directory reads as one set of runs.
func openOwnLog(dir, tag string, keep int) (*os.File, string, error) {
	assert.Assert(dir != "", "this process's log is opened in a resolved directory", tag)
	assert.Assert(tag != "", "this process's log is named after the process that writes it")
	assert.Assert(keep > 0, "a prune keeps at least the log it is opening for", keep)

	own.Lock()
	defer own.Unlock()
	if own.file != nil {
		return own.file, own.file.Name(), nil
	}

	// Nothing to spare yet: this branch is the one where the process has no log.
	file, path, err := openRunLog(dir, tag, keep, "")
	if err != nil {
		return nil, "", err
	}
	own.file = file

	assert.Assert(file != nil && path != "", "this process's log is an open file with a path", tag)
	return file, path, nil
}

// OpenOwnLog opens the log this process writes itself, in the directory LogDir names.
// Never closed: the process ends in os.Exit, where nothing deferred runs,
// and an os.File write reaches the disk without one.
func OpenOwnLog(tag string) (*os.File, string, error) {
	dir, err := LogDir()
	if err != nil {
		return nil, "", err
	}
	return openOwnLog(dir, tag, runLogKeep)
}
