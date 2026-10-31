//go:build darwin && cgo

package launcher

/*
#cgo LDFLAGS: -framework CoreGraphics
#include <CoreGraphics/CoreGraphics.h>

static void awMainDisplaySize(int *width, int *height) {
	CGDirectDisplayID display = CGMainDisplayID();
	*width = (int)CGDisplayPixelsWide(display);
	*height = (int)CGDisplayPixelsHigh(display);
}
*/
import "C"

// currentDisplaySize returns the main display's size in points (the logical
// resolution the user chose, not the Retina backing pixels), which is the
// right unit for sizing a session window. A session window should leave room
// for the menu bar and window chrome, so callers get a little less than the
// whole display.
func currentDisplaySize() (int, int) {
	var width, height C.int
	C.awMainDisplaySize(&width, &height)
	w, h := int(width), int(height)
	if w <= 0 || h <= 0 {
		return 0, 0
	}
	// 90 % of the display, rounded to even numbers (RDP desktop widths must be
	// even), never below the FreeRDP default of 1024x768.
	w, h = w*9/10&^1, h*9/10&^1
	if w < 1024 {
		w = 1024
	}
	if h < 768 {
		h = 768
	}
	return w, h
}
