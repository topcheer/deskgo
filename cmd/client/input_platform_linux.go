//go:build desktop && linux
// +build desktop,linux

package main

import (
	"fmt"
	"os"
	"sync"

	"github.com/robotn/xgb/xproto"
	"github.com/robotn/xgb/xtest"
	"github.com/robotn/xgbutil"
	"github.com/robotn/xgbutil/keybind"
)

type linuxInputController struct {
	mu         sync.Mutex
	xu         *xgbutil.XUtil
	keycodeMap map[string]xproto.Keycode
}

var (
	linuxInputOnce               sync.Once
	linuxInputControllerInstance *linuxInputController
	linuxInputInitErr            error
)

func getLinuxInputController() (*linuxInputController, error) {
	linuxInputOnce.Do(func() {
		linuxInputControllerInstance, linuxInputInitErr = newLinuxInputController()
	})

	return linuxInputControllerInstance, linuxInputInitErr
}

func newLinuxInputController() (*linuxInputController, error) {
	display := os.Getenv("DISPLAY")
	if display == "" {
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			return nil, fmt.Errorf("当前 Linux 输入控制仅支持 X11/XWayland；检测到 Wayland 会话但 DISPLAY 未设置")
		}
		return nil, fmt.Errorf("DISPLAY 未设置，无法连接到 X11 输入会话")
	}

	xu, err := xgbutil.NewConnDisplay(display)
	if err != nil {
		return nil, fmt.Errorf("连接 X11 显示 %q 失败: %w", display, err)
	}

	keybind.Initialize(xu)
	if err := xtest.Init(xu.Conn()); err != nil {
		return nil, fmt.Errorf("XTEST 扩展不可用: %w", err)
	}

	return &linuxInputController{
		xu:         xu,
		keycodeMap: make(map[string]xproto.Keycode),
	}, nil
}

func platformMouseMove(x, y int) error {
	controller, err := getLinuxInputController()
	if err != nil {
		return err
	}

	controller.mu.Lock()
	defer controller.mu.Unlock()

	if err := xproto.WarpPointerChecked(
		controller.xu.Conn(),
		xproto.WindowNone,
		controller.xu.RootWin(),
		0, 0, 0, 0,
		int16(x),
		int16(y),
	).Check(); err != nil {
		return fmt.Errorf("X11 鼠标移动失败: %w", err)
	}

	controller.xu.Sync()
	return nil
}

func platformMouseButton(button string, down bool, x, y int) error {
	controller, err := getLinuxInputController()
	if err != nil {
		return err
	}

	controller.mu.Lock()
	defer controller.mu.Unlock()

	if err := xproto.WarpPointerChecked(
		controller.xu.Conn(),
		xproto.WindowNone,
		controller.xu.RootWin(),
		0, 0, 0, 0,
		int16(x),
		int16(y),
	).Check(); err != nil {
		return fmt.Errorf("X11 鼠标移动失败: %w", err)
	}

	var detail byte
	switch button {
	case "left":
		detail = 1
	case "middle":
		detail = 2
	case "right":
		detail = 3
	default:
		return fmt.Errorf("unsupported mouse button: %s", button)
	}

	eventType := byte(xproto.ButtonPress)
	if !down {
		eventType = byte(xproto.ButtonRelease)
	}

	if err := xtest.FakeInputChecked(
		controller.xu.Conn(),
		eventType,
		detail,
		0,
		controller.xu.RootWin(),
		0,
		0,
		0,
	).Check(); err != nil {
		return fmt.Errorf("X11 鼠标按键事件失败: %w", err)
	}

	controller.xu.Sync()
	return nil
}

func platformKeyTap(keycode int) error {
	if err := platformKeyToggle(keycode, true); err != nil {
		return err
	}
	return platformKeyToggle(keycode, false)
}

func platformKeyToggle(keycode int, down bool) error {
	controller, err := getLinuxInputController()
	if err != nil {
		return err
	}
	if keycode <= 0 || keycode > 0xFF {
		return fmt.Errorf("unsupported X11 keycode: %d", keycode)
	}

	controller.mu.Lock()
	defer controller.mu.Unlock()

	eventType := byte(xproto.KeyPress)
	if !down {
		eventType = byte(xproto.KeyRelease)
	}

	if err := xtest.FakeInputChecked(
		controller.xu.Conn(),
		eventType,
		byte(keycode),
		0,
		controller.xu.RootWin(),
		0,
		0,
		0,
	).Check(); err != nil {
		return fmt.Errorf("X11 键盘事件失败: %w", err)
	}

	controller.xu.Sync()
	return nil
}

func mapJSKeyCodeToPlatformKeyCode(jsKeyCode int) int {
	controller, err := getLinuxInputController()
	if err != nil {
		return -1
	}

	keyName, ok := linuxJSKeyCodeToXKey[jsKeyCode]
	if !ok {
		return -1
	}

	controller.mu.Lock()
	defer controller.mu.Unlock()

	if keycode, ok := controller.keycodeMap[keyName]; ok {
		return int(keycode)
	}

	keycodes := keybind.StrToKeycodes(controller.xu, keyName)
	if len(keycodes) == 0 {
		return -1
	}

	controller.keycodeMap[keyName] = keycodes[0]
	return int(keycodes[0])
}

var linuxJSKeyCodeToXKey = map[int]string{
	8:   "BackSpace",
	9:   "Tab",
	13:  "Return",
	16:  "Shift_L",
	17:  "Control_L",
	18:  "Alt_L",
	20:  "Caps_Lock",
	27:  "Escape",
	32:  "space",
	33:  "Page_Up",
	34:  "Page_Down",
	35:  "End",
	36:  "Home",
	37:  "Left",
	38:  "Up",
	39:  "Right",
	40:  "Down",
	45:  "Insert",
	46:  "Delete",
	48:  "0",
	49:  "1",
	50:  "2",
	51:  "3",
	52:  "4",
	53:  "5",
	54:  "6",
	55:  "7",
	56:  "8",
	57:  "9",
	65:  "a",
	66:  "b",
	67:  "c",
	68:  "d",
	69:  "e",
	70:  "f",
	71:  "g",
	72:  "h",
	73:  "i",
	74:  "j",
	75:  "k",
	76:  "l",
	77:  "m",
	78:  "n",
	79:  "o",
	80:  "p",
	81:  "q",
	82:  "r",
	83:  "s",
	84:  "t",
	85:  "u",
	86:  "v",
	87:  "w",
	88:  "x",
	89:  "y",
	90:  "z",
	91:  "Super_L",
	92:  "Super_R",
	93:  "Menu",
	112: "F1",
	113: "F2",
	114: "F3",
	115: "F4",
	116: "F5",
	117: "F6",
	118: "F7",
	119: "F8",
	120: "F9",
	121: "F10",
	122: "F11",
	123: "F12",
	186: "semicolon",
	187: "equal",
	188: "comma",
	189: "minus",
	190: "period",
	191: "slash",
	192: "grave",
	219: "bracketleft",
	220: "backslash",
	221: "bracketright",
	222: "apostrophe",
}
