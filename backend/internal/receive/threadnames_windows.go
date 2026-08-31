package receive

/*
#include <windows.h>

// The exception code MSVC's debugger protocol reserves for "this thread is now called X".
// A thread raises it with the name in the parameters and a debugger picks it up.
// With no debugger attached, nothing is supposed to happen at all.
#define SCREENSHARE_THREAD_NAME_EXCEPTION 0x406D1388

// The vectored handler that makes "nothing happens" true in this process.
//
// Every other exception is passed on untouched, which keeps the Go runtime's own crash
// reporting intact: its handlers are next in both chains, and a nil dereference in Go code
// still panics.
static LONG CALLBACK screenshare_swallow_thread_name(EXCEPTION_POINTERS *info) {
  if (info->ExceptionRecord->ExceptionCode == SCREENSHARE_THREAD_NAME_EXCEPTION) {
    return EXCEPTION_CONTINUE_EXECUTION;
  }
  return EXCEPTION_CONTINUE_SEARCH;
}

// The same function goes into both chains, and both entries are needed.
//
// Windows walks the continue handlers even when an exception handler already said
// the exception was handled, and the Go runtime's last continue handler crashes the process
// on anything it does not recognise.
// An exception handler alone is therefore answered and then killed anyway, and a continue
// handler alone is a resume from an exception nothing handled,
// which Windows ends the process over.
// Both go in first, ahead of the runtime's own pair, the shape the runtime uses
// for the exceptions it handles itself.
static void screenshare_install_thread_name_handler(void) {
  AddVectoredExceptionHandler(1, screenshare_swallow_thread_name);
  AddVectoredContinueHandler(1, screenshare_swallow_thread_name);
}
*/
import "C"

// ignoreThreadNameExceptions makes this process survive a native library naming its own threads.
//
// GLib names every thread it starts,
// and on Windows that is spelled as raising exception 0x406D1388,
// with a debugger expected to be listening.
// None is, and the Go runtime's last-chance handler treats an exception nothing claimed as a crash:
// register dump, then exit.
// Without this handler the first GStreamer pipeline in the process kills the backend from inside
// gst_parse_launch, over a notification that means nothing went wrong.
//
// Answered here rather than avoided upstream, there being nothing upstream to avoid:
// the raise is how thread naming is spelled on this platform,
// and every GStreamer thread this backend starts does it.
// Declining the exception is the process saying it has no debugger and needs none.
//
// Called before Init, the threads that raise it starting during the registry scan.
func ignoreThreadNameExceptions() {
	C.screenshare_install_thread_name_handler()
}
