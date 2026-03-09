# DeskGo 构建与发布矩阵

本文档说明 `./build.sh` 当前会生成哪些二进制、哪些目标需要特定主机环境，以及 Docker 打包时会包含哪些产物。

## 上游仓库

- GitHub: <https://github.com/topcheer/deskgo>

## 一键构建脚本输出

执行：

```bash
./build.sh
```

输出目录：

- `bin/relay-server`：当前主机可直接运行的 Relay
- `bin/deskgo-desktop` 或 `bin/deskgo-desktop-h264`：当前主机可直接运行的桌面 CLI
- `downloads/`：多架构发布包
- `downloads/deskgo-autostart.sh` / `downloads/deskgo-autostart.ps1`：自动运行安装脚本
- `downloads/SHA256SUMS.txt`：发布包校验文件

## Desktop CLI 发布矩阵

| 操作系统 | 架构 | 文件名 | 备注 |
| --- | --- | --- | --- |
| macOS | amd64 | `deskgo-desktop-darwin-amd64` | 仅在 macOS 主机运行 `./build.sh` 时生成 |
| macOS | arm64 | `deskgo-desktop-darwin-arm64` | 仅在 macOS 主机运行 `./build.sh` 时生成 |
| Windows | amd64 | `deskgo-desktop-windows-amd64.exe` | 默认发布 |
| Windows | arm64 | `deskgo-desktop-windows-arm64.exe` | 默认发布 |
| Linux | amd64 | `deskgo-desktop-linux-amd64` | 默认发布 |
| Linux | arm64 | `deskgo-desktop-linux-arm64` | 默认发布 |
| Linux | armv7 | `deskgo-desktop-linux-armv7` | 默认发布 |
| Linux | riscv64 | `deskgo-desktop-linux-riscv64` | 默认发布 |

### 编码与平台说明

- macOS：默认优先 H.264 原生编码路径
- Linux：默认优先 H.264；如果系统可用 ffmpeg/libx264，则使用该路径，否则回退到 JPEG
- Windows：当前默认以稳定兼容路径为主，并保留输入控制能力

## Relay 发布矩阵

| 操作系统 | 架构 | 文件名 |
| --- | --- | --- |
| macOS | amd64 | `deskgo-relay-darwin-amd64` |
| macOS | arm64 | `deskgo-relay-darwin-arm64` |
| Windows | amd64 | `deskgo-relay-windows-amd64.exe` |
| Windows | arm64 | `deskgo-relay-windows-arm64.exe` |
| Linux | amd64 | `deskgo-relay-linux-amd64` |
| Linux | arm64 | `deskgo-relay-linux-arm64` |
| Linux | armv7 | `deskgo-relay-linux-armv7` |
| Linux | riscv64 | `deskgo-relay-linux-riscv64` |
| Linux | ppc64le | `deskgo-relay-linux-ppc64le` |
| Linux | s390x | `deskgo-relay-linux-s390x` |

## Docker 镜像矩阵

GitHub Actions 当前会构建以下镜像平台：

- `linux/amd64`
- `linux/arm64`
- `linux/arm/v7`

镜像标签策略：

- `ghcr.io/topcheer/deskgo:latest`
- `ghcr.io/topcheer/deskgo:<tag>`
- `ghcr.io/topcheer/deskgo:sha-<commit>`

## GitHub Actions

- `.github/workflows/release-artifacts.yml`：构建多架构 Desktop CLI 与 Relay 产物，附带自动运行安装脚本，并汇总 `SHA256SUMS.txt`
- `.github/workflows/docker-image.yml`：构建并发布多架构 GHCR 镜像

## Docker 打包说明

`Dockerfile` 在构建阶段会执行 `./build.sh`，随后把以下内容放入最终镜像：

- `relay-server`
- `web/`
- `downloads/`

注意：

- Linux 容器内执行 `./build.sh` 时，不会生成 macOS Desktop CLI 包
- 如果你希望镜像内也包含 macOS Desktop CLI，请先在 macOS 主机生成相应文件，再执行镜像构建

## 网站下载页行为

Relay 首页会扫描 `downloads/` 目录，并自动展示：

- Desktop CLI 分组下载
- Relay 服务器分组下载
- `SHA256SUMS.txt` 校验入口
- `/` 中文首页与 `/en` 英文首页

## 验证建议

推荐至少验证以下命令：

```bash
go build ./cmd/relay
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags desktop ./cmd/client
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags desktop ./cmd/client
./build.sh
```
