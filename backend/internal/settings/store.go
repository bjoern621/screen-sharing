package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"bjoernblessin.de/go-utils/util/assert"
)

const configDirName = "screenshare"
const configFileName = "settings.json"

// Mode the config directory is created with.
const configDirMode = 0o755

// Mode the settings and preset files are written with.
// Owner-only, because the settings carry Relay.GroupKey, possession of which is membership of a
// group, and Relay.SrtPassphrase, which decides whether the packets are readable at all.
// cmd/groupd writes its signing key the same way.
const storeFileMode = 0o600

// writeStore writes a store file and holds it at storeFileMode.
//
// Written beside the target and renamed onto it rather than truncated in place.
// A rename inside one directory replaces the file in a single step, so a reader finds the whole old
// file or the whole new one: a truncating write leaves the head of one and the tail of the other
// wherever it is interrupted, and what these files carry is a settings store that then reads as
// corrupt and is moved aside.
// The temporary file is created in the same directory because a rename across filesystems is not
// one operation and would fail.
//
// The mode is set on the temporary file, before it is the one anything reads under this name.
// A rename brings its own mode with it, so a file an earlier build wrote wider does not keep that
// mode the way it would through a write in place.
func writeStore(path string, data []byte) error {
	assert.Assert(path != "", "a written store file is named")

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	// Cleans up every path that did not reach the rename, and finds nothing on the one that did.
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// An Umgebungsfehler, and reported rather than swallowed: a file the mode could not be taken off
	// is a secret readable by every local user, so it is not renamed into place at all.
	if err := os.Chmod(tmp.Name(), storeFileMode); err != nil {
		return fmt.Errorf("cannot restrict %s to its owner: %w", path, err)
	}
	return os.Rename(tmp.Name(), path)
}

// configDir is the directory holding the settings and preset files, created where it is absent.
//
// A directory that cannot be resolved or created is an Umgebungsfehler and leaves as an error,
// never a path written into anyway.
// No fallback to the working directory: that is whatever the app was started from, so the files
// would land where the next launch does not look and every setting would read as a first start.
func configDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine user config directory: %w", err)
	}

	dir := filepath.Join(base, configDirName)
	if err := os.MkdirAll(dir, configDirMode); err != nil {
		return "", fmt.Errorf("cannot create config directory %s: %w", dir, err)
	}

	assert.Assert(dir != "", "a resolved config directory is a path", base)
	return dir, nil
}

func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

// What an unusable store file is renamed with: "settings.json.corrupt".
const corruptSuffix = ".corrupt"

// setAside renames a store file that cannot be read or parsed, and returns the reason with the
// copy's path named.
//
// Both stores are rewritten in full from what their loader returned, the working settings on the
// next field change and the presets on the next save.
// A file left in place is therefore a file the next write replaces, so the values in it move out of
// reach first and the caller has a path to name.
//
// An existing copy is the user's real data from the first failure and is kept.
// The file failing this time was written after it, so it holds the defaults rather than anything
// worth a second copy.
func setAside(path string, cause error) error {
	assert.Assert(path != "", "a file set aside is named")
	assert.IsNotNil(cause, "a file is set aside for a reason", path)

	kept := path + corruptSuffix
	if _, err := os.Stat(kept); err == nil {
		return &StoreUnreadable{Kept: kept, cause: fmt.Errorf("%w - %s holds an earlier unreadable copy and is left untouched", cause, kept)}
	}
	if err := os.Rename(path, kept); err != nil {
		return &StoreUnreadable{cause: fmt.Errorf("%w - moving it to %s failed: %v", cause, kept, err)}
	}
	return &StoreUnreadable{Kept: kept, cause: fmt.Errorf("%w - it was moved to %s, so the values in it survive the next write", cause, kept)}
}

// StoreUnreadable is a store file that could not be used, and where the values in it went.
//
// The path is a field rather than a phrase inside the message, being the one part of this a surface
// has to show: a surface that read it out of the error string would be parsing prose.
// What went wrong stays prose, being the operating system's answer or the JSON decoder's, neither
// of which is this app's vocabulary (api/proto/screenshare/v1/text.proto).
type StoreUnreadable struct {
	// Kept is the copy holding the old values, empty where none could be made.
	Kept  string
	cause error
}

func (e *StoreUnreadable) Error() string { return e.cause.Error() }

func (e *StoreUnreadable) Unwrap() error { return e.cause }

// Load reads the persisted settings, and answers Defaults() with the reason beside them where the
// stored ones cannot be used.
// A missing file is no failure: a first start has nothing to read.
//
// A file that exists and cannot be read or parsed is renamed (setAside) before the defaults go
// back, so the run that opens on defaults does not take the stored values down with it.
// Every failure on this path is an Umgebungsfehler, the file belonging to a user who can edit it,
// move it or take its directory away.
func Load() (Settings, error) {
	path, err := configPath()
	if err != nil {
		return Defaults(), err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Defaults(), nil
	}
	if err != nil {
		return Defaults(), setAside(path, fmt.Errorf("cannot read settings file %s: %w", path, err))
	}

	s := Defaults()
	if err := json.Unmarshal(data, &s); err != nil {
		return Defaults(), setAside(path, fmt.Errorf("settings file %s is corrupt: %w", path, err))
	}
	// A file written before the three groups carries none of them, so the decode above left every
	// group at its default.
	// Its flat keys are still in these bytes, which is what makes the read an upgrade rather than a
	// loss (migrate.go).
	if flat, ok := decodeFlat(data); ok {
		s = flat
	}

	return migrate(s), nil
}

// Save writes the whole file from the given settings, so saving the same ones twice leaves the same
// bytes.
// Marshalling a plain struct is a contract this code holds, the write a condition it survives.
func Save(s Settings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	assert.IsNil(err, "marshalling a plain settings struct cannot fail")
	assert.Assert(len(data) > 0, "marshalled settings carry bytes")

	path, err := configPath()
	if err != nil {
		return err
	}
	return writeStore(path, data)
}
