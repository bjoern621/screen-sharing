// Package member is this machine's identity in each group whose key it holds.
//
// One file per group under the config directory, holding the secret this app drew for itself there:
//
//	<config>/screenshare/members/<group id>.json   {"memberSecret": "..."}
//
// The secret is issued by nobody.
// Membership is stated over the member id it derives under the group key (internal/group),
// so no other member and no service can state this member's presence or take the name it claimed.
// Drawn once and kept: a second secret is a second member,
// with the first one's connections still open at the relay.
//
// The name goes with the statement rather than into the file (settings.Relay.DisplayName),
// so the one a member is listed under is the one their settings hold.
//
// Nothing here reaches the network.
// Which groups this machine is in is all this package knows,
// and stating any of it is the group service's side (internal/groupclient).
//
// The files belong to a user who can edit, move or delete them,
// so every failure here is an Umgebungsfehler and leaves as an error.
package member

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/group"
)

// Where the identities live, under the directory settings.json is in.
const configDirName = "mirrorme"
const membersDirName = "members"

// identityDirMode and identityFileMode keep both to their owner.
//
// The file carries a secret, and the directory carries which groups this machine is in,
// a list of who this user shares a screen with.
// settings.json is held the same way (internal/settings, store.go).
const identityDirMode = 0o700
const identityFileMode = 0o600

// Identity is what this machine is inside one group.
type Identity struct {
	// Secret is base64, as the group service takes it and as group.ParseMemberSecret reads it.
	Secret string `json:"memberSecret"`
}

// Load answers the identity held for a group, and false where this machine holds none.
//
// A file that will not read is an error and never a drawn identity:
// a second secret drawn over a damaged file makes this machine a second member,
// in a group it is already in, under a name the first one holds.
func Load(groupID string) (Identity, bool, error) {
	path, err := identityPath(groupID)
	if err != nil {
		return Identity{}, false, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Identity{}, false, nil
	}
	if err != nil {
		return Identity{}, false, fmt.Errorf("cannot read the identity file %s: %w", path, err)
	}

	var held Identity
	if err := json.Unmarshal(data, &held); err != nil {
		return Identity{}, false, fmt.Errorf("the identity file %s is corrupt: %w", path, err)
	}
	// Read through the service's own parser,
	// so a secret deriving a member id nobody knows is caught here
	// rather than at a relay refusing a connection.
	if _, err := group.ParseMemberSecret(held.Secret); err != nil {
		return Identity{}, false, fmt.Errorf("the identity file %s holds no member secret: %w", path, err)
	}

	assert.Assert(held.Secret != "", "a held identity carries the secret it is known by", path)
	return held, true, nil
}

// Draw draws a secret where none is held, and hands back the one held where there is one.
//
// Idempotent: the state it names is that this machine has an identity in this group,
// so a second call writes nothing and answers the secret already drawn.
// Drawing is not being in the group: presence is stated at the service over the id this derives,
// and the name that goes with it is claimed there (internal/groupclient).
func Draw(groupID string) (Identity, error) {
	held, drawn, err := Load(groupID)
	if err != nil {
		return Identity{}, err
	}
	if drawn {
		return held, nil
	}

	path, err := identityPath(groupID)
	if err != nil {
		return Identity{}, err
	}
	secret, err := group.NewMemberSecret()
	if err != nil {
		return Identity{}, fmt.Errorf("drawing this machine's identity in group %s: %w", groupID, err)
	}
	held.Secret = secret.String()

	data, err := json.MarshalIndent(held, "", "  ")
	assert.IsNil(err, "marshalling a plain identity struct cannot fail")
	if err := writeIdentity(path, data); err != nil {
		return Identity{}, err
	}

	assert.Assert(held.Secret != "", "a drawn identity carries the secret it is known by", groupID)
	return held, nil
}

// Forget drops the identity, which is what leaving a group does.
//
// Idempotent: a group with no file is already in the state this names.
// The secret goes with it, so rejoining is a new member rather than the one that left.
func Forget(groupID string) error {
	path, err := identityPath(groupID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("cannot drop the identity file %s: %w", path, err)
	}
	return nil
}

// identityPath is where one group's identity lives, with the directory created where it is absent.
//
// A group id is a base32 digest this app derived from the key it holds (internal/group),
// so one that is not a single file name is a caller that made it up rather than a path to follow.
func identityPath(groupID string) (string, error) {
	assert.Assert(groupID != "", "an identity belongs to a group that names itself")
	assert.Assert(filepath.Base(groupID) == groupID, "a group id is one file name", groupID)

	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine user config directory: %w", err)
	}
	dir := filepath.Join(base, configDirName, membersDirName)
	if err := os.MkdirAll(dir, identityDirMode); err != nil {
		return "", fmt.Errorf("cannot create identity directory %s: %w", dir, err)
	}
	return filepath.Join(dir, groupID+".json"), nil
}

// writeIdentity writes one identity file and holds it at identityFileMode.
//
// Written beside the target and renamed onto it, as the settings store writes its own
// (internal/settings, writeStore):
// a rename inside one directory replaces the file in a single step,
// so a reader finds one whole file or the other,
// never the head of one and the tail of the other.
//
// The mode is taken on the temporary file, before anything reads it under this name,
// and a file the mode cannot be taken off is not renamed into place at all,
// that file being a secret every local user can read.
func writeIdentity(path string, data []byte) error {
	assert.Assert(path != "", "a written identity file is named")
	assert.Assert(len(data) > 0, "a written identity carries bytes", path)

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("cannot write the identity file %s: %w", path, err)
	}
	// Cleans up every path that did not reach the rename, and finds nothing on the one that did.
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("cannot write the identity file %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("cannot write the identity file %s: %w", path, err)
	}
	if err := os.Chmod(tmp.Name(), identityFileMode); err != nil {
		return fmt.Errorf("cannot restrict %s to its owner: %w", path, err)
	}
	return os.Rename(tmp.Name(), path)
}
