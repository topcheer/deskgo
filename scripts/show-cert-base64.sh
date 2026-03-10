#!/bin/bash
# 显示证书的 Base64 编码（用于复制到 GitHub Secrets）

set -e

CERT_FILE="scripts/certs/certificate.pfx"

if [ ! -f "$CERT_FILE" ]; then
    echo "❌ 错误: 未找到证书文件"
    echo "   请先运行: ./scripts/generate-cert.sh"
    exit 1
fi

echo "========================================"
echo "证书 Base64 编码"
echo "========================================"
echo
echo "复制下面的内容到 GitHub Secrets → CERTIFICATE_PFX_BASE64"
echo
echo "----------------------------------------"
base64 -i "$CERT_FILE"
echo "----------------------------------------"
echo
echo "========================================"
echo "同时在 GitHub Secrets 中添加:"
echo "========================================"
echo
echo "Secret 名称: CERTIFICATE_PASSWORD"
echo "Secret 值: deskgo123"
echo
echo "========================================"
