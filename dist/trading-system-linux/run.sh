#!/bin/bash

echo "╔════════════════════════════════════════════════════════════════╗"
echo "║          三脚本交易系统 - Linux x86_64 版本                    ║"
echo "╚════════════════════════════════════════════════════════════════╝"
echo ""

# 设置权限
chmod +x analyzer seller daemon

echo "✅ 二进制文件权限已设置"
echo ""

echo "📚 可用命令:"
echo ""
echo "1️⃣ 分析脚本 (发现机会 + 生成订单):"
echo "   ./analyzer -budget 50"
echo ""
echo "2️⃣ 出售脚本 (需要私钥):"
echo "   export YOUPIN_PRIVATE_KEY=\"\$(cat private_key.pem)\""
echo "   ./seller -target \"P250 | 污染物\" -price 23.00"
echo ""
echo "3️⃣ 后台守护进程:"
echo "   ./daemon"
echo ""
echo "后台运行 (关闭终端后继续):"
echo "   nohup ./daemon > daemon.log 2>&1 &"
echo ""
echo "查看日志:"
echo "   tail -f daemon.log"
echo ""
echo "停止进程:"
echo "   Ctrl+C 或 pkill -f 'daemon'"
echo ""
