// Apple Event plumbing for access-workspace:// launches; see urlevent_darwin.go.

#import <Cocoa/Cocoa.h>
#include <string.h>
#include "_cgo_export.h"

// Both counters are read and written on the main thread only.
static int awActiveLaunches = 0;
static BOOL awSawURL = NO;

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
@end

@implementation AWURLEventHandler
- (void)handleGetURL:(NSAppleEventDescriptor *)event withReplyEvent:(NSAppleEventDescriptor *)reply {
	NSString *url = [[event paramDescriptorForKeyword:keyDirectObject] stringValue];
	if (url == nil) {
		return;
	}
	awSawURL = YES;
	awActiveLaunches++;
	awGoHandleLaunchURL(strdup([url UTF8String]));
}
@end

// Called from any thread once a launch is done; the loop ends with the last one.
void awLaunchFinished(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (--awActiveLaunches == 0) {
			awStopApp();
		}
	});
}

int awServeLaunchURLs(double firstURLTimeoutSeconds) {
	@autoreleasepool {
		[NSApplication sharedApplication];
		AWURLEventHandler *handler = [[AWURLEventHandler alloc] init];
		NSAppleEventManager *manager = [NSAppleEventManager sharedAppleEventManager];
		[manager setEventHandler:handler
		             andSelector:@selector(handleGetURL:withReplyEvent:)
		           forEventClass:kInternetEventClass
		              andEventID:kAEGetURL];
		dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(firstURLTimeoutSeconds * NSEC_PER_SEC)),
		               dispatch_get_main_queue(), ^{
			if (!awSawURL) {
				awStopApp();
			}
		});
		// A URL that arrives between the last launch finishing and the stop
		// taking effect starts a launch after the loop ended; run again for it.
		do {
			[NSApp run];
		} while (awActiveLaunches > 0);
		[manager removeEventHandlerForEventClass:kInternetEventClass andEventID:kAEGetURL];
		return awSawURL ? 1 : 0;
	}
}
