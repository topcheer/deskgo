# DeskGo

DeskGo is a Go-based remote desktop streaming stack composed of a Desktop CLI, a relay server, and a browser viewer.
This repository has been cleaned up to remove the old experimental scope and now focuses on native desktop capture, WebSocket relay delivery, browser viewing, and multi-architecture releases.

- Upstream repository: <https://github.com/topcheer/deskgo>
- Live demo: <https://deskgo.zty8.cn>
- Chinese README: [`README.md`](README.md)
- Build matrix: [`docs/BUILD_MATRIX.en.md`](docs/BUILD_MATRIX.en.md)
- Deployment guide: [`docs/DEPLOYMENT.en.md`](docs/DEPLOYMENT.en.md)
- Latest release notes draft: [`docs/releases/v0.1.1.md`](docs/releases/v0.1.1.md)

## Core capabilities

### Desktop CLI

- macOS: H.264-first by default with native desktop capture
- Linux: H.264-first by default when ffmpeg/libx264 is available, with X11/XTEST input control
- Windows: session output and input behavior aligned as closely as possible with macOS
- Relay proxy support via `-proxy`, the `proxy` field in `deskgo.json`, or `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY` (with automatic `ws://` / `wss://` mapping, plus optional `WS_PROXY` / `WSS_PROXY`)

### Relay server

- Forwards WebSocket sessions between the desktop side and browser viewers
- Serves the landing page and session pages directly
- Exposes multi-architecture downloads from `downloads/`
- Proactively notifies and disconnects viewers when the CLI exits

### Web viewer

- Automatic H.264 / JPEG negotiation
- Header and footer chrome hidden while streaming is active
- Bilingual entry points: `/`, `/en`, `/session/:id`, and `/en/session/:id`

## Quick start

### 1. Build from source

```bash
git clone https://github.com/topcheer/deskgo.git
cd deskgo
./build.sh
```

This produces:

- Host binaries: `bin/relay-server` and `bin/deskgo-desktop*`
- Cross-architecture release packages in `downloads/`
- A checksum manifest at `downloads/SHA256SUMS.txt`

### 2. Start the relay

```bash
./bin/relay-server
```

Default endpoints:

- Chinese landing page: <http://localhost:8082>
- English landing page: <http://localhost:8082/en>
- Chinese session page: `/session/<session-id>`
- English session page: `/en/session/<session-id>`

### 3. Start the Desktop CLI

macOS:

```bash
./bin/deskgo-desktop-h264 -server ws://localhost:8082/api/desktop -session demo -codec h264
```

Linux / Windows:

```bash
./bin/deskgo-desktop -server ws://localhost:8082/api/desktop -session demo
```

Use a relay proxy:

```bash
./bin/deskgo-desktop -server wss://deskgo.zty8.cn/api/desktop -session demo -proxy http://proxy.internal:8080
```

You can also rely on environment variables:

```bash
HTTPS_PROXY=http://proxy.internal:8080 ./bin/deskgo-desktop -server wss://deskgo.zty8.cn/api/desktop -session demo
```

## Docker and cloud deployment

### Build locally

```bash
docker compose up -d --build
```

### Deploy the published image

```bash
docker compose -f docker-compose.prod.yml up -d
```

Published image:

- `ghcr.io/topcheer/deskgo:latest`

Notes:

- The Docker image bundles the relay and the generated release artifacts from `downloads/`
- In a Linux build environment, `./build.sh` produces Linux / Windows Desktop CLI packages and the expanded relay matrix
- If you also want macOS Desktop CLI artifacts inside the image, run `./build.sh` once on a macOS host first
- Cloud deployment, reverse proxy, and Cloudflare Tunnel guidance lives in [`docs/DEPLOYMENT.en.md`](docs/DEPLOYMENT.en.md)

## Release matrix summary

### Desktop CLI packages

- macOS: `darwin/amd64`, `darwin/arm64` (built only on macOS hosts or GitHub macOS runners)
- Windows: `windows/amd64`, `windows/arm64`
- Linux: `linux/amd64`, `linux/arm64`, `linux/armv7`, `linux/riscv64`

### Relay packages

- macOS: `darwin/amd64`, `darwin/arm64`
- Windows: `windows/amd64`, `windows/arm64`
- Linux: `linux/amd64`, `linux/arm64`, `linux/armv7`, `linux/riscv64`, `linux/ppc64le`, `linux/s390x`

See [`docs/BUILD_MATRIX.en.md`](docs/BUILD_MATRIX.en.md) for the full matrix.

## CI / release automation

The repository includes two GitHub Actions workflows:

- `.github/workflows/release-artifacts.yml`: builds multi-architecture Desktop CLI and relay artifacts and uploads them as Actions artifacts
- `.github/workflows/docker-image.yml`: builds and publishes the multi-architecture GHCR image

## Documentation index

- [`README.md`](README.md)
- [`docs/BUILD_MATRIX.md`](docs/BUILD_MATRIX.md)
- [`docs/BUILD_MATRIX.en.md`](docs/BUILD_MATRIX.en.md)
- [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md)
- [`docs/DEPLOYMENT.en.md`](docs/DEPLOYMENT.en.md)
- [`docs/releases/v0.1.1.zh-CN.md`](docs/releases/v0.1.1.zh-CN.md)
- [`docs/releases/v0.1.1.md`](docs/releases/v0.1.1.md)
- [`docs/releases/v0.1.0.zh-CN.md`](docs/releases/v0.1.0.zh-CN.md)
- [`docs/releases/v0.1.0.md`](docs/releases/v0.1.0.md)

## License

DeskGo is released under the MIT License, which allows free commercial use, modification, distribution, and redistribution.
See [`LICENSE`](LICENSE).
