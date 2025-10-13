#!/bin/bash

# 求购监控脚本启动器
# 用途：定时检查求购订单，自动调整价格或删除不盈利的求购

set -e

# 配置参数
YOUPIN_TOKEN="${YOUPIN_TOKEN:-}"
INTERVAL=300                    # 检查间隔（秒），默认5分钟
MIN_PROFIT_RATE=0.05           # 最小利润率 5%
PRICE_DECREMENT=0.005          # 降价幅度 0.5%
MIN_RANK_GAP=0.02              # 第一名与第二名最小差距 2%
DRY_RUN=false                  # 演练模式

# 工作目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

# 日志目录
LOG_DIR="logs"
mkdir -p "$LOG_DIR"

# 二进制文件路径
BINARY="purchase-monitor"

# 检查token
if [ -z "$YOUPIN_TOKEN" ]; then
    echo "错误: 请设置 YOUPIN_TOKEN 环境变量"
    echo "使用方法: export YOUPIN_TOKEN='your_token_here'"
    exit 1
fi

# 解析命令行参数
while [[ $# -gt 0 ]]; do
    case $1 in
        --token)
            YOUPIN_TOKEN="$2"
            shift 2
            ;;
        --interval)
            INTERVAL="$2"
            shift 2
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --once)
            ONCE_FLAG="--once"
            shift
            ;;
        --help)
            echo "求购监控脚本"
            echo ""
            echo "用法: $0 [选项]"
            echo ""
            echo "选项:"
            echo "  --token <token>              YouPin API Token"
            echo "  --interval <seconds>         检查间隔（秒），默认300"
            echo "  --dry-run                    演练模式，不实际修改"
            echo "  --once                       只运行一次"
            echo "  --help                       显示帮助信息"
            echo ""
            echo "环境变量:"
            echo "  YOUPIN_TOKEN                 YouPin API Token"
            echo ""
            exit 0
            ;;
        *)
            echo "未知参数: $1"
            echo "使用 --help 查看帮助"
            exit 1
            ;;
    esac
done

# 编译程序（如果需要）
if [ ! -f "$BINARY" ] || [ "cmd/purchase-monitor/main.go" -nt "$BINARY" ]; then
    echo "正在编译程序..."
    go build -o "$BINARY" ./cmd/purchase-monitor/
    echo "编译完成"
fi

# 构建命令
CMD="./$BINARY \
    -token=\"$YOUPIN_TOKEN\" \
    -interval=$INTERVAL \
    -min-profit=$MIN_PROFIT_RATE \
    -decrement=$PRICE_DECREMENT \
    -min-rank-gap=$MIN_RANK_GAP"

if [ "$DRY_RUN" = true ]; then
    CMD="$CMD -dry-run"
fi

if [ -n "$ONCE_FLAG" ]; then
    CMD="$CMD $ONCE_FLAG"
fi

# 显示配置
echo "=========================================="
echo "求购监控脚本"
echo "=========================================="
echo "检查间隔:     ${INTERVAL}秒"
echo "最小利润率:   ${MIN_PROFIT_RATE}"
echo "降价幅度:     ${PRICE_DECREMENT}"
echo "最小排名差距: ${MIN_RANK_GAP}"
echo "演练模式:     ${DRY_RUN}"
echo "加价策略:     阶梯式（0~1元:0.01, 1~50元:0.1, 50~1000元:1, 1000元以上:10）"
echo "=========================================="
echo ""

# 运行程序
echo "启动监控..."
eval $CMD

# 如果程序异常退出，记录日志
if [ $? -ne 0 ]; then
    echo "程序异常退出，退出码: $?"
    exit 1
fi
