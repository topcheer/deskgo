# GitHub Actions 自动签名 Windows Exe

## 概述

本指南介绍如何在 GitHub Actions CI/CD 中自动编译并签名 Windows 可执行文件。

## 架构

```
┌─────────────────┐
│  GitHub Push    │
│  Tag: v*.*.*    │
└────────┬────────┘
         │
         ▼
┌─────────────────────────────┐
│  GitHub Actions Workflow    │
├─────────────────────────────┤
│  1. Checkout code           │
│  2. Setup Go                │
│  3. Install osslsigncode    │
│  4. Build Windows exe       │
│  5. Decode certificate      │
│  6. Sign all executables    │
│  7. Verify signatures       │
│  8. Create release package  │
│  9. Upload artifacts        │
└─────────────────────────────┘
         │
         ▼
┌─────────────────┐
│  Signed Binaries│
│  + Certificate  │
│  (Artifacts)    │
└─────────────────┘
```

## 设置步骤

### 1️⃣ 生成本地证书

如果还没有证书，先在本地生成：

```bash
./scripts/generate-cert.sh
```

### 2️⃣ 获取证书 Base64 编码

```bash
./scripts/show-cert-base64.sh
```

这会输出类似这样的内容：

```
MIIGcwIBAzCCBmAGCSqGSIb3DQEHAaCCBlYEggliMIIGXjCCBf...（很长的字符串）
```

**复制这个字符串！**

### 3️⃣ 在 GitHub 上添加 Secrets

1. 打开 GitHub 仓库页面
2. 点击 **Settings** → **Secrets and variables** → **Actions**
3. 点击 **New repository secret**
4. 添加以下两个 Secrets：

#### Secret 1: 证书文件

- **Name**: `CERTIFICATE_PFX_BASE64`
- **Value**: （粘贴上一步复制的 Base64 字符串）

#### Secret 2: 证书密码

- **Name**: `CERTIFICATE_PASSWORD`
- **Value**: `deskgo123`

### 4️⃣ 创建 GitHub Actions Workflow

运行脚本自动创建 workflow 文件：

```bash
./scripts/create-workflow.sh
```

或手动创建 `.github/workflows/build-windows-release.yml`：

```yaml
name: Build and Sign Windows Release

on:
  push:
    tags:
      - 'v*.*.*'
  workflow_dispatch:

jobs:
  build-and-sign-windows:
    runs-on: windows-latest

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.23'

      - name: Install osslsigncode
        run: choco install osslsigncode -y

      - name: Build
        run: |
          go build -o bin/relay-server.exe ./cmd/relay
          go build -tags desktop -o bin/deskgo-desktop-h264.exe ./cmd/client

      - name: Decode certificate
        run: |
          $certBase64 = "${{ secrets.CERTIFICATE_PFX_BASE64 }}"
          $certBytes = [System.Convert]::FromBase64String($certBase64)
          [System.IO.File]::WriteAllBytes("certificate.pfx", $certBytes)

      - name: Sign executables
        run: |
          osslsigncode sign `
            -pkcs12 certificate.pfx `
            -pass "${{ secrets.CERTIFICATE_PASSWORD }}" `
            -n "DeskGo" `
            -t http://timestamp.digicert.com `
            -h sha256 `
            -in bin/relay-server.exe `
            -out bin/relay-server-signed.exe

          Move-Item -Force bin/relay-server-signed.exe bin/relay-server.exe

      - name: Upload artifacts
        uses: actions/upload-artifact@v4
        with:
          name: deskgo-windows-release
          path: bin/
```

### 5️⃣ 提交并推送

```bash
git add .github/workflows/
git commit -m "Add GitHub Actions workflow for Windows build"
git push
```

## 使用方法

### 方法 1: 自动触发（推送标签）

```bash
# 创建标签
git tag v0.1.5

# 推送标签到 GitHub（触发 workflow）
git push origin v0.1.5
```

### 方法 2: 手动触发

1. 打开 GitHub 仓库
2. 点击 **Actions** 标签
3. 选择 **Build and Sign Windows Release**
4. 点击 **Run workflow**
5. 输入版本号（如 `v0.1.5`）
6. 点击 **Run workflow**

### 下载构建产物

1. 等待 workflow 完成（约 2-3 分钟）
2. 点击进入完成的 workflow run
3. 滚动到底部 **Artifacts** 区域
4. 下载 `deskgo-windows-release`

## 工作流程详解

### Workflow 步骤

| 步骤 | 描述 | 时间 |
|------|------|------|
| Checkout | 检出代码 | 5s |
| Setup Go | 安装 Go 1.23 | 10s |
| Install osslsigncode | 安装签名工具 | 30s |
| Build | 编译 Windows exe | 30s |
| Decode certificate | 解码证书 | 1s |
| Sign | 签名所有 exe | 10s |
| Verify | 验证签名 | 5s |
| Package | 打包发布文件 | 5s |
| Upload | 上传 artifacts | 20s |
| **总计** | | **~2 分钟** |

### 签名过程

```powershell
# 1. 解码 Base64 证书
$certBytes = [System.Convert]::FromBase64String($certBase64)
[System.IO.File]::WriteAllBytes("certificate.pfx", $certBytes)

