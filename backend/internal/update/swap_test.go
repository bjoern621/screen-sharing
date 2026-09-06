package update

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// The install directory ends up holding what the archive carries and nothing of what it held,
// which is what separates an update from an unpack over the top of one.
func TestTheInstallDirectoryIsReplaced(t *testing.T) {
	root := t.TempDir()

	target := filepath.Join(root, "mirrorme")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("laying out the install: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "mirrorme"), []byte("old"), 0o755); err != nil {
		t.Fatalf("writing the old binary: %v", err)
	}
	// A file the new release dropped, which has to be gone afterwards.
	if err := os.WriteFile(filepath.Join(target, "leftover"), []byte("x"), 0o644); err != nil {
		t.Fatalf("writing the leftover: %v", err)
	}

	archive := filepath.Join(root, "mirrorme-0.5.1-linux-x86_64-portable.tar.gz")
	writeTarball(t, archive, "mirrorme-0.5.1-linux-x86_64-portable", map[string]string{
		"mirrorme": "new",
	})

	if err := Swap(archive, target); err != nil {
		t.Fatalf("swapping the install: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(target, "mirrorme"))
	if err != nil {
		t.Fatalf("reading the installed binary: %v", err)
	}
	if string(got) != "new" {
		t.Errorf("the installed binary reads %q, want the archive's", got)
	}
	if _, err := os.Stat(filepath.Join(target, "leftover")); err == nil {
		t.Error("a file of the old install survived the swap")
	}
	if _, err := os.Stat(target + ".outgoing"); err == nil {
		t.Error("the old tree was left beside the new one")
	}
}

// An archive is somebody else's file, so an entry climbing out of the extract is refused
// rather than written where it points.
func TestAnArchiveCannotWriteOutsideItself(t *testing.T) {
	root := t.TempDir()

	archive := filepath.Join(root, "escape.tar.gz")
	writeTarball(t, archive, "", map[string]string{"../escaped": "x"})

	if err := extract(archive, filepath.Join(root, "into")); err == nil {
		t.Error("an entry naming a path outside the extract was accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "escaped")); err == nil {
		t.Error("an entry naming a path outside the extract was written there")
	}
}

// writeTarball builds one archive, with every file under the named top-level directory.
func writeTarball(t *testing.T, path, root string, files map[string]string) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating the archive: %v", err)
	}
	defer func() { _ = file.Close() }()

	zipped := gzip.NewWriter(file)
	writer := tar.NewWriter(zipped)

	if root != "" {
		if err := writer.WriteHeader(&tar.Header{
			Name: root + "/", Typeflag: tar.TypeDir, Mode: 0o755,
		}); err != nil {
			t.Fatalf("writing the archive root: %v", err)
		}
	}

	for name, body := range files {
		if root != "" {
			name = root + "/" + name
		}
		if err := writer.WriteHeader(&tar.Header{
			Name: name, Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(body)),
		}); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
		if _, err := writer.Write([]byte(body)); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("closing the archive: %v", err)
	}
	if err := zipped.Close(); err != nil {
		t.Fatalf("closing the compressor: %v", err)
	}
}
