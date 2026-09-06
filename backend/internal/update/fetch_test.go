package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"bjoernblessin.de/screenshare/internal/release"
)

// The asset is named after the version and the release after the tag,
// so the "v" comes off exactly once.
func TestWhichFileAReleaseIsFetchedAs(t *testing.T) {
	got := AssetName("mirrorme-%s-linux-x86_64-portable.tar.gz", "v0.5.1")
	if want := "mirrorme-0.5.1-linux-x86_64-portable.tar.gz"; got != want {
		t.Errorf("the asset is %q, want %q", got, want)
	}
}

// A download nothing can be checked against is refused before it starts,
// so the caller states that rather than installing bytes of unknown origin.
func TestAReleaseWithNoHashIsNotVerifiable(t *testing.T) {
	if Verifiable(release.Asset{Digest: ""}) {
		t.Error("an asset with no digest reads as verifiable")
	}
	if Verifiable(release.Asset{Digest: "md5:abc"}) {
		t.Error("an asset hashed with something else reads as verifiable")
	}
	if !Verifiable(release.Asset{Digest: digestPrefix + hex.EncodeToString(make([]byte, 32))}) {
		t.Error("an asset carrying a sha256 digest reads as unverifiable")
	}
}

// What lands on disk is what the release names, or nothing lands at all.
func TestADownloadIsHeldAgainstItsHash(t *testing.T) {
	body := []byte("the release")
	sum := sha256.Sum256(body)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer server.Close()

	asset := release.Asset{
		Name:   "mirrorme-0.5.1-linux-x86_64-portable.tar.gz",
		URL:    server.URL,
		Size:   int64(len(body)),
		Digest: digestPrefix + hex.EncodeToString(sum[:]),
	}

	dir := t.TempDir()
	reached := 0
	path, err := Fetch(context.Background(), asset, dir, func(int) { reached++ })
	if err != nil {
		t.Fatalf("fetching the release: %v", err)
	}
	if reached == 0 {
		t.Error("the download reported no progress at all")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading what landed: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("what landed reads %q", got)
	}

	// A hash that does not match is a file nothing may run, so it is deleted rather than staged.
	asset.Digest = digestPrefix + hex.EncodeToString(make([]byte, 32))
	if _, err := Fetch(context.Background(), asset, dir, func(int) {}); !errors.Is(err, ErrCorrupt) {
		t.Errorf("a download that does not match its hash answered %v", err)
	}
	if entries, _ := filepath.Glob(filepath.Join(dir, "*.partial")); len(entries) > 0 {
		t.Errorf("a refused download left %v behind", entries)
	}
}
