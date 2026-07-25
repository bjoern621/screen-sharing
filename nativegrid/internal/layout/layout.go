// Package layout keeps the arrangement the grid window reopens on: the watched
// streams, their display order, and the spotlit one.
//
// The Store seam separates what is remembered from where it is kept: FileStore
// writes the app's config directory, Memory holds it in the process for a test.
package layout

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

// Memory is a Store that keeps the layout in the process, for tests and for a
// run that must not touch the config directory.
type Memory struct {
	Layout Layout
}

func (m *Memory) Load() Layout { return m.Layout }

func (m *Memory) Save(l Layout) { m.Layout = l }
