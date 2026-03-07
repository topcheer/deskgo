# DeskGo - 跨平台远程桌面解决方案

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Platforms](https://img.shields.io/badge/platforms-30-blue)](#-支持的平台)

基于Go和WebSocket的跨平台远程桌面解决方案，支持VNC和RDP协议。

## ✨ 核心特性

- 🌍 **跨平台支持**：30个操作系统和架构（Windows、Linux、macOS、*BSD等）
- ⚡ **低延迟**：WebSocket实时通信，毫秒级响应
- 🔒 **安全连接**：端到端加密通信，支持HTTPS/WSS
- 🖥️ **VNC支持**：完整的VNC协议支持（RFB）
- 🪟 **RDP支持**：支持Windows远程桌面协议
- 🚀 **开箱即用**：Docker一键部署，无需复杂配置
- 🎨 **现代Web界面**：基于noVNC的专业远程桌面UI
- 💓 **心跳保活**：双向心跳机制，自动重连和死连接清理
- 👥 **多会话支持**：同时管理多个远程桌面连接

## 🏗️ 系统架构

```
CLI客户端              中继服务器              Web浏览器
┌──────────────┐              ┌──────────┐              ┌─────────────┐
│ VNC/RDP      │──WebSocket──▶│ 消息转发 │◀──WebSocket──│ noVNC       │
│ 捕获模块     │              │          │              │ HTML5 Canvas│
│              │              │          │              │             │
│ 30个平台     │              │          │              │ 键盘/鼠标   │
└──────────────┘              └──────────┘              └─────────────┘
```

**核心特点**：
- ✅ **真实VNC/RDP**：CLI客户端提供完整的远程桌面连接
- ✅ **消息转发**：中继服务器负责WebSocket消息路由
- ✅ **独立界面**：Web界面基于noVNC，支持任意浏览器
- ✅ **无状态设计**：内存中维护会话，无需数据库依赖
- ✅ **开箱即用**：单容器部署，无需复杂配置

## 🌍 支持的平台

### Windows (2个)
- Windows amd64 (Intel/AMD 64位)
- Windows arm64 (Surface Pro X等ARM设备)

### Linux (13个)
- Linux amd64 (Ubuntu、Debian、CentOS等64位系统)
- Linux arm64 (树莓派4/5、ARM服务器)
- Linux 386 (32位x86系统)
- Linux arm (树莓派等32位ARM设备)
- Linux armbe (ARM64 Big-Endian)
- Linux ppc64le (PowerPC Little-Endian)
- Linux ppc64 (PowerPC Big-Endian)
- Linux riscv64 (RISC-V架构)
- Linux s390x (IBM System z大型机)
- Linux mips (MIPS 32-bit)
- Linux mips64le (MIPS 64-bit Little-Endian)
- Linux mips64 (MIPS 64-bit Big-Endian)
- Linux loong64 (LoongArch)

### *BSD系统 (12个)
- FreeBSD amd64/arm64/386/arm/riscv64
- OpenBSD amd64/arm64
- NetBSD amd64/arm64/arm/386
- DragonFlyBSD amd64

### macOS (2个)
- macOS Intel (Intel处理器)
- macOS ARM (Apple M1/M2/M3/M4)

## 🚀 快速开始

### 方法一：Docker Compose部署（推荐）

```bash
# 1. 克隆仓库
git clone https://github.com/deskgo/deskgo.git
cd deskgo

# 2. 启动服务
docker-compose up -d

# 3. 访问Web界面
open http://localhost:8082
```

### 方法二：从源码构建

```bash
# 1. 克隆仓库
git clone https://github.com/deskgo/deskgo.git
cd deskgo

# 2. 构建中继服务器
go build -o relay-server ./cmd/relay

# 3. 运行中继服务器
export RELAY_HOST=0.0.0.0
export RELAY_PORT=8080
./relay-server
```

## 📥 客户端安装

### 连接到VNC服务器

```bash
# 基本用法
deskgo -server http://localhost:8082 -host 192.168.1.100:5900

# 使用密码认证
deskgo -server http://localhost:8082 -host 192.168.1.100:5900 -password secret

# 连接到RDP服务器
deskgo -server http://localhost:8082 -protocol rdp -host 192.168.1.100:3389 -user admin -password pass
```

## 💻 使用场景

- **远程办公**：从浏览器安全访问办公电脑
- **服务器管理**：图形化管理Linux/Windows服务器
- **技术支持**：远程协助解决技术问题
- **移动办公**：在手机/平板上操作电脑
- **多平台管理**：统一管理不同操作系统的电脑

## 🔧 技术栈

- **CLI客户端**：Go 1.24 + VNC/RDP库
- **中继服务**：Go 1.24 + Gin + WebSocket
- **Web界面**：noVNC + HTML5 Canvas
- **部署**：Docker + docker-compose

## 📚 文档

- [快速开始指南](QUICKSTART.md)
- [VNC配置指南](docs/VNC_SETUP.md)
- [RDP配置指南](docs/RDP_SETUP.md)
- [部署指南](docs/DEPLOYMENT.md)
- [API文档](docs/API.md)

## 🤝 贡献指南

欢迎提交Issue和Pull Request！

## 📄 许可证

MIT License - see [LICENSE](LICENSE) file for details

## 🙏 致谢

- [noVNC](https://github.com/novnc/noVNC) - HTML5 VNC客户端
- [gorilla/websocket](https://github.com/gorilla/websocket) - WebSocket库
- [go-vnc](https://github.com/mitchellh/go-vnc) - Go VNC客户端库
- [FreeRDP](https://github.com/FreeRDP/FreeRDP) - RDP实现

---

**如果DeskGo对你有帮助，请给我们一个⭐️！**
