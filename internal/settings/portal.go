package settings

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// The xdg-desktop-portal restore token, stored on its own.
//
// The token is the compositor's receipt for one consent on one machine.
// It names neither a stream nor a setting, so it is not a field of Publish:
// a preset carries what the user chose about a stream, and a preset moved to another machine would
// carry a token no compositor there ever issued.
// Its own file keeps it out of both the working settings and every preset.

const portalFileName = "portal.json"

// portalStore is the file's shape.
// A single field rather than a bare string so a second portal fact can be added without rewriting
// what is already stored.
type portalStore struct {
	// RestoreToken is what SelectSources is given to skip the picker, empty when no consent has been
	// persisted yet.
	RestoreToken string `json:"restoreToken"`
}

// portalPath returns the token file's path.
func portalPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, portalFileName), nil
}

// PortalToken returns the stored ScreenCast restore token, empty when none is stored or the file
// cannot be used.
//
// A token that cannot be read costs a picker, so there is nothing to report and nothing to move
// aside: the next Open acquires a consent and overwrites the file with the token it produced.
// This is the one store where the failure and the empty value have the same remedy,
// which is why it does not follow Load's setAside path.
func PortalToken() string {
	path, err := portalPath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var store portalStore
	if err := json.Unmarshal(data, &store); err != nil {
		return ""
	}
	return store.RestoreToken
}

// SavePortalToken stores the token the last ScreenCast session returned, and reports whether it was
// written.
//
// What the compositor returned is stored as it stands, an empty token included.
// An empty one means the consent was not persisted, so the token already on disk is spent,
// and keeping it would send the next session to SelectSources with a value no compositor will
// honour.
// That is why the empty token is not a precondition here.
func SavePortalToken(token string) error {
	path, err := portalPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(portalStore{RestoreToken: token}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, storeFileMode)
}

// ForgetPortalToken drops the stored consent, so the next capture pops the picker.
// A file that is not there is already forgotten, which is what makes this idempotent.
func ForgetPortalToken() error {
	path, err := portalPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
