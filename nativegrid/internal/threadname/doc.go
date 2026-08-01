// Package threadname keeps a native library's thread-naming exception from
// ending the process.
//
// Only Windows reports a thread's name that way, so everywhere else Ignore does
// nothing and the call site needs no build tag of its own.
package threadname
