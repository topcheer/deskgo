//go:build desktop && linux
// +build desktop,linux

package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/robotn/xgb/xproto"
	"github.com/robotn/xgb/xtest"
	"github.com/robotn/xgbutil"
	"github.com/robotn/xgbutil/keybind"
)

type linuxInputBackend string

const (
	linuxInputBackendX11     linuxInputBackend = "x11"
	linuxInputBackendYdotool linuxInputBackend = "ydotool"
)

type linuxInputController struct {
	mu                sync.Mutex
	backend           linuxInputBackend
	xu                *xgbutil.XUtil
	keycodeMap        map[string]xproto.Keycode
	ydotoolPath       string
	ydotooldPath      string
	ydotoolSocketPath string
	lastMoveLog       time.Time
	warnedWheelNative bool
}

type linuxCommandSpec struct {
	Name string
	Args []string
	Env  map[string]string
}

type linuxPackageInstallPlan struct {
	Manager  string
	Commands []linuxCommandSpec
}

type linuxPrivilegeMethod string

const (
	linuxPrivilegeDirect linuxPrivilegeMethod = "direct"
	linuxPrivilegeSudo   linuxPrivilegeMethod = "sudo"
	linuxPrivilegeDoas   linuxPrivilegeMethod = "doas"
	linuxPrivilegePkexec linuxPrivilegeMethod = "pkexec"
)

type linuxPrivilegeRunner struct {
	method linuxPrivilegeMethod
	path   string
}

type linuxYdotoolDaemonLaunch struct {
	Args            []string
	SocketShareable bool
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
	waylandDisplay := os.Getenv("WAYLAND_DISPLAY")
	sessionType := strings.ToLower(os.Getenv("XDG_SESSION_TYPE"))
	backendOverride := strings.ToLower(strings.TrimSpace(os.Getenv("DESKGO_LINUX_INPUT_BACKEND")))
	waylandSession := waylandDisplay != "" || sessionType == "wayland"

	if backendOverride != "" && backendOverride != "auto" && backendOverride != string(linuxInputBackendX11) && backendOverride != string(linuxInputBackendYdotool) {
		return nil, fmt.Errorf("未知的 Linux 输入后端 %q（支持: auto, x11, ydotool）", backendOverride)
	}

	if backendOverride == string(linuxInputBackendYdotool) || (backendOverride == "" || backendOverride == "auto") && waylandSession {
		ydotoolPath, ydotooldPath, ydotoolSocketPath, err := prepareYdotoolBackend()
		if err == nil {
			log.Printf("🧪 [linux-input] 检测到 Wayland 会话，使用 ydotool 输入后端: %s (socket=%s)", ydotoolPath, ydotoolSocketPath)
			return &linuxInputController{
				backend:           linuxInputBackendYdotool,
				keycodeMap:        make(map[string]xproto.Keycode),
				ydotoolPath:       ydotoolPath,
				ydotooldPath:      ydotooldPath,
				ydotoolSocketPath: ydotoolSocketPath,
			}, nil
		}
		if backendOverride == string(linuxInputBackendYdotool) {
			return nil, fmt.Errorf("已强制使用 ydotool 后端，但初始化失败: %w", err)
		}
		if waylandSession {
			log.Printf("⚠️  [linux-input] %v", err)
			log.Printf("⚠️  [linux-input] 将尝试 X11/XTEST。注意：这只能控制 XWayland 应用，无法控制 Wayland 原生应用。")
		}
	}

	if display == "" {
		if waylandSession {
			return nil, fmt.Errorf("检测到 Wayland 会话，但既没有可用的 ydotool 后端，也没有 DISPLAY 可供 X11/XTEST 使用")
		}
		return nil, fmt.Errorf("DISPLAY 未设置，无法连接到 X11 输入会话")
	}

	if waylandSession && backendOverride != string(linuxInputBackendYdotool) {
		log.Printf("⚠️  [linux-input] 当前处于 Wayland 会话；X11/XTEST 只能控制 XWayland 应用。若需控制 Wayland 原生桌面，请安装 ydotool 或切换到 X11 会话。")
	}

	xu, err := xgbutil.NewConnDisplay(display)
	if err != nil {
		return nil, fmt.Errorf("连接 X11 显示 %q 失败: %w", display, err)
	}

	keybind.Initialize(xu)
	if err := xtest.Init(xu.Conn()); err != nil {
		return nil, fmt.Errorf("XTEST 扩展不可用: %w", err)
	}
	if err := xtest.GrabControlChecked(xu.Conn(), true).Check(); err != nil {
		log.Printf("⚠️  [linux-input] 无法启用 XTEST GrabControl: %v", err)
	}

	log.Printf("🧪 [linux-input] 已连接 X11 输入会话: DISPLAY=%s, root=%d", display, xu.RootWin())

	return &linuxInputController{
		backend:    linuxInputBackendX11,
		xu:         xu,
		keycodeMap: make(map[string]xproto.Keycode),
	}, nil
}

