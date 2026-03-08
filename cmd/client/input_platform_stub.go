//go:build desktop && !darwin && !windows
// +build desktop,!darwin,!windows

package main

import "log"

func platformMouseMove(x, y int) error {
	log.Printf("🖱️  鼠标移动: (%d, %d) [平台不支持]", x, y)
	return nil
}

func platformMouseButton(button string, down bool, x, y int) error {
	log.Printf("🖱️  鼠标按键: button=%s, down=%v, x=%d, y=%d [平台不支持]", button, down, x, y)
	return nil
}

func platformKeyTap(keycode int) error {
	log.Printf("⌨️  按键: keycode=%d [平台不支持]", keycode)
	return nil
}

func platformKeyToggle(keycode int, down bool) error {
	log.Printf("⌨️  键切换: keycode=%d, down=%v [平台不支持]", keycode, down)
	return nil
}

func mapJSKeyCodeToPlatformKeyCode(jsKeyCode int) int {
	if jsKeyCode > 0 && jsKeyCode <= 0xFF {
		return jsKeyCode
	}
	return -1
}
