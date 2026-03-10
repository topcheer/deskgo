#!/bin/bash
# 验证 Windows exe 的签名

set -e

BIN_DIR="bin"

echo "========================================"
echo "验证 Windows Exe 签名"
echo "========================================"
echo

# 检查 osslsigncode
if ! command -v osslsigncode &> /dev/null; then
    echo "❌ 错误: 未找到 osslsigncode"
    echo "   请运行: brew install osslsigncode"
    exit 1
fi

# 查找 exe 文件
EXE_FILES=$(find "$BIN_DIR" -type f -name "*.exe" 2>/dev/null || true)

if [ -z "$EXE_FILES" ]; then
    echo "❌ 错误: bin 目录中没有找到 .exe 文件"
    exit 1
fi

echo "找到以下 exe 文件:"
echo "$EXE_FILES" | while read -r file; do
    echo "  - $file"
done
echo

echo "========================================"
echo "验证详情"
echo "========================================"
echo

echo "$EXE_FILES" | while read -r file; do
    echo "📄 文件: $(basename "$file")"
    echo "路径: $file"
    echo

    osslsigncode verify -in "$file" 2>&1 | while read -r line; do
        if [[ "$line" =~ (Signed|Current|SHA2|Attributes|Certificate|Issuer|Subject) ]]; then
            echo "  $line"
        fi
    done

    echo
    echo "----------------------------------------"
    echo
done

echo "========================================"
echo "✓ 验证完成"
echo "========================================"
