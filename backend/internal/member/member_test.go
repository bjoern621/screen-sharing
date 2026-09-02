package member

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/group"
)

// isolateConfig points os.UserConfigDir at a fresh temp directory.
//
// All three variables, os.UserConfigDir reading a different one per platform:
// XDG_CONFIG_HOME on Linux, AppData on Windows, HOME on macOS.
// Isolating one platform's is a test that draws identities
// into the developer's own config directory on the others.
func isolateConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("AppData", dir)
	t.Setenv("HOME", dir)

	got, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("isolating the config directory: %v", err)
	}
	if !strings.HasPrefix(got, dir) {
		t.Fatalf("os.UserConfigDir is %s, outside the temp directory %s: this test would write into the real config directory", got, dir)
	}
}

const aGroup = "MFRGGZDFMZTWQ2LKNNWG3"
const anotherGroup = "NBSWY3DPEB3W64TMMQQQ2"

// A group nothing was drawn in holds no identity,
// which is what the app reads before deciding whether to draw one.
func TestAGroupNothingWasDrawnInHoldsNothing(t *testing.T) {
	isolateConfig(t)

	held, drawn, err := Load(aGroup)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if drawn {
		t.Errorf("a group nothing was drawn in holds %+v", held)
	}
}

// Drawing twice is drawing once: the secret is what this member is known by inside the group,
// and a second one would be a second member with the first one's connections still open.
func TestDrawingTwiceKeepsTheSecret(t *testing.T) {
	isolateConfig(t)

	first, err := Draw(aGroup)
	if err != nil {
		t.Fatalf("Draw: %v", err)
	}
	if first.Secret == "" {
		t.Fatalf("Draw = %+v, want a secret drawn for this group", first)
	}

	again, err := Draw(aGroup)
	if err != nil {
		t.Fatalf("Draw again: %v", err)
	}
	if again.Secret != first.Secret {
		t.Errorf("a second draw drew %q, want the secret this machine is already known by", again.Secret)
	}

	held, drawn, err := Load(aGroup)
	if err != nil || !drawn {
		t.Fatalf("Load after drawing: %+v, %v, %v", held, drawn, err)
	}
	if held != again {
		t.Errorf("the held identity is %+v, want %+v", held, again)
	}
}

// A drawn secret is what nobody else can state this member's presence with,
// so it is drawn whole and stored as the service reads it.
func TestADrawnSecretIsWholeAndOneGroupsOwn(t *testing.T) {
	isolateConfig(t)

	mine, err := Draw(aGroup)
	if err != nil {
		t.Fatalf("Draw: %v", err)
	}
	raw, err := group.ParseMemberSecret(mine.Secret)
	if err != nil {
		t.Fatalf("the stored secret does not read back: %v", err)
	}
	if len(raw) != group.MemberSecretBytes {
		t.Errorf("the drawn secret is %d bytes, want %d", len(raw), group.MemberSecretBytes)
	}

	other, err := Draw(anotherGroup)
	if err != nil {
		t.Fatalf("Draw in another group: %v", err)
	}
	if other.Secret == mine.Secret {
		t.Error("two groups share one secret, so one group's members are the other's")
	}
}

// The file carries a secret, so it is the owner's alone,
// and so is the directory holding one file per group:
// which groups this machine is in is nobody else's reading either.
func TestAnIdentityIsTheOwnersAlone(t *testing.T) {
	isolateConfig(t)

	if _, err := Draw(aGroup); err != nil {
		t.Fatalf("Draw: %v", err)
	}

	path, err := identityPath(aGroup)
	if err != nil {
		t.Fatalf("resolving the identity file: %v", err)
	}
	file, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the identity file is not there: %v", err)
	}
	if mode := file.Mode().Perm(); mode != identityFileMode {
		t.Errorf("the identity file is %04o, want %04o", mode, identityFileMode)
	}
	dir, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("the identity directory is not there: %v", err)
	}
	if mode := dir.Mode().Perm(); mode != identityDirMode {
		t.Errorf("the identity directory is %04o, want %04o", mode, identityDirMode)
	}
}

