//go:build desktop && darwin
// +build desktop,darwin

package main

/*
#include <ApplicationServices/ApplicationServices.h>
#include <string.h>
#include <stdlib.h>

void execMouseMove(int x, int y) {
    CGEventRef move = CGEventCreateMouseEvent(NULL, kCGEventMouseMoved, CGPointMake(x, y), 0);
    CGEventPost(kCGHIDEventTap, move);
    CFRelease(move);
}

void execMouseButton(char* button, bool down, int x, int y) {
    CGEventType type;
    CGMouseButton buttonType;

    if (strcmp(button, "left") == 0) {
        type = down ? kCGEventLeftMouseDown : kCGEventLeftMouseUp;
        buttonType = kCGMouseButtonLeft;
    } else if (strcmp(button, "right") == 0) {
        type = down ? kCGEventRightMouseDown : kCGEventRightMouseUp;
        buttonType = kCGMouseButtonRight;
    } else {
        type = down ? kCGEventOtherMouseDown : kCGEventOtherMouseUp;
        buttonType = kCGMouseButtonCenter;
    }

    CGEventRef click = CGEventCreateMouseEvent(NULL, type, CGPointMake(x, y), buttonType);
    CGEventPost(kCGHIDEventTap, click);
    CFRelease(click);
}

void execKeyTap(int keycode) {
    CGEventRef keyDown = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keycode, true);
    CGEventRef keyUp = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keycode, false);
    CGEventPost(kCGHIDEventTap, keyDown);
    CFRelease(keyDown);
    CGEventPost(kCGHIDEventTap, keyUp);
    CFRelease(keyUp);
}

void execKeyToggle(int keycode, bool down) {
    CGEventRef keyEvent = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keycode, down);
    CGEventPost(kCGHIDEventTap, keyEvent);
    CFRelease(keyEvent);
}
*/
import "C"

import "unsafe"

func platformMouseMove(x, y int) error {
	C.execMouseMove(C.int(x), C.int(y))
	return nil
}

func platformMouseButton(button string, down bool, x, y int) error {
	cButton := C.CString(button)
	defer C.free(unsafe.Pointer(cButton))

	C.execMouseButton(cButton, C.bool(down), C.int(x), C.int(y))
	return nil
}

func platformKeyTap(keycode int) error {
	C.execKeyTap(C.int(keycode))
	return nil
}

func platformKeyToggle(keycode int, down bool) error {
	C.execKeyToggle(C.int(keycode), C.bool(down))
	return nil
}

func platformSyncExtraMouseButtons(c *DesktopCapture, state mouseButtonState, x, y int) error {
	return nil
}

func resolvePlatformKeyCode(event *ControlEvent) int {
	jsKeyCode := normalizeBrowserKeyCode(event.KeyCode)
	if jsKeyCode == 0 {
		return -1
	}
	return mapJSKeyCodeToPlatformKeyCode(jsKeyCode)
}

func platformTracksKeyState() bool {
	return false
}

func platformLogMouseControlEvent(event *ControlEvent, screenX, screenY int) {}

func platformLogKeyboardControlEvent(event *ControlEvent, platformKeyCode int, down bool) {}

func mapJSKeyCodeToPlatformKeyCode(jsKeyCode int) int {
	keyMap := map[int]int{
		65: 0, 66: 11, 67: 8, 68: 2, 69: 14, 70: 3, 71: 5, 72: 4,
		73: 34, 74: 38, 75: 40, 76: 37, 77: 46, 78: 45, 79: 31, 80: 35,
		81: 12, 82: 15, 83: 1, 84: 17, 85: 32, 86: 9, 87: 13, 88: 7,
		89: 16, 90: 6,
		48: 29, 49: 18, 50: 19, 51: 20, 52: 21, 53: 23, 54: 22, 55: 26,
		56: 28, 57: 25,
		112: 122, 113: 120, 114: 99, 115: 118, 116: 96, 117: 97,
		118: 98, 119: 100, 120: 101, 121: 109, 122: 103, 123: 111,
		8: 51, 9: 48, 13: 36, 16: 54, 17: 55, 18: 56, 27: 53, 32: 49,
		37: 123, 38: 126, 39: 124, 40: 125, 46: 117,
		186: 41, 187: 24, 188: 43, 189: 27, 190: 47, 191: 44,
		192: 50, 219: 33, 220: 42, 221: 30, 222: 39,
	}

	if keycode, ok := keyMap[jsKeyCode]; ok {
		return keycode
	}
	return -1
}
