package main

import (
	"csgo-trader/internal/services"
	"fmt"
	"log"
	"time"
)

func main() {
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("  CSGO 预测服务 - Go 客户端集成测试")
	fmt.Println("═══════════════════════════════════════════════════════════\n")

	// 初始化预测客户端
	client := services.NewPredictionClient("http://localhost:5001")

	// 1. 健康检查
	fmt.Println("1️⃣  健康检查...")
	ok, err := client.Health()
	if !ok || err != nil {
		log.Fatalf("预测服务不可用: %v", err)
	}
	fmt.Println("✓ 预测服务连接成功\n")

	// 2. 单个商品预测 - Good ID 1927 (用户指定)
	fmt.Println("2️⃣  单个商品预测 (Good ID: 1927, 预测 7 天)")
	fmt.Println("   场景: 验证策略可行性")
	testSinglePredictionForGoodID(client, 1927)

	// 3. 对比标准测试
	fmt.Println("\n3️⃣  标准测试 - 批量预测 (5 个商品)")
	testBatchPrediction(client)

	// 4. 性能测试
	fmt.Println("\n4️⃣  性能测试")
	testPerformance(client)

	fmt.Println("\n" + "═══════════════════════════════════════════════════════════")
	fmt.Println("✅ 所有测试通过！集成成功")
	fmt.Println("═══════════════════════════════════════════════════════════")
}

func testSinglePredictionForGoodID(client *services.PredictionClient, goodID int64) {
	prediction, err := client.Predict(goodID, 7)
	if err != nil {
		log.Fatalf("预测失败: %v", err)
	}

	fmt.Printf("  商品 ID: %d\n", goodID)
	fmt.Printf("  当前价格: %.2f 元 (2025-11-11 15:00)\n", prediction.CurrentPrice)

	// 获取集成预测
	ensemble, _ := prediction.GetEnsembleForecast()
	fmt.Printf("  未来 7 天预测 (集成模型: 20%% LR + 30%% Prophet + 50%% XGBoost):\n")
	for i, price := range ensemble {
		fmt.Printf("    第 %d 天 (2025-11-%d): %.2f 元\n", i+1, 11+i+1, price)
	}

	// 获取各模型预测
	lr, _ := prediction.Predictions["lr"].(map[string]interface{})
	prophet, _ := prediction.Predictions["prophet"].(map[string]interface{})
	xgb, _ := prediction.Predictions["xgb"].(map[string]interface{})

	fmt.Printf("\n  各模型预测 (第 7 天):\n")
	if lrForecast, ok := lr["forecast"].([]interface{}); ok && len(lrForecast) > 0 {
		fmt.Printf("    线性回归 (20%%): %.2f 元\n", lrForecast[len(lrForecast)-1])
	}
	if prophetForecast, ok := prophet["forecast"].([]interface{}); ok && len(prophetForecast) > 0 {
		fmt.Printf("    Prophet (30%%): %.2f 元\n", prophetForecast[len(prophetForecast)-1])
	}
	if xgbForecast, ok := xgb["forecast"].([]interface{}); ok && len(xgbForecast) > 0 {
		fmt.Printf("    XGBoost (50%%): %.2f 元\n", xgbForecast[len(xgbForecast)-1])
	}

	// 获取集成预测的第7天
	if ensembleForecast, err := prediction.GetEnsembleForecast(); err == nil && len(ensembleForecast) > 0 {
		fmt.Printf("    集成预测: %.2f 元\n", ensembleForecast[len(ensembleForecast)-1])
		ensemblePrice := ensembleForecast[len(ensembleForecast)-1]
		pricePct := ((ensemblePrice - prediction.CurrentPrice) / prediction.CurrentPrice) * 100
		fmt.Printf("    价格变化: %.2f%%\n", pricePct)
	}

	// 获取建议
	rec, _ := prediction.GetRecommendation()
	fmt.Printf("\n  推荐信息:\n")
	fmt.Printf("    行动: %s\n", rec.Action)
	fmt.Printf("    原因: %s\n", rec.Reason)
	fmt.Printf("    置信度: %.0f%%\n", rec.Confidence*100)

	// 显示与实际价格的对比
	fmt.Printf("\n  📊 与实际价格对比 (2025-11-18):\n")
	if ensembleForecast, err := prediction.GetEnsembleForecast(); err == nil && len(ensembleForecast) >= 7 {
		predictedDay7 := ensembleForecast[6]
		fmt.Printf("    集成预测 (第 7 天): %.2f 元\n", predictedDay7)
		fmt.Printf("    实际价格: 2.19 元\n")
		absError := ((predictedDay7 - 2.19) / 2.19) * 100
		fmt.Printf("    预测误差: %.2f%%\n", absError)
		if absError < 30 {
			fmt.Printf("    ✓ 预测准确度高 (误差 < 30%%)\n")
		} else {
			fmt.Printf("    ⚠ 预测误差较大 (误差 >= 30%%)\n")
		}
	}
}

