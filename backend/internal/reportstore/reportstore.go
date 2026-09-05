// Package reportstore keeps the report bundles members send in,
// one file per report (internal/groupsvc, POST /reports).
//
// A report is stored as it arrived and read by the operator with ordinary tools,
// so nothing here parses a bundle.
package reportstore

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
)

// Store writes each report into one directory.
type Store struct{ dir string }

// New stores reports under dir, created on the first save.
func New(dir string) *Store {
	assert.Assert(dir != "", "a report store writes into a named directory")
	return &Store{dir: dir}
}

// Save writes one report and answers the name it was stored under.
//
// The stamp orders the directory by arrival,
// and the random suffix tells two reports landing in one second apart.
func (s *Store) Save(body io.Reader) (string, error) {
	assert.IsNotNil(body, "a saved report carries a body")

	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return "", fmt.Errorf("cannot create the report directory %s: %w", s.dir, err)
	}

	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return "", fmt.Errorf("cannot draw a report name: %w", err)
	}
	id := fmt.Sprintf("%s-%s.tar.gz",
		time.Now().UTC().Format("20060102-150405"), hex.EncodeToString(suffix))

	file, err := os.OpenFile(filepath.Join(s.dir, id), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("cannot create the report %s: %w", id, err)
	}
	defer file.Close()

	// A body that stops mid-copy leaves no partial file to read as a whole report.
	if _, err := io.Copy(file, body); err != nil {
		os.Remove(file.Name())
		return "", fmt.Errorf("cannot write the report %s: %w", id, err)
	}
	return id, nil
}
