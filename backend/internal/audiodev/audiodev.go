// Package audiodev reports what is inside each audio capture kind on this machine.
//
// Which kinds exist is declared (platform.AudioSources),
// and is the same on every machine of one operating system.
// What is inside a kind is not:
// which output this machine plays into and which application plays are facts about this moment,
// so they are read off the machine rather than listed anywhere.
//
// The reading is separated from the form's resolve for the reason the encoder probe's is.
// A form resolves on every keystroke and cannot pay for a subprocess,
// so a read here is a lock and a copy.
// What keeps the copy current is the daemon's own add and remove events,
// carried by one process that lives as long as this one (watch).
// An answer taken once would be wrong about the application that just launched,
// which is the entry worth selecting.
//
// PipeWire answers, and is the right server to ask rather than the convenient one.
// It alone reports the applications playing as nodes of their own, the per-application kind.
// Its sinks carry the names its Pulse server serves them under,
// so one enumeration describes what either engine opens:
// a pulsesrc takes a sink's monitor by name, and only a pipewiresrc takes one application's output.
//
// A machine with no PipeWire enumerates nothing and every kind keeps its own default,
// which is what an entry naming no device takes.
// Every failure on that path is an Umgebungsfehler,
// so nothing here asserts on what the daemon says.
package audiodev

import (
	"context"
	"sort"
	"sync"
	"time"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/platform"
)

// SeedWait bounds the first read alone.
// The watcher answers with the daemon's whole state before any event,
// so a read arriving before that has nothing to copy and waits for it.
// A machine running no daemon waits this out once and answers empty from then on.
const SeedWait = 3 * time.Second

// held is what the watcher last saw, keyed by the daemon's own object id.
//
// Keyed by id rather than by handle: an event names the object it is about by id,
// and a removal names nothing else at all.
var (
	mu   sync.Mutex
	held = map[int]platform.AudioDevice{}

	start  sync.Once
	seeded = make(chan struct{})
)

// Devices is what this machine offers inside each kind, as of the last event the daemon sent.
//
// The first call starts the watcher and waits for it to report the daemon's whole state,
// bounded by SeedWait.
// Every call after that is a lock and a copy, which is what lets a form resolve on a keystroke.
//
// The watcher outlives ctx: it is the process's, and ctx bounds the first wait alone.
func Devices(ctx context.Context) []platform.AudioDevice {
	assert.IsNotNil(ctx, "an enumeration runs under a context")

	start.Do(func() { go watch() })

	select {
	case <-seeded:
	case <-time.After(SeedWait):
	case <-ctx.Done():
	}

	mu.Lock()
	defer mu.Unlock()
	return snapshot()
}

// snapshot is the held devices in one order, held by the caller's lock.
//
// One order per machine, on every read:
// an option list is built from this,
// and a list reshuffled between two resolves would move an entry out from under the cursor.
func snapshot() []platform.AudioDevice {
	out := make([]platform.AudioDevice, 0, len(held))
	for _, d := range held {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ID < out[j].ID
	})

	for _, d := range out {
		assert.Assert(d.Kind != "" && d.ID != "",
			"an enumerated device names its kind and its handle", d.Kind, d.ID)
	}
	return out
}

// apply folds one batch of the daemon's objects into the held set.
//
// An object carrying no info is one the daemon destroyed, and its id is all a removal names.
// An object that sorts into no kind is dropped from the set,
// which is how a node that stopped being one of ours leaves.
// A partial update naming no media class leaves its entry standing:
// the daemon re-sends an object for changes of every sort,
// and reclassifying on one that says nothing about the class would drop a live device.
func apply(batch []node) {
	mu.Lock()
	defer mu.Unlock()

	for _, o := range batch {
		if o.Info == nil {
			delete(held, o.ID)
			continue
		}
		device, sorted, classified := sortNode(o)
		switch {
		case sorted:
			held[o.ID] = device
		case classified:
			delete(held, o.ID)
		}
	}
}

// reseed replaces the held set with one whole dump,
// so a watcher that reconnected describes the daemon it found rather than the one it left.
func reseed(batch []node) {
	fresh := map[int]platform.AudioDevice{}
	for _, o := range batch {
		if o.Info == nil {
			continue
		}
		if device, sorted, _ := sortNode(o); sorted {
			fresh[o.ID] = device
		}
	}

	mu.Lock()
	defer mu.Unlock()
	held = fresh
}

// markSeeded releases the first read, once.
func markSeeded() {
	select {
	case <-seeded:
	default:
		close(seeded)
	}
}
