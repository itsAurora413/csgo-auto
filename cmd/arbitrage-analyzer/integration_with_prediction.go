package main

import (
	"csgo-trader/internal/services"
	"fmt"
	"log"
	"time"
)

/*
演示如何在套利分析中使用 Prophet + XGBoost 预测服务

这个文件展示了如何将新的预测服务集成到现有的套利分析流程中
*/

// PredictionEnrichedOpportunity 包含预测信息的套利机会
type PredictionEnrichedOpportunity struct {
	GoodID              int64
	GoodName            string
	CurrentBuyPrice     float64
	CurrentSellPrice    float64
	ProfitRate          float64
	EstimatedProfit     float64

	// 新的预测字段
	PredictedNextPrice  float64
	PredictionConfidence float64
	RecommendedAction   string // buy, sell, hold
	PredictionReason    string
}

// IntegrateWithPredictionService 演示如何集成预测服务
func IntegrateWithPredictionService() {
	// 初始化预测客户端
	predictionClient := services.NewPredictionClient("http://localhost:5001")

	// 检查服务健康
	if ok, err := predictionClient.Health(); !ok {
		log.Fatalf("预测服务不可用: %v", err)
	}
	fmt.Println("✓ 预测服务连接成功")

	// 示例 1: 单个商品预测
	fmt.Println("\n═══════════════════════════════════════════════════════════")
	fmt.Println("示例 1: 单个商品预测")
	fmt.Println("═══════════════════════════════════════════════════════════\n")

	goodID := int64(24026)
	prediction, err := predictionClient.Predict(goodID, 7)
	if err != nil {
		log.Printf("预测失败: %v", err)
		return
	}

	fmt.Printf("商品 ID: %d\n", goodID)
	fmt.Printf("当前价格: %.2f 元\n", prediction.CurrentPrice)

	// 获取集成预测
	ensembleForecast, _ := prediction.GetEnsembleForecast()
	fmt.Printf("未来 7 天预测价格:\n")
	for i, price := range ensembleForecast {
		fmt.Printf("  第 %d 天: %.2f 元\n", i+1, price)
	}

	// 获取建议
	rec, _ := prediction.GetRecommendation()
	fmt.Printf("\n📊 推荐信息:\n")
	fmt.Printf("  行动: %s\n", rec.Action)
	fmt.Printf("  预测价格: %.2f 元\n", rec.NextPrice)
	fmt.Printf("  价格变化: %.2f%%\n", rec.PriceChangePct)
	fmt.Printf("  原因: %s\n", rec.Reason)
	fmt.Printf("  置信度: %.0f%%\n", rec.Confidence*100)

	// 示例 2: 批量预测
	fmt.Println("\n═══════════════════════════════════════════════════════════")
	fmt.Println("示例 2: 批量预测 (5 个商品)")
	fmt.Println("═══════════════════════════════════════════════════════════\n")

	goodIDs := []int64{24026, 24028, 24029, 24021, 24030}
	results, err := predictionClient.BatchPredict(goodIDs, 7, "bid")
	if err != nil {
		log.Printf("批量预测失败: %v", err)
		return
	}

	fmt.Printf("成功预测 %d 个商品\n\n", len(results))

	for goodID, pred := range results {
		rec, _ := pred.GetRecommendation()
		fmt.Printf("商品 %d: %s (价格变化: %.2f%%, 置信度: %.0f%%)\n",
			goodID,
			rec.Action,
			rec.PriceChangePct,
			rec.Confidence*100)
	}

	// 示例 3: 实际应用 - 增强套利分析
	fmt.Println("\n═══════════════════════════════════════════════════════════")
	fmt.Println("示例 3: 增强的套利机会分析")
	fmt.Println("═══════════════════════════════════════════════════════════\n")

	enrichedOpps := EnhanceArbitrageOpportunitiesWithPredictions(
		predictionClient,
		goodIDs,
	)

	fmt.Printf("增强后的套利机会:\n\n")
	for _, opp := range enrichedOpps {
		fmt.Printf("商品 %d:\n", opp.GoodID)
		fmt.Printf("  当前买价: %.2f 元, 售价: %.2f 元\n", opp.CurrentBuyPrice, opp.CurrentSellPrice)
		fmt.Printf("  利润率: %.2f%%\n", opp.ProfitRate*100)
		fmt.Printf("  预测价格: %.2f 元\n", opp.PredictedNextPrice)
		fmt.Printf("  推荐: %s (%s)\n", opp.RecommendedAction, opp.PredictionReason)
		fmt.Println()
	}

	// 示例 4: 性能测试
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("示例 4: 性能测试")
	fmt.Println("═══════════════════════════════════════════════════════════\n")

	TestPredictionServicePerformance(predictionClient)
}