func prepareYdotoolBackend() (string, string, string, error) {
	ydotoolPath, ydotooldPath, err := findYdotoolBinaries()
	if err != nil {
		if !linuxAutoInstallYdotoolEnabled() {
			return "", "", "", fmt.Errorf("检测到 Wayland 会话，但 %v；自动安装已被 DESKGO_LINUX_AUTO_INSTALL_YDOTOOL=0 禁用", err)
		}
		if installErr := installYdotoolPackage(); installErr != nil {
			return "", "", "", fmt.Errorf("检测到 Wayland 会话，但 %v；自动安装失败: %w", err, installErr)
		}
		ydotoolPath, ydotooldPath, err = findYdotoolBinaries()
		if err != nil {
			return "", "", "", fmt.Errorf("自动安装完成后仍未找到 ydotool/ydotoold: %w", err)
		}
	}

	socketPath, err := ensureYdotoolDaemonReady(ydotoolPath, ydotooldPath)
	if err != nil {
		return "", "", "", fmt.Errorf("ydotool 后端尚未就绪: %w", err)
	}

	return ydotoolPath, ydotooldPath, socketPath, nil
}

func findYdotoolBinaries() (string, string, error) {
	ydotoolPath, ydotoolErr := exec.LookPath("ydotool")
	ydotooldPath, ydotooldErr := exec.LookPath("ydotoold")

	missing := make([]string, 0, 2)
	if ydotoolErr != nil {
		missing = append(missing, "ydotool")
	}
	if ydotooldErr != nil {
		missing = append(missing, "ydotoold")
	}
	if len(missing) > 0 {
		return "", "", fmt.Errorf("系统中未找到 %s", strings.Join(missing, " 和 "))
	}

	return ydotoolPath, ydotooldPath, nil
}

func linuxAutoInstallYdotoolEnabled() bool {
	return linuxBoolEnv("DESKGO_LINUX_AUTO_INSTALL_YDOTOOL", true)
}

func linuxAllowInteractiveInstall() bool {
	defaultAllowed := os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
	return linuxBoolEnv("DESKGO_LINUX_ALLOW_INTERACTIVE_INSTALL", defaultAllowed)
}

func linuxBoolEnv(name string, defaultValue bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	switch raw {
	case "":
		return defaultValue
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultValue
	}
}

func installYdotoolPackage() error {
	plan := detectYdotoolInstallPlan(exec.LookPath)
	if plan == nil {
		return fmt.Errorf("未识别到受支持的 Linux 包管理器，请手动安装 ydotool")
	}

	runner, err := detectLinuxPrivilegeRunner()
	if err != nil {
		return fmt.Errorf("无法获取自动安装所需权限: %w；请手动执行: %s", err, plan.commandString())
	}

	log.Printf("🧪 [linux-input] 未找到 ydotool，尝试通过 %s 自动安装...", plan.Manager)
	log.Printf("🧪 [linux-input] 自动安装授权方式: %s", runner.method)

	for _, command := range plan.Commands {
		log.Printf("🧪 [linux-input] 执行安装命令: %s", linuxCommandString(command))
		if err := runner.run(command, 4*time.Minute); err != nil {
			return err
		}
	}

	return nil
}

