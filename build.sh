#!/bin/bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT_DIR"

HOST_OS="$(go env GOOS)"

echo "🔨 开始构建 DeskGo 发布矩阵..."

mkdir -p bin downloads
rm -f bin/relay-server bin/deskgo-desktop bin/deskgo-desktop-h264
find downloads -mindepth 1 -maxdepth 1 -type f -delete

build_go_binary() {
  local output="$1"
  local pkg="$2"
  local goos="$3"
  local goarch="$4"
  local goarm="${5:-}"
  local cgo="${6:-0}"
  local tags="${7:-}"

  local -a cmd=(go build)
  if [[ -n "$tags" ]]; then
    cmd+=(-tags "$tags")
  fi
  cmd+=(-o "$output" "$pkg")

  if [[ -n "$goarm" ]]; then
    GOOS="$goos" GOARCH="$goarch" GOARM="$goarm" CGO_ENABLED="$cgo" "${cmd[@]}"
  else
    GOOS="$goos" GOARCH="$goarch" CGO_ENABLED="$cgo" "${cmd[@]}"
  fi
}

arch_suffix() {
  local goarch="$1"
  local goarm="${2:-}"

  if [[ "$goarch" == "arm" && -n "$goarm" ]]; then
    echo "armv${goarm}"
    return
  fi

  echo "$goarch"
}

binary_extension() {
  local goos="$1"
  if [[ "$goos" == "windows" ]]; then
    echo ".exe"
    return
  fi
  echo ""
}

build_relay_target() {
  local goos="$1"
  local goarch="$2"
  local goarm="${3:-}"
  local suffix
  local ext
  local output

  suffix="$(arch_suffix "$goarch" "$goarm")"
  ext="$(binary_extension "$goos")"
  output="downloads/deskgo-relay-${goos}-${suffix}${ext}"

  echo "📦 构建 Relay: ${goos}/${goarch}${goarm:+ v${goarm}}"
  build_go_binary "$output" ./cmd/relay "$goos" "$goarch" "$goarm" 0
}

build_desktop_target() {
  local goos="$1"
  local goarch="$2"
  local goarm="${3:-}"
  local cgo="${4:-0}"
  local suffix
  local ext
  local output

  suffix="$(arch_suffix "$goarch" "$goarm")"
  ext="$(binary_extension "$goos")"
  output="downloads/deskgo-desktop-${goos}-${suffix}${ext}"

  echo "📦 构建 Desktop CLI: ${goos}/${goarch}${goarm:+ v${goarm}}"
  build_go_binary "$output" ./cmd/client "$goos" "$goarch" "$goarm" "$cgo" desktop
}

copy_support_scripts() {
  echo "📄 复制自动运行安装脚本..."
  cp scripts/deskgo-autostart.sh downloads/deskgo-autostart.sh
  cp scripts/deskgo-autostart.ps1 downloads/deskgo-autostart.ps1
}

write_checksums() {
  local checksum_file="downloads/SHA256SUMS.txt"
  local -a files=()
  local path

  for path in downloads/*; do
    [[ -f "$path" ]] || continue
    case "$(basename "$path")" in
      SHA256SUMS.txt)
        continue
        ;;
    esac
    files+=("$(basename "$path")")
  done

  if [[ ${#files[@]} -eq 0 ]]; then
    return
  fi

  mapfile -t files < <(printf '%s\n' "${files[@]}" | LC_ALL=C sort)
  : > "$checksum_file"

  if command -v sha256sum >/dev/null 2>&1; then
    (
      cd downloads
      for file in "${files[@]}"; do
        sha256sum "$file"
      done
    ) > "$checksum_file"
    return
  fi

  (
    cd downloads
    for file in "${files[@]}"; do
      shasum -a 256 "$file"
    done
  ) > "$checksum_file"
}

echo "📦 构建当前平台 Relay..."
CGO_ENABLED=0 go build -o bin/relay-server ./cmd/relay

if [[ "$HOST_OS" == "darwin" ]]; then
  echo "📦 构建当前平台 Desktop CLI（macOS，H.264 优先）..."
  go build -tags desktop -o bin/deskgo-desktop-h264 ./cmd/client
else
  echo "📦 构建当前平台 Desktop CLI..."
  CGO_ENABLED=0 go build -tags desktop -o bin/deskgo-desktop ./cmd/client
fi

RELAY_TARGETS=(
  "darwin amd64"
  "darwin arm64"
  "linux amd64"
  "linux arm64"
  "linux arm 7"
  "linux riscv64"
  "linux ppc64le"
  "linux s390x"
  "windows amd64"
  "windows arm64"
)

DESKTOP_TARGETS=(
  "linux amd64"
  "linux arm64"
  "linux arm 7"
  "linux riscv64"
  "windows amd64"
  "windows arm64"
)

for target in "${RELAY_TARGETS[@]}"; do
  # shellcheck disable=SC2086
  build_relay_target $target
done

for target in "${DESKTOP_TARGETS[@]}"; do
  # shellcheck disable=SC2086
  build_desktop_target $target
done

if [[ "$HOST_OS" == "darwin" ]]; then
  build_desktop_target darwin amd64 "" 1
  build_desktop_target darwin arm64 "" 1
else
  echo "ℹ️  当前主机不是 macOS，已跳过 macOS Desktop CLI 下载包；如需生成，请在 macOS 上运行 ./build.sh。"
fi

copy_support_scripts
write_checksums

echo "✅ 构建完成！"
echo ""
echo "运行中继服务器："
echo "  ./bin/relay-server"
echo ""
if [[ "$HOST_OS" == "darwin" ]]; then
  echo "运行客户端（当前平台）："
  echo "  ./bin/deskgo-desktop-h264 -server ws://localhost:8082/api/desktop -session test123 -codec h264"
else
  echo "运行客户端（当前平台）："
  echo "  ./bin/deskgo-desktop -server ws://localhost:8082/api/desktop -session test123"
fi
echo ""
echo "下载包输出目录："
echo "  ./downloads"
echo "  ./downloads/deskgo-autostart.sh"
echo "  ./downloads/deskgo-autostart.ps1"
echo ""
echo "校验文件："
echo "  ./downloads/SHA256SUMS.txt"
