package decode

import (
	"errors"
	"sync"
	"time"

	"bjoernblessin.de/screenshare/internal/pointer"
	"bjoernblessin.de/screenshare/internal/receive"
)

// What stands in for a pipeline, so the contract runs with no GPU.

// wait bounds every handover the tests wait on.
// Long enough that a loaded machine does not fail a passing case, short enough that a broken one
// reports rather than hangs.
const wait = 2 * time.Second

// released is what one release named, kept so a test can assert the consumer's release arrived.
type released struct {
	Generation uint64
	Slot       int
	Serial     uint64
}

// fakeSub stands in for receive.Subscription, driven by the test rather than by a pipeline.
type fakeSub struct {
	events chan receive.Event

	mu    sync.Mutex
	back  []released
	sizes [][2]int
	err   error
}

func newFakeSub() *fakeSub {
	return &fakeSub{events: make(chan receive.Event, 8)}
}

func (f *fakeSub) Events() <-chan receive.Event { return f.events }

func (f *fakeSub) Err() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.err
}

func (f *fakeSub) Release(generation uint64, slot int, serial uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.back = append(f.back, released{generation, slot, serial})
}

func (f *fakeSub) SetRenderSize(width, height int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sizes = append(f.sizes, [2]int{width, height})
}

func (f *fakeSub) Close() {}

// end stops the subscription the way a pipeline that failed does: the reason on Err, then
// the channel closed.
func (f *fakeSub) end(reason string) {
	f.mu.Lock()
	f.err = errors.New(reason)
	f.mu.Unlock()
	close(f.events)
}

func (f *fakeSub) releases() []released {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]released(nil), f.back...)
}

// fakeDecoder stands in for receive.Receiver.
type fakeDecoder struct {
	toneMap bool
	// onEnd is the host's own callback, which a test calls to stop the pipeline from inside.
	onEnd func(string)

	mu      sync.Mutex
	stats   receive.Stats
	volume  float64
	muted   bool
	spot    pointer.Spot
	hasSpot bool
	stopped bool
	subs    []*fakeSub
}

func (d *fakeDecoder) ToneMap() bool { return d.toneMap }

func (d *fakeDecoder) Stats() receive.Stats {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stats
}

func (d *fakeDecoder) Audio() (float64, bool, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.volume, d.muted, true
}

func (d *fakeDecoder) Level() (float64, float64, bool) { return -12, -18, true }

func (d *fakeDecoder) Pointer() (pointer.Spot, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.spot, d.hasSpot
}

func (d *fakeDecoder) SetAudio(volume float64, muted bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.volume, d.muted = volume, muted
}

func (d *fakeDecoder) Subscribe() Subscription {
	sub := newFakeSub()

	d.mu.Lock()
	d.subs = append(d.subs, sub)
	d.mu.Unlock()

	return sub
}

func (d *fakeDecoder) Stop() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stopped = true
	return true
}

func (d *fakeDecoder) wasStopped() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stopped
}