func detectYdotoolInstallPlan(lookPath func(string) (string, error)) *linuxPackageInstallPlan {
	switch {
	case linuxHasCommand(lookPath, "apt-get"):
		return &linuxPackageInstallPlan{
			Manager: "apt-get",
			Commands: []linuxCommandSpec{
				{
					Name: "apt-get",
					Args: []string{"update"},
					Env:  map[string]string{"DEBIAN_FRONTEND": "noninteractive"},
				},
				{
					Name: "apt-get",
					Args: []string{"install", "-y", "ydotool"},
					Env:  map[string]string{"DEBIAN_FRONTEND": "noninteractive"},
				},
			},
		}
	case linuxHasCommand(lookPath, "apt"):
		return &linuxPackageInstallPlan{
			Manager: "apt",
			Commands: []linuxCommandSpec{
				{
					Name: "apt",
					Args: []string{"update"},
					Env:  map[string]string{"DEBIAN_FRONTEND": "noninteractive"},
				},
				{
					Name: "apt",
					Args: []string{"install", "-y", "ydotool"},
					Env:  map[string]string{"DEBIAN_FRONTEND": "noninteractive"},
				},
			},
		}
	case linuxHasCommand(lookPath, "dnf"):
		return &linuxPackageInstallPlan{
			Manager: "dnf",
			Commands: []linuxCommandSpec{{
				Name: "dnf",
				Args: []string{"install", "-y", "ydotool"},
			}},
		}
	case linuxHasCommand(lookPath, "yum"):
		return &linuxPackageInstallPlan{
			Manager: "yum",
			Commands: []linuxCommandSpec{{
				Name: "yum",
				Args: []string{"install", "-y", "ydotool"},
			}},
		}
	case linuxHasCommand(lookPath, "pacman"):
		return &linuxPackageInstallPlan{
			Manager: "pacman",
			Commands: []linuxCommandSpec{{
				Name: "pacman",
				Args: []string{"-Sy", "--noconfirm", "ydotool"},
			}},
		}
	case linuxHasCommand(lookPath, "zypper"):
		return &linuxPackageInstallPlan{
			Manager: "zypper",
			Commands: []linuxCommandSpec{{
				Name: "zypper",
				Args: []string{"--non-interactive", "install", "ydotool"},
			}},
		}
	case linuxHasCommand(lookPath, "apk"):
		return &linuxPackageInstallPlan{
			Manager: "apk",
			Commands: []linuxCommandSpec{{
				Name: "apk",
				Args: []string{"add", "ydotool"},
			}},
		}
	default:
		return nil
	}
}

func linuxHasCommand(lookPath func(string) (string, error), name string) bool {
	_, err := lookPath(name)
	return err == nil
}

func (plan *linuxPackageInstallPlan) commandString() string {
	if plan == nil {
		return ""
	}

	parts := make([]string, 0, len(plan.Commands))
	for _, command := range plan.Commands {
		parts = append(parts, linuxCommandString(command))
	}
	return strings.Join(parts, " && ")
}

func linuxCommandString(command linuxCommandSpec) string {
	parts := make([]string, 0, len(command.Env)+1+len(command.Args))
	keys := make([]string, 0, len(command.Env))
	for key := range command.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := command.Env[key]
		parts = append(parts, fmt.Sprintf("%s=%s", key, value))
	}
	parts = append(parts, command.Name)
	parts = append(parts, command.Args...)
	return strings.Join(parts, " ")
}

func detectLinuxPrivilegeRunner() (*linuxPrivilegeRunner, error) {
	if os.Geteuid() == 0 {
		return &linuxPrivilegeRunner{method: linuxPrivilegeDirect}, nil
	}

	if sudoPath, err := exec.LookPath("sudo"); err == nil && linuxCanRunWithoutPassword(sudoPath, "-n", "true") {
		return &linuxPrivilegeRunner{method: linuxPrivilegeSudo, path: sudoPath}, nil
	}

	if doasPath, err := exec.LookPath("doas"); err == nil && linuxCanRunWithoutPassword(doasPath, "-n", "true") {
		return &linuxPrivilegeRunner{method: linuxPrivilegeDoas, path: doasPath}, nil
	}

	if linuxAllowInteractiveInstall() {
		if pkexecPath, err := exec.LookPath("pkexec"); err == nil {
			return &linuxPrivilegeRunner{method: linuxPrivilegePkexec, path: pkexecPath}, nil
		}
	}

	return nil, fmt.Errorf("当前用户不是 root，且没有可用的 sudo/doas 无密码授权，也没有可用的 pkexec")
}

func linuxCanRunWithoutPassword(path string, args ...string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, args...)
	return cmd.Run() == nil
}

func (runner *linuxPrivilegeRunner) run(command linuxCommandSpec, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := runner.commandContext(ctx, command)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return fmt.Errorf("%s 失败: %w", linuxCommandString(command), err)
	}
	return fmt.Errorf("%s 失败: %w: %s", linuxCommandString(command), err, trimmed)
}

