#!/bin/bash

# 市场分析脚本
# 用途：分析CSGO饰品市场整体走势，识别市场事件，对比历史数据

set -e

# 工作目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

# 日志目录
LOG_DIR="logs"
mkdir -p "$LOG_DIR"

# 二进制文件路径
BINARY="market-analyzer"

# 默认参数
DAYS=90
OUTPUT_FILE="market_analysis.json"
VERBOSE=false

# 解析命令行参数
while [[ $# -gt 0 ]]; do
    case $1 in
        --days)
            DAYS="$2"
            shift 2
            ;;
        --output)
            OUTPUT_FILE="$2"
            shift 2
            ;;
        --verbose|-v)
            VERBOSE=true
            shift
            ;;
        --help)
            echo "市场分析脚本"
            echo ""
            echo "用法: $0 [选项]"
            echo ""
            echo "选项:"
            echo "  --days <天数>              分析天数，默认90天"
            echo "  --output <文件>            输出文件路径，默认market_analysis.json"
            echo "  --verbose, -v              详细输出"
            echo "  --help                     显示帮助信息"
            echo ""
            echo "示例:"
            echo "  $0 --days 30               # 分析最近30天"
            echo "  $0 --days 180 --verbose    # 分析最近180天并详细输出"
            echo "  $0 --output report.json    # 指定输出文件"
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
if [ ! -f "$BINARY" ] || [ "cmd/market-analyzer/main.go" -nt "$BINARY" ]; then
    echo "正在编译程序..."
    go build -o "$BINARY" ./cmd/market-analyzer/
    echo "编译完成"
fi

# 构建命令
CMD="./$BINARY -days=$DAYS -output=\"$OUTPUT_FILE\""

if [ "$VERBOSE" = true ]; then
    CMD="$CMD -verbose"
fi

# 显示配置
echo "=========================================="
echo "市场分析脚本"
echo "=========================================="
echo "分析天数:     ${DAYS}"
echo "输出文件:     ${OUTPUT_FILE}"
echo "详细输出:     ${VERBOSE}"
echo "=========================================="
echo ""

# 运行程序
echo "开始分析市场数据..."
eval $CMD

# 如果程序异常退出，记录日志
if [ $? -ne 0 ]; then
    echo "程序异常退出，退出码: $?"
    exit 1
fi

echo ""
echo "分析完成！"

# 如果生成了JSON文件，提示用户
if [ -f "$OUTPUT_FILE" ]; then
    echo "结果已保存到: $OUTPUT_FILE"
    echo ""
    echo "查看结果:"
    echo "  cat $OUTPUT_FILE | jq '.insights'"
    echo "  cat $OUTPUT_FILE | jq '.top_gainers'"
fi