// EnhanceArbitrageOpportunitiesWithPredictions 使用预测增强套利分析
func EnhanceArbitrageOpportunitiesWithPredictions(
	client *services.PredictionClient,
	goodIDs []int64,
) []PredictionEnrichedOpportunity {

	results, err := client.BatchPredict(goodIDs, 7, "bid")
	if err != nil {
		log.Printf("批量预测失败: %v", err)
		return []PredictionEnrichedOpportunity{}
	}

	var opportunities []PredictionEnrichedOpportunity

	for goodID, pred := range results {
		rec, _ := pred.GetRecommendation()

		// 模拟从数据库获取的套利机会
		// 实际应该从数据库查询真实数据
		opp := PredictionEnrichedOpportunity{
			GoodID:              goodID,
			GoodName:            fmt.Sprintf("商品 %d", goodID),
			CurrentBuyPrice:     pred.CurrentPrice - 0.5,
			CurrentSellPrice:    pred.CurrentPrice,
			ProfitRate:          0.08, // 8%
			EstimatedProfit:     pred.CurrentPrice * 0.08,
			PredictedNextPrice:  rec.NextPrice,
			PredictionConfidence: rec.Confidence,
			RecommendedAction:   rec.Action,
			PredictionReason:    rec.Reason,
		}

		opportunities = append(opportunities, opp)
	}

	return opportunities
}

// TestPredictionServicePerformance 性能测试
func TestPredictionServicePerformance(client *services.PredictionClient) {
	// 测试缓存效果
	goodID := int64(24026)

	fmt.Println("第一次预测 (无缓存)...")
	start := time.Now()
	_, err := client.Predict(goodID, 7)
	firstDuration := time.Since(start)
	if err != nil {
		log.Printf("预测失败: %v", err)
		return
	}
	fmt.Printf("  耗时: %v\n", firstDuration)

	fmt.Println("\n第二次预测 (有缓存)...")
	start = time.Now()
	_, _ = client.Predict(goodID, 7)
	secondDuration := time.Since(start)
	fmt.Printf("  耗时: %v\n", secondDuration)

	improvement := float64(firstDuration.Milliseconds()) / float64(secondDuration.Milliseconds())
	fmt.Printf("\n性能提升: %.1fx\n", improvement)

	// 批量预测性能
	fmt.Println("\n批量预测 (10 个商品)...")
	goodIDs := []int64{24026, 24028, 24029, 24021, 24030, 24026, 24028, 24029, 24021, 24030}
	start = time.Now()
	_, _ = client.BatchPredict(goodIDs, 7, "bid")
	batchDuration := time.Since(start)
	fmt.Printf("  耗时: %v\n", batchDuration)
	fmt.Printf("  平均每个商品: %v\n", batchDuration/time.Duration(len(goodIDs)))
}

// ExampleIntegrationWithArbitrageAnalyzer 展示如何在主分析器中使用
func ExampleIntegrationWithArbitrageAnalyzer() {
	/*
	// 在 main.go 中集成

	predictionClient := services.NewPredictionClient("http://localhost:5001")

	// 在分析套利机会时
	if opportunity.ProfitRate > *minProfitRate {
		// 获取预测信息
		prediction, err := predictionClient.Predict(opportunity.GoodID, 7)
		if err == nil {
			rec, _ := prediction.GetRecommendation()

			// 根据预测调整推荐
			if rec.Action == "sell" && opportunity.CurrentSellPrice > rec.NextPrice {
				// 价格可能下跌，建议更谨慎
				opportunity.RiskLevel = "high"
			} else if rec.Action == "buy" && opportunity.CurrentBuyPrice < rec.NextPrice {
				// 价格可能上升，是好的买入机会
				opportunity.Score += 10
			}

			// 保存预测信息
			opportunity.RecommendedSellPrice = rec.NextPrice * 1.08 // 期望 8% 利润
		}
	}
	*/
}