func (runner *linuxPrivilegeRunner) commandContext(ctx context.Context, command linuxCommandSpec) *exec.Cmd {
	if runner == nil || runner.method == linuxPrivilegeDirect {
		cmd := exec.CommandContext(ctx, command.Name, command.Args...)
		cmd.Env = append(os.Environ(), linuxEnvPairs(command.Env)...)
		return cmd
	}

	envPath := linuxEnvCommand()
	args := []string{}
	switch runner.method {
	case linuxPrivilegeSudo, linuxPrivilegeDoas:
		args = append(args, "-n")
	}

	args = append(args, envPath, "PATH="+os.Getenv("PATH"))
	args = append(args, linuxEnvPairs(command.Env)...)
	args = append(args, command.Name)
	args = append(args, command.Args...)

	cmd := exec.CommandContext(ctx, runner.path, args...)
	cmd.Env = os.Environ()
	return cmd
}

func linuxEnvCommand() string {
	if envPath, err := exec.LookPath("env"); err == nil {
		return envPath
	}
	return "env"
}

func linuxEnvPairs(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}

	pairs := make([]string, 0, len(env))
	for key, value := range env {
		pairs = append(pairs, fmt.Sprintf("%s=%s", key, value))
	}
	return pairs
}

func ensureYdotoolDaemonReady(ydotoolPath, ydotooldPath string) (string, error) {
	candidates := ydotoolSocketCandidates(os.Getenv("YDOTOOL_SOCKET"), os.Getenv("XDG_RUNTIME_DIR"), os.Getuid())
	if len(candidates) == 0 {
		return "", fmt.Errorf("无法确定 ydotoold socket 路径")
	}

	for index, socketPath := range candidates {
		if socketPath == "" {
			continue
		}
		if index > 0 {
			if _, err := os.Stat(socketPath); err != nil {
				continue
			}
		}
		if err := probeYdotoolSocket(ydotoolPath, socketPath); err == nil {
			log.Printf("🧪 [linux-input] 检测到可用的 ydotoold socket: %s", socketPath)
			return socketPath, nil
		}
	}

	preferredSocket := candidates[0]
	log.Printf("🧪 [linux-input] 未检测到可用的 ydotoold socket，尝试启动 ydotoold: %s", preferredSocket)
	if err := startYdotoolDaemon(ydotooldPath, preferredSocket); err != nil {
		return "", err
	}
	if err := waitForYdotoolSocket(ydotoolPath, preferredSocket, 5*time.Second); err != nil {
		return "", fmt.Errorf("ydotoold 已启动但尚未就绪: %w", err)
	}

	log.Printf("🧪 [linux-input] ydotoold 已就绪: socket=%s", preferredSocket)
	return preferredSocket, nil
}

func ydotoolSocketCandidates(explicitSocket, runtimeDir string, uid int) []string {
	candidates := make([]string, 0, 6)
	seen := make(map[string]struct{})
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		candidates = append(candidates, value)
	}

	add(explicitSocket)
	if runtimeDir != "" {
		add(filepath.Join(runtimeDir, "deskgo-ydotool.sock"))
		add(filepath.Join(runtimeDir, ".ydotool_socket"))
		add(filepath.Join(runtimeDir, "ydotool.sock"))
	}
	add(filepath.Join(os.TempDir(), fmt.Sprintf("deskgo-ydotool-%d.sock", uid)))
	add("/tmp/.ydotool_socket")
	add("/tmp/ydotool.sock")

	return candidates
}

func probeYdotoolSocket(ydotoolPath, socketPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, ydotoolPath, "mousemove", "0", "0")
	cmd.Env = append(os.Environ(), "YDOTOOL_SOCKET="+socketPath)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return fmt.Errorf("probe socket %s 失败: %w", socketPath, err)
	}
	return fmt.Errorf("probe socket %s 失败: %w: %s", socketPath, err, trimmed)
}

func waitForYdotoolSocket(ydotoolPath, socketPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := probeYdotoolSocket(ydotoolPath, socketPath); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(250 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("未知错误")
	}
	return lastErr
}

