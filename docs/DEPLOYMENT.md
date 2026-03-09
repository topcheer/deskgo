# DeskGo 云平台与生产部署指南

本指南参考了 `topcheer/cligool` 的云部署思路，但根据 DeskGo 的实际架构做了修正。

## 与 CliGool 的关键差异

下面这些信息在复用 CliGool 思路时最容易出错：

- DeskGo 仓库地址是 `https://github.com/topcheer/deskgo.git`
- DeskGo 默认容器端口是 `8082`，不是 `8080/8081`
- DeskGo 不需要 PostgreSQL、Redis 或额外 worker，Relay 单服务即可运行
- DeskGo 面向桌面串流，不是 PTY/终端协作产品

## 方案 1：使用 GHCR 镜像部署（推荐）

### Docker Compose

仓库已经提供 `docker-compose.prod.yml`：

```bash
docker compose -f docker-compose.prod.yml up -d
```

核心镜像：

- `ghcr.io/topcheer/deskgo:latest`

启动后检查：

```bash
curl http://127.0.0.1:8082/api/health
```

### 适用场景

- 云主机 / VPS
- 自建机房节点
- 希望直接使用 GitHub Actions 构建好的镜像

## 方案 2：本地源码构建后部署

```bash
git clone https://github.com/topcheer/deskgo.git
cd deskgo
./build.sh
docker compose up -d --build
```

适用场景：

- 需要把当前源码修改一起发布
- 需要在镜像里加入本地先构建出的 macOS Desktop CLI 包

## 方案 3：Cloudflare Tunnel

如果不希望直接暴露公网端口，可以像 CliGool 一样使用 Cloudflare Tunnel，但要把端口改成 DeskGo 的 `8082`。

### 示例配置

```yaml
tunnel: <your-tunnel-id>
credentials-file: /path/to/credentials.json

ingress:
  - hostname: deskgo.example.com
    service: http://localhost:8082
  - service: http_status:404
```

运行：

```bash
cloudflared tunnel run
```

优点：

- 自动 HTTPS / WSS
- 无需直接暴露服务器端口
- 便于给浏览器查看者提供稳定访问入口

## 方案 4：反向代理（Nginx / Caddy）

DeskGo 浏览器查看器依赖 WebSocket，因此反向代理必须正确透传 Upgrade 头。

### Nginx 示例

```nginx
server {
    listen 80;
    server_name deskgo.example.com;

    location / {
        proxy_pass http://127.0.0.1:8082;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

### Caddy 示例

```caddy
deskgo.example.com {
    reverse_proxy 127.0.0.1:8082
}
```

## 常见云平台建议

### Render

- 直接部署 Docker 镜像或使用仓库 Dockerfile
- 服务端口设置为 `8082`
- 健康检查路径：`/api/health`
- 免费实例可能存在冷启动，适合演示与轻量使用

### Railway

- 从 GitHub 仓库或 GHCR 镜像创建服务
- 暴露端口 `8082`
- 设置健康检查为 `/api/health`
- 适合快速托管和团队内部测试

### Fly.io

- 推荐把内部端口配置为 `8082`
- 至少保留 1 台常驻实例，避免长时间冷启动
- 适合多地域边缘部署

## 生产建议

### 1. 使用 HTTPS / WSS

无论使用反向代理还是 Cloudflare Tunnel，都应保证浏览器侧使用 HTTPS/WSS。

### 2. 做访问控制

当前首发版本主要聚焦串流和发布能力，Relay 本身不内置强认证层。
生产环境建议：

- 放在企业 VPN 后面
- 使用 Cloudflare Access
- 或由 Nginx / Caddy 前置 Basic Auth / SSO

### 3. 保留日志与校验文件

- 保留 `logs/` 目录方便排障
- 分发 CLI 时保留 `downloads/SHA256SUMS.txt`

### 4. 理解当前限制

- Linux 输入控制需要 X11 / XWayland
- 纯 Wayland 尚未作为首发能力发布
- HEVC/H.265 仍处于后续实验规划中，首发只承诺 H.264 / JPEG

## 部署检查清单

- [ ] Relay 健康检查 `GET /api/health` 正常
- [ ] 浏览器可访问 `/` 与 `/en`
- [ ] 浏览器可访问 `/session/<id>` 与 `/en/session/<id>`
- [ ] `downloads/` 能下载所需架构产物
- [ ] 反向代理已正确透传 WebSocket Upgrade
- [ ] 已配置 HTTPS/WSS 或 Cloudflare Tunnel
