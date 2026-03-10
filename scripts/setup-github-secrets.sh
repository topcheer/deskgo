#!/bin/bash
# 设置 GitHub Secrets 的自动化脚本

set -e

echo "========================================"
echo "GitHub Secrets 自动设置"
echo "========================================"
echo

# 检查 gh CLI
if ! command -v gh &> /dev/null; then
    echo "❌ 错误: 未找到 GitHub CLI (gh)"
    echo "   请安装: brew install gh"
    exit 1
fi

# 检查认证状态
echo "[检查] GitHub CLI 认证状态..."
if ! gh auth status &> /dev/null; then
    echo "❌ 错误: GitHub CLI 未认证"
    echo "   请运行: gh auth login"
    exit 1
fi

echo "✓ GitHub CLI 已认证"
echo

# 检查证书文件
if [ ! -f "scripts/certs/certificate.pfx" ]; then
    echo "❌ 错误: 未找到证书文件"
    echo "   请先运行: ./scripts/generate-cert.sh"
    exit 1
fi

echo "[步骤 1] 读取证书文件..."
CERT_BASE64=$(base64 -i scripts/certs/certificate.pfx)
echo "✓ 证书已读取"
echo

echo "[步骤 2] 设置 GitHub Secrets..."
echo

# 设置证书 Base64
echo "正在设置 CERTIFICATE_PFX_BASE64..."
echo "$CERT_BASE64" | gh secret set CERTIFICATE_PFX_BASE64
if [ $? -eq 0 ]; then
    echo "✓ CERTIFICATE_PFX_BASE64 设置成功"
else
    echo "❌ CERTIFICATE_PFX_BASE64 设置失败"
    exit 1
fi
echo

# 设置证书密码
echo "正在设置 CERTIFICATE_PASSWORD..."
echo "deskgo123" | gh secret set CERTIFICATE_PASSWORD
if [ $? -eq 0 ]; then
    echo "✓ CERTIFICATE_PASSWORD 设置成功"
else
    echo "❌ CERTIFICATE_PASSWORD 设置失败"
    exit 1
fi
echo

# 验证 Secrets
echo "[步骤 3] 验证 Secrets..."
gh secret list
echo

echo "========================================"
echo "✅ GitHub Secrets 设置完成！"
echo "========================================"
echo
echo "已设置的 Secrets:"
echo "  • CERTIFICATE_PFX_BASE64  (证书)"
echo "  • CERTIFICATE_PASSWORD    (密码)"
echo
echo "========================================"
echo "下一步操作:"
echo "========================================"
echo
echo "1. 提交 workflow 文件:"
echo "   git add .github/workflows/"
echo "   git commit -m 'Add GitHub Actions workflow'"
echo "   git push"
echo
echo "2. 触发构建（任选其一）:"
echo "   方式 1 - 推送标签:"
echo "     git tag v0.1.5 && git push origin v0.1.5"
echo
echo "   方式 2 - 手动触发:"
echo "     gh workflow run build-windows.yml"
echo
echo "   方式 3 - 网页触发:"
echo "     https://github.com/zhanju/deskgo/actions"
echo
echo "3. 查看运行状态:"
echo "   gh run list --workflow=build-windows.yml"
echo
echo "4. 下载构建产物:"
echo "   gh run download <run-id> -n deskgo-windows-release"
echo
echo "========================================"
