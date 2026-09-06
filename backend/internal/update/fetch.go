package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/release"
)

// Downloading one release asset and proving it is the file the release names.
//
// The forge serves the artifact over HTTPS and records a hash of it beside the size.
// Both are checked, and a file failing either is deleted rather than staged:
// what would otherwise be installed is a truncated transfer or somebody else's bytes,
// and neither is something a restart should run.

// ErrCorrupt is a download whose bytes are not what the release names.
// Its own error so the statement a reader sees separates it from a transfer that stopped.
var ErrCorrupt = errors.New("the download is not the file the release names")

// digestPrefix is how the forge spells the hash it records.
const digestPrefix = "sha256:"

// AssetName is the file one channel installs, for the version a tag carries.
// The tag's leading "v" comes off: an asset is named after the version and a release after the tag.
func AssetName(pattern, tag string) string {
	assert.Assert(pattern != "", "a channel that installs names the file it installs")
	assert.Assert(tag != "", "an asset is named for a release")

	return fmt.Sprintf(pattern, strings.TrimPrefix(strings.TrimSpace(tag), "v"))
}

// Verifiable answers whether the release records a hash this app can check the download against.
func Verifiable(asset release.Asset) bool {
	raw, found := strings.CutPrefix(asset.Digest, digestPrefix)
	if !found {
		return false
	}
	_, err := hex.DecodeString(raw)
	return err == nil
}

// Fetch downloads one asset into dir and answers the path it landed at.
//
// progress is called as bytes arrive, with 0 to 100, and once more with 100 on a finished
// transfer, so a caller renders a bar without keeping a count of its own.
// It runs on the fetching goroutine, so a caller writing shared state guards it there.
//
// The file is written under a partial name and renamed once it verifies,
// which keeps a half-arrived transfer from ever being a path the staging record can name.
func Fetch(ctx context.Context, asset release.Asset, dir string, progress func(int)) (string, error) {
	assert.IsNotNil(ctx, "a download runs under a context")
	assert.Assert(asset.URL != "", "a download names where it comes from")
	assert.Assert(asset.Name != "", "a download names the file it arrives as")
	assert.Assert(Verifiable(asset), "a download is checked against a hash the release records", asset.Name)
	assert.Assert(dir != "", "a download lands in a directory")
	assert.IsNotNil(progress, "a download reports how far it got")

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return "", err
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the download answered %s", response.Status)
	}

	path := filepath.Join(dir, asset.Name)
	partial := path + ".partial"

	file, err := os.OpenFile(partial, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}

	sum := sha256.New()
	written, err := io.Copy(io.MultiWriter(file, sum), counted(response.Body, asset.Size, progress))
	closeErr := file.Close()
	if err != nil {
		_ = os.Remove(partial)
		return "", err
	}
	if closeErr != nil {
		_ = os.Remove(partial)
		return "", closeErr
	}

	if err := check(sum, written, asset); err != nil {
		_ = os.Remove(partial)
		return "", err
	}

	if err := os.Rename(partial, path); err != nil {
		_ = os.Remove(partial)
		return "", err
	}

	progress(100)
	return path, nil
}

// check holds the arrived file against what the release records.
// Both the length and the hash, so a transfer that stopped short is told from one that changed.
func check(sum hash.Hash, written int64, asset release.Asset) error {
	if asset.Size > 0 && written != asset.Size {
		return fmt.Errorf("%w: %d bytes arrived of %d", ErrCorrupt, written, asset.Size)
	}

	want := strings.TrimPrefix(asset.Digest, digestPrefix)
	if got := hex.EncodeToString(sum.Sum(nil)); !strings.EqualFold(got, want) {
		return fmt.Errorf("%w: %s against %s", ErrCorrupt, got, want)
	}
	return nil
}

// counted reports how much of a body has been read, in whole percent.
//
// Only on a change, so a fast transfer of a large file announces a hundred times
// rather than once per buffer.
// A size of zero leaves the reader silent: nothing states what a fraction would be of.
func counted(body io.Reader, size int64, progress func(int)) io.Reader {
	assert.IsNotNil(body, "a counted download reads a body")
	assert.IsNotNil(progress, "a counted download reports how far it got")

	if size <= 0 {
		return body
	}

	var read int64
	last := -1
	return readerFunc(func(p []byte) (int, error) {
		n, err := body.Read(p)
		read += int64(n)

		if percent := int(read * 100 / size); percent != last && percent <= 100 {
			last = percent
			progress(percent)
		}
		return n, err
	})
}

// readerFunc is a read step written as a function.
type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }
