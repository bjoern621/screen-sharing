package receive

import (
	"errors"
	"sync"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// One consumer's subscription to one receiver's frames: slots, serials and drops.
// What a slot physically is stays in share.go and the platform files.
//
// A frame is a loan: it takes a slot out of the pool, and the slot comes back only when
// the consumer releases it.
// A consumer that stops releasing runs at its own rate and never sees a torn picture, and
// the decode keeps running and drops the frames it has nowhere to put.

type Frame struct {
	Generation uint64
	Slot       int
	Serial     uint64
	// Dropped counts the decoded frames discarded since the last handover, every slot
	// having been out on loan.
	Dropped uint64
}

// Event is what a subscription emits, with exactly one field set.
type Event struct {
	Pool  *Pool
	Frame *Frame
}

// eventBuffer bounds the queue a consumer that is not reading yet builds up.
//
// The loan is the real bound: at most slotCount frames are outstanding, so the queue reaches
// this depth only where a pool announcement meets a full set of loans.
// A queue that fills is a consumer that stopped reading its own stream, which is not a case
// to buffer through.
const eventBuffer = slotCount + 4

// Subscription is one consumer's view of one receiver's frames.
//
// Ends on Close or with the pipeline, and both show on Events:
// the channel closes, and Err says why.
type Subscription struct {
	receiver *Receiver
	events   chan Event
	// done closes with the events channel, and every send checks it under mu, so
	// a producer racing a Close cannot send on a closed channel.
	done chan struct{}

	mu sync.Mutex
	// shared opens on the first frame and re-opens on every renegotiation.
	shared sharer
	// generation counts this consumer's pools, from one.
	// Slot numbers restart with each pool, so a release naming an older one is discarded
	// rather than freeing a slot of the pool that replaced it.
	generation uint64
	serial     uint64
	dropped    uint64
	// width and height are the open pool's size.
	// A frame of another size is a renegotiation,
	// not something to write into a slot too small for it.
	width, height int
	// lent holds the serial each slot was lent for.
	// Zero: free for this side to write.
	lent [slotCount]uint64
	// renderSize is this consumer's ask.
	// The receiver draws at the largest across its consumers.
	renderSize renderSize
	ended      bool
	err        error
}

// Subscribe opens one consumer's view of this receiver's frames.
//
// Allocates no pool: a pool matches the memory a frame turned out to be in,
// and no frame has arrived.
// The first one opens it, and the consumer learns the pool as the subscription's first event.
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

// Events closes when the subscription ends from either side, and Err then says why.
func (s *Subscription) Events() <-chan Event { return s.events }

// Err is nil while the subscription runs and after an ordinary Close.
func (s *Subscription) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Release hands one slot back.
//
// A release naming a pool that is gone, or a serial the slot was not lent for, is discarded
// rather than refused: pools change while frames of the old one are in flight, and the consumer
// released what it had been handed.
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

// SetRenderSize says how many pixels this consumer draws the frames at.
//
// One scaler serves every consumer and a size is a bound rather than a demand, so the receiver
// renders at the largest ask: the smallest consumer then scales a picture down at draw time,
// where the smallest ask would have the largest consumer scaling one up.
func (s *Subscription) SetRenderSize(width, height int) {
	if width < 0 || height < 0 {
		return
	}

	s.mu.Lock()
	s.renderSize = packSize(width, height)
	ended := s.ended
	s.mu.Unlock()

	if !ended {
		s.receiver.applyRenderSize()
	}
}

// Close ends the subscription from the consumer's side and frees the pool.
//
// Idempotent, and what a dropped connection runs.
// Every handle the consumer held names nothing afterwards, so a shell that crashed costs this
// process nothing.
func (s *Subscription) Close() {
	s.receiver.dropSub(s)
	s.finish(nil)
}

// finish ends the subscription once, with its reason.
// endLocked with the lock taken around it,
// so the producer's way of ending and the consumer's are one sequence.
func (s *Subscription) finish(err error) {
	s.mu.Lock()
	ended := s.ended
	if !ended {
		s.endLocked(err)
	}
	s.mu.Unlock()

	if ended {
		return
	}

	// Outside the lock: the receiver takes its own and reads every remaining
	// subscription's size under it.
	s.receiver.applyRenderSize()
}

// offer hands one decoded frame to this consumer, and is the whole producer side.
//
// Runs on the sink's streaming thread and never blocks:
// what it cannot do now is a dropped frame rather than a wait.
func (s *Subscription) offer(sample *gst.Sample) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ended {
		return
	}

	if err := s.poolFor(sample); err != nil {
		// The memory the chain converts into does not change mid-stream, so the next
		// frame fails the same way.
		// The subscription ends and the consumer is told why, rather than retrying.
		logger.Warnf("stream %q stopped feeding a consumer: %v", s.receiver.name, err)
		s.endLocked(err)
		return
	}

	slot := s.freeSlot()
	if slot < 0 {
		s.dropped++
		return
	}

	if err := s.shared.write(slot, sample); err != nil {
		// One frame rather than the stream: a copy fails on a busy device or on a slot
		// the consumer is still holding, and the next frame is offered normally.
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
		// The consumer stopped reading its own stream.
		// The slot goes back rather than being lent to nobody, and the frame counts
		// as dropped like any other the consumer had no room for.
		s.lent[slot] = 0
		s.dropped++
	}
}