func startYdotoolDaemon(ydotooldPath, socketPath string) error {
	launch, err := buildYdotoolDaemonLaunch(ydotooldPath, socketPath)
	if err != nil {
		return err
	}
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		log.Printf("⚠️  [linux-input] 无法删除旧的 ydotool socket %s: %v", socketPath, err)
	}

	if err := startYdotoolDaemonDetached(nil, ydotooldPath, launch.Args); err == nil {
		return nil
	} else {
		directErr := err
		runner, runnerErr := detectLinuxPrivilegeRunner()
		if runnerErr != nil {
			return fmt.Errorf("直接启动 ydotoold 失败: %v；同时也没有可用的提权方式", directErr)
		}
		if runner.method == linuxPrivilegeDirect {
			return fmt.Errorf("启动 ydotoold 失败: %w", directErr)
		}
		if !launch.SocketShareable && runner.method != linuxPrivilegeDirect {
			return fmt.Errorf("直接启动 ydotoold 失败: %v；且当前 ydotoold 版本不支持 --socket-own/--socket-perm，无法通过 %s 暴露 socket 给当前用户", directErr, runner.method)
		}
		log.Printf("🧪 [linux-input] 直接启动 ydotoold 失败，尝试通过 %s 提权启动...", runner.method)
		if err := startYdotoolDaemonDetached(runner, ydotooldPath, launch.Args); err != nil {
			return fmt.Errorf("直接启动 ydotoold 失败: %v；提权启动也失败: %w", directErr, err)
		}
		return nil
	}
}

func buildYdotoolDaemonLaunch(ydotooldPath, socketPath string) (*linuxYdotoolDaemonLaunch, error) {
	helpOutput, err := exec.Command(ydotooldPath, "--help").CombinedOutput()
	if err != nil && len(helpOutput) == 0 {
		return nil, fmt.Errorf("读取 ydotoold 帮助信息失败: %w", err)
	}

	launch := buildYdotoolDaemonLaunchFromHelp(string(helpOutput), socketPath, os.Getuid(), os.Getgid())
	return &launch, nil
}

func buildYdotoolDaemonLaunchFromHelp(helpText, socketPath string, uid, gid int) linuxYdotoolDaemonLaunch {
	launch := linuxYdotoolDaemonLaunch{
		Args: []string{"--socket-path", socketPath},
	}

	if strings.Contains(helpText, "--socket-own") {
		launch.Args = append(launch.Args, "--socket-own", fmt.Sprintf("%d:%d", uid, gid))
		launch.SocketShareable = true
		return launch
	}
	if strings.Contains(helpText, "--socket-perm") {
		launch.Args = append(launch.Args, "--socket-perm", "0666")
		launch.SocketShareable = true
	}

	return launch
}

func startYdotoolDaemonDetached(runner *linuxPrivilegeRunner, ydotooldPath string, args []string) error {
	command := linuxCommandSpec{Name: ydotooldPath, Args: args}
	cmd := runner.commandContext(context.Background(), command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 ydotoold 失败: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		trimmed := strings.TrimSpace(output.String())
		if trimmed == "" {
			return fmt.Errorf("ydotoold 立即退出: %w", err)
		}
		return fmt.Errorf("ydotoold 立即退出: %w: %s", err, trimmed)
	case <-time.After(300 * time.Millisecond):
		return nil
	}
}

func platformMouseMove(x, y int) error {
	controller, err := getLinuxInputController()
	if err != nil {
		return err
	}

	controller.mu.Lock()
	defer controller.mu.Unlock()

	if controller.backend == linuxInputBackendYdotool {
		if err := controller.runYdotoolLocked("mousemove", "--absolute", strconv.Itoa(x), strconv.Itoa(y)); err != nil {
			return err
		}
	} else {
		if err := xtest.FakeInputChecked(
			controller.xu.Conn(),
			byte(xproto.MotionNotify),
			0,
			0,
			controller.xu.RootWin(),
			int16(x),
			int16(y),
			0,
		).Check(); err != nil {
			return fmt.Errorf("X11 鼠标移动失败: %w", err)
		}
		controller.xu.Sync()
	}

	now := time.Now()
	if controller.lastMoveLog.IsZero() || now.Sub(controller.lastMoveLog) >= time.Second {
		log.Printf("🧪 [linux-input] 鼠标移动注入 -> backend=%s root=(%d,%d)", controller.backend, x, y)
		controller.lastMoveLog = now
	}
	return nil
}

func platformMouseButton(button string, down bool, x, y int) error {
	controller, err := getLinuxInputController()
	if err != nil {
		return err
	}

	controller.mu.Lock()
	defer controller.mu.Unlock()

	if controller.backend == linuxInputBackendYdotool {
		return controller.runYdotoolMouseButtonLocked(button, down)
	}

	return controller.injectX11MouseButtonLocked(button, down, x, y)
}

