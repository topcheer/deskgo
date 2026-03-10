# 在 macOS 上为 Windows Exe 签名

## 概述

本指南介绍如何在 macOS 上为 Windows 可执行文件（.exe）创建自签名证书并进行数字签名。

## 前置要求

### 安装工具

```bash
# 安装 osslsigncode（代码签名工具）
brew install osslsigncode
```

验证安装：
```bash
osslsigncode --version
```

## 使用流程

### 1️⃣ 交叉编译 Windows 版本

在 macOS 上编译 Windows 可执行文件：

```bash
cd scripts
./build-windows.sh
```

这会编译以下文件到 `bin/` 目录：
- `relay-server.exe` - Relay 中继服务器
- `deskgo-desktop-h264.exe` - 桌面捕获客户端

### 2️⃣ 生成自签名证书

首次使用需要创建证书：

```bash
./generate-cert.sh
```

**生成的文件：**
- `scripts/certs/certificate.pfx` - PKCS#12 证书（用于签名）
- `scripts/certs/DeskGo-Dev.cer` - 公钥证书（用于用户安装）
- `scripts/certs/certificate.crt` - PEM 格式证书
- `scripts/certs/private.key` - 私钥（保密！）

**证书信息：**
- 主题：CN=DeskGo Development, O=DeskGo, C=CN
- 有效期：10 年
- 密码：`deskgo123`（可在脚本中修改）

### 3️⃣ 签名可执行文件

```bash
./sign-macos.sh
```

脚本会自动：
1. 查找所有 `.exe` 和 `.dll` 文件
2. 使用 osslsigncode 进行签名
3. 添加时间戳（使用 DigiCert 公共时间戳服务器）
4. 验证签名结果

### 4️⃣ 验证签名

```bash
./verify-sign.sh
```

## 完整示例

```bash
# 1. 编译 Windows 版本
./scripts/build-windows.sh

# 2. 生成证书（首次使用）
./scripts/generate-cert.sh

# 3. 签名所有 exe 文件
./scripts/sign-macos.sh

# 4. 验证签名
./scripts/verify-sign.sh
```

## 输出文件

签名完成后：

```
bin/
├── relay-server.exe           # ✅ 已签名
└── deskgo-desktop-h264.exe    # ✅ 已签名

scripts/certs/
├── certificate.pfx            # 用于签名的证书
├── DeskGo-Dev.cer            # 分发给用户的证书
├── certificate.crt           # PEM 格式证书
└── private.key               # 私钥（保密！）
```

## 分发给用户

### 方案 1：提供证书文件

1. 将 `scripts/certs/DeskGo-Dev.cer` 随软件一起分发
2. 用户在首次运行前安装证书：

**Windows 安装证书步骤：**
```
1. 双击 DeskGo-Dev.cer
2. 点击"安装证书"
3. 选择"本地计算机"（需要管理员权限）
4. 选择"将所有证书放入下列存储"
5. 浏览到"受信任的根证书颁发机构"
6. 点击"完成"
```

### 方案 2：创建安装包

