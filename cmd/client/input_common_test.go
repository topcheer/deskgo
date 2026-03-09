//go:build desktop
// +build desktop

package main

import "testing"

func TestDecodeMouseMask(t *testing.T) {
	state := decodeMouseMask(1 | 4 | 8 | 16)

	if !state.Left {
		t.Fatalf("expected left button to be down")
	}
	if state.Right {
		t.Fatalf("expected right button to be up")
	}
	if !state.Middle {
		t.Fatalf("expected middle button to be down")
	}
	if !state.ScrollUp || !state.ScrollDown {
		t.Fatalf("expected scroll buttons to be decoded")
	}
}

func TestBrowserCodeToLegacyKeyCode(t *testing.T) {
	testCases := map[string]int{
		"KeyA":        65,
		"Digit7":      55,
		"ArrowLeft":   37,
		"ShiftRight":  16,
		"MetaLeft":    91,
		"Numpad1":     97,
		"BracketLeft": 219,
		"F12":         123,
	}

	for code, want := range testCases {
		if got := browserCodeToLegacyKeyCode(code); got != want {
			t.Fatalf("browserCodeToLegacyKeyCode(%q) = %d, want %d", code, got, want)
		}
	}

	if got := browserCodeToLegacyKeyCode("UnknownCode"); got != -1 {
		t.Fatalf("expected unknown code to return -1, got %d", got)
	}
}

func TestMapCoordsToScreenWithDisplayOrigin(t *testing.T) {
	capture := &DesktopCapture{
		width:          1600,
		height:         900,
		canvasWidth:    800,
		canvasHeight:   450,
		displayOriginX: 1600,
		displayOriginY: 0,
	}

	x, y := capture.mapCoordsToScreen(400, 225, 800, 450)
	if x != 2400 || y != 450 {
		t.Fatalf("expected mapped coordinates (2400,450), got (%d,%d)", x, y)
	}
}

func TestMapCoordsToScreenClampsToDisplayBounds(t *testing.T) {
	capture := &DesktopCapture{
		width:          1920,
		height:         1080,
		displayOriginX: 0,
		displayOriginY: 100,
	}

	x, y := capture.mapCoordsToScreen(5000, -20, 0, 0)
	if x != 1919 || y != 100 {
		t.Fatalf("expected clamped coordinates (1919,100), got (%d,%d)", x, y)
	}
}

func TestKeyEventIsDown(t *testing.T) {
	down := true
	up := false

	if !keyEventIsDown(&ControlEvent{KeyCode: -65, KeyDown: &down}) {
		t.Fatalf("expected explicit key_down=true to win")
	}
	if keyEventIsDown(&ControlEvent{KeyCode: 65, KeyDown: &up}) {
		t.Fatalf("expected explicit key_down=false to win")
	}
	if keyEventIsDown(&ControlEvent{KeyCode: -65}) {
		t.Fatalf("expected negative keyCode without key_down to mean key up")
	}
	if !keyEventIsDown(&ControlEvent{KeyCode: 65}) {
		t.Fatalf("expected positive keyCode without key_down to mean key down")
	}
}
