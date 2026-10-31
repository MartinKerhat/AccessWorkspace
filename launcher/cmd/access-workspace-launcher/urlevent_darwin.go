//go:build darwin && cgo

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>

// Implemented in urlevent_darwin.m (a file with //export directives may only
// declare C functions in its preamble).
int awServeLaunchURLs(double firstURLTimeoutSeconds);
void awLaunchFinished(void);
*/
import "C"

import (
	"runtime"
	"sync"
	"unsafe"
)

// Cocoa must run on the main thread; keep main() there.
func init() {
	runtime.LockOSThread()
}

var (
	launchURLMu      sync.Mutex
	launchURLHandler func(string) error
	launchURLErr     error
)

// awGoHandleLaunchURL is called on the main thread for every kAEGetURL
// Apple Event. The launch itself runs off the main thread so the run loop
// keeps receiving further URLs; awLaunchFinished lets the loop end once the
// last launch is done.
//
//export awGoHandleLaunchURL
func awGoHandleLaunchURL(raw *C.char) {
	url := C.GoString(raw)
	C.free(unsafe.Pointer(raw))
	go func() {
		defer C.awLaunchFinished()
		if err := launchURLHandler(url); err != nil {
			launchURLMu.Lock()
			if launchURLErr == nil {
				launchURLErr = err
			}
			launchURLMu.Unlock()
		}
	}()
}

// serveLaunchURLs runs launch for the access-workspace:// URL that opened the
// app and for every further one delivered while the process lives. macOS
// delivers a URL-scheme launch as an Apple Event after startup, never in
// argv; and as long as this process is the running instance of the bundle,
// LaunchServices routes every later click at the same URL scheme to it as
// well, so the handler stays installed until the last launch finished.
//
// handled is false when no URL arrived within the wait: the user opened the
// app itself (Finder, Launchpad), which runs the install flow. err is the
// first launch failure; each failure has already been shown to the user.
func serveLaunchURLs(launch func(url string) error) (handled bool, err error) {
	launchURLHandler = launch
	handled = C.awServeLaunchURLs(C.double(1.5)) != 0
	launchURLMu.Lock()
	defer launchURLMu.Unlock()
	return handled, launchURLErr
}
