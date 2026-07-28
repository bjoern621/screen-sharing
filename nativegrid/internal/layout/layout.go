// Package layout keeps what the grid window reopens on: the arrangement (the
// watched streams, their display order, and the spotlit one) and the window's
// own geometry.
//
// The two records have different owners. internal/session writes the
// arrangement, the window writes its geometry, and a writer touches only the
// keys of the record it holds, so neither erases the other (see file.go).
//
// The Store seam separates what is remembered from where it is kept: FileStore
// writes the app's config directory, Memory holds it in the process for a test.
package layout

import "slices"

// Layout is the arrangement carried across runs. Streams are keyed by name
// because a stream index only means something within one run. Both lists keep
// names the current roster does not offer, so a machine that goes away and
// comes back finds the slot and the watch state it left.
type Layout struct {
	Order   []string `json:"order"`
	Watched []string `json:"watched"`
	Spot    string   `json:"spot,omitempty"`
}

// Store reads and writes the remembered layout.
//
// Neither call reports an error. A layout that cannot be read is a first run,
// and a write that fails costs the next run's arrangement and nothing else, so
// an implementation reports the failure itself and returns the zero Layout.
type Store interface {
	Load() Layout
	Save(l Layout)
}

// Memory keeps both remembered records in the process, for tests and for a run
// that must not touch the config directory. It is a Store and a WindowStore, so
// one instance stands in for the state file the two share.
//
// What it holds is its own copy, and so is what it hands out. FileStore's
// records cross a JSON encoder and reach neither side by reference; a caller
// that kept writing the slice it saved would otherwise change what is
// remembered here and nowhere else.
type Memory struct {
	Layout Layout
	Window WindowState
}

func (m *Memory) Load() Layout { return copyOf(m.Layout) }

func (m *Memory) Save(l Layout) { m.Layout = copyOf(l) }

// copyOf is one arrangement with its own slices.
func copyOf(l Layout) Layout {
	l.Order = slices.Clone(l.Order)
	l.Watched = slices.Clone(l.Watched)
	return l
}

func (m *Memory) LoadWindow() WindowState { return m.Window }

func (m *Memory) SaveWindow(w WindowState) { m.Window = w }
