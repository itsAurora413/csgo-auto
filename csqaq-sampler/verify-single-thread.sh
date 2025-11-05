#!/bin/bash

echo "验证单线程采样器安装..."
echo ""

# 检查文件
check_file() {
    if [ -f "$1" ]; then
        echo "✓ $1"
        return 0
    else
        echo "✗ 缺失: $1"
        return 1
    fi
}

# 检查可执行文件
check_executable() {
    if [ -x "$1" ]; then
        echo "✓ $1 (可执行)"
        return 0
    else
        echo "✗ $1 (不可执行或不存在)"
        return 1
    fi
}

echo "📋 检查文件..."
check_file "cmd/single-thread-sampler/main.go"
check_file "internal/services/single_thread_sampler.go"
check_file "SINGLE_THREAD_SAMPLER.md"
check_file "QUICK_START_SINGLE_THREAD.md"

echo ""
echo "🔧 检查脚本和二进制..."
check_executable "run-single-thread-sampler.sh"
check_executable "bin/single-thread-sampler"

echo ""
echo "✅ 验证完成！"
echo ""
echo "🚀 快速启动:"
echo "   ./run-single-thread-sampler.sh"
