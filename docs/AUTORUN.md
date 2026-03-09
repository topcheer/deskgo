# DeskGo 自动运行说明

## 设计原则

DeskGo 的 Desktop CLI 需要运行在真实的登录用户桌面会话里，才能正确捕获屏幕并响应输入事件。

因此，自动运行安装脚本默认采用**当前用户级**的登录自启动方案，而不是传统的系统后台服务：

- **Windows**：计划任务（当前用户登录时启动）
- **macOS**：LaunchAgent（当前用户登录时启动）
- **Linux**：
  - 默认推荐 **XDG Autostart**（图形会话登录时启动）
  - 可选 **systemd user service**（有自动重启语义，但要求用户 systemd 会话工作正常）

这样的设计有几个好处：

- 普通用户无需管理员权限即可安装
- 能运行在真实桌面会话中，避免“后台服务跑起来但拿不到桌面”的问题
- 卸载时只清理当前用户目录，不污染系统级服务列表

## 脚本位置

- macOS / Linux：`scripts/deskgo-autostart.sh`
- Windows：`scripts/deskgo-autostart.ps1`

发布包中也会附带：

- `deskgo-autostart.sh`
- `deskgo-autostart.ps1`

## 在线安装

如果代码已经推送到 GitHub，也可以直接在线获取脚本：

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/topcheer/deskgo/master/scripts/deskgo-autostart.sh -o /tmp/deskgo-autostart.sh
bash /tmp/deskgo-autostart.sh install
```

### Windows

```powershell
$script = Join-Path $env:TEMP 'deskgo-autostart.ps1'
Invoke-WebRequest 'https://raw.githubusercontent.com/topcheer/deskgo/master/scripts/deskgo-autostart.ps1' -OutFile $script
powershell -ExecutionPolicy Bypass -File $script install
```

> 注意：在线命令下载的是安装脚本本身，脚本随后会去 GitHub release 下载对应平台的 CLI 二进制和 `SHA256SUMS.txt`。

## 引导式安装

### macOS / Linux

```bash
./scripts/deskgo-autostart.sh install
```

脚本会交互式询问：

- Relay 地址
- Linux 自动运行模式（`xdg-autostart` / `systemd-user`）
- Codec
- 固定 Session 名字
- 要安装的 release 版本

### Windows

```powershell
powershell -ExecutionPolicy Bypass -File .\\scripts\\deskgo-autostart.ps1 install
```

脚本会交互式询问：

- Relay 地址
- 固定 Session 名字
- 要安装的 release 版本

> 说明：Windows 当前自动运行模式仅支持 `jpeg`，因为 H.264 的 Windows 自动运行编码路径尚未完成。

## 非引导式安装

### macOS / Linux

```bash
./scripts/deskgo-autostart.sh install \
  --relay-server https://deskgo.example.com \
  --codec h264 \
  --session office-mac \
  --autostart-mode xdg-autostart \
  --version v0.1.1 \
  --non-interactive
```

Linux 如果更偏向自动重启语义，也可以使用：

```bash
./scripts/deskgo-autostart.sh install \
  --relay-server wss://deskgo.example.com/api/desktop \
  --codec jpeg \
  --session reception-linux \
  --autostart-mode systemd-user \
  --non-interactive
```

### Windows

```powershell
powershell -ExecutionPolicy Bypass -File .\\scripts\\deskgo-autostart.ps1 install `
  -RelayServer https://deskgo.example.com `
  -Session office-pc `
  -Version v0.1.1 `
  -NonInteractive
```

## 卸载

### macOS / Linux

```bash
./scripts/deskgo-autostart.sh uninstall
./scripts/deskgo-autostart.sh uninstall --non-interactive
```

### Windows

```powershell
powershell -ExecutionPolicy Bypass -File .\\scripts\\deskgo-autostart.ps1 uninstall
powershell -ExecutionPolicy Bypass -File .\\scripts\\deskgo-autostart.ps1 uninstall -NonInteractive
```

## Relay 地址输入规则

脚本支持几种常见写法，并会自动规范化为 CLI 需要的 WebSocket 地址：

- `https://deskgo.example.com`
- `https://deskgo.example.com/api/desktop`
- `wss://deskgo.example.com/api/desktop`
- `deskgo.example.com`

最终都会被转换成类似：

- `wss://deskgo.example.com/api/desktop`

本地明文环境也可以使用：

- `http://127.0.0.1:8082`
- `ws://127.0.0.1:8082/api/desktop`

## Session 名字规则

自动运行模式下，Session 名字必须稳定，不能每次启动随机生成。

脚本默认使用当前机器名，并自动清洗成安全格式，只保留：

- 字母
- 数字
- `.`
- `_`
- `-`

如果你输入了带空格或特殊字符的名字，脚本会自动把它们折叠成 `-`。

## 与 latest release 的兼容性

- 默认 `--version` 是 `latest`
- 这要求 latest release 中存在匹配平台的 `deskgo-desktop-<os>-<arch>` 资产和 `SHA256SUMS.txt`
- 如果 latest release 缺少对应二进制，脚本会在下载阶段明确失败
- 如果你需要可预测的批量部署，请显式传入 `--version vX.Y.Z`
- 安装脚本会把配置写到安装目录中的 `deskgo.json`，因此也兼容尚未支持 `-config` 的旧 release CLI

## 平台差异与建议

### Windows

- 使用**当前用户计划任务**而不是 Windows Service
- 原因是桌面捕获需要运行在交互式用户会话中；传统系统服务通常位于 Session 0，并不适合桌面串流
- 普通用户一般也可以安装，无需管理员权限

### macOS

- 使用 `~/Library/LaunchAgents/` 下的 LaunchAgent
- 这是桌面类应用在当前用户会话中自动启动的标准方式
- 不建议改成 LaunchDaemon，因为它不适合桌面捕获

### Linux

- 默认推荐 `xdg-autostart`
  - 最适合普通桌面登录场景
  - 不需要 root
  - 能自然获得图形会话环境变量
- `systemd-user` 是可选高级模式
  - 有自动重启能力
  - 适合 systemd 用户会话配置完善的桌面环境
  - 在极简或非标准桌面环境下，图形环境变量可能需要额外处理

## Dry run

两个脚本都支持 `--dry-run`，可以先看将要执行的动作：

```bash
./scripts/deskgo-autostart.sh install --non-interactive --dry-run
```

```powershell
powershell -ExecutionPolicy Bypass -File .\\scripts\\deskgo-autostart.ps1 install -NonInteractive -DryRun
```
