//go:build desktop && windows
// +build desktop,windows

package main

import (
	"fmt"
	"syscall"
)

const (
	mouseeventfLeftDown   = 0x0002
	mouseeventfLeftUp     = 0x0004
	mouseeventfRightDown  = 0x0008
	mouseeventfRightUp    = 0x0010
	mouseeventfMiddleDown = 0x0020
	mouseeventfMiddleUp   = 0x0040
	keyeventfKeyUp        = 0x0002
)

var (
	user32DLL        = syscall.NewLazyDLL("user32.dll")
	procSetCursorPos = user32DLL.NewProc("SetCursorPos")
	procMouseEvent   = user32DLL.NewProc("mouse_event")
	procKeybdEvent   = user32DLL.NewProc("keybd_event")
	procMapVirtual   = user32DLL.NewProc("MapVirtualKeyW")
)

func platformMouseMove(x, y int) error {
	result, _, callErr := procSetCursorPos.Call(uintptr(x), uintptr(y))
	if result == 0 {
		if callErr != syscall.Errno(0) {
			return callErr
		}
		return fmt.Errorf("SetCursorPos failed for coordinates (%d,%d)", x, y)
	}
	return nil
}

func platformMouseButton(button string, down bool, x, y int) error {
	if err := platformMouseMove(x, y); err != nil {
		return err
	}

	var flags uintptr
	switch button {
	case "left":
		if down {
			flags = mouseeventfLeftDown
		} else {
			flags = mouseeventfLeftUp
		}
	case "right":
		if down {
			flags = mouseeventfRightDown
		} else {
			flags = mouseeventfRightUp
		}
	case "middle":
		if down {
			flags = mouseeventfMiddleDown
		} else {
			flags = mouseeventfMiddleUp
		}
	default:
		return fmt.Errorf("unsupported mouse button: %s", button)
	}

	_, _, callErr := procMouseEvent.Call(flags, 0, 0, 0, 0)
	if callErr != syscall.Errno(0) {
		return callErr
	}
	return nil
}

func platformKeyTap(keycode int) error {
	if err := platformKeyToggle(keycode, true); err != nil {
		return err
	}
	return platformKeyToggle(keycode, false)
}

func platformKeyToggle(keycode int, down bool) error {
	if keycode <= 0 || keycode > 0xFF {
		return fmt.Errorf("unsupported virtual key code: %d", keycode)
	}

	scanCode, _, _ := procMapVirtual.Call(uintptr(keycode), 0)
	flags := uintptr(0)
	if !down {
		flags = keyeventfKeyUp
	}

	_, _, callErr := procKeybdEvent.Call(uintptr(keycode), scanCode, flags, 0)
	if callErr != syscall.Errno(0) {
		return callErr
	}
	return nil
}

func mapJSKeyCodeToPlatformKeyCode(jsKeyCode int) int {
	keyMap := map[int]int{
		8: 0x08, 9: 0x09, 13: 0x0D, 16: 0x10, 17: 0x11, 18: 0x12, 20: 0x14,
		27: 0x1B, 32: 0x20, 33: 0x21, 34: 0x22, 35: 0x23, 36: 0x24,
		37: 0x25, 38: 0x26, 39: 0x27, 40: 0x28, 45: 0x2D, 46: 0x2E,
		48: 0x30, 49: 0x31, 50: 0x32, 51: 0x33, 52: 0x34,
		53: 0x35, 54: 0x36, 55: 0x37, 56: 0x38, 57: 0x39,
		65: 0x41, 66: 0x42, 67: 0x43, 68: 0x44, 69: 0x45,
		70: 0x46, 71: 0x47, 72: 0x48, 73: 0x49, 74: 0x4A,
		75: 0x4B, 76: 0x4C, 77: 0x4D, 78: 0x4E, 79: 0x4F,
		80: 0x50, 81: 0x51, 82: 0x52, 83: 0x53, 84: 0x54,
		85: 0x55, 86: 0x56, 87: 0x57, 88: 0x58, 89: 0x59, 90: 0x5A,
		91: 0x5B, 92: 0x5C, 93: 0x5D,
		112: 0x70, 113: 0x71, 114: 0x72, 115: 0x73, 116: 0x74, 117: 0x75,
		118: 0x76, 119: 0x77, 120: 0x78, 121: 0x79, 122: 0x7A, 123: 0x7B,
		186: 0xBA, 187: 0xBB, 188: 0xBC, 189: 0xBD, 190: 0xBE, 191: 0xBF,
		192: 0xC0, 219: 0xDB, 220: 0xDC, 221: 0xDD, 222: 0xDE,
	}

	if keycode, ok := keyMap[jsKeyCode]; ok {
		return keycode
	}
	return -1
}
