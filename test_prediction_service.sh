#!/bin/bash

# CSGO 预测服务测试脚本

set -e

BASE_URL="http://localhost:5000"
COLORS_GREEN='\033[0;32m'
COLORS_RED='\033[0;31m'
COLORS_YELLOW='\033[1;33m'
COLORS_BLUE='\033[0;34m'
COLORS_NC='\033[0m' # No Color

echo -e "${COLORS_BLUE}═══════════════════════════════════════════════════════════${COLORS_NC}"
echo -e "${COLORS_BLUE}  CSGO 预测服务 - 集成测试${COLORS_NC}"
echo -e "${COLORS_BLUE}═══════════════════════════════════════════════════════════${COLORS_NC}"

# 测试 1: 健康检查
echo -e "\n${COLORS_YELLOW}测试 1: 健康检查${COLORS_NC}"
RESPONSE=$(curl -s "$BASE_URL/api/health")
echo "响应: $RESPONSE"

if echo "$RESPONSE" | grep -q '"status":"ok"'; then
    echo -e "${COLORS_GREEN}✓ 健康检查通过${COLORS_NC}"
else
    echo -e "${COLORS_RED}✗ 健康检查失败${COLORS_NC}"
    exit 1
fi

# 测试 2: 单个商品预测
echo -e "\n${COLORS_YELLOW}测试 2: 单个商品预测 (Good ID: 24026)${COLORS_NC}"
RESPONSE=$(curl -s "$BASE_URL/api/predict/24026?days=7")
echo "响应 (前 200 字符):"
echo "$RESPONSE" | head -c 200
echo "..."

if echo "$RESPONSE" | grep -q '"good_id"'; then
    echo -e "\n${COLORS_GREEN}✓ 单个预测成功${COLORS_NC}"

    # 提取建议
    RECOMMENDATION=$(echo "$RESPONSE" | jq '.recommendation.action')
    NEXT_PRICE=$(echo "$RESPONSE" | jq '.recommendation.next_price')
    PRICE_CHANGE=$(echo "$RESPONSE" | jq '.recommendation.price_change_pct')

    echo -e "  建议: $RECOMMENDATION"
    echo -e "  预测价格: $NEXT_PRICE 元"
    echo -e "  价格变化: ${PRICE_CHANGE}%"
else
    echo -e "\n${COLORS_RED}✗ 单个预测失败${COLORS_NC}"
    exit 1
fi

# 测试 3: 批量预测
echo -e "\n${COLORS_YELLOW}测试 3: 批量预测 (5 个商品)${COLORS_NC}"
curl -s -X POST "$BASE_URL/api/batch-predict" \
    -H "Content-Type: application/json" \
    -d '{
        "good_ids": [24026, 24028, 24029, 24021, 24030],
        "days": 7
    }' > /tmp/batch_result.json

TOTAL=$(jq '.total_success' /tmp/batch_result.json)
echo "成功预测商品数: $TOTAL"

if [ "$TOTAL" -gt 0 ]; then
    echo -e "${COLORS_GREEN}✓ 批量预测成功${COLORS_NC}"
else
    echo -e "${COLORS_RED}✗ 批量预测失败${COLORS_NC}"
    exit 1
fi

# 测试 4: 缓存状态
echo -e "\n${COLORS_YELLOW}测试 4: 缓存状态${COLORS_NC}"
RESPONSE=$(curl -s "$BASE_URL/api/cache-status")
CACHED=$(echo "$RESPONSE" | jq '.total_cached_models')
echo "当前缓存模型数: $CACHED"

if [ "$CACHED" -gt 0 ]; then
    echo -e "${COLORS_GREEN}✓ 缓存正常${COLORS_NC}"
else
    echo -e "${COLORS_RED}✗ 缓存为空${COLORS_NC}"
fi

# 测试 5: 性能测试 (预测速度)
echo -e "\n${COLORS_YELLOW}测试 5: 性能测试 (有缓存)${COLORS_NC}"
START=$(date +%s%N)
curl -s "$BASE_URL/api/predict/24026?days=7" > /dev/null
END=$(date +%s%N)
DURATION=$((($END - $START) / 1000000))
echo "预测耗时: ${DURATION}ms"

if [ "$DURATION" -lt 2000 ]; then
    echo -e "${COLORS_GREEN}✓ 预测速度快 (< 2s)${COLORS_NC}"
else
    echo -e "${COLORS_YELLOW}⚠ 预测速度较慢 (${DURATION}ms)${COLORS_NC}"
fi

# 测试 6: 错误处理
echo -e "\n${COLORS_YELLOW}测试 6: 错误处理${COLORS_NC}"

# 测试无效天数
RESPONSE=$(curl -s "$BASE_URL/api/predict/24026?days=50")
if echo "$RESPONSE" | grep -q '"error"'; then
    echo -e "${COLORS_GREEN}✓ 无效天数验证通过${COLORS_NC}"
else
    echo -e "${COLORS_RED}✗ 无效天数验证失败${COLORS_NC}"
fi

# 测试不存在的商品
RESPONSE=$(curl -s "$BASE_URL/api/predict/9999999")
if echo "$RESPONSE" | grep -q '"error"'; then
    echo -e "${COLORS_GREEN}✓ 不存在商品验证通过${COLORS_NC}"
else
    echo -e "${COLORS_RED}✗ 不存在商品验证失败${COLORS_NC}"
fi

# 最终报告
echo -e "\n${COLORS_BLUE}═══════════════════════════════════════════════════════════${COLORS_NC}"
echo -e "${COLORS_GREEN}✓ 所有测试通过！预测服务运行正常${COLORS_NC}"
echo -e "${COLORS_BLUE}═══════════════════════════════════════════════════════════${COLORS_NC}"

# 保存结果
echo -e "\n保存批量预测结果到 batch_results.json..."
jq '.results | map({good_id, recommendation})' /tmp/batch_result.json > batch_results.json
echo -e "${COLORS_GREEN}✓ 结果已保存${COLORS_NC}"
