package report

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
)

// markerName holds the file name of the last crash log a report went out about.
// In the log directory, beside what it records.
// No ".log" suffix, so the rotation leaves it alone (internal/ffmpeg, runLogSuffix).
const markerName = "crash-reported"

// A traceback the runtime writes into the run log starts with one of these
// (cmd/backend/log.go, debug.SetCrashOutput).
var tracebackMarks = []string{"panic:", "fatal error:"}

// scanLineCap bounds one scanned line.
// A log line is short; a longer one is data the scan may skip.
const scanLineCap = 1 << 20

// UnreportedCrash names the newest earlier run log of tag holding a crash traceback,
// ok=false where every earlier run ended clean or that crash already went out (MarkReported).
//
// ownName is the running process's own log, no earlier run and so never a candidate.
// One crash is answered: an older one behind it waits for nobody,
// the report being about what just happened.
func UnreportedCrash(logDir, tag, ownName string) (path string, ok bool) {
	assert.Assert(logDir != "", "a crash is looked for in a resolved directory")
	assert.Assert(tag != "", "a crash is looked for under a run log tag")

	type log struct {
		name string
		at   time.Time
	}
	var logs []log
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == ownName ||
			!strings.HasPrefix(name, tag+"-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		logs = append(logs, log{name: name, at: info.ModTime()})
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].at.After(logs[j].at) })

	for _, l := range logs {
		full := filepath.Join(logDir, l.name)
		if !holdsTraceback(full) {
			continue
		}
		if l.name == lastReported(logDir) {
			return "", false
		}
		return full, true
	}
	return "", false
}

// MarkReported records that a report about this crash log went out,
// so the next start sends no second one about it.
func MarkReported(logDir, name string) error {
	assert.Assert(logDir != "" && name != "", "a mark names the crash log it silences", logDir, name)

	return os.WriteFile(filepath.Join(logDir, markerName), []byte(name), 0o600)
}

// lastReported is the crash log the marker names, empty where none has.
func lastReported(logDir string) string {
	content, err := os.ReadFile(filepath.Join(logDir, markerName))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

// holdsTraceback reports whether one log carries a runtime traceback.
func holdsTraceback(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	lines := bufio.NewScanner(file)
	lines.Buffer(make([]byte, 0, 64<<10), scanLineCap)
	for lines.Scan() {
		for _, mark := range tracebackMarks {
			if strings.HasPrefix(lines.Text(), mark) {
				return true
			}
		}
	}
	return false
}
