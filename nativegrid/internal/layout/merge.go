package layout

// MergeOrder folds the order shown now into the remembered one. Names the
// current run never saw keep their place relative to the streams they sat
// behind, so watching a subset of the roster does not forget where the rest
// belongs. A name in both lists lands where the current run has it.
func MergeOrder(saved, current []string) []string {
	shown := make(map[string]bool, len(current))
	for _, n := range current {
		shown[n] = true
	}
	// Each unseen name is filed behind the last shown name in front of it; the
	// empty key holds the ones that lead the remembered order.
	behind := map[string][]string{}
	anchor := ""
	for _, n := range saved {
		if shown[n] {
			anchor = n
			continue
		}
		behind[anchor] = append(behind[anchor], n)
	}
	out := make([]string, 0, len(saved)+len(current))
	out = append(out, behind[""]...)
	for _, n := range current {
		out = append(out, n)
		out = append(out, behind[n]...)
	}
	return out
}
