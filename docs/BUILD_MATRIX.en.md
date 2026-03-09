# DeskGo build and release matrix

This document describes what `./build.sh` produces today, which targets depend on a specific host environment, and what is packaged into the Docker image.

## Upstream repository

- GitHub: <https://github.com/topcheer/deskgo>

## One-command build output

Run:

```bash
./build.sh
```

Output locations:

- `bin/relay-server`: host-runnable relay binary
- `bin/deskgo-desktop` or `bin/deskgo-desktop-h264`: host-runnable desktop CLI
- `downloads/`: cross-architecture release artifacts
- `downloads/SHA256SUMS.txt`: checksum manifest

## Desktop CLI release matrix

| OS | Arch | File name | Notes |
| --- | --- | --- | --- |
| macOS | amd64 | `deskgo-desktop-darwin-amd64` | Built only when `./build.sh` runs on a macOS host |
| macOS | arm64 | `deskgo-desktop-darwin-arm64` | Built only when `./build.sh` runs on a macOS host |
| Windows | amd64 | `deskgo-desktop-windows-amd64.exe` | Included by default |
| Windows | arm64 | `deskgo-desktop-windows-arm64.exe` | Included by default |
| Linux | amd64 | `deskgo-desktop-linux-amd64` | Included by default |
| Linux | arm64 | `deskgo-desktop-linux-arm64` | Included by default |
| Linux | armv7 | `deskgo-desktop-linux-armv7` | Included by default |
| Linux | riscv64 | `deskgo-desktop-linux-riscv64` | Included by default |

### Codec and platform notes

- macOS: native H.264-first path by default
- Linux: H.264-first by default; ffmpeg/libx264 is used when available, otherwise the client falls back to JPEG
- Windows: currently defaults to the stable compatibility path while keeping input control support

## Relay release matrix

| OS | Arch | File name |
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

## Docker image matrix

GitHub Actions currently builds these image platforms:

- `linux/amd64`
- `linux/arm64`
- `linux/arm/v7`

Image tag strategy:

- `ghcr.io/topcheer/deskgo:latest`
- `ghcr.io/topcheer/deskgo:<tag>`
- `ghcr.io/topcheer/deskgo:sha-<commit>`

## GitHub Actions

- `.github/workflows/release-artifacts.yml`: builds the multi-architecture Desktop CLI and relay artifacts and assembles `SHA256SUMS.txt`
- `.github/workflows/docker-image.yml`: builds and publishes the multi-architecture GHCR image

## Docker packaging notes

The `Dockerfile` runs `./build.sh` during the builder stage and then copies the following into the final image:

- `relay-server`
- `web/`
- `downloads/`

Important:

- When `./build.sh` runs inside a Linux container, macOS desktop CLI artifacts are not generated
- If you want macOS desktop CLI binaries bundled into an image, generate them on a macOS host first and then build the image

## Website download behavior

The relay landing page scans `downloads/` and automatically exposes:

- grouped Desktop CLI downloads
- grouped relay server downloads
- the `SHA256SUMS.txt` checksum entry
- both `/` (Chinese) and `/en` (English) landing routes

## Recommended validation

At minimum, validate the following commands:

```bash
go build ./cmd/relay
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags desktop ./cmd/client
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags desktop ./cmd/client
./build.sh
```
