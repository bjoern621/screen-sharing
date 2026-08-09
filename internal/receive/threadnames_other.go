//go:build !windows

package receive

// ignoreThreadNameExceptions is Windows-only. Everywhere else a thread is named
// through a system call that returns a value, so there is no notification for
// anything to mistake for a fault.
func ignoreThreadNameExceptions() {}
