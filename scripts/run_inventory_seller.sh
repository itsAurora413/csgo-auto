#!/bin/bash

# 库存自动出售脚本
# 用途：自动将库存中有利润的饰品上架出售

set -e

# 配置参数
YOUPIN_TOKEN="${YOUPIN_TOKEN:-}"
MIN_PROFIT_RATE=0.05           # 最小利润率 5%
PRICE_DISCOUNT=0.01            # 定价折扣 1%
DRY_RUN=false                  # 演练模式
FILTER_KEYWORDS=""             # 过滤关键词
EXCLUDE_COOLING=true           # 排除冷却期物品

# 工作目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

# 日志目录
LOG_DIR="logs"
mkdir -p "$LOG_DIR"

# 二进制文件路径
BINARY="inventory-seller"

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
        --min-profit)
            MIN_PROFIT_RATE="$2"
            shift 2
            ;;
        --discount)
            PRICE_DISCOUNT="$2"
            shift 2
            ;;
        --filter)
            FILTER_KEYWORDS="$2"
            shift 2
            ;;
        --no-exclude-cooling)
            EXCLUDE_COOLING=false
            shift
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --help)
            echo "库存自动出售脚本"
            echo ""
            echo "用法: $0 [选项]"
            echo ""
            echo "选项:"
            echo "  --token <token>              YouPin API Token"
            echo "  --min-profit <rate>          最小利润率，默认0.05"
            echo "  --discount <rate>            定价折扣，默认0.01"
            echo "  --filter <keywords>          过滤关键词（逗号分隔）"
            echo "  --no-exclude-cooling         不排除冷却期物品"
            echo "  --dry-run                    演练模式，不实际上架"
            echo "  --help                       显示帮助信息"
            echo ""
            echo "示例:"
            echo "  $0 --filter \"AK-47,M4A4\" --dry-run"
            echo "  $0 --filter \"红线\" --min-profit 0.10"
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
if [ ! -f "$BINARY" ] || [ "cmd/inventory-seller/main.go" -nt "$BINARY" ]; then
    echo "正在编译程序..."
    go build -o "$BINARY" ./cmd/inventory-seller/
    echo "编译完成"
fi

# 构建命令
CMD="./$BINARY \
    -token=\"$YOUPIN_TOKEN\" \
    -min-profit=$MIN_PROFIT_RATE \
    -discount=$PRICE_DISCOUNT"

if [ -n "$FILTER_KEYWORDS" ]; then
    CMD="$CMD -filter=\"$FILTER_KEYWORDS\""
fi

if [ "$EXCLUDE_COOLING" = false ]; then
    CMD="$CMD -exclude-cooling=false"
fi

if [ "$DRY_RUN" = true ]; then
    CMD="$CMD -dry-run"
fi

# 显示配置
echo "=========================================="
echo "库存批量出售脚本"
echo "=========================================="
echo "最小利润率:   ${MIN_PROFIT_RATE}"
echo "定价折扣:     ${PRICE_DISCOUNT}"
echo "过滤关键词:   ${FILTER_KEYWORDS:-无（处理所有物品）}"
echo "排除冷却期:   ${EXCLUDE_COOLING}"
echo "演练模式:     ${DRY_RUN}"
echo "=========================================="
echo ""

# 运行程序
echo "开始处理库存..."
eval $CMD

# 如果程序异常退出，记录日志
if [ $? -ne 0 ]; then
    echo "程序异常退出，退出码: $?"
    exit 1
fi