使用 **[NSIS](https://nsis.sourceforge.io/)** 或 **[Inno Setup](https://jrsoftware.org/isinfo.php)** 创建安装程序，自动：
1. 安装证书到"受信任的根证书颁发机构"
2. 复制程序文件
3. 创建桌面快捷方式

**NSIS 示例：**
```nsis
; 安装证书
ExecWait 'certutil -addstore "Root" "DeskGo-Dev.cer"'

; 复制文件
File "bin\relay-server.exe"
File "bin\deskgo-desktop-h264.exe"
```

## 命令行用法

### 手动签名单个文件

```bash
osslsigncode sign \
  -pkcs12 scripts/certs/certificate.pfx \
  -pass deskgo123 \
  -n "DeskGo" \
  -i "https://github.com/yourusername/deskgo" \
  -t http://timestamp.digicert.com \
  -h sha256 \
  -in bin/program.exe \
  -out bin/program-signed.exe
```

### 验证签名

```bash
osslsigncode verify -in bin/program.exe
```

### 查看证书信息

```bash
# 查看 pfx 文件信息
openssl pkcs12 -info -in scripts/certs/certificate.pfx

# 查看 cer 文件信息
openssl x509 -in scripts/certs/certificate.crt -text -noout
```

## 常见问题

### Q: 为什么要添加时间戳？

**A:** 时间戳确保签名在证书过期后仍然有效。没有时间戳的签名，在证书过期后会变成无效状态。

### Q: 自签名证书有什么限制？

**A:**
- ✅ 免费，无需购买
- ❌ 首次运行会显示 SmartScreen 警告（"未知发布者"）
- ❌ 用户需要手动信任你的根证书
- ❌ 无法立即建立 SmartScreen 信任声誉

### Q: 如何完全消除 SmartScreen 警告？

**A:** 需要购买 **EV 代码签名证书**（约 3,500-6,888 元/年）。EV 证书可以立即建立 SmartScreen 信任，用户不会看到警告。

### Q: 证书密码忘记了怎么办？

**A:** 重新运行 `./generate-cert.sh` 创建新证书。

### Q: osslsigncode 签名后，Windows 仍提示"未知发布者"？

**A:** 这是正常的。因为：
1. 证书不在 Windows 的受信任根证书列表中
2. 用户需要先安装你的 `DeskGo-Dev.cer` 证书
3. 或者在 SmartScreen 警告中点击"更多信息" → "仍要运行"

### Q: 可以在不同的 macOS 机器上使用同一个证书吗？

**A:** 可以。只需将 `scripts/certs/` 目录复制到其他机器即可。

## 安全注意事项

⚠️ **重要提示：**

1. **保护私钥**
   - 不要将 `private.key` 提交到 Git
   - 不要分享给他人
   - 建议添加到 `.gitignore`

2. **证书管理**
   - 定期更新证书（建议每年）
   - 记录证书密码
   - 备份证书文件

3. **Git 配置**

在项目根目录创建 `.gitignore`：
```gitignore
# 证书文件（不要提交）
scripts/certs/
*.pfx
*.key
*.p12

# 但保留 .cer 文件（可以提交）
!scripts/certs/*.cer
```

## 更新 .gitignore

```bash
cat >> .gitignore << 'EOF'

# 代码签名证书
scripts/certs/*.pfx
scripts/certs/*.key
scripts/certs/*.p12
scripts/certs/private.key
scripts/certs/certificate.pfx
EOF
```

## 进阶：购买正式证书

如果项目成熟，建议购买正式证书：

| 证书类型 | 价格范围 | 优势 | 推荐供应商 |
|---------|---------|------|-----------|
| **OV 证书** | 1,200-4,000 元/年 | 受信任，需时间建立声誉 | Certum |
| **EV 证书** | 3,500-6,888 元/年 | 立即建立 SmartScreen 信任 | DigiCert, Sectigo |

购买 EV 证书的好处：
- ✅ 无 SmartScreen 警告
- ✅ 立即建立信任
- ✅ 更好的用户体验
- ✅ 专业形象

## 脚本说明

### `build-windows.sh`
- 在 macOS 上交叉编译 Windows 版本
- 使用 `GOOS=windows GOARCH=amd64`

### `generate-cert.sh`
- 创建自签名证书
- 生成 PKCS#12 格式证书

### `sign-macos.sh`
- 批量签名所有 exe 和 dll 文件
- 自动添加时间戳
- 验证签名结果

### `verify-sign.sh`
- 验证文件签名状态
- 显示签名详情

## 相关链接

- [osslsigncode GitHub](https://github.com/mtrojnar/osslsigncode)
- [Windows 代码签名最佳实践](https://docs.microsoft.com/en-us/windows/win32/seccrypto/cryptography-tools)
- [DigiCert 代码签名文档](https://www.digicert.com/kb/code-signing/support.htm)

## 下一步

1. ✅ 编译 Windows 版本
2. ✅ 生成自签名证书
3. ✅ 签名可执行文件
4. 📤 在 Windows 上测试签名
5. 📦 准备分发（包含证书文件）

---

有问题？查看 [Windows 签名指南](./README-Windows-Signing.md) 了解更多信息。
