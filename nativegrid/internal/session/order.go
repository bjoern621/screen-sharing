package session

import (
	"slices"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// Order is the display order, as stream indexes over every stream the model
// knows. The result is the caller's copy: a view walks it while a drag reorders
// the model under it.
func (s *Session) Order() []int {
	return slices.Clone(s.order)
}

// SetOrder replaces the display order. A cancelled drag falls back through here
// to the order it started from.
func (s *Session) SetOrder(order []int) {
	s.assertPermutation(order)

	s.order = slices.Clone(order)
	s.notify(Change{Kind: OrderChanged, Index: noStream})
	s.persist.Schedule()
}

// Move re-slots stream from at stream to's position: past a later stream it lands
// right after it, past an earlier one right before it, so crossing into a
// neighbor swaps the two. This is what a drag commits, once per stream the pointer
// crosses.
func (s *Session) Move(from, to int) {
	s.at(from)
	s.at(to)
	if from == to {
		return
	}
	fromPos := slices.Index(s.order, from)
	// toPos reads before the delete: past a removed earlier slot it lands after
	// to, otherwise before it.
	toPos := slices.Index(s.order, to)
	assert.Assert(fromPos >= 0 && toPos >= 0, "both streams hold a slot in the display order", fromPos, toPos)

	s.order = slices.Delete(s.order, fromPos, fromPos+1)
	s.order = slices.Insert(s.order, toPos, from)
	s.assertPermutation(s.order)
	logger.Debugf("moved %q to slot %d", s.at(from).stream.Name, toPos)

	s.notify(Change{Kind: OrderChanged, Index: noStream})
	s.persist.Schedule()
}

// placeInOrder slots stream i into the display order. It runs on the append that
// made i a stream, so the order covers every stream again once it returns.
func (s *Session) placeInOrder(i int) {
	s.order = slices.Insert(s.order, s.slotFor(i), i)
	s.assertPermutation(s.order)
}

// slotFor is where the remembered order puts stream i: after every remembered
// stream that came before it, before the rest. A stream the remembered order
// does not list goes last, which is where a stream the window has never shown
// belongs.
func (s *Session) slotFor(i int) int {
	rank := slices.Index(s.savedOrder, s.at(i).stream.Name)
	if rank < 0 {
		return len(s.order)
	}
	for p, si := range s.order {
		r := slices.Index(s.savedOrder, s.at(si).stream.Name)
		if r < 0 || r > rank {
			return p
		}
	}
	return len(s.order)
}

// assertPermutation holds the display order's invariant: every stream the model
// knows holds exactly one slot. A view that drops or duplicates an index would
// otherwise lose a tile or attach one twice.
//
// It runs on the order a caller offers and on the order every mutation here
// leaves behind: a permutation checked only where it is built says nothing about
// the paths that reshuffle it.
func (s *Session) assertPermutation(order []int) {
	assert.Assert(len(order) == len(s.entries), "the display order covers every stream", len(order), len(s.entries))

	seen := make([]bool, len(s.entries))
	for _, i := range order {
		s.at(i)
		assert.Assert(!seen[i], "the display order holds a stream once", i)
		seen[i] = true
	}
}
