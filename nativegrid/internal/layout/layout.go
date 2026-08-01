// Package layout keeps what the grid window reopens on: the arrangement (the
// watched streams, their display order, and the spotlit one), the render chains
// it draws them through, and the window's own geometry.
//
// The three records have two owners. internal/session writes the arrangement and
// the render chains, the window writes its geometry, and a writer touches only the
// keys of the record it holds, so neither erases the other (see file.go).
//
// The Store seam separates what is remembered from where it is kept: FileStore
// writes the app's config directory, Memory holds it in the process for a test.
package layout

import (
	"maps"
	"slices"
)

// Layout is the arrangement carried across runs. Streams are keyed by name
// because a stream index only means something within one run. Both lists keep
// names the current roster does not offer, so a machine that goes away and
// comes back finds the slot and the watch state it left.
type Layout struct {
	Order   []string `json:"order"`
	Watched []string `json:"watched"`
	Spot    string   `json:"spot,omitempty"`
}

// Store reads and writes the two records internal/session owns: the arrangement
// and the render chains. They share one interface because they share one owner,
// which is what splits WindowStore off rather than which file the records live in.
//
// No call reports an error. A record that cannot be read is a first run, and a
// write that fails costs the next run's arrangement and nothing else, so an
// implementation reports the failure itself and returns the zero record.
type Store interface {
	Load() Layout
	Save(l Layout)
	LoadRender() Render
	SaveRender(r Render)
}

// Memory keeps every remembered record in the process, for tests and for a run
// that must not touch the config directory. It is a Store and a WindowStore, so
// one instance stands in for the state file they share.
//
// What it holds is its own copy, and so is what it hands out. FileStore's
// records cross a JSON encoder and reach neither side by reference; a caller
// that kept writing the slice or map it saved would otherwise change what is
// remembered here and nowhere else.
type Memory struct {
	Layout Layout
	Window WindowState
	Render Render
}

func (m *Memory) Load() Layout { return copyOf(m.Layout) }

func (m *Memory) Save(l Layout) { m.Layout = copyOf(l) }

// copyOf is one arrangement with its own slices.
func copyOf(l Layout) Layout {
	l.Order = slices.Clone(l.Order)
	l.Watched = slices.Clone(l.Watched)
	return l
}

func (m *Memory) LoadRender() Render { return copyOfRender(m.Render) }

func (m *Memory) SaveRender(r Render) { m.Render = copyOfRender(r) }

// copyOfRender is one render choice with its own map.
func copyOfRender(r Render) Render {
	r.Streams = maps.Clone(r.Streams)
	return r
}

func (m *Memory) LoadWindow() WindowState { return m.Window }

func (m *Memory) SaveWindow(w WindowState) { m.Window = w }
