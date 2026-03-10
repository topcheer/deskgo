# GitHub Actions 快速设置指南

## 🚀 3 步设置自动签名

### 步骤 1: 获取证书 Base64

在本地运行：

```bash
./scripts/show-cert-base64.sh
```

复制输出的 Base64 字符串（很长的那串字符）。

### 步骤 2: 添加 GitHub Secrets

1. 打开 https://github.com/zhanju/deskgo/settings/secrets/actions
2. 点击 **New repository secret**
3. 添加第一个 Secret：
   - **Name**: `CERTIFICATE_PFX_BASE64`
   - **Secret**: 粘贴步骤 1 的 Base64 字符串
4. 添加第二个 Secret：
   - **Name**: `CERTIFICATE_PASSWORD`
   - **Secret**: `deskgo123`

### 步骤 3: 触发构建

```bash
# 推送代码到 GitHub
git add .github/workflows/
git commit -m "Add GitHub Actions workflow"
git push

# 创建并推送标签（触发自动构建）
git tag v0.1.5
git push origin v0.1.5
```

## 📥 下载构建产物

1. 打开 https://github.com/zhanju/deskgo/actions
2. 点击最近的 workflow run
3. 滚动到底部 **Artifacts**
4. 下载 `deskgo-windows-release`

## 🎯 手动触发

不推送标签，也可以手动触发：

1. GitHub → Actions → Build and Sign Windows Release
2. 点击 **Run workflow**
3. 输入版本号（可选）
4. 点击 **Run workflow**

## ⚠️ 重要提示

- **证书安全**: Base64 编码的证书存储在 GitHub Secrets 中，不会暴露
- **密码**: 默认是 `deskgo123`，已在脚本中设置
- **.gitignore**: 证书文件已自动忽略，不会被提交

## 🔧 故障排查

### Workflow 失败？

检查：
1. ✅ Secrets 是否正确添加
2. ✅ Base64 编码是否完整复制
3. ✅ Workflow 权限是否开启（Settings → Actions → General）

### 签名失败？

检查日志中的错误信息，通常是：
- 证书解码失败 → 检查 Base64 编码
- 密码错误 → 确认是 `deskgo123`
- osslsigncode 未找到 → workflow 会自动安装

## 📚 完整文档

详细说明请查看：
- [scripts/README-GitHub-Actions.md](./README-GitHub-Actions.md)

---

**就这么简单！** 🎉
