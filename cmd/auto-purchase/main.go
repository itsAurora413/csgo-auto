package main

import (
	"context"
	"csgo-trader/internal/database"
	"csgo-trader/internal/models"
	"csgo-trader/internal/services/youpin"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

var (
	dbURL         = flag.String("db", "", "数据库连接字符串")
	dryRun        = flag.Bool("dry-run", true, "模拟运行模式，不实际发起求购（默认true）")
	maxTotal      = flag.Float64("max-total", 500.0, "单次最大求购总金额（默认500元）")
	minProfitRate = flag.Float64("min-profit", 0.08, "最小利润率过滤（默认8%）")
	riskLevel     = flag.String("risk", "low", "风险等级过滤：low/medium/high（默认low）")
	topN          = flag.Int("top", 10, "取前N个推荐商品（默认10）")
	autoReceive   = flag.Bool("auto-receive", false, "是否自动收货（默认false）")
	priceIncrease = flag.String("price-increase", "auto", "价格增幅模式：auto(自动)/conservative(保守)/aggressive(激进)")
)

func main() {
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	log.Printf("[自动求购] ==================== 启动 ====================")
	log.Printf("[自动求购] 运行模式: %s", getRunModeText())
	log.Printf("[自动求购] 配置:")
	log.Printf("  - 最大总金额: ¥%.2f", *maxTotal)
	log.Printf("  - 最小利润率: %.1f%%", *minProfitRate*100)
	log.Printf("  - 风险等级: %s", *riskLevel)
	log.Printf("  - 取前N个: %d", *topN)
	log.Printf("  - 自动收货: %v", *autoReceive)
	log.Printf("  - 价格策略: %s", *priceIncrease)
	log.Printf("")

	// 初始化数据库
	db, err := database.Initialize(*dbURL)
	if err != nil {
		log.Fatalf("[自动求购] 数据库初始化失败: %v", err)
	}

	// 初始化悠悠客户端
	youpinToken := os.Getenv("YOUPIN_TOKEN")
	if youpinToken == "" {
		log.Fatalf("[自动求购] 未设置 YOUPIN_TOKEN 环境变量")
	}

	youpinClient := youpin.NewClient()
	youpinClient.SetToken(youpinToken)

	ctx := context.Background()

	// 验证token有效性
	if !youpinClient.IsTokenValid(ctx) {
		log.Fatalf("[自动求购] YOUPIN_TOKEN 无效或已过期")
	}

	// 获取账户余额
	balances, err := youpinClient.GetBalances(ctx)
	if err != nil {
		log.Fatalf("[自动求购] 获取账户余额失败: %v", err)
	}

	log.Printf("[账户信息] 钱包余额: ¥%.2f", balances.WalletBalance)
	log.Printf("[账户信息] 求购余额: ¥%.2f", balances.PurchaseBalance)
	log.Printf("")

	// 检查余额是否足够
	if balances.PurchaseBalance < *maxTotal {
		log.Printf("⚠️  警告: 求购余额(¥%.2f) 小于设定的最大金额(¥%.2f)", balances.PurchaseBalance, *maxTotal)
		if !*dryRun {
			log.Fatalf("[自动求购] 余额不足，退出")
		}
	}

	// 运行自动求购
	if err := runAutoPurchase(ctx, db, youpinClient); err != nil {
		log.Fatalf("[自动求购] 执行失败: %v", err)
	}

	log.Printf("[自动求购] ==================== 完成 ====================")
}

func getRunModeText() string {
	if *dryRun {
		return "🔍 模拟运行（不会实际发起求购）"
	}
	return "⚡ 实际运行（会实际发起求购）"
}

// calculateOptimalBuyPrice 根据当前最高求购价计算最优求购价
// 规则：0～1需为0.01的倍数；1～50需为0.1的倍数；50～1000需为1的倍数
func calculateOptimalBuyPrice(currentMaxBuyPrice float64, strategy string) float64 {
	// 基础增量
	var increment float64

	if currentMaxBuyPrice < 1.0 {
		// 0～1元：0.01的倍数
		increment = 0.01
	} else if currentMaxBuyPrice < 50.0 {
		// 1～50元：0.1的倍数
		increment = 0.1
	} else if currentMaxBuyPrice < 1000.0 {
		// 50～1000元：1的倍数
		increment = 1.0
	} else {
		// 1000元以上：10的倍数
		increment = 10.0
	}

	// 根据策略调整
	multiplier := 1.0
	switch strategy {
	case "conservative":
		multiplier = 0.5 // 保守：增加一半
	case "aggressive":
		multiplier = 2.0 // 激进：增加两倍
	default:
		multiplier = 1.0 // 自动：增加一个单位
	}

	newPrice := currentMaxBuyPrice + increment*multiplier

	// 确保价格符合规则（四舍五入到正确的倍数）
	if newPrice < 1.0 {
		newPrice = math.Round(newPrice*100) / 100 // 精确到0.01
	} else if newPrice < 50.0 {
		newPrice = math.Round(newPrice*10) / 10 // 精确到0.1
	} else if newPrice < 1000.0 {
		newPrice = math.Round(newPrice) // 精确到1
	} else {
		newPrice = math.Round(newPrice/10) * 10 // 精确到10
	}

	return newPrice
}

// runAutoPurchase 执行自动求购
func runAutoPurchase(ctx context.Context, db *gorm.DB, client *youpin.Client) error {
	log.Printf("[数据查询] 开始查询推荐的套利机会...")

	// 1. 查询最新的推荐套利机会
	var opportunities []models.ArbitrageOpportunity

	// 找到最新的分析时间
	var latestAnalysis models.ArbitrageOpportunity
	if err := db.Order("analysis_time DESC").First(&latestAnalysis).Error; err != nil {
		return fmt.Errorf("查询最新分析时间失败: %w", err)
	}

	log.Printf("[数据查询] 最新分析时间: %s", latestAnalysis.AnalysisTime.Format("2006-01-02 15:04:05"))

	// 查询该时间的推荐（有推荐数量的）
	query := db.Where("analysis_time = ? AND recommended_quantity > 0", latestAnalysis.AnalysisTime)

	// 应用过滤条件
	if *minProfitRate > 0 {
		query = query.Where("profit_rate >= ?", *minProfitRate)
	}
	if *riskLevel != "" {
		query = query.Where("risk_level = ?", *riskLevel)
	}

	// 按利润率排序，取前N个
	if err := query.Order("profit_rate DESC").
		Limit(*topN).
		Find(&opportunities).Error; err != nil {
		return fmt.Errorf("查询套利机会失败: %w", err)
	}

	if len(opportunities) == 0 {
		log.Printf("[数据查询] 未找到符合条件的推荐商品")
		return nil
	}

	log.Printf("[数据查询] 找到 %d 个推荐商品", len(opportunities))
	log.Printf("")

	// 2. 对每个商品实时查询最新价格并发起求购
	totalCost := 0.0
	successCount := 0
	failCount := 0

	log.Printf("[求购清单] ==================== 开始处理 ====================")

	for i, opp := range opportunities {
		log.Printf("\n[%d/%d] 处理商品: %s (ID: %d)", i+1, len(opportunities), opp.GoodName, opp.GoodID)
		log.Printf("  原始推荐价格: ¥%.2f × %d件 = ¥%.2f",
			opp.RecommendedBuyPrice, opp.RecommendedQuantity,
			opp.RecommendedBuyPrice*float64(opp.RecommendedQuantity))

		// 查询商品的模板信息
		good, err := getGoodInfo(db, opp.GoodID)
		if err != nil {
			log.Printf("  ❌ 查询商品信息失败: %v", err)
			failCount++
			continue
		}

		if good.TemplateId == "" {
			log.Printf("  ❌ 商品缺少 TemplateId")
			failCount++
			continue
		}

		// 实时查询最新的求购价格
		purchaseInfo, err := client.GetTemplatePurchaseInfo(ctx, good.TemplateId)
		if err != nil {
			log.Printf("  ❌ 查询实时求购信息失败: %v", err)
			failCount++
			continue
		}

		currentMaxBuyPrice := purchaseInfo.Data.MaxPurchasePrice
		currentMinSellPrice := purchaseInfo.Data.MinSellPrice
		referencePrice := purchaseInfo.Data.ReferencePrice

		log.Printf("  实时价格:")
		log.Printf("    - 当前最高求购: ¥%.2f", currentMaxBuyPrice)
		log.Printf("    - 当前最低售价: ¥%.2f", currentMinSellPrice)
		log.Printf("    - 参考价格: ¥%.2f", referencePrice)

		// 计算最优求购价
		optimalPrice := calculateOptimalBuyPrice(currentMaxBuyPrice, *priceIncrease)
		log.Printf("  计算的最优求购价: ¥%.2f", optimalPrice)

		// 计算预期利润率
		expectedProfitRate := 0.0
		if optimalPrice > 0 {
			expectedProfitRate = (currentMinSellPrice*0.99 - optimalPrice) / optimalPrice
		}
		log.Printf("  预期利润率: %.2f%%", expectedProfitRate*100)

		// 检查是否超出预算
		itemCost := optimalPrice * float64(opp.RecommendedQuantity)
		if totalCost+itemCost > *maxTotal {
			log.Printf("  ⚠️  跳过: 超出预算限制 (已用¥%.2f + ¥%.2f > ¥%.2f)",
				totalCost, itemCost, *maxTotal)
			continue
		}

		// 检查利润率是否仍然满足要求
		if expectedProfitRate < *minProfitRate {
			log.Printf("  ⚠️  跳过: 实时利润率(%.2f%%)低于设定值(%.2f%%)",
				expectedProfitRate*100, *minProfitRate*100)
			failCount++
			continue
		}

		// 发起求购
		if *dryRun {
			log.Printf("  🔍 [模拟] 将发起求购:")
			log.Printf("    - 商品: %s", good.Name)
			log.Printf("    - 价格: ¥%.2f", optimalPrice)
			log.Printf("    - 数量: %d件", opp.RecommendedQuantity)
			log.Printf("    - 小计: ¥%.2f", itemCost)
			successCount++
			totalCost += itemCost
		} else {
			// 实际发起求购
			log.Printf("  ⚡ 正在发起求购...")

			response, err := client.CreatePurchaseOrderComplete(
				ctx,
				good.TemplateId,
				good.CommodityHashName,
				good.Name,
				optimalPrice,
				opp.RecommendedQuantity,
				fmt.Sprintf("%.2f", referencePrice),
				fmt.Sprintf("%.2f", currentMinSellPrice),
				fmt.Sprintf("%.2f", currentMaxBuyPrice),
				*autoReceive,
			)

			if err != nil {
				log.Printf("  ❌ 求购失败: %v", err)
				failCount++
				continue
			}

			log.Printf("  ✅ 求购成功!")
			log.Printf("    - 订单号: %s", response.Data.OrderNo)
			log.Printf("    - 价格: ¥%.2f", optimalPrice)
			log.Printf("    - 数量: %d件", opp.RecommendedQuantity)
			log.Printf("    - 小计: ¥%.2f", itemCost)
			successCount++
			totalCost += itemCost

			// 延迟，避免请求过快
			time.Sleep(2 * time.Second)
		}
	}

	// 3. 输出汇总
	log.Printf("\n[求购汇总] ==================== 汇总报告 ====================")
	log.Printf("📊 总计处理: %d 个商品", len(opportunities))
	log.Printf("✅ 成功: %d 个", successCount)
	log.Printf("❌ 失败/跳过: %d 个", failCount)
	log.Printf("💰 总花费: ¥%.2f / ¥%.2f", totalCost, *maxTotal)
	log.Printf("📈 预算使用率: %.1f%%", totalCost/ *maxTotal*100)

	return nil
}

// getGoodInfo 从数据库获取商品的完整信息
func getGoodInfo(db *gorm.DB, goodID int64) (*models.CSQAQGood, error) {
	var good models.CSQAQGood
	if err := db.Where("good_id = ?", goodID).First(&good).Error; err != nil {
		return nil, err
	}
	return &good, nil
}
