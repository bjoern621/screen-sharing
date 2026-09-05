package report

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// oldMtime is a moment steps hours back, for ordering files under test.
func oldMtime(steps int) time.Time {
	return time.Now().Add(-time.Duration(steps+1) * time.Hour)
}

func write(t *testing.T, dir, name, content string, age int) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	at := oldMtime(age)
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
	return path
}

// The newest earlier run's log holding a traceback is the crash to report.
// The running process's own log is no earlier run, and a clean run holds none.
func TestUnreportedCrashNamesTheNewestEarlierTraceback(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "backend-clean.log", "[INFO] served and stopped", 0)
	crashed := write(t, dir, "backend-crashed.log", "[INFO] serving\npanic: gone wrong\n\ngoroutine 1", 1)
	write(t, dir, "backend-own.log", "panic: mid-run state", 0)
	write(t, dir, "decode-host-1.log", "panic: a child's crash is the child's log", 0)

	path, ok := UnreportedCrash(dir, "backend", "backend-own.log")
	if !ok || path != crashed {
		t.Fatalf("the crash to report is %s, got %q, %v", crashed, path, ok)
	}
}

// A runtime that dies outside a panic writes "fatal error:", and it reads as a crash too.
func TestUnreportedCrashReadsAFatalError(t *testing.T) {
	dir := t.TempDir()
	crashed := write(t, dir, "backend-oom.log", "fatal error: out of memory", 1)

	path, ok := UnreportedCrash(dir, "backend", "")
	if !ok || path != crashed {
		t.Fatalf("a fatal error reads as a crash, got %q, %v", path, ok)
	}
}

// The marker keeps a crash to one report across restarts.
func TestMarkReportedSilencesTheCrash(t *testing.T) {
	dir := t.TempDir()
	crashed := write(t, dir, "backend-crashed.log", "panic: gone wrong", 1)

	if _, ok := UnreportedCrash(dir, "backend", ""); !ok {
		t.Fatal("an unmarked crash is one to report")
	}
	if err := MarkReported(dir, filepath.Base(crashed)); err != nil {
		t.Fatalf("marking: %v", err)
	}
	if _, ok := UnreportedCrash(dir, "backend", ""); ok {
		t.Fatal("a marked crash is reported once")
	}
}

// A clean history reports nothing.
func TestUnreportedCrashAnswersNothingOnACleanHistory(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "backend-clean.log", "[INFO] served and stopped", 0)

	if path, ok := UnreportedCrash(dir, "backend", ""); ok {
		t.Fatalf("a clean history holds no crash, got %q", path)
	}
}