// The file is what another member's app and the service both read this machine's identity out of,
// so its keys are the ones they spell.
func TestTheFileSpellsWhatTheServiceReads(t *testing.T) {
	isolateConfig(t)

	mine, err := Draw(aGroup)
	if err != nil {
		t.Fatalf("Draw: %v", err)
	}
	path, err := identityPath(aGroup)
	if err != nil {
		t.Fatalf("resolving the identity file: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the identity file: %v", err)
	}

	var held map[string]string
	if err := json.Unmarshal(data, &held); err != nil {
		t.Fatalf("the identity file is not readable JSON: %v", err)
	}
	if held["memberSecret"] != mine.Secret {
		t.Errorf("the identity file holds %v, want the drawn secret under memberSecret", held)
	}
}

// Leaving a group is dropping the identity,
// and forgetting one nothing was drawn in is the state the call names.
func TestForgettingIsIdempotent(t *testing.T) {
	isolateConfig(t)

	if err := Forget(aGroup); err != nil {
		t.Errorf("forgetting a group nothing was drawn in: %v", err)
	}

	if _, err := Draw(aGroup); err != nil {
		t.Fatalf("Draw: %v", err)
	}
	if err := Forget(aGroup); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if _, drawn, err := Load(aGroup); drawn || err != nil {
		t.Errorf("a forgotten group reads back as one this machine holds an identity in: %v", err)
	}
	if err := Forget(aGroup); err != nil {
		t.Errorf("forgetting twice: %v", err)
	}

	path, err := identityPath(aGroup)
	if err != nil {
		t.Fatalf("resolving the identity file: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the identity file survived being forgotten: %v", err)
	}
}

// A file the user can edit is a file that can come back damaged,
// and a secret read out of a damaged one derives a member id nobody knows.
// The reason names the file, that being the thing to move aside.
func TestADamagedIdentityIsReported(t *testing.T) {
	isolateConfig(t)

	if _, err := Draw(aGroup); err != nil {
		t.Fatalf("Draw: %v", err)
	}
	path, err := identityPath(aGroup)
	if err != nil {
		t.Fatalf("resolving the identity file: %v", err)
	}

	for what, content := range map[string]string{
		"a file that is not JSON":  "{",
		"a secret that is not one": `{"memberSecret":"not a secret"}`,
	} {
		if err := os.WriteFile(path, []byte(content), identityFileMode); err != nil {
			t.Fatalf("writing %s: %v", what, err)
		}
		_, drawn, err := Load(aGroup)
		if err == nil {
			t.Errorf("%s read back as an identity", what)
		}
		if drawn {
			t.Errorf("%s read back as an identity this machine holds", what)
		}
		if err != nil && !strings.Contains(err.Error(), path) {
			t.Errorf("%s is reported as %q, which does not name the file", what, err)
		}
	}
}

// The directory holds one file per group and nothing else,
// so a group id that is not one file name is a caller that made it up rather than a path to follow.
func TestOneGroupIsOneFile(t *testing.T) {
	isolateConfig(t)

	if _, err := Draw(aGroup); err != nil {
		t.Fatalf("Draw: %v", err)
	}
	if _, err := Draw(anotherGroup); err != nil {
		t.Fatalf("Draw in another group: %v", err)
	}

	path, err := identityPath(aGroup)
	if err != nil {
		t.Fatalf("resolving the identity file: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("reading the identity directory: %v", err)
	}
	held := []string{}
	for _, entry := range entries {
		held = append(held, entry.Name())
	}
	if len(held) != 2 {
		t.Errorf("the identity directory holds %v, want one file per group an identity was drawn in", held)
	}
	for _, name := range held {
		if filepath.Ext(name) != ".json" {
			t.Errorf("the identity directory holds %q, want a json file per group", name)
		}
	}
}
