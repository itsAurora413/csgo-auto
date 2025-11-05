#!/bin/bash

# 测试悠悠有品开放平台API配置

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo ""
echo "================================================"
echo -e "${BLUE}悠悠有品开放平台API配置测试${NC}"
echo "================================================"
echo ""

# 加载.env文件
if [ -f ".env" ]; then
    echo -e "${GREEN}✓${NC} 找到.env文件"
    source .env
else
    echo -e "${RED}✗${NC} 未找到.env文件"
    exit 1
fi

# 检查YOUPIN_APP_KEY
echo ""
echo "检查 YOUPIN_APP_KEY..."
if [ -z "$YOUPIN_APP_KEY" ] || [ "$YOUPIN_APP_KEY" = "your_app_key_here" ]; then
    echo -e "${RED}✗${NC} YOUPIN_APP_KEY 未配置"
    echo "  请在.env中设置正确的AppKey"
    exit 1
else
    echo -e "${GREEN}✓${NC} YOUPIN_APP_KEY: ${YOUPIN_APP_KEY:0:8}..."
fi

# 检查YOUPIN_PRIVATE_KEY
echo ""
echo "检查 YOUPIN_PRIVATE_KEY..."
if [ -z "$YOUPIN_PRIVATE_KEY" ] || [ "$YOUPIN_PRIVATE_KEY" = "your_private_key_here" ]; then
    echo -e "${RED}✗${NC} YOUPIN_PRIVATE_KEY 未配置"
    echo "  请在.env中设置正确的私钥"
    echo "  生成方式: go run cmd/test-youpin-openapi/main.go generate-keys"
    exit 1
else
    echo -e "${GREEN}✓${NC} YOUPIN_PRIVATE_KEY: ${YOUPIN_PRIVATE_KEY:0:20}..."
fi

# 检查YOUPIN_CALLBACK_PUBLIC_KEY（可选）
echo ""
echo "检查 YOUPIN_CALLBACK_PUBLIC_KEY..."
if [ -z "$YOUPIN_CALLBACK_PUBLIC_KEY" ] || [ "$YOUPIN_CALLBACK_PUBLIC_KEY" = "your_callback_public_key_here" ]; then
    echo -e "${YELLOW}⚠${NC}  YOUPIN_CALLBACK_PUBLIC_KEY 未配置（可选）"
else
    echo -e "${GREEN}✓${NC} YOUPIN_CALLBACK_PUBLIC_KEY: ${YOUPIN_CALLBACK_PUBLIC_KEY:0:20}..."
fi

# 运行API测试
echo ""
echo "================================================"
echo "运行API连接测试..."
echo "================================================"
echo ""

export YOUPIN_APP_KEY="$YOUPIN_APP_KEY"
export YOUPIN_PRIVATE_KEY="$YOUPIN_PRIVATE_KEY"

if go run cmd/test-batch-price/main.go; then
    echo ""
    echo "================================================"
    echo -e "${GREEN}✓ 配置测试通过！${NC}"
    echo "================================================"
    echo ""
    echo "你现在可以运行 ./run.sh 启动项目"
    echo ""
else
    echo ""
    echo "================================================"
    echo -e "${RED}✗ API测试失败${NC}"
    echo "================================================"
    echo ""
    echo "可能的原因："
    echo "  1. 公钥未在悠悠有品开放平台配置"
    echo "  2. AppKey或私钥不匹配"
    echo "  3. 网络连接问题"
    echo ""
    exit 1
fi
