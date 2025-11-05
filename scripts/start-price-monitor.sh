#!/bin/bash

# 价格监控器启动脚本
# 用于后台监控和自动调整求购/出售价格

set -e

# 项目根目录
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MONITOR_BIN="${PROJECT_ROOT}/bin/price-monitor"
LOG_DIR="${PROJECT_ROOT}/logs"

# 创建日志目录
mkdir -p "$LOG_DIR"

# 配置
YOUPIN_TOKEN="${YOUPIN_TOKEN:-}"
DB_URL="${DB_URL:-}"
MONITOR_INTERVAL="${MONITOR_INTERVAL:-60}"
MIN_PROFIT="${MIN_PROFIT:-0.08}"
PRICE_STEP="${PRICE_STEP:-0.01}"
MAX_CHANGE="${MAX_CHANGE:-100}"
LOG_FILE="${LOG_DIR}/price_monitor_$(date +%Y%m%d_%H%M%S).log"

# 检查必要的配置
if [ -z "$YOUPIN_TOKEN" ]; then
    echo "❌ 错误: 未设置 YOUPIN_TOKEN 环境变量"
    echo "使用方式: YOUPIN_TOKEN=xxx ./start-price-monitor.sh"
    exit 1
fi

# 检查二进制文件
if [ ! -f "$MONITOR_BIN" ]; then
    echo "❌ 错误: 监控器二进制文件不存在: $MONITOR_BIN"
    echo "请先构建: go build -o bin/price-monitor cmd/price-monitor/main.go cmd/price-monitor/impl.go"
    exit 1
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🚀 启动价格监控器"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📊 配置信息:"
echo "  ├─ 监控间隔: ${MONITOR_INTERVAL}秒"
echo "  ├─ 最小利润率: $(echo "scale=2; ${MIN_PROFIT}*100" | bc)%"
echo "  ├─ 价格步长: ¥${PRICE_STEP}"
echo "  ├─ 最大改价: ¥${MAX_CHANGE}"
echo "  ├─ 日志文件: ${LOG_FILE}"
echo "  └─ Token: ${YOUPIN_TOKEN:0:10}***"
echo ""

# 启动监控器
echo "📌 启动命令:"
echo "$MONITOR_BIN \\"
echo "  -token \"$YOUPIN_TOKEN\" \\"
echo "  -interval $MONITOR_INTERVAL \\"
echo "  -min-profit $MIN_PROFIT \\"
echo "  -price-step $PRICE_STEP \\"
echo "  -max-change $MAX_CHANGE \\"
echo "  -log $LOG_FILE"
echo ""

# 检查是否已有进程运行
if pgrep -f "price-monitor" > /dev/null; then
    echo "⚠️  警告: 已有 price-monitor 进程运行"
    echo "是否继续启动新进程? (y/n)"
    read -r response
    if [ "$response" != "y" ]; then
        echo "❌ 取消启动"
        exit 1
    fi
fi

# 后台启动
nohup "$MONITOR_BIN" \
    -token "$YOUPIN_TOKEN" \
    -interval "$MONITOR_INTERVAL" \
    -min-profit "$MIN_PROFIT" \
    -price-step "$PRICE_STEP" \
    -max-change "$MAX_CHANGE" \
    -log "$LOG_FILE" \
    > /dev/null 2>&1 &

PID=$!
sleep 1

# 检查进程是否成功启动
if kill -0 $PID 2>/dev/null; then
    echo "✅ 监控器已启动 (PID: $PID)"
    echo "📝 日志文件: $LOG_FILE"
    echo ""
    echo "📌 查看日志: tail -f $LOG_FILE"
    echo "📌 停止监控: kill $PID"
    echo "📌 查看所有进程: pgrep -a price-monitor"
else
    echo "❌ 启动失败，请检查日志"
    exit 1
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
