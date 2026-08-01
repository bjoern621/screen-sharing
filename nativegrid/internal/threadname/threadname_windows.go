package threadname

/*
#include <windows.h>

// msVCException is the exception a native library raises to tell an attached
// debugger what to call one of its threads. It carries no error: the raiser
// expects nothing to be listening and execution to carry straight on.
#define MS_VC_EXCEPTION 0x406D1388

// ignore resumes the thread that raised the naming exception and leaves every
// other exception to the handlers behind this one, the Go runtime's included.
static LONG CALLBACK ignore(EXCEPTION_POINTERS *info) {
    if (info->ExceptionRecord->ExceptionCode == MS_VC_EXCEPTION) {
        return EXCEPTION_CONTINUE_EXECUTION;
    }
    return EXCEPTION_CONTINUE_SEARCH;
}

// install registers ignore as a continue handler rather than as a first-chance
// one, because that is where the process is lost: the runtime's first-chance
// handler passes an exception raised in C code on, nothing else claims it, and
// the runtime's continue handler ends the process over it. First = 1 puts this
// one ahead of that.
static void install(void) {
    AddVectoredContinueHandler(1, ignore);
}
*/
import "C"

// Ignore stops a thread-naming exception raised by a native library from ending
// the process.
//
// libsrt names its threads as it starts up, so on Windows every pipeline
// carrying an srtsrc or srtsink died the moment it was built: the Go runtime saw
// an exception it had no owner for and reported it as "Exception 0x406d1388 ...
// signal arrived during external code execution". A C program never notices,
// which is why gst-launch-1.0 plays the same pipeline.
//
// Call it before anything creates a GStreamer element. A handler registered
// after the exception has been raised is a handler that was not there for it.
func Ignore() {
	C.install()
}
