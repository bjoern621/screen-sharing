//go:build !windows

package window

// List answers nothing on a session with no enumerator (see enumerators).
// An empty list rather than a refusal:
// the absence is stated once, by Statement, and a caller that asked anyway gets the same answer
// a machine with no windows open would give.
func List() []Window { return nil }
