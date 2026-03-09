# DeskGo autostart guide

## Design approach

The DeskGo Desktop CLI must run inside a real logged-in desktop session so it can capture the screen and handle input events correctly.

Because of that, the installer scripts intentionally prefer **per-user login autostart** instead of a traditional system background service:

- **Windows**: Scheduled Task at user logon
- **macOS**: LaunchAgent at user login
- **Linux**:
  - **XDG Autostart** by default (recommended for desktop sessions)
  - optional **systemd user service** (better restart semantics, but it depends on a healthy user systemd session)

This keeps the behavior aligned with desktop platform best practices:

- normal users can usually install it without admin privileges
- the process runs in the real interactive session instead of an isolated system service context
- uninstall stays scoped to the current user profile

## Script locations

- macOS / Linux: `scripts/deskgo-autostart.sh`
- Windows: `scripts/deskgo-autostart.ps1`

The release bundle also includes:

- `deskgo-autostart.sh`
- `deskgo-autostart.ps1`

## Online install

The recommended flow is to fetch the installer directly from GitHub Raw:

### macOS / Linux

```bash
rm -f /tmp/deskgo-autostart.sh
curl -fsSL -H 'Cache-Control: no-cache' "https://raw.githubusercontent.com/topcheer/deskgo/master/scripts/deskgo-autostart.sh?ts=$(date +%s)" -o /tmp/deskgo-autostart.sh
bash /tmp/deskgo-autostart.sh install
```

### Windows

```powershell
$script = Join-Path $env:TEMP 'deskgo-autostart.ps1'
$uri = "https://raw.githubusercontent.com/topcheer/deskgo/master/scripts/deskgo-autostart.ps1?ts=$([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())"
Remove-Item $script -ErrorAction SilentlyContinue
Invoke-WebRequest $uri -UseBasicParsing -Headers @{ 'Cache-Control' = 'no-cache' } -OutFile $script
powershell -ExecutionPolicy Bypass -File $script install
```

> Note: the online command fetches the GitHub-hosted installer script itself; the installer then downloads the matching CLI binary and `SHA256SUMS.txt` from the GitHub latest release. The `?ts=` query string and `Cache-Control: no-cache` header help avoid stale caches.

## Guided install

### macOS / Linux

```bash
./scripts/deskgo-autostart.sh install
```

The script walks through:

- relay URL
- Linux autostart mode (`xdg-autostart` or `systemd-user`)
- codec
- fixed session name
- release version to install

### Windows

```powershell
powershell -ExecutionPolicy Bypass -File .\\scripts\\deskgo-autostart.ps1 install
```

The script walks through:

- relay URL
- fixed session name
- release version to install

> Note: Windows autostart currently supports `jpeg` only because the Windows H.264 autostart encoder path is not implemented yet.

## Unattended install

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

If you want restart semantics on Linux and your user systemd session is reliable:

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

## Uninstall

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

## Relay URL normalization

The scripts accept several friendly input forms and normalize them into the WebSocket URL expected by the CLI, for example:

- `https://deskgo.example.com`
- `https://deskgo.example.com/api/desktop`
- `wss://deskgo.example.com/api/desktop`
- `deskgo.example.com`

They are normalized into a value like:

- `wss://deskgo.example.com/api/desktop`

For local plaintext environments you can also use:

- `http://127.0.0.1:8082`
- `ws://127.0.0.1:8082/api/desktop`

## Session naming rules

Autostart mode requires a stable session name, so the scripts do not allow random per-run session IDs.

By default, the scripts use the machine name and sanitize it down to:

- letters
- numbers
- `.`
- `_`
- `-`

If the provided value contains spaces or other special characters, the scripts fold them into `-`.

## Compatibility with the latest release

- `--version` defaults to `latest`
- That requires the latest release to contain the matching `deskgo-desktop-<os>-<arch>` asset and `SHA256SUMS.txt`
- If the latest release does not contain the requested binary, the installer fails explicitly during download
- For repeatable fleet rollouts, pass `--version vX.Y.Z`
- The installers write configuration to `deskgo.json` in the install root, so they also work with older release CLIs that do not support `-config`

## Platform notes

### Windows

- uses a **current-user Scheduled Task** instead of a Windows Service
- desktop capture needs an interactive session; a classic Session 0 service is not a good fit
- normal users can usually install it without elevation

### macOS

- uses a LaunchAgent in `~/Library/LaunchAgents/`
- this is the standard autostart model for desktop apps in the current user session
- LaunchDaemons are intentionally not used because they are a poor fit for desktop capture

### Linux

- `xdg-autostart` is the default recommendation
  - best fit for normal graphical desktop logins
  - no root required
  - naturally inherits the desktop session environment
- `systemd-user` is the optional advanced mode
  - better restart semantics
  - best on desktops with a well-integrated user systemd session
  - may need extra environment handling on minimal or unusual desktop environments

## Dry run

Both scripts support `--dry-run` so you can inspect the planned actions first:

```bash
./scripts/deskgo-autostart.sh install --non-interactive --dry-run
```

```powershell
powershell -ExecutionPolicy Bypass -File .\\scripts\\deskgo-autostart.ps1 install -NonInteractive -DryRun
```