func platformSyncExtraMouseButtons(c *DesktopCapture, state mouseButtonState, x, y int) error {
	return nil
}

func resolvePlatformKeyCode(event *ControlEvent) int {
	controller, err := getLinuxInputController()
	if err != nil {
		log.Printf("⚠️  [linux-input] 无法初始化输入控制器: %v", err)
		return -1
	}
	if controller.backend == linuxInputBackendYdotool {
		return mapControlEventToLinuxInputKeyCode(event)
	}
	return mapControlEventToPlatformKeyCode(event)
}

func platformTracksKeyState() bool {
	return true
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

	controller.mu.Lock()
	defer controller.mu.Unlock()

	if controller.backend == linuxInputBackendYdotool {
		log.Printf("🧪 [linux-input] 键盘事件注入 -> backend=%s keycode=%d down=%v", controller.backend, keycode, down)
		return controller.runYdotoolKeyToggleLocked(keycode, down)
	}
	if keycode <= 0 || keycode > 0xFF {
		return fmt.Errorf("unsupported X11 keycode: %d", keycode)
	}

	eventType := byte(xproto.KeyPress)
	if !down {
		eventType = byte(xproto.KeyRelease)
	}

	rootX, rootY, err := queryPointerRootPosition(controller)
	if err != nil {
		log.Printf("⚠️  [linux-input] 读取当前指针位置失败，键盘事件将回退到 (0,0): %v", err)
		rootX = 0
		rootY = 0
	}

	log.Printf("🧪 [linux-input] 键盘事件注入 -> backend=%s keycode=%d down=%v root=(%d,%d)", controller.backend, keycode, down, rootX, rootY)
	if err := xtest.FakeInputChecked(
		controller.xu.Conn(),
		eventType,
		byte(keycode),
		0,
		controller.xu.RootWin(),
		rootX,
		rootY,
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
		log.Printf("⚠️  [linux-input] 无法初始化输入控制器: %v", err)
		return -1
	}
	if controller.backend != linuxInputBackendX11 {
		return mapJSKeyCodeToLinuxInputKeyCode(jsKeyCode)
	}

	keyName, ok := linuxJSKeyCodeToXKey[jsKeyCode]
	if !ok {
		log.Printf("⚠️  [linux-input] 未找到 JS keyCode=%d 对应的 X11 keysym", jsKeyCode)
		return -1
	}

	controller.mu.Lock()
	defer controller.mu.Unlock()

	if keycode, ok := controller.keycodeMap[keyName]; ok {
		return int(keycode)
	}

	keycodes := keybind.StrToKeycodes(controller.xu, keyName)
	if len(keycodes) == 0 {
		log.Printf("⚠️  [linux-input] keysym=%s 未解析到 X11 keycode", keyName)
		return -1
	}

	controller.keycodeMap[keyName] = keycodes[0]
	return int(keycodes[0])
}

func mapControlEventToLinuxInputKeyCode(event *ControlEvent) int {
	if event.Code != "" {
		if keycode, ok := linuxBrowserCodeToInputKeyCode[event.Code]; ok {
			return keycode
		}
	}

	jsKeyCode := normalizeBrowserKeyCode(event.KeyCode)
	if jsKeyCode == 0 {
		return -1
	}
	return mapJSKeyCodeToLinuxInputKeyCode(jsKeyCode)
}

func queryPointerRootPosition(controller *linuxInputController) (int16, int16, error) {
	reply, err := xproto.QueryPointer(controller.xu.Conn(), controller.xu.RootWin()).Reply()
	if err != nil {
		return 0, 0, err
	}

	return reply.RootX, reply.RootY, nil
}

