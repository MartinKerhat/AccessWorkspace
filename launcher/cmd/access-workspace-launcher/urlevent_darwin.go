//go:build darwin && cgo

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <stdlib.h>
#include <string.h>

// Ends [NSApp run]: stop only takes effect after the next event, so post one.
static void awStopApp(void) {
	[NSApp stop:nil];
	NSEvent *wake = [NSEvent otherEventWithType:NSEventTypeApplicationDefined
	                                   location:NSZeroPoint
	                              modifierFlags:0
	                                  timestamp:0
	                               windowNumber:0
	                                    context:nil
	                                    subtype:0
	                                      data1:0
	                                      data2:0];
	[NSApp postEvent:wake atStart:YES];
}

@interface AWURLEventHandler : NSObject
@property(nonatomic, copy) NSString *url;
@end

@implementation AWURLEventHandler
- (void)handleGetURL:(NSAppleEventDescriptor *)event withReplyEvent:(NSAppleEventDescriptor *)reply {
	self.url = [[event paramDescriptorForKeyword:keyDirectObject] stringValue];
	awStopApp();
}
@end

static char *awWaitForLaunchURL(double timeoutSeconds) {
	@autoreleasepool {
		[NSApplication sharedApplication];
		AWURLEventHandler *handler = [[AWURLEventHandler alloc] init];
		NSAppleEventManager *manager = [NSAppleEventManager sharedAppleEventManager];
		[manager setEventHandler:handler
		             andSelector:@selector(handleGetURL:withReplyEvent:)
		           forEventClass:kInternetEventClass
		              andEventID:kAEGetURL];
		dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(timeoutSeconds * NSEC_PER_SEC)),
		               dispatch_get_main_queue(), ^{ awStopApp(); });
		[NSApp run];
		[manager removeEventHandlerForEventClass:kInternetEventClass andEventID:kAEGetURL];
		if (handler.url == nil) {
			return NULL;
		}
		return strdup([handler.url UTF8String]);
	}
}
*/
import "C"

import (
	"runtime"
	"unsafe"
)

// Cocoa must run on the main thread; keep main() there.
func init() {
	runtime.LockOSThread()
}

// waitForLaunchURL returns the access-workspace:// URL that opened the app.
// macOS delivers a URL-scheme launch as an Apple Event after startup, never
// in argv, so an argument-less start waits briefly for one; "" means the user
// opened the app itself (Finder, Launchpad), which runs the install flow.
func waitForLaunchURL() string {
	raw := C.awWaitForLaunchURL(C.double(1.5))
	if raw == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(raw))
	return C.GoString(raw)
}