# 2. 使用 osslsigncode 签名
osslsigncode sign `
  -pkcs12 certificate.pfx `
  -pass ${{ secrets.CERTIFICATE_PASSWORD }} `
  -n "DeskGo" `
  -i "https://github.com/zhanju/deskgo" `
  -t http://timestamp.digicert.com `
  -h sha256 `
  -in bin/program.exe `
  -out bin/program-signed.exe

# 3. 替换原文件
Move-Item -Force bin/program-signed.exe bin/program.exe
```

## 安全最佳实践

### ✅ 应该做的

1. **使用 GitHub Secrets**
   - 证书存储在 Secrets 中，不在代码库中
   - Secrets 是加密的，不会出现在日志中

2. **最小权限原则**
   - 只给 workflow 必需的权限
   - 不要添加不必要的 Secrets

3. **定期更新证书**
   - 每年更新一次证书
   - 删除旧的 Secrets

### ❌ 不应该做的

1. **不要将证书提交到 Git**
   ```gitignore
   # 证书文件
   scripts/certs/*.pfx
   scripts/certs/*.key
   ```

2. **不要在日志中打印证书**
   - GitHub Actions 会自动屏蔽 Secrets
   - 但仍需小心，不要 `echo` 或 `Write-Output`

3. **不要分享 Base64 编码**
   - Base64 编码不是加密！
   - 任何人都可以解码

## 高级配置

### 自动创建 GitHub Release

取消注释 workflow 文件中的这部分：

```yaml
- name: Create GitHub Release
  uses: softprops/action-gh-release@v1
  with:
    files: deskgo-windows-release.zip
    draft: false
    prerelease: false
  env:
    GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### 多平台构建

可以扩展 workflow 构建多个平台：

```yaml
jobs:
  build-windows:
    runs-on: windows-latest
    # ... Windows 构建步骤

  build-linux:
    runs-on: ubuntu-latest
    # ... Linux 构建步骤

  build-macos:
    runs-on: macos-latest
    # ... macOS 构建步骤
```

### 矩阵构建

一次构建多个架构：

```yaml
strategy:
  matrix:
    goos: [windows, linux, darwin]
    goarch: [amd64, arm64]

steps:
  - name: Build
    run: |
      go build -o bin/program-${{ matrix.goos }}-${{ matrix.goarch }}
        env GOOS=${{ matrix.goos }} GOARCH=${{ matrix.goarch }}
```

## 故障排查

### 问题 1: 证书解码失败

**错误信息**: `Invalid base64 string`

**解决方案**:
- 检查 Base64 编码是否完整复制
- 确保没有多余的空格或换行
- 重新运行 `./scripts/show-cert-base64.sh`

### 问题 2: 签名失败

**错误信息**: `Failed to open certificate file`

**解决方案**:
- 确认 Secret 名称正确：`CERTIFICATE_PFX_BASE64`
- 确认密码正确：`CERTIFICATE_PASSWORD = deskgo123`

### 问题 3: Workflow 无权限

**错误信息**: `Resource not accessible by integration`

**解决方案**:
1. Settings → Actions → General
2. Workflow permissions → **Read and write permissions**
3. 保存

### 问题 4: osslsigncode 未找到

**错误信息**: `osslsigncode: command not found`

**解决方案**:
```yaml
- name: Install osslsigncode
  run: choco install osslsigncode -y
```

## 脚本说明

### `setup-github-secrets.sh`
- 显示 GitHub Secrets 配置说明
- 包含 Base64 编码的证书

### `create-workflow.sh`
- 自动创建 GitHub Actions workflow 文件
- 包含完整的构建、签名、上传流程

### `show-cert-base64.sh`
- 快速显示证书的 Base64 编码
- 方便复制到 GitHub Secrets

## 示例输出

### Workflow 运行成功

```
✓ Relay server built
✓ Desktop client built
Certificate decoded
✓ relay-server.exe signed
✓ deskgo-desktop-h264.exe signed
Verifying signatures...
-> Signed successfully
Build artifacts:
  relay-server.exe
  deskgo-desktop-h264.exe
```

### Artifacts 下载

下载并解压后：

```
deskgo-windows-release/
├── relay-server.exe              # ✅ 已签名
├── deskgo-desktop-h264.exe       # ✅ 已签名
├── web/
│   └── desktop.html
├── README.md
└── CERTIFICATE.txt               # 证书安装说明
```

## 相关文档

- [GitHub Actions 文档](https://docs.github.com/en/actions)
- [GitHub Secrets 文档](https://docs.github.com/en/actions/security-guides/encrypted-secrets)
- [osslsigncode 文档](https://github.com/mtrojnar/osslsigncode)
- [README-MacOS-Signing.md](./README-MacOS-Signing.md) - 本地签名指南
- [README-Windows-Signing.md](./README-Windows-Signing.md) - Windows 环境指南

## 下一步

1. ✅ 运行 `./scripts/show-cert-base64.sh` 获取证书编码
2. ✅ 在 GitHub 添加 Secrets
3. ✅ 运行 `./scripts/create-workflow.sh` 创建 workflow
4. ✅ 提交并推送到 GitHub
5. 🎉 创建标签触发自动构建！

---

有问题？查看 GitHub Actions 运行日志，通常能找到问题所在。
