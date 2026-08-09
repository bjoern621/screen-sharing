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

// configDirMode is the permission the config directory is created with.
const configDirMode = 0o755

// storeFileMode is the permission the settings and preset files are written with.
const storeFileMode = 0o644

// configDir returns the directory holding the settings and preset files, creating
// it if needed.
//
// A directory that cannot be resolved or created is an error and not a path to
// write into anyway: the working directory it used to fall back to is whatever the
// app was started from, so the files landed somewhere the next launch did not look
// and every setting read as a first start.
func configDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine user config directory: %w", err)
	}

	dir := filepath.Join(base, configDirName)
	if err := os.MkdirAll(dir, configDirMode); err != nil {
		return "", fmt.Errorf("cannot create config directory %s: %w", dir, err)
	}
	return dir, nil
}

// configPath returns the settings file path.
func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

// corruptSuffix names the copy an unusable store file is moved to.
const corruptSuffix = ".corrupt"

// setAside moves a store file that cannot be read or parsed out of the way, and
// returns the reason with the copy's path named.
//
// Both stores are rewritten in full from what their loader returned: the working
// settings on the next field change, the presets on the next save. A file left in
// place is therefore a file the next write replaces, so the values in it are
// renamed out of reach first and the caller has a path to name.
//
// An existing copy is the user's real data from the first failure and is kept: the
// file failing now was written after it, so it holds the defaults rather than
// anything worth a second copy.
func setAside(path string, cause error) error {
	kept := path + corruptSuffix
	if _, err := os.Stat(kept); err == nil {
		return &StoreUnreadable{Kept: kept, cause: fmt.Errorf("%w - %s holds an earlier unreadable copy and is left untouched", cause, kept)}
	}
	if err := os.Rename(path, kept); err != nil {
		return &StoreUnreadable{cause: fmt.Errorf("%w - moving it to %s failed: %v", cause, kept, err)}
	}
	return &StoreUnreadable{Kept: kept, cause: fmt.Errorf("%w - it was moved to %s, so the values in it survive the next write", cause, kept)}
}

// StoreUnreadable is a store file that could not be used, and where the values in it
// went.
//
// The path is a field rather than a phrase inside the message because it is the one
// part of this a surface has to show: a user whose settings did not come back wants to
// know where they are, and a surface that had to find that by reading an error string
// would be parsing prose. What went wrong stays prose, since it is the operating
// system's answer or the JSON decoder's and neither is this app's vocabulary
// (api/proto/screenshare/v1/text.proto).
type StoreUnreadable struct {
	// Kept is the copy holding the old values, and is empty where none could be made.
	Kept  string
	cause error
}

func (e *StoreUnreadable) Error() string { return e.cause.Error() }

func (e *StoreUnreadable) Unwrap() error { return e.cause }

// Load reads the persisted settings, and answers with Defaults() and the reason
// when the stored ones cannot be used. A missing file is not a failure: a first
// start has nothing to read.
//
// A file that exists and cannot be read or parsed is moved aside (setAside) before
// the defaults are handed back, so the run that opens on defaults does not take the
// stored values with it.
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
	// The groups are what a stored file is read into now, and a file written before
	// them carries none of them, so unmarshalling it left every group at its default.
	// The flat keys are still in the bytes, which is what makes the upgrade possible
	// rather than a loss (migrate.go).
	if flat, ok := decodeFlat(data); ok {
		s = flat
	}

	return migrate(s), nil
}

// Save persists the settings.
func Save(s Settings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	assert.IsNil(err, "marshalling a plain settings struct cannot fail")

	path, err := configPath()
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, storeFileMode)
}
