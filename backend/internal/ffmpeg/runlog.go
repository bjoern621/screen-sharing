package ffmpeg

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// runLogSuffix is what a prune recognises as its own,
// so a file somebody else put in the directory stays there.
const runLogSuffix = ".log"

// runLogKeep is how many run logs survive.
//
// A log answers "what did the run that just failed do", and the exit that names one is on screen
// for as long as that session lasts, so what is worth keeping is recent rather than complete.
// One file per child start and nothing taking any off grows without end:
// an ordinary few months of use reaches thousands of files and tens of megabytes,
// in the directory the settings live in.
const runLogKeep = 200

// newRunLog opens the log one child run writes, having taken the oldest ones off the directory.
//
// The prune runs before the file is created,
// so the log a run is about to write is never among the candidates.
// Failing to prune is not failing to run: the directory is the user's
// and whatever stopped the prune is theirs to see rather than a reason to refuse a stream.
func newRunLog(dir, tag string, keep int) (*os.File, string, error) {
	assert.Assert(dir != "", "a run log is opened in a resolved directory", tag)
	assert.Assert(tag != "", "a run log is named after the run that writes it")
	assert.Assert(keep > 0, "a prune keeps at least the run it is opening for", keep)

	if err := pruneRunLogs(dir, keep-1); err != nil {
		logger.Warnf("cannot prune the run logs in %s: %v", dir, err)
	}

	// The stamp reads to the second and two runs of one kind can start inside one,
	// so the name carries what tells those apart.
	// Without it the later run truncates the earlier one's log, which is the one a reader wants.
	stamp := time.Now()
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.log", sanitizeTag(tag), stamp.Format("20060102-150405")))
	for attempt := 1; ; attempt++ {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}
		path = filepath.Join(dir, fmt.Sprintf("%s-%s-%d.log", sanitizeTag(tag), stamp.Format("20060102-150405"), attempt))
	}

	file, err := os.Create(path)
	if err != nil {
		return nil, "", fmt.Errorf("cannot create run log: %w", err)
	}
	return file, path, nil
}

// pruneRunLogs leaves the newest keep run logs in the directory and takes the rest off.
//
// Newest by the clock rather than by the name: a name carries the moment its run started,
// and a run that outlives a later one would sort under it.
// Anything that is not a run log this side wrote is left alone,
// the directory being one a user opens (OpenLogsFolder).
func pruneRunLogs(dir string, keep int) error {
	assert.Assert(dir != "", "a prune runs in a resolved directory")
	assert.Assert(keep >= 0, "a prune keeps a count of logs", keep)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("cannot read the run logs in %s: %w", dir, err)
	}

	type log struct {
		name string
		at   time.Time
	}
	var logs []log
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), runLogSuffix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			// Gone between the walk and the read, the state a prune leaves it in anyway.
			continue
		}
		logs = append(logs, log{name: entry.Name(), at: info.ModTime()})
	}
	if len(logs) <= keep {
		return nil
	}

	sort.Slice(logs, func(i, j int) bool { return logs[i].at.After(logs[j].at) })
	for _, old := range logs[keep:] {
		if err := os.Remove(filepath.Join(dir, old.name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cannot remove the run log %s: %w", old.name, err)
		}
	}
	return nil
}

// NewRunLog opens the log one child run writes, in the directory LogDir names.
// The oldest logs come off as it does, so the directory stays the size of what a reader can use.
func NewRunLog(tag string) (*os.File, string, error) {
	dir, err := LogDir()
	if err != nil {
		return nil, "", err
	}
	return newRunLog(dir, tag, runLogKeep)
}