func (controller *linuxInputController) injectX11MouseButtonLocked(button string, down bool, x, y int) error {
	var detail byte
	switch button {
	case "left":
		detail = 1
	case "middle":
		detail = 2
	case "right":
		detail = 3
	case "scroll_up":
		detail = 4
	case "scroll_down":
		detail = 5
	default:
		return fmt.Errorf("unsupported mouse button: %s", button)
	}

	eventType := byte(xproto.ButtonPress)
	if !down {
		eventType = byte(xproto.ButtonRelease)
	}

	rootX, rootY, err := queryPointerRootPosition(controller)
	if err != nil {
		log.Printf("⚠️  [linux-input] 读取当前指针位置失败，鼠标按键事件将回退到 (0,0): %v", err)
		rootX = 0
		rootY = 0
	}

	log.Printf("🧪 [linux-input] 鼠标按键注入 -> backend=%s button=%s down=%v root=(%d,%d) target=(%d,%d)", controller.backend, button, down, rootX, rootY, x, y)
	if err := xtest.FakeInputChecked(
		controller.xu.Conn(),
		eventType,
		detail,
		0,
		controller.xu.RootWin(),
		rootX,
		rootY,
		0,
	).Check(); err != nil {
		return fmt.Errorf("X11 鼠标按键事件失败: %w", err)
	}

	controller.xu.Sync()
	return nil
}

func (controller *linuxInputController) runYdotoolMouseButtonLocked(button string, down bool) error {
	if button == "scroll_up" || button == "scroll_down" {
		if down && !controller.warnedWheelNative {
			controller.warnedWheelNative = true
			log.Printf("⚠️  [linux-input] 当前 ydotool 后端尚未实现滚轮注入，滚轮事件将被忽略。")
		}
		return nil
	}

	buttonID := map[string]byte{
		"left":   0x00,
		"right":  0x01,
		"middle": 0x02,
	}
	id, ok := buttonID[button]
	if !ok {
		return fmt.Errorf("unsupported ydotool mouse button: %s", button)
	}

	action := byte(0x80)
	if down {
		action = 0x40
	}
	command := fmt.Sprintf("0x%02X", action|id)
	log.Printf("🧪 [linux-input] 鼠标按键注入 -> backend=%s button=%s down=%v", controller.backend, button, down)
	return controller.runYdotoolLocked("click", command)
}

func (controller *linuxInputController) runYdotoolKeyToggleLocked(keycode int, down bool) error {
	if keycode <= 0 {
		return fmt.Errorf("unsupported Linux input keycode: %d", keycode)
	}

	state := "0"
	if down {
		state = "1"
	}
	return controller.runYdotoolLocked("key", fmt.Sprintf("%d:%s", keycode, state))
}

func (controller *linuxInputController) runYdotoolLocked(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, controller.ydotoolPath, args...)
	if controller.ydotoolSocketPath != "" {
		cmd.Env = append(os.Environ(), "YDOTOOL_SOCKET="+controller.ydotoolSocketPath)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		repairErr := controller.repairYdotoolLocked()
		if repairErr == nil {
			retryCtx, retryCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer retryCancel()
			retryCmd := exec.CommandContext(retryCtx, controller.ydotoolPath, args...)
			if controller.ydotoolSocketPath != "" {
				retryCmd.Env = append(os.Environ(), "YDOTOOL_SOCKET="+controller.ydotoolSocketPath)
			}
			retryOutput, retryErr := retryCmd.CombinedOutput()
			if retryErr == nil {
				return nil
			}
			trimmed := strings.TrimSpace(string(retryOutput))
			if trimmed == "" {
				return fmt.Errorf("ydotool %s 重试失败: %w", strings.Join(args, " "), retryErr)
			}
			return fmt.Errorf("ydotool %s 重试失败: %w: %s", strings.Join(args, " "), retryErr, trimmed)
		}
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return fmt.Errorf("ydotool %s 失败: %w；并且 ydotoold 修复失败: %v", strings.Join(args, " "), err, repairErr)
		}
		return fmt.Errorf("ydotool %s 失败: %w: %s；并且 ydotoold 修复失败: %v", strings.Join(args, " "), err, trimmed, repairErr)
	}
	return nil
}

func (controller *linuxInputController) repairYdotoolLocked() error {
	if controller.ydotoolPath == "" || controller.ydotooldPath == "" {
		return fmt.Errorf("ydotool/ydotoold 路径尚未初始化")
	}

	socketPath, err := ensureYdotoolDaemonReady(controller.ydotoolPath, controller.ydotooldPath)
	if err != nil {
		return err
	}
	controller.ydotoolSocketPath = socketPath
	return nil
}

var (
	linuxInputDebugMu      sync.Mutex
	linuxLastMouseEventLog time.Time
)

