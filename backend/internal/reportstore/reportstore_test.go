package reportstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Each save lands one file whose name is the answered id,
// so the operator finds a quoted report by listing the directory.
func TestSaveWritesOneFilePerReport(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "reports")
	s := New(dir)

	id, err := s.Save(strings.NewReader("bundle"))
	if err != nil {
		t.Fatalf("saving a report: %v", err)
	}
	if !strings.HasSuffix(id, ".tar.gz") {
		t.Fatalf("a report id names the stored file, got %q", id)
	}

	stored, err := os.ReadFile(filepath.Join(dir, id))
	if err != nil {
		t.Fatalf("reading the stored report: %v", err)
	}
	if string(stored) != "bundle" {
		t.Fatalf("a report is stored as it arrived, got %q", stored)
	}

	second, err := s.Save(strings.NewReader("another"))
	if err != nil {
		t.Fatalf("saving a second report: %v", err)
	}
	if second == id {
		t.Fatalf("two reports land under two names, both got %q", id)
	}
}
