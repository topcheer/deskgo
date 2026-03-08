#!/bin/bash

# DeskGo 构建脚本

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT_DIR"

echo "🔨 开始构建 DeskGo..."

# 创建输出目录
mkdir -p bin downloads

# 构建中继服务器
echo "📦 构建中继服务器..."
go build -o bin/relay-server ./cmd/relay

# 构建当前平台 CLI
if [ "$(go env GOOS)" = "darwin" ]; then
  echo "📦 构建当前平台 Desktop CLI（macOS，H.264 优先）..."
  go build -tags desktop -o bin/deskgo-desktop-h264 ./cmd/client
else
  echo "📦 构建当前平台 Desktop CLI..."
  go build -tags desktop -o bin/deskgo-desktop ./cmd/client
fi

# 构建用于分发的下载包
echo "📦 构建 Linux CLI 下载包..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags desktop -o downloads/deskgo-desktop-linux-amd64 ./cmd/client

echo "📦 构建 Windows CLI 下载包..."
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -tags desktop -o downloads/deskgo-desktop-windows-amd64.exe ./cmd/client

echo "✅ 构建完成！"
echo ""
echo "运行中继服务器："
echo "  ./bin/relay-server"
echo ""
if [ -f "bin/deskgo-desktop-h264" ]; then
  echo "运行客户端（当前平台）："
  echo "  ./bin/deskgo-desktop-h264 -server ws://localhost:8082/api/desktop -session test123 -codec h264"
else
  echo "运行客户端（当前平台）："
  echo "  ./bin/deskgo-desktop -server ws://localhost:8082/api/desktop -session test123"
fi
echo ""
echo "下载包输出目录："
echo "  ./downloads/deskgo-desktop-linux-amd64"
echo "  ./downloads/deskgo-desktop-windows-amd64.exe"
