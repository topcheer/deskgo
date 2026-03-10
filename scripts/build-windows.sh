#!/bin/bash
# 在 macOS 上交叉编译 Windows 版本

set -e

BIN_DIR="bin"

echo "========================================"
echo "DeskGo Windows 版本交叉编译 (macOS)"
echo "========================================"
echo

# 检查 Go
if ! command -v go &> /dev/null; then
    echo "❌ 错误: 未找到 Go 编译器"
    exit 1
fi

echo "✓ Go 版本: $(go version)"
echo

# 创建 bin 目录
mkdir -p "$BIN_DIR"

# 编译 Relay 服务器
echo "[1/2] 编译 Relay 服务器 (Windows amd64)..."
GOOS=windows GOARCH=amd64 go build -o "$BIN_DIR/relay-server.exe" ./cmd/relay

if [ $? -eq 0 ]; then
    echo "✓ relay-server.exe 编译成功"
    ls -lh "$BIN_DIR/relay-server.exe"
else
    echo "❌ relay-server.exe 编译失败"
    exit 1
fi
echo

# 编译桌面捕获客户端
echo "[2/2] 编译桌面捕获客户端 (Windows amd64, 带桌面标签)..."
GOOS=windows GOARCH=amd64 go build -tags desktop -o "$BIN_DIR/deskgo-desktop-h264.exe" ./cmd/client

if [ $? -eq 0 ]; then
    echo "✓ deskgo-desktop-h264.exe 编译成功"
    ls -lh "$BIN_DIR/deskgo-desktop-h264.exe"
else
    echo "❌ deskgo-desktop-h264.exe 编译失败"
    exit 1
fi
echo

echo "========================================"
echo "✓ 编译完成！"
echo "========================================"
echo
echo "输出文件:"
echo "  - $BIN_DIR/relay-server.exe"
echo "  - $BIN_DIR/deskgo-desktop-h264.exe"
echo
echo "文件大小:"
du -h "$BIN_DIR"/*.exe
echo
echo "下一步: 运行 ./sign-macos.sh 进行签名"
echo "========================================"
