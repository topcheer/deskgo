#!/bin/bash
# 创建自签名代码签名证书

set -e

CERT_NAME="DeskGo-Dev-Certificate"
CERT_PASSWORD="deskgo123"  # 你可以修改这个密码
OUTPUT_DIR="scripts/certs"
VALIDITY_DAYS=3650  # 10年

echo "========================================"
echo "创建 DeskGo 自签名证书"
echo "========================================"
echo

# 创建输出目录
mkdir -p "$OUTPUT_DIR"

echo "[步骤 1] 创建私钥..."
openssl genrsa -des3 -passout pass:"$CERT_PASSWORD" -out "$OUTPUT_DIR/private.key" 2048

echo
echo "[步骤 2] 创建证书签名请求 (CSR)..."
openssl req -new -key "$OUTPUT_DIR/private.key" -passin pass:"$CERT_PASSWORD" -out "$OUTPUT_DIR/cert.csr" -subj "/C=CN/ST=Beijing/L=Beijing/O=DeskGo/OU=Development/CN=DeskGo Development"

echo
echo "[步骤 3] 创建自签名证书..."
openssl x509 -req -days $VALIDITY_DAYS -in "$OUTPUT_DIR/cert.csr" -signkey "$OUTPUT_DIR/private.key" -passin pass:"$CERT_PASSWORD" -out "$OUTPUT_DIR/certificate.crt"

echo
echo "[步骤 4] 导出为 PKCS#12 格式 (.pfx)..."
openssl pkcs12 -export -out "$OUTPUT_DIR/certificate.pfx" -inkey "$OUTPUT_DIR/private.key" -in "$OUTPUT_DIR/certificate.crt" -passin pass:"$CERT_PASSWORD" -passout pass:"$CERT_PASSWORD"

echo
echo "[步骤 5] 导出公钥证书 (.cer)..."
openssl x509 -in "$OUTPUT_DIR/certificate.crt" -outform der -out "$OUTPUT_DIR/DeskGo-Dev.cer"

echo
echo "========================================"
echo "✓ 证书创建成功！"
echo "========================================"
echo
echo "输出文件："
echo "  - $OUTPUT_DIR/certificate.pfx   (用于签名)"
echo "  - $OUTPUT_DIR/DeskGo-Dev.cer   (用于用户安装)"
echo "  - $OUTPUT_DIR/certificate.crt  (PEM 格式证书)"
echo "  - $OUTPUT_DIR/private.key      (私钥)"
echo
echo "证书信息："
echo "  - 主题: CN=DeskGo Development, O=DeskGo, C=CN"
echo "  - 有效期: $VALIDITY_DAYS 天"
echo "  - 密码: $CERT_PASSWORD"
echo
echo "⚠️  请妥善保管 $OUTPUT_DIR 目录！"
echo "   不要将 private.key 分发给他人！"
echo
echo "下一步: 运行 ./sign-macos.sh 对 Windows exe 进行签名"
echo "========================================"
