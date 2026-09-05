package report

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/settings"
)

// maxLogs is how many run logs ride in one bundle.
// The newest ones, a report being about what just happened.
const maxLogs = 15

// maxLogBytes caps each log at its tail, the newest lines being the ones a crash ends in.
// The caps together keep every bundle inside the service's body bound
// (internal/groupsvc, reportBodyLimit).
const maxLogBytes = 512 << 10

// Build writes one bundle: report.json, the redacted settings,
// and the newest run logs beside any named outright.
//
// A log that goes missing between the listing and the read is left out,
// and the bundle carries the rest.
func Build(w io.Writer, facts Facts, s settings.Settings, logDir string, include ...string) error {
	assert.IsNotNil(w, "a bundle is written somewhere")
	assert.Assert(logDir != "", "a bundle reads a resolved log directory")

	zipped := gzip.NewWriter(w)
	archive := tar.NewWriter(zipped)

	if err := writeJSON(archive, "report.json", facts); err != nil {
		return err
	}
	// Redacted here rather than by the caller, so no path into a bundle skips it.
	if err := writeJSON(archive, "settings.json", s.Redacted()); err != nil {
		return err
	}

	for _, path := range chosen(logDir, include) {
		content, err := tail(path, maxLogBytes)
		if err != nil {
			continue
		}
		if err := writeEntry(archive, "logs/"+filepath.Base(path), content); err != nil {
			return err
		}
	}

	if err := archive.Close(); err != nil {
		return fmt.Errorf("closing the bundle: %w", err)
	}
	return zipped.Close()
}

// writeJSON writes one indented JSON entry.
// A marshal of these plain structs cannot fail, so one failing is asserted.
func writeJSON(archive *tar.Writer, name string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	assert.IsNil(err, "the report structs marshal", name)
	return writeEntry(archive, name, encoded)
}

func writeEntry(archive *tar.Writer, name string, content []byte) error {
	header := &tar.Header{Name: name, Mode: 0o600, Size: int64(len(content)), ModTime: time.Now()}
	if err := archive.WriteHeader(header); err != nil {
		return fmt.Errorf("writing %s: %w", name, err)
	}
	if _, err := archive.Write(content); err != nil {
		return fmt.Errorf("writing %s: %w", name, err)
	}
	return nil
}

// chosen is the newest maxLogs run logs, and every included path beside them.
func chosen(logDir string, include []string) []string {
	type log struct {
		path string
		at   time.Time
	}
	var logs []log
	entries, err := os.ReadDir(logDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			logs = append(logs, log{path: filepath.Join(logDir, entry.Name()), at: info.ModTime()})
		}
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].at.After(logs[j].at) })
	if len(logs) > maxLogs {
		logs = logs[:maxLogs]
	}

	out := make([]string, 0, len(logs)+len(include))
	taken := map[string]bool{}
	for _, l := range logs {
		out = append(out, l.path)
		taken[filepath.Base(l.path)] = true
	}
	for _, path := range include {
		if !taken[filepath.Base(path)] {
			out = append(out, path)
		}
	}
	return out
}

// tail is the last of one file, whole where it fits the cap.
func tail(path string, limit int64) ([]byte, error) {
	assert.Assert(limit > 0, "a tail keeps something", limit)

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		if _, err := file.Seek(-limit, io.SeekEnd); err != nil {
			return nil, err
		}
	}
	return io.ReadAll(file)
}
