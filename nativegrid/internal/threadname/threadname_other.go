//go:build !windows

package threadname

// Ignore does nothing: no platform outside Windows reports a thread's name by
// raising an exception.
func Ignore() {}
