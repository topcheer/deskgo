#!/bin/bash
# 在 macOS 上为 Windows exe 签名

set -e

CERT_DIR="scripts/certs"
CERT_FILE="$CERT_DIR/certificate.pfx"
CERT_PASSWORD="deskgo123"
BIN_DIR="bin"

# 时间戳服务器（使用免费的公共服务器）
TIMESTAMP_SERVER="http://timestamp.digicert.com"

echo "========================================"
echo "DeskGo Windows Exe 签名工具 (macOS)"
echo "========================================"
echo

# 检查 osslsigncode
if ! command -v osslsigncode &> /dev/null; then
    echo "❌ 错误: 未找到 osslsigncode"
    echo "   请运行: brew install osslsigncode"
    exit 1
fi

echo "✓ 找到 osslsigncode: $(which osslsigncode)"
echo

# 检查证书
if [ ! -f "$CERT_FILE" ]; then
    echo "❌ 错误: 未找到证书文件"
    echo "   证书路径: $CERT_FILE"
    echo
    echo "   请先运行: ./generate-cert.sh"
    exit 1
fi

echo "✓ 找到证书: $CERT_FILE"
echo

# 检查 bin 目录
if [ ! -d "$BIN_DIR" ]; then
    echo "❌ 错误: 未找到 bin 目录"
    exit 1
fi

# 查找需要签名的文件
echo "[检查] 查找需要签名的文件..."
FILES_TO_SIGN=()

# 查找 .exe 文件
while IFS= read -r -d '' file; do
    FILES_TO_SIGN+=("$file")
done < <(find "$BIN_DIR" -type f -name "*.exe" -print0)

# 查找 .dll 文件
while IFS= read -r -d '' file; do
    FILES_TO_SIGN+=("$file")
done < <(find "$BIN_DIR" -type f -name "*.dll" -print0)

if [ ${#FILES_TO_SIGN[@]} -eq 0 ]; then
    echo "❌ 错误: bin 目录中没有找到 .exe 或 .dll 文件"
    echo
    echo "   请先编译 Windows 版本："
    echo "   GOOS=windows GOARCH=amd64 go build -o bin/relay-server.exe ./cmd/relay"
    echo "   GOOS=windows GOARCH=amd64 go build -tags desktop -o bin/deskgo-desktop-h264.exe ./cmd/client"
    exit 1
fi

echo
echo "找到 ${#FILES_TO_SIGN[@]} 个文件需要签名:"
for file in "${FILES_TO_SIGN[@]}"; do
    echo "  - $file"
done
echo

# 签名每个文件
echo "========================================"
echo "[签名] 开始处理..."
echo "========================================"
echo

SUCCESS_COUNT=0
FAIL_COUNT=0

for file in "${FILES_TO_SIGN[@]}"; do
    echo "📝 正在签名: $(basename "$file")"

    # 使用 osslsigncode 签名（输出到临时文件）
    TEMP_FILE="$file.signed"

    if osslsigncode sign \
        -pkcs12 "$CERT_FILE" \
        -pass "$CERT_PASSWORD" \
        -n "DeskGo" \
        -i "https://github.com/yourusername/deskgo" \
        -t "$TIMESTAMP_SERVER" \
        -h sha256 \
        -in "$file" \
        -out "$TEMP_FILE"; then

        # 替换原文件
        mv "$TEMP_FILE" "$file"
        echo "✓ 签名成功: $(basename "$file")"
        ((SUCCESS_COUNT++))
    else
        # 删除临时文件
        rm -f "$TEMP_FILE"
        echo "❌ 签名失败: $(basename "$file")"
        ((FAIL_COUNT++))
    fi
    echo
done

echo "========================================"
echo "签名完成！"
echo "========================================"
echo
echo "成功: $SUCCESS_COUNT 个"
echo "失败: $FAIL_COUNT 个"
echo

if [ $SUCCESS_COUNT -gt 0 ]; then
    echo "✓ 已签名的文件："
    for file in "${FILES_TO_SIGN[@]}"; do
        if [ ! -f "$file.bak" ]; then
            echo "  - $file"
        fi
    done
    echo
fi

if [ $FAIL_COUNT -gt 0 ]; then
    echo "❌ 签名失败的文件："
    for file in "${FILES_TO_SIGN[@]}"; do
        if [ -f "$file.bak" ]; then
            echo "  - $file"
        fi
    done
    echo
fi

echo "========================================"
echo "验证签名"
echo "========================================"
echo

# 验证签名（仅对成功签名的文件）
for file in "${FILES_TO_SIGN[@]}"; do
    if [ ! -f "$file.bak" ]; then
        echo "验证: $(basename "$file")"
        osslsigncode verify -in "$file" | grep -E "(Signed|Current|SHA2)" || true
        echo
    fi
done

echo "========================================"
echo "🎉 全部完成！"
echo "========================================"
echo
echo "下一步："
echo "1. 将签名的 exe 文件复制到 Windows 上测试"
echo "2. 将 $CERT_DIR/DeskGo-Dev.cer 分发给用户"
echo
echo "用户安装证书说明："
echo "1. 双击 DeskGo-Dev.cer"
echo "2. 点击'安装证书'"
echo "3. 选择'本地计算机'"
echo "4. 选择'将所有证书放入下列存储'"
echo "5. 浏览到'受信任的根证书颁发机构'"
echo "6. 点击'完成'"
echo
echo "========================================"
