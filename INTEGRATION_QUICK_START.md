# 快速集成指南 - 在套利分析中使用预测服务

## 🚀 5 分钟快速开始

### 步骤 1: 启动 Python 预测服务

```bash
# 在后台启动服务 (推荐)
python3 prediction_service.py --port=5001 &

# 或者前台启动 (开发)
python3 prediction_service.py --port=5001
```

输出示例:
```
2025-11-18 10:30:45,123 - INFO - 启动 CSGO 预测服务...
2025-11-18 10:30:45,124 - INFO - 监听端口: 5001
 * Running on http://127.0.0.1:5001
```

### 步骤 2: 运行测试验证

```bash
go run cmd/test-prediction/main.go
```

预期输出:
```
✓ 预测服务连接成功
✅ 所有测试通过！集成成功
```

### 步骤 3: 在主程序中集成

编辑 `cmd/arbitrage-analyzer/main.go`:

```go
import (
    "csgo-trader/internal/services"
    // ... 其他导入
)

func main() {
    // ... 其他初始化代码

    // 初始化预测客户端
    predictionClient := services.NewPredictionClient("http://localhost:5001")

    // 在分析套利机会时使用
    if opportunity.ProfitRate > *minProfitRate {
        // 获取预测信息
        prediction, err := predictionClient.Predict(opportunity.GoodID, 7)
        if err == nil {
            rec, _ := prediction.GetRecommendation()

            // 根据预测调整策略
            if rec.Action == "sell" && opportunity.CurrentSellPrice > rec.NextPrice {
                // 价格可能下跌，降低风险评分
                opportunity.RiskScore += 10
            } else if rec.Action == "buy" && opportunity.CurrentBuyPrice < rec.NextPrice {
                // 价格可能上升，增加机会评分
                opportunity.Score += 15
            }

            // 保存预测信息用于后续分析
            opportunity.PredictedPrice = rec.NextPrice
            opportunity.Confidence = rec.Confidence
        }
    }
}
```

---

## 📚 API 参考

### 单个商品预测

```go
client := services.NewPredictionClient("http://localhost:5001")

// 预测 Good ID 24026 未来 7 天的价格
result, err := client.Predict(24026, 7)
if err != nil {
    log.Fatal(err)
}

// 获取当前价格
currentPrice := result.CurrentPrice

// 获取集成预测 (包含 7 个预测值)
forecast, _ := result.GetEnsembleForecast()
day7Price := forecast[6] // 第 7 天价格

// 获取推荐信息
rec, _ := result.GetRecommendation()
fmt.Printf("推荐: %s, 预测价格: %.2f, 置信度: %.0f%%\n",
    rec.Action, rec.NextPrice, rec.Confidence*100)

// 获取各模型预测
ensemble, _ := result.GetEnsembleForecast()    // 集成预测
xgboost, _ := result.GetXGBoostForecast()      // XGBoost 预测
```

### 批量预测

```go
// 批量预测 5 个商品
goodIDs := []int64{24026, 24028, 24029, 24021, 24030}
results, err := client.BatchPredict(goodIDs, 7)
if err != nil {
    log.Fatal(err)
}

// 遍历结果
for goodID, pred := range results {
    rec, _ := pred.GetRecommendation()
    fmt.Printf("Good %d: %s (价格: %.2f)\n", goodID, rec.Action, rec.NextPrice)
}
```

### 健康检查

```go
ok, err := client.Health()
if !ok {
    fmt.Println("预测服务不可用")
}
```

---

## 🎯 实际应用示例

### 场景 1: 基于预测调整止损止盈

```go
// 原始套利机会
opportunity := &Opportunity{
    GoodID: 24026,
    BuyPrice: 10.00,
    SellPrice: 10.80,
    ProfitRate: 0.08,  // 8%
}

// 获取 7 天预测
pred, _ := client.Predict(opportunity.GoodID, 7)
rec, _ := pred.GetRecommendation()

// 动态调整止盈价格
if rec.Action == "sell" {
    // 预测价格上升，可以设置更高的止盈
    opportunity.TargetSellPrice = rec.NextPrice * 0.95
} else {
    // 预测价格稳定或下跌，保守止盈
    opportunity.TargetSellPrice = opportunity.SellPrice * 1.05
}

// 动态调整止损
if rec.PriceChangePct < -10 {
    // 预测大幅下跌，提前止损
    opportunity.StopLossPrice = opportunity.BuyPrice * 0.98
}
```

### 场景 2: 为商品组合打分

