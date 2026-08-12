package rules

import "bjoernblessin.de/go-utils/util/assert"

// Match is what one axis has to read, or which of a control's values a verdict takes.
//
// The same type serves both because they are the same question asked in two directions:
// a rule binds where the facts match and lands on the values that match, and writing the
// second as a bare list would leave a numeric control unable to say "everything above
// this". A codec's bitrate ceiling is therefore a rule like every other constraint,
// rather than a column with a consumer of its own.
//
// The zero Match is everything. In When that would be an axis the rule does not care
// about, which is written by leaving the axis out instead, and registration refuses it.
// In Values it is every value the control has, which is how a whole control is greyed.
type Match struct {
	// any is the values that match, for a text axis.
	any []string
	// low and high bound a numeric match, inclusive, each read only where its flag is
	// set. Two flags rather than a sentinel: zero is a legal bitrate and a legal
	// quantizer, so a sentinel would make one real value unmatchable.
	low     int
	high    int
	hasLow  bool
	hasHigh bool
	// numeric records which of the two halves above was written, so a match built for a
	// numeric control cannot be silently read as an empty list of identifiers.
	numeric bool
}

// OneOf matches any of these identifiers.
func OneOf(values ...string) Match {
	assert.Assert(len(values) > 0, "a value match names at least one value")
	for _, v := range values {
		assert.Assert(v != "", "a matched value is one the domain declares")
	}
	return Match{any: values}
}

// AtLeast matches every number from n upward.
func AtLeast(n int) Match {
	return Match{low: n, hasLow: true, numeric: true}
}

// AtMost matches every number up to and including n.
func AtMost(n int) Match {
	return Match{high: n, hasHigh: true, numeric: true}
}

// Between matches the closed band from low to high.
func Between(low, high int) Match {
	assert.Assert(low <= high, "a band runs from its low end to its high one", low, high)
	return Match{low: low, high: high, hasLow: true, hasHigh: true, numeric: true}
}

// everything reports whether this match is the zero one, which names no value and
// therefore stands for all of them.
func (m Match) everything() bool {
	return len(m.any) == 0 && !m.hasLow && !m.hasHigh
}

// binds reports whether a reading satisfies this match.
func (m Match) binds(v Value) bool {
	if m.everything() {
		return true
	}
	if m.numeric {
		n := v.Number()
		return (!m.hasLow || n >= m.low) && (!m.hasHigh || n <= m.high)
	}
	for _, want := range m.any {
		if want == v.Text() {
			return true
		}
	}
	return false
}

// listed is the identifiers this match names, empty for a numeric one.
func (m Match) listed() []string {
	return m.any
}

// narrow removes this match's band from the offered range, returning what is left.
//
// It is what turns a numeric refusal into the ends a control is offered between, so the
// slider and the refusal cannot disagree. A band that clips neither end would leave a
// hole in the middle, which no offered range can express, and it is asserted against
// rather than approximated: a control silently offering a value the publish refuses is
// the failure this whole system exists to remove.
func (m Match) narrow(low, high int) (int, int) {
	assert.Assert(m.numeric, "a range is narrowed by a numeric refusal")
	assert.Assert(!m.everything(), "a narrowed range is refused in part, not entirely")
	assert.Assert(low <= high, "an offered range runs from its low end to its high one", low, high)

	switch {
	case m.hasLow && !m.hasHigh:
		// Everything from low upward is refused, so what is left ends below it.
		return low, min(high, m.low-1)
	case m.hasHigh && !m.hasLow:
		return max(low, m.high+1), high
	case m.low <= low && m.high >= high:
		// The band covers the whole range. Nothing is left, which the caller reads as a
		// control with no legal value rather than as an inverted range.
		return low, low - 1
	case m.low <= low:
		return m.high + 1, high
	case m.high >= high:
		return low, m.low - 1
	default:
		assert.Never("a numeric refusal clips an end of the offered range", m.low, m.high, low, high)
		return low, high
	}
}
