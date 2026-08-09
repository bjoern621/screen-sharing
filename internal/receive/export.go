package receive

import (
	"errors"
	"sync"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// One consumer's subscription to one receiver's frames: the slot bookkeeping, the
// serials, and the drops. What a slot physically is stays in share.go and the
// platform files; what is here is whose turn it is to touch one.
//
// The protocol is a loan. Every frame handed over takes a slot out of the pool, and
// the slot comes back only when the consumer says so. A consumer that stops releasing
// therefore slows to its own rate and never sees a torn picture; it does not stall the
// decode, which keeps running and drops the frames it has nowhere to put. That is the
// property the whole design is for: a window that is busy costs frames and never
// costs the pipeline.

// Frame is one handed-over frame.
type Frame struct {
	Generation uint64
	Slot       int
	Serial     uint64
	// Dropped is how many decoded frames were discarded since the last handover
	// because every slot was still out on loan.
	Dropped uint64
}

// Event is one thing a subscription says. Exactly one field is set.
type Event struct {
	Pool  *Pool
	Frame *Frame
}

// eventBuffer is how many events a subscription queues for a consumer that is not
// reading yet.
//
// It is bounded by the loan rather than by this number: at most slotCount frames can
// be outstanding, so the queue is only ever this deep when a pool announcement and a
// full set of loans coincide. A queue that does fill is a consumer that has stopped
// reading its own stream, which is not a case to buffer through.
const eventBuffer = slotCount + 4

// Subscription is one consumer's view of one receiver's frames.
//
// It is created by Subscribe and lives until Close or until the pipeline ends. Both
// ends of that are visible on Events: the channel closes, and Err says why.
type Subscription struct {
	receiver *Receiver
	events   chan Event
	// done is closed with the events channel, so a producer that races a Close does
	// not send on a closed channel: every send checks it under the same lock.
	done chan struct{}

	mu sync.Mutex
	// shared is the platform's exporter, opened on the first frame and re-opened
	// whenever the pipeline renegotiates.
	shared sharer
	// generation counts the pools handed to this consumer, from one. A release naming
	// an older one is discarded: the slot numbers start again with each pool, so
	// honouring it would free a slot of a pool it was never lent from.
	generation uint64
	serial     uint64
	dropped    uint64
	// width and height are the pool's size, kept so a frame that no longer fits is
	// recognised as a renegotiation rather than written into a slot too small for it.
	width, height int
	// lent is the serial each slot was lent for, and zero for a slot this side may
	// write into.
	lent [slotCount]uint64
	// renderSize is what this consumer asked to be drawn at, width packed over
	// height. The receiver takes the largest of its consumers' asks.
	renderSize uint64
	ended      bool
	err        error
}

// Subscribe opens one consumer's view of this receiver's frames.
//
// It takes no pool and allocates nothing: what a pool has to match is the memory a
// frame turned out to be in, and no frame has arrived yet. The first one opens it, and
// the consumer learns the pool as the subscription's first event.
func (r *Receiver) Subscribe() *Subscription {
	sub := &Subscription{
		receiver: r,
		events:   make(chan Event, eventBuffer),
		done:     make(chan struct{}),
		shared:   newSharer(),
	}

	r.mu.Lock()
	r.subs = append(r.subs, sub)
	r.mu.Unlock()

	logger.Debugf("stream %q gained a frame consumer", r.name)
	return sub
}

// Events is what the consumer reads. It closes when the subscription ends, from either
// side, and Err then says why.
func (s *Subscription) Events() <-chan Event { return s.events }

// Err is why the subscription ended, and nil while it runs or after an ordinary Close.
func (s *Subscription) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Release hands one slot back.
//
// A release naming a pool that is gone is discarded rather than refused: a pool
// changes while frames of the old one are still in flight, and the consumer released
// what it had been handed. A release naming a serial the slot was not lent for is the
// same case one step finer, and is discarded for the same reason.
func (s *Subscription) Release(generation uint64, slot int, serial uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ended || generation != s.generation || slot < 0 || slot >= slotCount {
		return
	}
	if s.lent[slot] != serial {
		return
	}
	s.lent[slot] = 0
}

// SetRenderSize says how many pixels this consumer will draw the frames at.
//
// The receiver renders at the largest of what its consumers asked for, because the
// pipeline has one scaler and a size is a bound rather than a demand: rendering at the
// largest ask means the smallest consumer scales a picture down at draw time, where
// rendering at the smallest would mean the largest one scaling one up.
func (s *Subscription) SetRenderSize(width, height int) {
	if width < 0 || height < 0 {
		return
	}

	s.mu.Lock()
	s.renderSize = uint64(uint32(width))<<32 | uint64(uint32(height))
	ended := s.ended
	s.mu.Unlock()

	if !ended {
		s.receiver.applyRenderSize()
	}
}

// Close ends the subscription from the consumer's side and frees the pool.
//
// It is idempotent, and it is what a dropped connection runs: the handles the consumer
// held name nothing afterwards, which is the property that makes a shell that crashed
// cost this process nothing.
func (s *Subscription) Close() {
	s.receiver.dropSub(s)
	s.finish(nil)
}

// finish ends the subscription once, with the reason it ended for.
func (s *Subscription) finish(err error) {
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		return
	}
	s.ended = true
	s.err = err
	s.shared.close()
	close(s.done)
	close(s.events)
	s.mu.Unlock()

	// Outside the lock: the receiver takes its own, and the render size it recomputes
	// reads every remaining subscription's.
	s.receiver.applyRenderSize()
}