// poolFor opens a pool where there is none, and where the frame has outgrown the one there is.
//
// A slot is allocated at one size, so a picture of another size cannot be copied into it.
// Either renegotiation, a source that resized or a render size that moved the scaler's output,
// arrives as a frame whose caps disagree with the pool.
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

// freeSlot yields -1 where every slot is out on loan.
func (s *Subscription) freeSlot() int {
	for i, serial := range s.lent {
		if serial == 0 {
			return i
		}
	}
	return -1
}

// endLocked is the whole of ending a subscription with mu held:
// the flags, the pool and both channels.
// Stated once and wrapped by finish,
// so the producer's path and the consumer's cannot end a subscription differently.
//
// The receiver is dropped from outside: taking its lock under this one inverts the order
// the teardown path takes them in.
func (s *Subscription) endLocked(err error) {
	assert.Assert(!s.ended, "a subscription ends once", s.receiver.name)

	s.ended = true
	s.err = err
	s.shared.close()
	close(s.done)
	close(s.events)
}

// frameSize is the size on the sample's caps, and zero where it carries none.
// Zero leaves an open pool where it is rather than reopening it on a fact nobody stated.
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

// fanOut hands one sample to every consumer.
//
// Runs on the sink's streaming thread and must not wait: every offer drops rather than blocks.
func (r *Receiver) fanOut(sample *gst.Sample) {
	r.mu.Lock()
	subs := make([]*Subscription, len(r.subs))
	copy(subs, r.subs)
	r.mu.Unlock()

	for _, sub := range subs {
		sub.offer(sample)
	}

	// A subscription the producer ended goes off the list on the next frame rather than
	// from inside offer, which would edit the list this loop walks.
	r.sweepSubs()
}

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

// endSubs tells every consumer the pipeline is over, in the pipeline's own words.
// ReceiveExit carries the same fact on the control stream.
// This says it on the call each consumer is blocked reading.
func (r *Receiver) endSubs(message string) {
	r.mu.Lock()
	subs := r.subs
	r.subs = nil
	r.mu.Unlock()

	for _, sub := range subs {
		sub.finish(errors.New(message))
	}
}

// applyRenderSize writes the largest of the consumers' asks onto the pipeline.
// With no consumer the ask is zero, which SetRenderSize leaves the pipeline alone for.
func (r *Receiver) applyRenderSize() {
	r.mu.Lock()
	var width, height int
	for _, sub := range r.subs {
		sub.mu.Lock()
		size := sub.renderSize
		sub.mu.Unlock()

		w, h := size.unpack()
		if w > width {
			width = w
		}
		if h > height {
			height = h
		}
	}
	r.mu.Unlock()

	r.SetRenderSize(width, height)
}
