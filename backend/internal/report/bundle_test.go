package report

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/settings"
)

// entries reads one bundle back, entry name to content.
func entries(t *testing.T, bundle *bytes.Buffer) map[string]string {
	t.Helper()
	unzipped, err := gzip.NewReader(bundle)
	if err != nil {
		t.Fatalf("a bundle is gzip: %v", err)
	}
	out := map[string]string{}
	archive := tar.NewReader(unzipped)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("a bundle is a tar: %v", err)
		}
		content, err := io.ReadAll(archive)
		if err != nil {
			t.Fatalf("reading %s: %v", header.Name, err)
		}
		out[header.Name] = string(content)
	}
	return out
}

// A bundle carries the facts, the redacted settings and the newest logs,
// each under a name the operator opens directly.
func TestBuildBundlesFactsSettingsAndLogs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "backend-1.log"), []byte("first run"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "decode-host-1.log"), []byte("a decode"), 0o600); err != nil {
		t.Fatal(err)
	}

	s := settings.Defaults()
	s.Relay.GroupKey = "the-group-key"

	var bundle bytes.Buffer
	if err := Build(&bundle, Facts{Kind: KindManual, Version: "dev"}, s, dir); err != nil {
		t.Fatalf("building a bundle: %v", err)
	}

	got := entries(t, &bundle)
	if !strings.Contains(got["report.json"], `"manual"`) {
		t.Errorf("report.json carries the facts, got %q", got["report.json"])
	}
	if got["logs/backend-1.log"] != "first run" || got["logs/decode-host-1.log"] != "a decode" {
		t.Errorf("the logs ride under logs/, got %v", keysOf(got))
	}
	if strings.Contains(got["settings.json"], "the-group-key") {
		t.Error("the group key never leaves this machine")
	}
	if !strings.Contains(got["settings.json"], settings.RedactedMark) {
		t.Errorf("a set secret leaves as the mark, got %q", got["settings.json"])
	}
}

// A log past the cap rides tail alone, the newest lines being the ones a crash ends in.
func TestBuildCapsEachLogAtItsTail(t *testing.T) {
	dir := t.TempDir()
	long := strings.Repeat("x", maxLogBytes) + "the tail"
	if err := os.WriteFile(filepath.Join(dir, "backend-1.log"), []byte(long), 0o600); err != nil {
		t.Fatal(err)
	}

	var bundle bytes.Buffer
	if err := Build(&bundle, Facts{Kind: KindManual}, settings.Defaults(), dir); err != nil {
		t.Fatalf("building a bundle: %v", err)
	}

	got := entries(t, &bundle)["logs/backend-1.log"]
	if len(got) != maxLogBytes {
		t.Fatalf("a log rides capped at %d bytes, got %d", maxLogBytes, len(got))
	}
	if !strings.HasSuffix(got, "the tail") {
		t.Error("the cap keeps the tail")
	}
}

// The newest logs ride and the rest stay, so a directory of two hundred runs
// leaves as a bundle the body bound takes (groupsvc, reportBodyLimit).
func TestBuildTakesTheNewestLogsAlone(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < maxLogs+3; i++ {
		name := filepath.Join(dir, "run-"+strings.Repeat("a", i+1)+".log")
		if err := os.WriteFile(name, []byte("log"), 0o600); err != nil {
			t.Fatal(err)
		}
		at := oldMtime(i)
		if err := os.Chtimes(name, at, at); err != nil {
			t.Fatal(err)
		}
	}

	var bundle bytes.Buffer
	if err := Build(&bundle, Facts{Kind: KindManual}, settings.Defaults(), dir); err != nil {
		t.Fatalf("building a bundle: %v", err)
	}

	logs := 0
	for name := range entries(t, &bundle) {
		if strings.HasPrefix(name, "logs/") {
			logs++
		}
	}
	if logs != maxLogs {
		t.Fatalf("a bundle carries the newest %d logs, got %d", maxLogs, logs)
	}
}

// A named log rides whatever its age, which is how a crash log older than the newest set
// reaches the operator.
func TestBuildAlwaysIncludesANamedLog(t *testing.T) {
	dir := t.TempDir()
	crashed := filepath.Join(dir, "backend-crashed.log")
	if err := os.WriteFile(crashed, []byte("panic: gone"), 0o600); err != nil {
		t.Fatal(err)
	}
	at := oldMtime(maxLogs + 10)
	if err := os.Chtimes(crashed, at, at); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxLogs; i++ {
		name := filepath.Join(dir, "run-"+strings.Repeat("a", i+1)+".log")
		if err := os.WriteFile(name, []byte("log"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var bundle bytes.Buffer
	if err := Build(&bundle, Facts{Kind: KindCrash}, settings.Defaults(), dir, crashed); err != nil {
		t.Fatalf("building a bundle: %v", err)
	}

	if entries(t, &bundle)["logs/backend-crashed.log"] != "panic: gone" {
		t.Error("a named log rides whatever its age")
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
