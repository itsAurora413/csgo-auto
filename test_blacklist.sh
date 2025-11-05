#!/bin/bash

echo "========== 黑名单功能测试脚本 =========="
echo ""

# 检查黑名单文件
echo "1. 检查黑名单文件..."
if [ -f ~/Desktop/goods_black_note.xlsx ]; then
    echo "✅ 黑名单文件存在: ~/Desktop/goods_black_note.xlsx"
    ls -lh ~/Desktop/goods_black_note.xlsx
else
    echo "❌ 黑名单文件不存在"
fi

echo ""
echo "2. 检查编译后的二进制文件..."
if [ -f ./bin/arbitrage-analyzer-blacklist ]; then
    echo "✅ 二进制文件存在"
    ls -lh ./bin/arbitrage-analyzer-blacklist
else
    echo "❌ 二进制文件不存在，执行编译..."
    go mod tidy
    go build -o bin/arbitrage-analyzer-blacklist cmd/arbitrage-analyzer/main.go
    if [ $? -eq 0 ]; then
        echo "✅ 编译成功"
    else
        echo "❌ 编译失败"
        exit 1
    fi
fi

echo ""
echo "3. 黑名单文件内容摘要..."
python3 << 'PYEOF'
import openpyxl

try:
    wb = openpyxl.load_workbook('/Users/user/Desktop/goods_black_note.xlsx')
    ws = wb.active
    print(f"Sheet 名称: {ws.title}")
    print(f"总行数: {ws.max_row} (包含表头)")
    print(f"黑名单商品数: {ws.max_row - 1}")
    print("")
    print("前5条黑名单记录:")
    for i, row in enumerate(ws.iter_rows(values_only=True), 1):
        if i <= 6:
            if i == 1:
                print(f"[表头] {row}")
            else:
                print(f"[记录{i-1}] template_id={row[1]}, 名称={row[3] if len(row) > 3 else 'N/A'}")
except Exception as e:
    print(f"❌ 错误: {e}")
PYEOF

echo ""
echo "========== 测试完成 =========="
echo ""
echo "后续使用命令："
echo "  ./bin/arbitrage-analyzer-blacklist -once -db 'your-connection-string'"
echo ""
