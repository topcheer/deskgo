# DeskGo

DeskGo 是一个以 Go 编写的远程桌面串流方案，包含 Desktop CLI、Relay 服务器和浏览器查看器三部分。
当前仓库已经完成遗留实验代码清理，聚焦在原生桌面采集、WebSocket 中继、浏览器查看与多架构发布。

- 上游仓库：<https://github.com/topcheer/deskgo>
- 在线 Demo：<https://deskgo.ystone.us>
- 英文文档：[`README.en.md`](README.en.md)
- 自动运行说明：[`docs/AUTORUN.md`](docs/AUTORUN.md)
- 构建矩阵：[`docs/BUILD_MATRIX.md`](docs/BUILD_MATRIX.md)
- 部署指南：[`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md)
- 最新 Release Note 草案：[`docs/releases/v0.1.4.zh-CN.md`](docs/releases/v0.1.4.zh-CN.md)

## 核心能力

### Desktop CLI

- macOS：默认优先 H.264，支持原生桌面捕获
- Linux：默认优先 H.264（检测到 ffmpeg/libx264 时启用），支持 X11/XTEST 输入控制
- Windows：保持与 macOS 接近的一致会话输出与输入交互体验
- 支持通过 `-proxy`、`deskgo.json` 中的 `proxy` 字段，或 `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY`（自动适配 `ws://` 与 `wss://`，也支持 `WS_PROXY` / `WSS_PROXY`）连接 Relay

### Relay Server

- 负责桌面端与浏览器端之间的 WebSocket 会话转发
- 网站首页与会话页直接由 Relay 提供
- 自动从 `downloads/` 目录暴露多架构下载产物
- CLI 断开时会主动通知查看器并关闭连接

### Web Viewer

- H.264 / JPEG 自动协商
- 串流开始时自动隐藏页头页尾
- 中英双语入口：`/`、`/en`、`/session/:id`、`/en/session/:id`

## 快速开始

### 1. 从源码构建

```bash
git clone https://github.com/topcheer/deskgo.git
cd deskgo
./build.sh
```

构建完成后将得到：

- 当前平台运行文件：`bin/relay-server`、`bin/deskgo-desktop*`
- 多架构下载包：`downloads/`
- 自动运行安装脚本：`downloads/deskgo-autostart.sh`、`downloads/deskgo-autostart.ps1`
- 校验文件：`downloads/SHA256SUMS.txt`

### 2. 启动 Relay

```bash
./bin/relay-server
```

默认访问地址：

- 中文首页：<http://localhost:8082>
- 英文首页：<http://localhost:8082/en>
- 中文会话页：`/session/<session-id>`
- 英文会话页：`/en/session/<session-id>`

### 3. 启动 Desktop CLI

macOS：

```bash
./bin/deskgo-desktop-h264 -server ws://localhost:8082/api/desktop -session demo -codec h264
```

Linux / Windows：

```bash
./bin/deskgo-desktop -server ws://localhost:8082/api/desktop -session demo
```

通过代理连接 Relay：

```bash
./bin/deskgo-desktop -server wss://deskgo.ystone.us/api/desktop -session demo -proxy http://proxy.internal:8080
```

也可以使用环境变量：

```bash
HTTPS_PROXY=http://proxy.internal:8080 ./bin/deskgo-desktop -server wss://deskgo.ystone.us/api/desktop -session demo
```

### 4. 安装自动运行

推荐直接使用 GitHub 在线脚本安装（安装脚本来自 GitHub Raw，CLI 默认从 GitHub latest release 下载）：

```bash
rm -f /tmp/deskgo-autostart.sh
curl -fsSL -H 'Cache-Control: no-cache' "https://raw.githubusercontent.com/topcheer/deskgo/master/scripts/deskgo-autostart.sh?ts=$(date +%s)" -o /tmp/deskgo-autostart.sh
bash /tmp/deskgo-autostart.sh install
```

```powershell
$script = Join-Path $env:TEMP 'deskgo-autostart.ps1'
$uri = "https://raw.githubusercontent.com/topcheer/deskgo/master/scripts/deskgo-autostart.ps1?ts=$([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())"
Remove-Item $script -ErrorAction SilentlyContinue
Invoke-WebRequest $uri -UseBasicParsing -Headers @{ 'Cache-Control' = 'no-cache' } -OutFile $script
powershell -ExecutionPolicy Bypass -File $script install -Codec h264
```

上面的 `?ts=` 与 `Cache-Control: no-cache` 用于尽量绕开代理或 CDN 的陈旧缓存，确保拉到最新脚本。

如果你已经 clone 仓库，也可以直接运行仓库内的同一份脚本：

macOS / Linux：

```bash
./scripts/deskgo-autostart.sh install
```

Windows：

```powershell
powershell -ExecutionPolicy Bypass -File .\\scripts\\deskgo-autostart.ps1 install -Codec h264
```

在线安装成功仍依赖 GitHub latest release 中存在匹配平台的 `deskgo-desktop-<os>-<arch>` 资产和 `SHA256SUMS.txt`；如果你要做可重复部署，请显式传入 `--version` 固定版本。
Windows 自动运行当前会注册**当前用户级、隐藏启动**的计划任务；它会在登录后延迟约 15 秒再尝试启动 CLI，日志位于 `%LOCALAPPDATA%\DeskGo\logs\desktop.log`。
更多引导式与非引导式示例见 [`docs/AUTORUN.md`](docs/AUTORUN.md)。

## Docker 与云部署

### 本地构建镜像

```bash
docker compose up -d --build
```

### 使用预构建镜像部署

```bash
docker compose -f docker-compose.prod.yml up -d
```

预构建镜像地址：

- `ghcr.io/topcheer/deskgo:latest`

说明：

- Docker 镜像会打包 Relay 与 `downloads/` 中的发布产物
- 在 Linux 构建环境中，`./build.sh` 会生成 Linux / Windows Desktop CLI 与多架构 Relay 包
- 如果希望镜像中也包含 macOS Desktop CLI，请先在 macOS 主机上运行一次 `./build.sh`
- 云平台部署、反向代理与 Cloudflare Tunnel 说明见 [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md)

## 发布矩阵摘要

### Desktop CLI 发布包

- macOS：`darwin/amd64`、`darwin/arm64`（仅在 macOS 主机或 GitHub macOS Runner 上构建）
- Windows：`windows/amd64`、`windows/arm64`
- Linux：`linux/amd64`、`linux/arm64`、`linux/armv7`、`linux/riscv64`

### Relay 发布包

- macOS：`darwin/amd64`、`darwin/arm64`
- Windows：`windows/amd64`、`windows/arm64`
- Linux：`linux/amd64`、`linux/arm64`、`linux/armv7`、`linux/riscv64`、`linux/ppc64le`、`linux/s390x`

更多细节见 [`docs/BUILD_MATRIX.md`](docs/BUILD_MATRIX.md)。

## CI / 发布自动化

仓库包含两套 GitHub Actions：

- `.github/workflows/release-artifacts.yml`：构建多架构 Desktop CLI 与 Relay 产物并上传为 Actions artifacts
- `.github/workflows/docker-image.yml`：构建并发布多架构 GHCR 镜像

## 文档索引

- [`README.en.md`](README.en.md)
- [`docs/AUTORUN.md`](docs/AUTORUN.md)
- [`docs/AUTORUN.en.md`](docs/AUTORUN.en.md)
- [`docs/BUILD_MATRIX.md`](docs/BUILD_MATRIX.md)
- [`docs/BUILD_MATRIX.en.md`](docs/BUILD_MATRIX.en.md)
- [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md)
- [`docs/DEPLOYMENT.en.md`](docs/DEPLOYMENT.en.md)
- [`docs/releases/v0.1.4.zh-CN.md`](docs/releases/v0.1.4.zh-CN.md)
- [`docs/releases/v0.1.4.md`](docs/releases/v0.1.4.md)
- [`docs/releases/v0.1.3.zh-CN.md`](docs/releases/v0.1.3.zh-CN.md)
- [`docs/releases/v0.1.3.md`](docs/releases/v0.1.3.md)
- [`docs/releases/v0.1.2.zh-CN.md`](docs/releases/v0.1.2.zh-CN.md)
- [`docs/releases/v0.1.2.md`](docs/releases/v0.1.2.md)
- [`docs/releases/v0.1.1.zh-CN.md`](docs/releases/v0.1.1.zh-CN.md)
- [`docs/releases/v0.1.1.md`](docs/releases/v0.1.1.md)
- [`docs/releases/v0.1.0.zh-CN.md`](docs/releases/v0.1.0.zh-CN.md)
- [`docs/releases/v0.1.0.md`](docs/releases/v0.1.0.md)

## 许可证

本项目采用 MIT License，允许免费商业使用、修改、分发和再发布。
详见 [`LICENSE`](LICENSE)。
