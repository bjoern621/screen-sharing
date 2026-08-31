package settings

import (
	"os"
	"runtime"
	"testing"
)

// mustConfigPath is the settings file, for a test reading its mode.
func mustConfigPath(t *testing.T) string {
	t.Helper()
	path, err := configPath()
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	return path
}

func TestASavedStoreIsReadableByItsOwnerAlone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows carries no owner-group-other mode")
	}
	isolateConfig(t)

	if err := Save(Defaults()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(mustConfigPath(t))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != storeFileMode {
		t.Errorf("mode = %o, want %o: the file carries the group key and the SRT passphrase", info.Mode().Perm(), storeFileMode)
	}
}

// A store an earlier build wrote is world-readable,
// and os.WriteFile leaves an existing file's mode alone,
// so the next write is what has to take it down.
func TestAWiderStoreFileIsTightenedByTheNextWrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows carries no owner-group-other mode")
	}
	isolateConfig(t)

	path := mustConfigPath(t)
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatalf("seeding the old file: %v", err)
	}

	if err := Save(Defaults()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != storeFileMode {
		t.Errorf("mode = %o, want %o", info.Mode().Perm(), storeFileMode)
	}
}
