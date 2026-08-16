package ffmpeg

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeLogs lays down run logs a minute apart, oldest first, and answers their paths in that order.
func writeLogs(t *testing.T, dir string, names ...string) []string {
	t.Helper()

	var paths []string
	stamp := time.Now().Add(-time.Duration(len(names)) * time.Minute)
	for _, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("run\n"), 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatalf("stamping %s: %v", name, err)
		}
		stamp = stamp.Add(time.Minute)
		paths = append(paths, path)
	}
	return paths
}

func names(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	var out []string
	for _, entry := range entries {
		out = append(out, entry.Name())
	}
	return out
}

// One file per run and nothing taking them off leaves a directory that grows for as long as the
// product is used.
// What survives is the newest, a reader who asks for a log asking about a run that just ended.
func TestPruningKeepsTheNewestRunLogs(t *testing.T) {
	dir := t.TempDir()
	writeLogs(t, dir, "a.log", "b.log", "c.log", "d.log", "e.log")

	if err := pruneRunLogs(dir, 2); err != nil {
		t.Fatalf("pruning: %v", err)
	}

	got := names(t, dir)
	if len(got) != 2 {
		t.Fatalf("%d logs survived a prune to 2: %v", len(got), got)
	}
	for _, want := range []string{"d.log", "e.log"} {
		if !contains(got, want) {
			t.Errorf("%s was taken off, want the two newest kept: %v", want, got)
		}
	}
}

// A directory already inside the count is left exactly as it was.
func TestPruningLeavesADirectoryUnderTheCount(t *testing.T) {
	dir := t.TempDir()
	writeLogs(t, dir, "a.log", "b.log")

	if err := pruneRunLogs(dir, 5); err != nil {
		t.Fatalf("pruning: %v", err)
	}

	if got := names(t, dir); len(got) != 2 {
		t.Errorf("%d logs survived a prune to 5, want both: %v", len(got), got)
	}
}

// The directory is the user's, and a prune reaches the logs this side wrote and nothing else.
func TestPruningTouchesNothingButRunLogs(t *testing.T) {
	dir := t.TempDir()
	writeLogs(t, dir, "a.log", "b.log", "c.log")
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("mine\n"), 0o600); err != nil {
		t.Fatalf("writing notes.txt: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "keep"), 0o755); err != nil {
		t.Fatalf("making keep: %v", err)
	}

	if err := pruneRunLogs(dir, 1); err != nil {
		t.Fatalf("pruning: %v", err)
	}

	got := names(t, dir)
	for _, want := range []string{"notes.txt", "keep", "c.log"} {
		if !contains(got, want) {
			t.Errorf("%s is gone after a prune: %v", want, got)
		}
	}
}

// A run opens its own log, so the prune runs before the file exists and can never take it.
func TestARunLogSurvivesItsOwnPrune(t *testing.T) {
	dir := t.TempDir()
	writeLogs(t, dir, "a.log", "b.log", "c.log")

	file, path, err := newRunLog(dir, "publish", 1)
	if err != nil {
		t.Fatalf("opening a run log: %v", err)
	}
	file.Close()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the log a run just opened is not there: %v", err)
	}
	if got := names(t, dir); len(got) != 1 {
		t.Errorf("%d logs remain, want the one this run opened: %v", len(got), got)
	}
}

// Two runs of one kind inside a second are two runs, and a name carrying only the second would let
// the later one truncate the earlier one's log.
func TestTwoRunLogsInOneSecondAreTwoFiles(t *testing.T) {
	dir := t.TempDir()

	first, firstPath, err := newRunLog(dir, "publish", 100)
	if err != nil {
		t.Fatalf("opening the first run log: %v", err)
	}
	first.Close()

	second, secondPath, err := newRunLog(dir, "publish", 100)
	if err != nil {
		t.Fatalf("opening the second run log: %v", err)
	}
	second.Close()

	if firstPath == secondPath {
		t.Fatalf("both runs opened %s", firstPath)
	}
	if got := names(t, dir); len(got) != 2 {
		t.Errorf("%d logs remain, want one per run: %v", len(got), got)
	}
}

func contains(haystack []string, needle string) bool {
	for _, straw := range haystack {
		if straw == needle {
			return true
		}
	}
	return false
}
