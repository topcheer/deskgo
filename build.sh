#!/bin/bash

# DeskGo 构建脚本

set -e

echo "🔨 开始构建 DeskGo..."

# 构建中继服务器
echo "📦 构建中继服务器..."
go build -o relay-server ./cmd/relay

# 构建客户端（当前平台）
echo "📦 构建CLI客户端..."
go build -o deskgo ./cmd/client

echo "✅ 构建完成！"
echo ""
echo "运行中继服务器："
echo "  ./relay-server"
echo ""
echo "运行客户端："
echo "  ./deskgo -server http://localhost:8082 -host 192.168.1.100:5900"