func platformLogMouseControlEvent(event *ControlEvent, screenX, screenY int) {
	linuxInputDebugMu.Lock()
	defer linuxInputDebugMu.Unlock()

	now := time.Now()
	if event.MouseMask == 0 && !linuxLastMouseEventLog.IsZero() && now.Sub(linuxLastMouseEventLog) < time.Second {
		return
	}
	linuxLastMouseEventLog = now

	log.Printf("🧪 [linux-input] 鼠标事件 -> raw=(%d,%d) canvas=%dx%d mask=%d mapped=(%d,%d)",
		event.MouseX, event.MouseY, event.CanvasWidth, event.CanvasHeight, event.MouseMask, screenX, screenY)
}

func platformLogKeyboardControlEvent(event *ControlEvent, platformKeyCode int, down bool) {
	log.Printf("🧪 [linux-input] 键盘事件 -> code=%q keyCode=%d mapped=%d down=%v",
		event.Code, normalizeBrowserKeyCode(event.KeyCode), platformKeyCode, down)
}

var linuxJSKeyCodeToXKey = map[int]string{
	8:   "BackSpace",
	9:   "Tab",
	13:  "Return",
	19:  "Pause",
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
	96:  "KP_0",
	97:  "KP_1",
	98:  "KP_2",
	99:  "KP_3",
	100: "KP_4",
	101: "KP_5",
	102: "KP_6",
	103: "KP_7",
	104: "KP_8",
	105: "KP_9",
	106: "KP_Multiply",
	107: "KP_Add",
	109: "KP_Subtract",
	110: "KP_Decimal",
	111: "KP_Divide",
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
	144: "Num_Lock",
	145: "Scroll_Lock",
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

var linuxJSKeyCodeToInputKeyCode = map[int]int{
	8:   14,
	9:   15,
	13:  28,
	16:  42,
	17:  29,
	18:  56,
	19:  119,
	20:  58,
	27:  1,
	32:  57,
	33:  104,
	34:  109,
	35:  107,
	36:  102,
	37:  105,
	38:  103,
	39:  106,
	40:  108,
	45:  110,
	46:  111,
	48:  11,
	49:  2,
	50:  3,
	51:  4,
	52:  5,
	53:  6,
	54:  7,
	55:  8,
	56:  9,
	57:  10,
	65:  30,
	66:  48,
	67:  46,
	68:  32,
	69:  18,
	70:  33,
	71:  34,
	72:  35,
	73:  23,
	74:  36,
	75:  37,
	76:  38,
	77:  50,
	78:  49,
	79:  24,
	80:  25,
	81:  16,
	82:  19,
	83:  31,
	84:  20,
	85:  22,
	86:  47,
	87:  17,
	88:  45,
	89:  21,
	90:  44,
	91:  125,
	92:  126,
	93:  139,
	96:  82,
	97:  79,
	98:  80,
	99:  81,
	100: 75,
	101: 76,
	102: 77,
	103: 71,
	104: 72,
	105: 73,
	106: 55,
	107: 78,
	109: 74,
	110: 83,
	111: 98,
	112: 59,
	113: 60,
	114: 61,
	115: 62,
	116: 63,
	117: 64,
	118: 65,
	119: 66,
	120: 67,
	121: 68,
	122: 87,
	123: 88,
	144: 69,
	145: 70,
	186: 39,
	187: 13,
	188: 51,
	189: 12,
	190: 52,
	191: 53,
	192: 41,
	219: 26,
	220: 43,
	221: 27,
	222: 40,
}

var linuxBrowserCodeToInputKeyCode = map[string]int{
	"ShiftRight":     54,
	"ControlRight":   97,
	"AltRight":       100,
	"MetaLeft":       125,
	"MetaRight":      126,
	"ContextMenu":    139,
	"Numpad0":        82,
	"Numpad1":        79,
	"Numpad2":        80,
	"Numpad3":        81,
	"Numpad4":        75,
	"Numpad5":        76,
	"Numpad6":        77,
	"Numpad7":        71,
	"Numpad8":        72,
	"Numpad9":        73,
	"NumpadAdd":      78,
	"NumpadDivide":   98,
	"NumpadDecimal":  83,
	"NumpadEnter":    96,
	"NumpadMultiply": 55,
	"NumpadSubtract": 74,
}

func mapJSKeyCodeToLinuxInputKeyCode(jsKeyCode int) int {
	keycode, ok := linuxJSKeyCodeToInputKeyCode[jsKeyCode]
	if !ok {
		log.Printf("⚠️  [linux-input] 未找到 JS keyCode=%d 对应的 Linux input keycode", jsKeyCode)
		return -1
	}
	return keycode
}
