# DeskGo Windows 编译和签名指南

## 概述

本指南说明如何在 Windows 上编译 DeskGo 并使用自签名证书对可执行文件进行签名。

## 前置要求

### 必需工具

1. **Go 编译器** (1.23+)
   - 下载：https://golang.org/dl/
   - 验证：`go version`

2. **Windows SDK** (包含 SignTool.exe)
   - 下载：https://developer.microsoft.com/en-us/windows/downloads/windows-sdk/
   - 或安装 Visual Studio 2022（包含 Windows SDK）

3. **管理员权限**
   - 所有脚本都需要以管理员身份运行

## 使用步骤

### 1. 编译 Windows 版本

```batch
cd scripts
build-windows.bat
```

这将编译以下文件到 `bin/` 目录：
- `relay-server.exe` - Relay 中继服务器
- `deskgo-desktop-h264.exe` - 桌面捕获客户端（带 H.264 支持）

### 2. 创建自签名证书并签名

```batch
sign-windows.bat
```

脚本会自动：
1. 检查或创建自签名证书
2. 查找 SignTool.exe
3. 签名所有 `.exe` 和 `.dll` 文件
4. 验证签名结果

### 3. 导出证书（用于用户安装）

```batch
export-cert.bat
```

这将导出 `DeskGo-Dev.cer` 证书文件。

## 证书信息

- **证书名称**：DeskGo-Dev-Certificate
- **主题**：CN=DeskGo Development, O=DeskGo, C=CN
- **有效期**：10 年
- **存储位置**：本地计算机 → 个人

## 分发给用户

### 方案 A：提供证书文件

1. 将 `DeskGo-Dev.cer` 随软件一起分发
2. 用户首次运行前，先安装证书：
   - 双击 `DeskGo-Dev.cer`
   - 点击"安装证书"
   - 选择"本地计算机"
   - 放置到"受信任的根证书颁发机构"
3. 之后所有使用该证书签名的程序都不会显示警告

### 方案 B：创建安装程序

使用 NSIS 或 Inno Setup 创建安装程序：
1. 自动安装证书到"受信任的根证书颁发机构"
2. 安装程序文件
3. 用户只会在安装时看到一次警告

## 手动签名命令

如果你想手动签名某个文件：

```batch
REM 获取证书指纹
powershell -Command "(Get-ChildItem Cert:\LocalMachine\My | Where-Object {$_.Subject -like '*DeskGo*'}).Thumbprint"

REM 使用 SignTool 签名（替换 THUMBPRINT）
"C:\Program Files (x86)\Windows Kits\10\bin\10.0.22621.0\x64\signtool.exe" sign /f Cert:\LocalMachine\My\THUMBPRINT /fd sha256 /tr http://timestamp.digicert.com /td sha256 bin\deskgo-desktop-h264.exe
```

## 常见问题

### Q: 为什么要添加时间戳服务器？

A: 时间戳确保签名在证书过期后仍然有效。使用公共时间戳服务器（如 DigiCert）可以提供长期有效性验证。

### Q: 自签名证书有什么限制？

A:
- **首次运行警告**：SmartScreen 会显示未知发布者警告
- **需要手动信任**：用户必须安装并信任你的根证书
- **不是 EV 证书**：无法立即建立 SmartScreen 信任声誉

### Q: 如何消除 SmartScreen 警告？

A: 自签名证书无法完全消除警告。要完全消除警告，需要购买 EV 代码签名证书（约 3,500-6,888 元/年）。

### Q: SignTool.exe 在哪里？

A: 通常在以下路径之一：
- `C:\Program Files (x86)\Windows Kits\10\bin\<version>\x64\signtool.exe`
- `C:\Program Files\Windows Kits\10\bin\<version>\x64\signtool.exe`

脚本会自动查找，如果找不到，请安装 Windows SDK。

## 安全注意事项

⚠️ **重要提示**：
1. 保护好你的私钥！不要将证书导出为 PFX 格式并泄露
2. 只导出公钥（.cer 文件）用于分发
3. 定期更新证书（建议每年更新）

## 购买正式证书

如果项目成熟，建议购买正式证书：
- **OV 证书**：约 1,200-4,000 元/年
- **EV 证书**：约 3,500-6,888 元/年（推荐，立即建立 SmartScreen 信任）

推荐供应商：
- Certum（性价比高）
- DigiCert
- Sectigo
- GlobalSign

## 相关文件

- `build-windows.bat` - 编译脚本
- `sign-windows.bat` - 签名脚本
- `export-cert.bat` - 证书导出脚本

## 下一步

1. 在 Windows 上运行 `build-windows.bat`
2. 运行 `sign-windows.bat` 进行签名
3. 运行 `export-cert.bat` 导出证书
4. 测试签名是否生效：
   ```batch
   "C:\Program Files (x86)\Windows Kits\10\bin\10.0.22621.0\x64\signtool.exe" verify /pa bin\deskgo-desktop-h264.exe
   ```