```go
func scoreOpportunities(client *services.PredictionClient, opps []Opportunity) {
    for i := range opps {
        opp := &opps[i]

        // 基础分数
        score := opp.ProfitRate * 100  // 利润率作为基础分数

        // 预测加成
        pred, err := client.Predict(opp.GoodID, 7)
        if err == nil {
            rec, _ := pred.GetRecommendation()

            // 根据推荐调整分数
            if rec.Action == "buy" {
                score += 10  // 看涨，加分
            } else if rec.Action == "sell" {
                score -= 5   // 看跌，扣分
            }
            // hold 不加分也不扣分

            // 根据置信度调整
            score *= rec.Confidence  // 乘以置信度
        }

        opp.Score = score
    }

    // 按分数排序
    sort.Slice(opps, func(i, j int) bool {
        return opps[i].Score > opps[j].Score
    })
}
```

### 场景 3: 批量预测套利列表

```go
func enrichOpportunitiesWithPredictions(
    client *services.PredictionClient,
    opps []Opportunity,
) []EnrichedOpportunity {

    // 提取 Good ID 列表
    goodIDs := make([]int64, len(opps))
    oppMap := make(map[int64]*Opportunity)

    for i, opp := range opps {
        goodIDs[i] = opp.GoodID
        oppMap[opp.GoodID] = &opp
    }

    // 批量预测 (最多 100 个)
    predictions, err := client.BatchPredict(goodIDs, 7)
    if err != nil {
        log.Printf("批量预测失败: %v", err)
        return nil
    }

    // 合并结果
    var enriched []EnrichedOpportunity
    for goodID, pred := range predictions {
        opp := oppMap[goodID]
        rec, _ := pred.GetRecommendation()

        enriched = append(enriched, EnrichedOpportunity{
            Opportunity: *opp,
            PredictedPrice: rec.NextPrice,
            Recommendation: rec.Action,
            Confidence: rec.Confidence,
        })
    }

    return enriched
}
```

---

## 🔍 故障排查

### 问题 1: 连接拒绝

```
错误: dial tcp 127.0.0.1:5001: connect: connection refused
```

**解决**:
1. 确认服务已启动: `lsof -i :5001`
2. 如果未启动，运行: `python3 prediction_service.py --port=5001 &`
3. 检查日志: `tail -f /tmp/pred_service.log`

### 问题 2: 数据不足错误

```
错误: 数据不足 (< 10 条记录)
```

**解决**:
- 该商品的历史数据不足 10 条
- 系统自动需要 30 天的历史数据来训练模型
- 等待足够数据或使用其他商品测试

### 问题 3: 预测结果异常

```
症状: 返回的价格完全不合理 (过高或过低)
```

**解决**:
1. 检查数据库数据质量
2. 清空缓存强制重训: `client.ClearCache()`
3. 查看各模型的个别预测值以定位问题模型

---

## 📊 性能指标

基于 `cmd/test-prediction/main.go` 的实际测试:

| 指标 | 值 |
|------|-----|
| 单个预测延迟 | ~59 ms |
| 批量预测吞吐量 | 17.6 商品/秒 |
| 缓存命中延迟 | ~58 ms (受网络开销) |
| 最大批量大小 | 100 个商品 |

**建议**:
- 对关键商品每次预测
- 对批量分析使用批量 API
- 充分利用服务端缓存 (每小时自动清空)

---

## 🔄 工作流建议

### 每日 ETL 流程

```bash
#!/bin/bash

# 1. 启动预测服务
python3 prediction_service.py --port=5001 &
SERVICE_PID=$!

# 2. 等待服务就绪
sleep 2

# 3. 运行套利分析 (已集成预测)
go run cmd/arbitrage-analyzer/main.go

# 4. 清空缓存 (准备次日数据)
curl -X POST http://localhost:5001/api/clear-cache

# 5. 关闭服务
kill $SERVICE_PID
```

### 部署检查清单

- [ ] Python 依赖已安装: `pip install -r requirements_prediction.txt`
- [ ] 数据库连接正常: `python3 -c "import pymysql; print('OK')"`
- [ ] 预测服务可启动: `python3 prediction_service.py --port=5001 &`
- [ ] Go 程序可编译: `go build cmd/test-prediction/main.go`
- [ ] 测试通过: `go run cmd/test-prediction/main.go`

---

## 💡 最佳实践

1. **启动顺序**: 先启动 Python 服务，再启动 Go 程序
2. **错误处理**: 预测失败时，使用基础策略而不是崩溃
3. **缓存利用**: 同一商品多次查询时充分利用服务端缓存
4. **监控**: 记录预测误差，定期分析模型效果
5. **降级策略**: 预测服务不可用时，回退到线性回归

---

## 📞 支持

- 完整文档: `IMPLEMENTATION_COMPLETE.md`
- PoC 报告: `POC_REPORT.md`
- 测试结果: `TEST_PREDICTION_RESULTS.md`
- 源代码: `prediction_service.py`, `internal/services/prediction_client.go`

现在可以将预测功能集成到您的套利分析系统中了！🚀
