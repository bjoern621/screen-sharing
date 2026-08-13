package rules

import "bjoernblessin.de/go-utils/util/assert"

// Match is what one axis has to read, or which of a control's values a verdict takes.
//
// One type for both, they being one question asked in two directions: a rule binds where the facts
// match and lands on the values that match.
// A bare list would leave a numeric control unable to say "everything above this", so a codec's
// bitrate ceiling is a rule like every other constraint rather than a column with a consumer of its
// own.
//
// The zero Match is everything.
// In When that is an axis the rule does not care about, which is written by leaving the axis out,
// and registration refuses it.
// In Values it is every value the control has, which is how a whole control is greyed.
type Match struct {
	// Identifiers that match, for a text axis.
	any []string
	// Inclusive numeric bounds, each read only where its flag is set.
	// Flags rather than a sentinel: zero is a legal bitrate and a legal quantizer, and a sentinel
	// would make one real value unmatchable.
	low     int
	high    int
	hasLow  bool
	hasHigh bool
	// Which of the two halves above was written, so a match built for a numeric control is never read
	// as an empty list of identifiers.
	numeric bool
}

func OneOf(values ...string) Match {
	assert.Assert(len(values) > 0, "a value match names at least one value")
	for _, v := range values {
		assert.Assert(v != "", "a matched value is one the domain declares")
	}
	return Match{any: values}
}

// AtLeast matches n and every number above it.
func AtLeast(n int) Match {
	return Match{low: n, hasLow: true, numeric: true}
}

// AtMost matches n and every number below it.
func AtMost(n int) Match {
	return Match{high: n, hasHigh: true, numeric: true}
}

// Between matches the closed band low..high.
func Between(low, high int) Match {
	assert.Assert(low <= high, "a band runs from its low end to its high one", low, high)
	return Match{low: low, high: high, hasLow: true, hasHigh: true, numeric: true}
}

// everything reports whether the match names nothing, which stands for every value.
func (m Match) everything() bool {
	return len(m.any) == 0 && !m.hasLow && !m.hasHigh
}

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

// listed is the identifiers this match names, empty for a numeric match.
func (m Match) listed() []string {
	return m.any
}

// narrow takes this match's band off the offered range and returns what is left, which is what
// turns a numeric refusal into the ends a control is offered between.
//
// A band clipping neither end would leave a hole in the middle, which no offered range can express.
// That asserts rather than being approximated: a control offering a value the publish refuses is
// the failure this system exists to remove.
func (m Match) narrow(low, high int) (int, int) {
	assert.Assert(m.numeric, "a range is narrowed by a numeric refusal")
	assert.Assert(!m.everything(), "a narrowed range is refused in part, not entirely")
	assert.Assert(low <= high, "an offered range runs from its low end to its high one", low, high)

	switch {
	case m.hasLow && !m.hasHigh:
		// Refused from the band's low end upward, so what is left ends below it.
		return low, min(high, m.low-1)
	case m.hasHigh && !m.hasLow:
		return max(low, m.high+1), high
	case m.low <= low && m.high >= high:
		// The band covers the range, so nothing is left.
		// A high end below the low one reads as a control with no legal value, not as an inverted range.
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
