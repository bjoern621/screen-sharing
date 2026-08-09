package receive

/*
#include <windows.h>

// The exception code MSVC's debugger protocol reserves for "this thread is now
// called X". A thread that wants a name raises it with the name in the parameters,
// a debugger picks it up, and with no debugger attached nothing is supposed to
// happen at all.
#define SCREENSHARE_THREAD_NAME_EXCEPTION 0x406D1388

// screenshare_swallow_thread_name is the vectored handler that makes "nothing
// happens" true in this process.
//
// Every other exception is passed on untouched, which is what keeps the Go
// runtime's own crash reporting intact: its handlers are next in both chains, and a
// nil dereference in Go code still panics exactly as it did before.
static LONG CALLBACK screenshare_swallow_thread_name(EXCEPTION_POINTERS *info) {
  if (info->ExceptionRecord->ExceptionCode == SCREENSHARE_THREAD_NAME_EXCEPTION) {
    return EXCEPTION_CONTINUE_EXECUTION;
  }
  return EXCEPTION_CONTINUE_SEARCH;
}

// The same function goes into both chains, and both entries are needed.
//
// Windows walks the continue handlers even when an exception handler already said
// the exception was handled, and the Go runtime's last continue handler crashes the
// process on anything it does not recognise. So an exception handler alone is
// answered and then killed anyway, and a continue handler alone is a resume from an
// exception nothing handled, which Windows ends the process over. Registered first
// in both chains, ahead of the runtime's own pair, which is the same shape the
// runtime uses for the exceptions it handles itself.
static void screenshare_install_thread_name_handler(void) {
  AddVectoredExceptionHandler(1, screenshare_swallow_thread_name);
  AddVectoredContinueHandler(1, screenshare_swallow_thread_name);
}
*/
import "C"

// ignoreThreadNameExceptions makes this process survive a native library naming its
// own threads.
//
// GLib names every thread it starts, and on Windows the way to do that is to raise
// exception 0x406D1388 and expect a debugger to be listening. Nobody is, and the Go
// runtime's last-chance handler treats an exception nothing claimed as a crash: it
// prints the register dump and exits. So the first GStreamer pipeline built in this
// process killed the backend from inside gst_parse_launch, on a notification that
// means nothing went wrong.
//
// It is answered here rather than avoided upstream because there is nothing upstream
// to avoid: the raise is how thread naming is spelled on this platform, and every
// GStreamer thread this backend will ever start does it. A handler that declines the
// exception is the process saying it has no debugger and does not need one.
//
// Called before Init, because the threads that raise it are started during the
// registry scan.
func ignoreThreadNameExceptions() {
	C.screenshare_install_thread_name_handler()
}
