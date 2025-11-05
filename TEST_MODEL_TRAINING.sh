#!/bin/bash

echo "╔════════════════════════════════════════════════════════════════╗"
echo "║       CSGO 饰品指数 - 模型训练模块 快速测试脚本               ║"
echo "╚════════════════════════════════════════════════════════════════╝"
echo ""

cd /Users/user/Downloads/csgoAuto

echo "🔍 检查 Python 环境..."
python3 --version

echo ""
echo "📦 检查必要的库..."
python3 << 'PYEOF'
try:
    import pandas
    print("  ✅ pandas")
except:
    print("  ❌ pandas 需要安装")

try:
    import numpy
    print("  ✅ numpy")
except:
    print("  ❌ numpy 需要安装")

try:
    import statsmodels
    print("  ✅ statsmodels")
except:
    print("  ❌ statsmodels 需要安装")

try:
    import lightgbm
    print("  ✅ lightgbm")
except:
    print("  ⚠️  lightgbm (可选)")

try:
    import arch
    print("  ✅ arch (GARCH支持)")
except:
    print("  ⚠️  arch (可选，用于GARCH建模)")
PYEOF

echo ""
echo "🚀 模式选择:"
echo "   1 - 运行完整分析（快速模式，推荐）"
echo "   2 - 运行模型训练（深度模式，耗时较长）"
echo "   3 - 退出"
echo ""
read -p "请选择 [1-3]: " choice

case $choice in
    1)
        echo ""
        echo "启动完整分析模式..."
        echo ""
        python3 << 'ANALYSISEOF'
import sys
sys.path.insert(0, '/Users/user/Downloads/csgoAuto')
from kline_analyzer import main_complete_analysis
main_complete_analysis()
ANALYSISEOF
        ;;
    2)
        echo ""
        echo "启动模型训练模式..."
        echo ""
        python3 << 'TRAININGEOF'
import sys
sys.path.insert(0, '/Users/user/Downloads/csgoAuto')
from kline_analyzer import run_quick_model_training
run_quick_model_training()
TRAININGEOF
        ;;
    *)
        echo "退出"
        exit 0
        ;;
esac

echo ""
echo "✅ 测试完成！"
echo ""
echo "📁 输出文件位置:"
echo "   /Users/user/Downloads/csgoAuto/"
echo ""
