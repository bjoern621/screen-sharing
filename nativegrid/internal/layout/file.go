package layout

import (
	"encoding/json"
	"os"
	"path/filepath"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// dirName and fileName put the remembered layout beside the app's settings.json,
// the directory os.UserConfigDir resolves for both processes (%APPDATA% on
// Windows, XDG_CONFIG_HOME or ~/.config on Linux). The directory name is
// repeated from desktop/settings rather than shared because no Go symbol
// crosses the module boundary.
const (
	dirName  = "screenshare"
	fileName = "nativegrid.json"

	dirMode  = 0o755
	fileMode = 0o644
)

// FileStore keeps the layout in a JSON file in the app's config directory.
type FileStore struct {
	path string
}

// NewFileStore resolves the state file's path and creates its directory. A
// config directory that cannot be resolved falls back to the working directory,
// the same fallback the app's settings package takes.
func NewFileStore() *FileStore {
	base, err := os.UserConfigDir()
	if err != nil {
		logger.Warnf("no user config directory, keeping the grid layout in the working directory: %v", err)
		base = "."
	}
	dir := filepath.Join(base, dirName)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		logger.Warnf("cannot create %s: %v", dir, err)
	}
	return &FileStore{path: filepath.Join(dir, fileName)}
}

// Load reads the remembered layout. A missing file yields the zero Layout,
// which is a first run: nothing watched, no remembered order. A corrupt file is
// reported and treated the same way, so a bad file cannot keep the window from
// opening.
func (f *FileStore) Load() Layout {
	assert.Assert(f.path != "", "a file store resolves its path at construction")

	data, err := os.ReadFile(f.path)
	if err != nil {
		logger.Debugf("no grid layout at %s: %v", f.path, err)
		return Layout{}
	}
	var l Layout
	if err := json.Unmarshal(data, &l); err != nil {
		logger.Warnf("grid layout at %s is corrupt, starting fresh: %v", f.path, err)
		return Layout{}
	}
	logger.Debugf("grid layout restored from %s", f.path)
	return l
}

// Save writes the layout, reporting a failed write rather than propagating it.
func (f *FileStore) Save(l Layout) {
	assert.Assert(f.path != "", "a file store resolves its path at construction")

	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		logger.Warnf("cannot encode the grid layout: %v", err)
		return
	}
	if err := os.WriteFile(f.path, data, fileMode); err != nil {
		logger.Warnf("cannot write %s: %v", f.path, err)
	}
}
