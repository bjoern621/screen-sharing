package layout

import "bjoernblessin.de/go-utils/util/assert"

// MergeOrder folds the order shown now into the remembered one. Names the
// current run never saw keep their place relative to the streams they sat
// behind, so watching a subset of the roster does not forget where the rest
// belongs. A name in both lists lands where the current run has it.
func MergeOrder(saved, current []string) []string {
	assertRanking(saved)
	assertRanking(current)

	shown := make(map[string]bool, len(current))
	for _, n := range current {
		shown[n] = true
	}
	// Each unseen name is filed behind the last shown name in front of it; the
	// empty key holds the ones that lead the remembered order.
	behind := map[string][]string{}
	anchor := ""
	unseen := 0
	for _, n := range saved {
		if shown[n] {
			anchor = n
			continue
		}
		behind[anchor] = append(behind[anchor], n)
		unseen++
	}
	out := make([]string, 0, len(saved)+len(current))
	out = append(out, behind[""]...)
	for _, n := range current {
		out = append(out, n)
		out = append(out, behind[n]...)
	}
	assert.Assert(len(out) == len(current)+unseen, "the merged order ranks every shown stream and every remembered one", len(out), len(current), unseen)

	return out
}

// assertRanking holds what both lists are: a ranking of streams by name, which
// only means something while a name stands for one stream. A repeated name
// ranks two streams the same, and the merge would file the same stream twice.
func assertRanking(names []string) {
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		assert.Assert(n != "", "a ranked stream carries a name")
		assert.Assert(!seen[n], "a ranking holds a stream once", n)

		seen[n] = true
	}
}