// offer hands one decoded frame to this consumer, and is the whole of the producer
// side. It runs on the sink's streaming thread and never blocks: everything it cannot
// do now is a dropped frame rather than a wait.
func (s *Subscription) offer(sample *gst.Sample) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ended {
		return
	}

	if err := s.poolFor(sample); err != nil {
		// A pool that cannot be opened is not a frame that can be retried: the memory
		// the chain converts into does not change mid-stream, so the next frame fails
		// the same way. The subscription ends and the consumer is told why.
		s.endLocked(err)
		return
	}

	slot := s.freeSlot()
	if slot < 0 {
		s.dropped++
		return
	}

	if err := s.shared.write(slot, sample); err != nil {
		// One frame rather than the stream: a copy can fail on a device that is busy
		// or on a consumer that is still holding the slot, and the next frame is
		// offered normally.
		s.dropped++
		logger.Debugf("stream %q could not fill slot %d: %v", s.receiver.name, slot, err)
		return
	}

	s.serial++
	frame := Frame{Generation: s.generation, Slot: slot, Serial: s.serial, Dropped: s.dropped}
	s.lent[slot] = s.serial

	select {
	case s.events <- Event{Frame: &frame}:
		s.dropped = 0
	default:
		// The consumer has stopped reading its own stream. The slot goes back rather
		// than being lent to nobody, and the frame counts as dropped like any other
		// the consumer had no room for.
		s.lent[slot] = 0
		s.dropped++
	}
}

// poolFor is the pool this frame belongs in, opening a new one where there is none or
// where the frame no longer fits the one there is.
//
// A size change is the renegotiation that matters: the source resized, or the tile's
// own render size moved the scaler's output. Both arrive here as a frame whose caps
// disagree with the pool, and both are answered the same way, because a slot is
// allocated at one size and a picture of another size cannot be copied into it.
func (s *Subscription) poolFor(sample *gst.Sample) error {
	width, height := frameSize(sample)
	if s.generation > 0 && (width == 0 || (width == s.width && height == s.height)) {
		return nil
	}

	pool, err := s.shared.open(sample, slotCount)
	if err != nil {
		return err
	}

	s.generation++
	s.serial = 0
	s.dropped = 0
	s.width, s.height = pool.Width, pool.Height
	s.lent = [slotCount]uint64{}
	pool.Generation = s.generation

	assert.Assert(len(pool.Slots) > 1, "a lent pool holds a slot to draw from and one to write into",
		s.receiver.name, len(pool.Slots))

	select {
	case s.events <- Event{Pool: &pool}:
	default:
		return errors.New("the consumer stopped reading before it was told where the frames are")
	}
	return nil
}