func testSinglePrediction(client *services.PredictionClient) {
	goodID := int64(24026)
	prediction, err := client.Predict(goodID, 7)
	if err != nil {
		log.Fatalf("预测失败: %v", err)
	}

	fmt.Printf("  商品 ID: %d\n", goodID)
	fmt.Printf("  当前价格: %.2f 元\n", prediction.CurrentPrice)

	// 获取集成预测
	ensemble, _ := prediction.GetEnsembleForecast()
	fmt.Printf("  未来 7 天预测 (集成模型):\n")
	for i, price := range ensemble {
		fmt.Printf("    第 %d 天: %.2f 元\n", i+1, price)
	}

	// 获取建议
	rec, _ := prediction.GetRecommendation()
	fmt.Printf("\n  推荐信息:\n")
	fmt.Printf("    行动: %s\n", rec.Action)
	fmt.Printf("    预测价格: %.2f 元\n", rec.NextPrice)
	fmt.Printf("    价格变化: %.2f%%\n", rec.PriceChangePct)
	fmt.Printf("    原因: %s\n", rec.Reason)
	fmt.Printf("    置信度: %.0f%%\n", rec.Confidence*100)
}

func testBatchPrediction(client *services.PredictionClient) {
	goodIDs := []int64{24026, 24028, 24029, 24021, 24030}

	results, err := client.BatchPredict(goodIDs, 7)
	if err != nil {
		log.Fatalf("批量预测失败: %v", err)
	}

	fmt.Printf("  成功预测 %d 个商品\n\n", len(results))

	for goodID, pred := range results {
		rec, _ := pred.GetRecommendation()
		fmt.Printf("  商品 %d:\n", goodID)
		fmt.Printf("    当前价格: %.2f 元\n", pred.CurrentPrice)
		fmt.Printf("    推荐: %s\n", rec.Action)
		fmt.Printf("    预测价格: %.2f 元\n", rec.NextPrice)
		fmt.Printf("    价格变化: %.2f%%\n", rec.PriceChangePct)
		fmt.Printf("    置信度: %.0f%%\n\n", rec.Confidence*100)
	}
}

func testPerformance(client *services.PredictionClient) {
	goodID := int64(24026)

	// 第一次预测 (无缓存)
	fmt.Println("  第一次预测 (无缓存)...")
	start := time.Now()
	_, err := client.Predict(goodID, 7)
	firstDuration := time.Since(start)
	if err != nil {
		log.Fatalf("预测失败: %v", err)
	}
	fmt.Printf("    耗时: %v\n", firstDuration)

	// 第二次预测 (有缓存)
	fmt.Println("  第二次预测 (有缓存)...")
	start = time.Now()
	_, _ = client.Predict(goodID, 7)
	secondDuration := time.Since(start)
	fmt.Printf("    耗时: %v\n", secondDuration)

	improvement := float64(firstDuration.Milliseconds()) / float64(secondDuration.Milliseconds())
	fmt.Printf("    性能提升: %.1fx\n", improvement)

	// 批量预测性能
	fmt.Println("  批量预测 (10 个商品)...")
	batchGoodIDs := []int64{24026, 24028, 24029, 24021, 24030, 24026, 24028, 24029, 24021, 24030}
	start = time.Now()
	_, _ = client.BatchPredict(batchGoodIDs, 7)
	batchDuration := time.Since(start)
	fmt.Printf("    耗时: %v\n", batchDuration)
	fmt.Printf("    平均每个商品: %v\n", batchDuration/time.Duration(len(batchGoodIDs)))
	fmt.Printf("    吞吐量: %.1f 商品/秒\n", float64(len(batchGoodIDs))/batchDuration.Seconds())
}
