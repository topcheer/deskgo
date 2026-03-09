# DeskGo cloud and production deployment guide

This guide borrows the deployment ideas from `topcheer/cligool`, but corrects them for DeskGo's actual architecture.

## Key differences from CliGool

These are the details most likely to be wrong if you copy CliGool guidance directly:

- The DeskGo repository is `https://github.com/topcheer/deskgo.git`
- DeskGo uses container port `8082`, not `8080/8081`
- DeskGo does not require PostgreSQL, Redis, or extra workers; the relay runs as a single service
- DeskGo is a desktop streaming product, not a PTY / terminal collaboration service

## Option 1: deploy the GHCR image (recommended)

### Docker Compose

The repository already includes `docker-compose.prod.yml`:

```bash
docker compose -f docker-compose.prod.yml up -d
```

Primary image:

- `ghcr.io/topcheer/deskgo:latest`

Health check:

```bash
curl http://127.0.0.1:8082/api/health
```

### Best fit

- cloud VMs / VPS
- self-hosted nodes
- teams that want to consume the image built by GitHub Actions directly

## Option 2: build from source and deploy

```bash
git clone https://github.com/topcheer/deskgo.git
cd deskgo
./build.sh
docker compose up -d --build
```

Best fit:

- you need to deploy local source changes
- you want macOS Desktop CLI artifacts bundled into the image after building them locally first

## Option 3: Cloudflare Tunnel

If you do not want to expose a public port directly, you can use Cloudflare Tunnel just like CliGool, but you must point it at DeskGo's `8082` port.

### Example config

```yaml
tunnel: <your-tunnel-id>
credentials-file: /path/to/credentials.json

ingress:
  - hostname: deskgo.example.com
    service: http://localhost:8082
  - service: http_status:404
```

Run it with:

```bash
cloudflared tunnel run
```

Benefits:

- automatic HTTPS / WSS
- no direct public port exposure
- stable browser-facing URL for viewers

## Option 4: reverse proxy (Nginx / Caddy)

The DeskGo viewer depends on WebSocket traffic, so your reverse proxy must forward the Upgrade headers correctly.

### Nginx example

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

### Caddy example

```caddy
deskgo.example.com {
    reverse_proxy 127.0.0.1:8082
}
```

## Recommended cloud platforms

### Render

- deploy the Docker image directly or use the repository Dockerfile
- set the service port to `8082`
- health check path: `/api/health`
- free instances can cold start, so Render is better for demos and light usage

### Railway

- deploy from the GitHub repo or the GHCR image
- expose port `8082`
- configure `/api/health` as the health check
- works well for fast internal deployments and team testing

### Fly.io

- set the internal port to `8082`
- keep at least one instance warm if you want to avoid long cold starts
- useful for multi-region edge deployment

## Production recommendations

### 1. Use HTTPS / WSS

Whether you deploy behind a reverse proxy or Cloudflare Tunnel, browser users should always connect over HTTPS / WSS.

### 2. Add access control

The first public release focuses on streaming and release delivery, not on a built-in authentication layer.
For production, place DeskGo behind one of the following:

- a company VPN
- Cloudflare Access
- Basic Auth / SSO on Nginx or Caddy

### 3. Keep logs and checksums

- keep the `logs/` directory for troubleshooting
- distribute `downloads/SHA256SUMS.txt` with your Desktop CLI packages

### 4. Understand current limitations

- Linux input control requires X11 / XWayland
- pure Wayland is not part of the first release scope yet
- HEVC/H.265 is still planned as a later experiment; the first release guarantees H.264 / JPEG only

## Deployment checklist

- [ ] `GET /api/health` succeeds
- [ ] the browser can load `/` and `/en`
- [ ] the browser can load `/session/<id>` and `/en/session/<id>`
- [ ] the `downloads/` directory serves the required artifacts
- [ ] your reverse proxy forwards WebSocket Upgrade headers correctly
- [ ] HTTPS/WSS or Cloudflare Tunnel is configured