// freeSlot is a slot the consumer is not holding, and -1 when it holds all of them.
func (s *Subscription) freeSlot() int {
	for i, serial := range s.lent {
		if serial == 0 {
			return i
		}
	}
	return -1
}

// endLocked ends the subscription from the producer's side, with the lock already
// held. It is the same finish the consumer's Close runs, minus the lock dance: the
// receiver is dropped from outside, because taking its lock under this one is the
// order the teardown path takes the other way round.
func (s *Subscription) endLocked(err error) {
	s.ended = true
	s.err = err
	s.shared.close()
	close(s.done)
	close(s.events)
	logger.Warnf("stream %q stopped feeding a consumer: %v", s.receiver.name, err)
}

// frameSize is the frame's own size, off the caps the sample carries. Zero where the
// sample carries none, which leaves an open pool where it is rather than reopening it
// on a fact nobody stated.
func frameSize(sample *gst.Sample) (int, int) {
	caps := sample.GetCaps()
	if caps == nil || caps.GetSize() == 0 {
		return 0, 0
	}
	st := caps.GetStructure(0)
	width, wok := st.GetInt("width")
	height, hok := st.GetInt("height")
	if !wok || !hok {
		return 0, 0
	}
	return int(width), int(height)
}

// --- The receiver's side --------------------------------------------------------

// fanOut hands one sample to every consumer. It runs on the sink's streaming thread,
// so what it must not do is wait: each consumer's offer drops rather than blocks, and
// a consumer whose subscription ended under it is dropped from the list here.
func (r *Receiver) fanOut(sample *gst.Sample) {
	r.mu.Lock()
	subs := make([]*Subscription, len(r.subs))
	copy(subs, r.subs)
	r.mu.Unlock()

	for _, sub := range subs {
		sub.offer(sample)
	}

	// A subscription the producer ended is taken off the list on the next frame rather
	// than from inside offer, which would mean editing the list this loop walks.
	r.sweepSubs()
}

// sweepSubs drops the subscriptions that have ended.
func (r *Receiver) sweepSubs() {
	r.mu.Lock()
	kept := r.subs[:0]
	dropped := false
	for _, sub := range r.subs {
		select {
		case <-sub.done:
			dropped = true
		default:
			kept = append(kept, sub)
		}
	}
	r.subs = kept
	r.mu.Unlock()

	if dropped {
		r.applyRenderSize()
	}
}

// dropSub takes one subscription off the receiver.
func (r *Receiver) dropSub(sub *Subscription) {
	r.mu.Lock()
	kept := r.subs[:0]
	for _, held := range r.subs {
		if held != sub {
			kept = append(kept, held)
		}
	}
	r.subs = kept
	r.mu.Unlock()
}

// endSubs tells every consumer the pipeline is over, in the words the pipeline used.
// It is the same fact ReceiveExit carries on the control stream, said on the call each
// consumer is actually blocked reading.
func (r *Receiver) endSubs(message string) {
	r.mu.Lock()
	subs := r.subs
	r.subs = nil
	r.mu.Unlock()

	for _, sub := range subs {
		sub.finish(errors.New(message))
	}
}

// applyRenderSize writes the largest size any consumer asked for onto the pipeline,
// and does nothing while none has asked.
func (r *Receiver) applyRenderSize() {
	r.mu.Lock()
	var width, height int
	for _, sub := range r.subs {
		sub.mu.Lock()
		size := sub.renderSize
		sub.mu.Unlock()

		if w := int(uint32(size >> 32)); w > width {
			width = w
		}
		if h := int(uint32(size)); h > height {
			height = h
		}
	}
	r.mu.Unlock()

	r.SetRenderSize(width, height)
}
