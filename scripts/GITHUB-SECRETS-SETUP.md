# ✅ GitHub Secrets 已自动设置成功！

## 已设置的 Secrets

| Secret 名称 | 创建时间 | 说明 |
|-------------|---------|------|
| `CERTIFICATE_PFX_BASE64` | 2026-03-10 00:57:18 | Base64 编码的证书 |
| `CERTIFICATE_PASSWORD` | 2026-03-10 00:57:20 | 证书密码 (deskgo123) |

## 🔐 安全信息

- ✅ 证书已加密存储在 GitHub Secrets
- ✅ 不会出现在代码或日志中
- ✅ 只有仓库管理员可以查看和修改
- ✅ GitHub Actions 运行时自动解密使用

## 📋 下一步操作

### 选项 1: 自动触发（推送标签）

```bash
# 提交 workflow 文件
git add .github/workflows/
git commit -m "Add GitHub Actions workflow for Windows signing"
git push

# 创建并推送标签（触发自动构建）
git tag v0.1.5
git push origin v0.1.5
```

### 选项 2: 手动触发

```bash
# 先提交 workflow 文件
git add .github/workflows/
git commit -m "Add GitHub Actions workflow"
git push

# 然后在 GitHub 网页上手动触发：
# 1. 打开 https://github.com/zhanju/deskgo/actions
# 2. 点击 "Build and Sign Windows Release"
# 3. 点击 "Run workflow" → "Run workflow"
```

### 选项 3: 使用 CLI 触发

```bash
# 先提交 workflow
git add .github/workflows/
git commit -m "Add GitHub Actions workflow"
git push

# 使用 gh CLI 触发 workflow
gh workflow run build-windows.yml

# 查看运行状态
gh run list --workflow=build-windows.yml
gh run view --watch
```

## 📥 下载构建产物

Workflow 运行完成后（约 2-3 分钟）：

```bash
# 查看最近的运行
gh run list --workflow=build-windows.yml --limit 5

# 下载 Artifacts
gh run download <run-id> -n deskgo-windows-release

# 或者直接从网页下载：
# https://github.com/zhanju/deskgo/actions
```

## 🎯 工作流程说明

当你推送标签或手动触发时：

1. ✅ GitHub Actions 自动编译 Windows 版本
2. ✅ 从 Secrets 解码证书
3. ✅ 使用 osslsigncode 签名所有 exe 文件
4. ✅ 添加时间戳（DigiCert）
5. ✅ 验证签名
6. ✅ 打包并上传到 Artifacts

## 📊 文件输出

每次运行会生成：

```
deskgo-windows-release/
├── relay-server.exe              # ✅ 已签名
├── deskgo-desktop-h264.exe       # ✅ 已签名
├── web/
│   └── desktop.html
├── README.md
└── CERTIFICATE.txt               # 证书安装说明
```

## 🎉 完成！

现在你的 CI/CD 流程已经完全配置好了：

- ✅ Secrets 已设置（使用 gh CLI）
- ✅ Workflow 文件已创建
- ✅ 证书安全存储在 GitHub Secrets
- ✅ 准备好自动构建和签名

**只需要推送标签就能触发自动签名构建！**

---

**提示**: 如果需要查看 workflow 运行日志：
```bash
gh run view --log <run-id>
```
