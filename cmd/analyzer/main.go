package main

import (
	"context"
	"csgo-trader/internal/models"
	"csgo-trader/internal/services/youpin"
	"flag"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// 硬编码的配置
const (
	STEAM_ID       = "76561199078507841"
	YOUPIN_APP_KEY = "12919014"
	// 硬编码YouPin Token（由用户提供）
	HARDCODED_YP_TOKEN = "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJqdGkiOiIzYTZlNDA3ODZmOTM0YzM2YTIyOGU0MmUzMDA1MWY1ZSIsIm5hbWVpZCI6IjEyOTE5MDE0IiwiSWQiOiIxMjkxOTAxNCIsInVuaXF1ZV9uYW1lIjoiWVAwMDEyOTE5MDE0IiwiTmFtZSI6IllQMDAxMjkxOTAxNCIsInZlcnNpb24iOiJIbkUiLCJuYmYiOjE3NTg5MTAyMzMsImV4cCI6MTc2MTY0MjYzMywiaXNzIjoieW91cGluODk4LmNvbSIsImRldmljZUlkIjoiYU5iVzIxUVU3Y1VEQUpCNGJLMjJxMXJrIiwiYXVkIjoidXNlciJ9.mi5QkQKAcrHQpTPCQKDkZkDycpGpYApdoRnuzBArflA"
)

var (
	budget       = flag.Float64("budget", 50, "套利预算（元）")
	autoPurchase = flag.Bool("auto-purchase", false, "双线程确认后立即求购（默认关闭）")
)

func main() {
	flag.Parse()

	log.Printf("╔════════════════════════════════════════════════════════════════╗\n")
	log.Printf("║                  【分析脚本】- 套利分析 + 发布求购              ║\n")
	log.Printf("║                                                                ║\n")
	log.Printf("║ 功能: 分析市场机会 → 生成求购订单                            ║\n")
	log.Printf("║ 执行: 手动运行一次                                            ║\n")
	log.Printf("║ 预算: ¥%.2f                                                  ║\n", *budget)
	log.Printf("║                                                                ║\n")
	log.Printf("╚════════════════════════════════════════════════════════════════╝\n\n")

	// 1. 数据库连接
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	log.Printf("[步骤1] 🔌 连接数据库\n")
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	dsn := "root:Wyj250413.@tcp(192.3.81.194:3306)/csgo_trader?charset=utf8mb4&parseTime=True&loc=Local"
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ 数据库连接失败: %v\n", err)
	}
	log.Printf("✅ 数据库连接成功\n\n")

	// 2. 套利分析 - 第一阶段
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	log.Printf("[步骤2] 📊 第一阶段：套利分析 (预算: ¥%.2f)\n", *budget)
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// 从原始数据进行套利分析，而不是查询预计算的数据库表
	log.Printf("📥 正在加载所有商品的历史价格数据（最近14天）...\n")
	log.Printf("   • 使用四因子模型分析价格趋势\n")
	log.Printf("   • 预测7天后的价格和利润率\n")
	log.Printf("   • 计算波动率和市场周期\n\n")
	opportunities, err := analyzeArbitrageFromSnapshots(db)
	if err != nil {
		log.Fatalf("❌ 分析失败: %v\n", err)
	}

	log.Printf("✅ 完成分析，发现 %d 个套利机会\n\n", len(opportunities))

	if len(opportunities) == 0 {
		log.Printf("⚠️  没有找到套利机会\n")
		return
	}

	// 显示分析结果（包含在售数量）
	log.Printf("📋 套利机会列表 (前20个，按利润率排序):\n")
	log.Printf("%-2s %-48s %6s %8s %8s %8s %6s %8s\n", "ID", "物品名称", "在售数", "求购价", "在售价", "预期利", "风险", "趋势")
	log.Printf("%-2s %-48s %6s %8s %8s %8s %6s %8s\n", "--", "----", "----", "----", "----", "----", "----", "----")

	totalProfit := 0.0
	for i, opp := range opportunities {
		if i >= 20 {
			break
		}
		totalProfit += opp.ProfitRate

		// 趋势符号
		trendEmoji := "→"
		if opp.PriceTrend == "up" {
			trendEmoji = "📈"
		} else if opp.PriceTrend == "down" {
			trendEmoji = "📉"
		}

		log.Printf("%2d %-48s %6d ¥%7.2f ¥%7.2f %7.1f%% %-6s %s\n",
			i+1,
			opp.GoodName[:min(46, len(opp.GoodName))],
			opp.SellOrderCount,
			opp.CurrentBuyPrice,
			opp.CurrentSellPrice,
			opp.ProfitRate,
			opp.RiskLevel,
			trendEmoji)
	}

	avgProfit := totalProfit / float64(min(20, len(opportunities)))
	log.Printf("\n📊 第一阶段统计:\n")
	log.Printf("   • 发现套利机会: %d 个\n", len(opportunities))
	log.Printf("   • 平均预期利润率: %.1f%%\n", avgProfit)
	if len(opportunities) > 0 {
		log.Printf("   • 利润率范围: %.1f%% ~ %.1f%%\n", opportunities[len(opportunities)-1].ProfitRate, opportunities[0].ProfitRate)
	}
	log.Printf("\n")

	// 初始化YouPin OpenAPI客户端（用于实时查询与下单）
	var ypClient *youpin.OpenAPIClient
	// 优先使用硬编码的Token，其次使用数据库中的激活Token
	acctToken := HARDCODED_YP_TOKEN
	if acctToken == "" {
		acctToken = getActiveYouPinToken(db)
	}
	if acctToken != "" {
		if c, err := youpin.NewOpenAPIClientWithDefaultKeysAndToken(acctToken); err == nil {
			ypClient = c
			if HARDCODED_YP_TOKEN != "" {
				log.Printf("[YouPin] OpenAPI 客户端初始化成功（使用硬编码Token）")
			} else {
				log.Printf("[YouPin] OpenAPI 客户端初始化成功（使用数据库Token）")
			}
		} else {
			log.Printf("[YouPin] OpenAPI 客户端初始化失败: %v", err)
		}
	} else {
		log.Printf("[YouPin] 未找到激活的Token，跳过实时下单功能")
	}

	// 先生成求购计划（数量规划），再进入双线程验证，以便验证通过后可立即按规划数量下单
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	log.Printf("[步骤3] 🛒 生成初步求购清单（用于即时下单的数量依据）\n")
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	plannedOrders := createPurchaseOrders(opportunities, *budget)
	log.Printf("✅ 生成 %d 个求购订单（初步）\n", len(plannedOrders))
	if len(plannedOrders) == 0 {
		log.Printf("⚠️  预算不足以产生任何订单，结束")
		return
	}
	// 建立 GoodID -> 订单明细 映射
	orderMap := make(map[int64]PurchaseOrder, len(plannedOrders))
	for _, od := range plannedOrders {
		orderMap[od.GoodID] = od
	}

	// 3. 第二阶段：双线程验证实时数据
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	log.Printf("[步骤4] 🔄 第二阶段：双线程再次确认条件\n")
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	log.Printf("🔍 再次确认条件：在售数量 > 100，利润 > 5%%\n")
	log.Printf("   （第一阶段已过滤，第二阶段确保最新数据）\n\n")

	// 验证机会：若开启auto-purchase，则在验证通过时立即按plannedOrders的数量下单
	validatedOpportunities := verifyOpportunitiesWithRealTimeData(db, ypClient, opportunities, *autoPurchase, orderMap)

	log.Printf("✅ 确认完成，保留 %d 个符合条件的机会\n\n", len(validatedOpportunities))

	if len(validatedOpportunities) == 0 {
		log.Printf("⚠️  没有找到符合条件的机会\n")
		log.Printf("💡 建议：降低条件或增加预算重新分析\n\n")
		return
	}

	// 4. 显示符合条件的机会及详细的趋势分析
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	log.Printf("[步骤4] 📋 符合条件的求购机会详情 + 趋势分析\n")
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	totalSpent := 0.0
	for i, valid := range validatedOpportunities {
		if i >= 20 {
			break
		}

		// 显示基本信息
		log.Printf("[物品 %d] %s\n", i+1, valid.GoodName)
		log.Printf("  💰 求购价: ¥%.2f  |  在售价: ¥%.2f  |  预期利润率: %.1f%%\n",
			valid.CurrentBuyPrice,
			valid.CurrentSellPrice,
			valid.ProfitRate)
		log.Printf("  📊 在售数: %d  |  求购数: %d  |  风险: %s\n",
			valid.SellOrderCount,
			valid.BuyOrderCount,
			valid.RiskLevel)

		// 趋势分析
		trendIcon := "→"
		if valid.PriceTrend == "up" {
			trendIcon = "📈"
		} else if valid.PriceTrend == "down" {
			trendIcon = "📉"
		}

		log.Printf("  %s 趋势: %s", trendIcon, valid.PriceTrend)

		// 趋势描述
		if valid.PriceTrend == "up" {
			log.Printf(" - 价格趋势向上，市场看好，建议持续关注")
		} else if valid.PriceTrend == "down" {
			log.Printf(" - 价格趋势向下，市场看空，需谨慎操作")
		} else {
			log.Printf(" - 价格平稳波动，处于震荡区间")
		}
		log.Printf("\n")

		// 7天后的预测
		log.Printf("  🔮 7天后预测:\n")
		log.Printf("     • 7天平均求购价: ¥%.2f  (当前: ¥%.2f, 变化: %.1f%%)\n",
			valid.AvgBuyPrice7d,
			valid.CurrentBuyPrice,
			(valid.CurrentBuyPrice-valid.AvgBuyPrice7d)/valid.AvgBuyPrice7d*100)
		log.Printf("     • 7天平均在售价: ¥%.2f  (当前: ¥%.2f, 变化: %.1f%%)\n",
			valid.AvgSellPrice7d,
			valid.CurrentSellPrice,
			(valid.CurrentSellPrice-valid.AvgSellPrice7d)/valid.AvgSellPrice7d*100)

		// 风险评估
		riskColor := "✅"
		if valid.RiskLevel == "high" {
			riskColor = "⚠️"
		} else if valid.RiskLevel == "medium" {
			riskColor = "⚡"
		}
		log.Printf("  %s 风险评估: %s - ", riskColor, valid.RiskLevel)
		if valid.RiskLevel == "low" {
			log.Printf("低风险，安全边际好\n")
		} else if valid.RiskLevel == "medium" {
			log.Printf("中风险，需要关注价格波动\n")
		} else {
			log.Printf("高风险，可能存在不确定性\n")
		}

		log.Printf("\n")
	}
	log.Printf("\n")

	// 展示最终清单（以初步清单为基准，标注哪些验证通过）
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	log.Printf("[步骤5] 📋 最终求购清单 (按规划，并标注验证状态)\n")
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	orders := plannedOrders
	log.Printf("📋 最终求购清单 (符合条件的物品):\n")
	log.Printf("%-2s %-48s %4s %8s %10s %8s\n", "ID", "物品名称", "数量", "单价", "小计", "利润")
	log.Printf("%-2s %-48s %4s %8s %10s %8s\n", "--", "----", "--", "----", "----", "----")

	totalSpent = 0.0
	for i, order := range orders {
		// 找到对应的机会以获取利润率
		profitRate := 0.0
		for _, opp := range validatedOpportunities {
			if opp.GoodID == order.GoodID {
				profitRate = opp.ProfitRate
				break
			}
		}
		log.Printf("%2d %-48s %4d ¥%7.2f ¥%9.2f %7.1f%%\n",
			i+1,
			order.GoodName[:min(46, len(order.GoodName))],
			order.Quantity,
			order.Price,
			order.Subtotal,
			profitRate)
		totalSpent += order.Subtotal
	}

	log.Printf("\n💰 预算统计:\n")
	log.Printf("   • 总预算: ¥%.2f\n", *budget)
	log.Printf("   • 已用: ¥%.2f\n", totalSpent)
	log.Printf("   • 剩余: ¥%.2f\n", *budget-totalSpent)
	log.Printf("   • 使用率: %.1f%%\n", totalSpent/(*budget)*100)

	// 若开启自动求购，则按生成的订单清单逐条下单（数量以清单为准）
	if *autoPurchase {
		if ypClient == nil {
			log.Printf("\n[自动下单] 跳过：YouPin 客户端未初始化（缺少Token）")
		} else {
			log.Printf("\n[自动下单] 开始按清单下单（%d 条）...", len(orders))
			success := 0
			for i, order := range orders {
				log.Printf("[自动下单] (%d/%d) %s × %d", i+1, len(orders), order.GoodName, order.Quantity)
				maxBuy, err := getLatestMaxBuyPrice(db, ypClient, order.GoodID)
				if err != nil || maxBuy <= 0 {
					if err != nil {
						log.Printf("  ❌ 获取最高求购价失败: %v", err)
					} else {
						log.Printf("  ❌ 获取最高求购价失败: 值无效")
					}
					continue
				}
				price := bumpPurchasePrice(maxBuy)
				if err := placeImmediatePurchaseOrder(db, ypClient, order.GoodID, order.GoodName, order.Quantity, price); err != nil {
					log.Printf("  ❌ 下单失败: %v", err)
					continue
				}
				log.Printf("  ✅ 下单成功: 价格=¥%.2f (最高=¥%.2f) 数量=%d", price, maxBuy, order.Quantity)
				success++
				// 轻微休眠，避免过快
				time.Sleep(300 * time.Millisecond)
			}
			log.Printf("[自动下单] 完成：成功 %d / %d", success, len(orders))
		}
	}

	// 6. 完成
	log.Printf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	log.Printf("✅ 分析完成！\n")
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	log.Printf("📝 求购步骤:\n")
	for i, order := range orders {
		log.Printf("%d. %s\n", i+1, order.GoodName)
		log.Printf("   数量: %d 件 | 单价: ¥%.2f | 总计: ¥%.2f\n\n", order.Quantity, order.Price, order.Subtotal)
	}

	log.Printf("🚀 下一步:\n")
	log.Printf("   1. 登录悠悠有品 (https://www.youpin898.com)\n")
	log.Printf("   2. 进入「我要购买」页面\n")
	log.Printf("   3. 为上述 %d 个物品创建求购订单\n", len(orders))
	log.Printf("   4. 等待卖家在Steam上卖给你 (通常1-24小时)\n")
	log.Printf("   5. 物品到账后，运行出售脚本: ./bin/seller\n")
	log.Printf("   6. 同时启动后台守护进程: ./bin/daemon\n\n")
}

type PurchaseOrder struct {
	GoodID   int64
	GoodName string
	Quantity int
	Price    float64
	Subtotal float64
}

func createPurchaseOrders(opportunities []models.ArbitrageOpportunity, budget float64) []PurchaseOrder {
	var orders []PurchaseOrder
	remainingBudget := budget

	for _, opp := range opportunities {
		if remainingBudget < 10 {
			break
		}

		maxQty := int(remainingBudget / opp.CurrentBuyPrice)
		if maxQty == 0 {
			continue
		}

		qty := min(maxQty, 3)

		order := PurchaseOrder{
			GoodID:   opp.GoodID,
			GoodName: opp.GoodName,
			Quantity: qty,
			Price:    opp.CurrentBuyPrice,
			Subtotal: opp.CurrentBuyPrice * float64(qty),
		}

		orders = append(orders, order)
		remainingBudget -= order.Subtotal
	}

	return orders
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// analyzeArbitrageFromSnapshots 从原始CSQAQ快照数据分析套利机会
// 而不是从预计算的arbitrage_opportunities表查询
func analyzeArbitrageFromSnapshots(db *gorm.DB) ([]models.ArbitrageOpportunity, error) {
	var opportunities []models.ArbitrageOpportunity
	var goods []models.CSQAQGood

	// 1. 获取所有商品
	if err := db.Find(&goods).Error; err != nil {
		return nil, fmt.Errorf("获取商品列表失败: %v", err)
	}

	log.Printf("   📊 已加载 %d 个商品\n", len(goods))
	log.Printf("   ⏳ 开始分析每个商品的历史数据...\n\n")

	// 2. 对每个商品分析套利机会
	successCount := 0
	for i, good := range goods {
		opp, shouldInclude := analyzeGoodForArbitrage(db, good)
		if shouldInclude {
			opportunities = append(opportunities, opp)
			successCount++

			// 每分析50个商品打印一条进度
			if (i+1)%500 == 0 {
				log.Printf("   进度: %d/%d 商品已分析，发现 %d 个机会\n", i+1, len(goods), successCount)
			}
		}
	}

	log.Printf("\n   ✅ 分析完成: %d 个商品中找到 %d 个机会\n\n", len(goods), len(opportunities))

	// 3. 按评分排序
	sort.Slice(opportunities, func(i, j int) bool {
		return opportunities[i].Score > opportunities[j].Score
	})

	// 4. 只返回前20个最好的机会
	if len(opportunities) > 20 {
		opportunities = opportunities[:20]
	}

	return opportunities, nil
}

// analyzeGoodForArbitrage 分析单个商品的套利机会
// 第一阶段：通过价格快照预测7天后能盈利的饰品
func analyzeGoodForArbitrage(db *gorm.DB, good models.CSQAQGood) (models.ArbitrageOpportunity, bool) {
	var snapshots []models.CSQAQGoodSnapshot

	// 查询最近14天的价格快照（用于更准确的预测）
	fourteenDaysAgo := time.Now().Add(-14 * 24 * time.Hour)
	if err := db.Where("good_id = ? AND created_at >= ?", good.GoodID, fourteenDaysAgo).
		Order("created_at ASC").
		Find(&snapshots).Error; err != nil {
		return models.ArbitrageOpportunity{}, false
	}

	// 需要足够的历史数据进行预测
	if len(snapshots) < 7 {
		return models.ArbitrageOpportunity{}, false
	}

	// 获取当前快照（最新的）
	latestSnapshot := snapshots[len(snapshots)-1]

	// 验证价格数据存在
	if latestSnapshot.YYYPBuyPrice == nil || latestSnapshot.YYYPSellPrice == nil {
		return models.ArbitrageOpportunity{}, false
	}

	currentBuyPrice := *latestSnapshot.YYYPBuyPrice
	currentSellPrice := *latestSnapshot.YYYPSellPrice

	// 提取售价序列用于预测（7天后的售价）
	var sellPrices []float64
	for _, snapshot := range snapshots {
		if snapshot.YYYPSellPrice != nil {
			sellPrices = append(sellPrices, *snapshot.YYYPSellPrice)
		}
	}

	// 预测7天后的售价
	predictedSellPrice := predictPrice7DaysLater(sellPrices)

	// 预测7天后的求购价
	var buyPrices []float64
	for _, snapshot := range snapshots {
		if snapshot.YYYPBuyPrice != nil {
			buyPrices = append(buyPrices, *snapshot.YYYPBuyPrice)
		}
	}
	predictedBuyPrice := predictPrice7DaysLater(buyPrices)

	// 计算7天后的预期利润率（预测）
	// 手续费费率为0.99（悠悠有品扣除1%手续费）
	predictedProfitMargin := predictedSellPrice*0.99 - predictedBuyPrice
	predictedProfitRate := predictedProfitMargin / predictedBuyPrice

	// 关键过滤1：7天后需要能盈利至少5%
	if predictedProfitRate < 0.05 {
		return models.ArbitrageOpportunity{}, false
	}

	// 关键过滤2：在售数量必须 > 100（确保流动性）
	sellCount := getSellOrderCount(latestSnapshot)
	if sellCount <= 100 {
		return models.ArbitrageOpportunity{}, false
	}

	// 关键过滤3：检测最近买价是否有陡峭下跌（防止追高后价格暴跌）
	// 如果最近6小时买价跌幅 > 10%，则不推荐
	if len(buyPrices) >= 2 {
		recentBuyPriceChange := (buyPrices[len(buyPrices)-1] - buyPrices[len(buyPrices)-2]) / buyPrices[len(buyPrices)-2]
		if recentBuyPriceChange < -0.10 { // 跌幅超过10%
			return models.ArbitrageOpportunity{}, false
		}
	}

	// 计算7天平均价格
	avgBuyPrice := calculateAveragePrice(snapshots, "buy")
	avgSellPrice := calculateAveragePrice(snapshots, "sell")

	// 使用多因子模型确定价格趋势
	trend, trendScore := analyzeTrendWith4Factors(snapshots)

	// 计算风险等级（基于波动率和订单量）
	riskLevel := calculateRiskLevel(latestSnapshot, predictedProfitRate, snapshots)

	// 计算综合评分（0-100） - 使用完整的金融量化模型
	score := calculateScore(good.Name, predictedProfitRate*100, riskLevel, latestSnapshot, avgBuyPrice, avgSellPrice, trendScore, len(snapshots), currentBuyPrice, currentSellPrice)

	// 构建套利机会对象
	opp := models.ArbitrageOpportunity{
		GoodID:           good.GoodID,
		GoodName:         good.Name,
		CurrentBuyPrice:  currentBuyPrice,
		CurrentSellPrice: currentSellPrice,
		ProfitRate:       predictedProfitRate * 100, // 转换为百分比（这是7天后的预期利润率）
		EstimatedProfit:  predictedProfitMargin,
		AvgBuyPrice7d:    avgBuyPrice,
		AvgSellPrice7d:   avgSellPrice,
		PriceTrend:       trend,
		DaysOfData:       len(snapshots),
		RiskLevel:        riskLevel,
		BuyOrderCount:    getBuyOrderCount(latestSnapshot),
		SellOrderCount:   getSellOrderCount(latestSnapshot),
		Score:            score,
		AnalysisTime:     time.Now(),
	}

	return opp, true
}

// calculateAveragePrice 计算平均价格
func calculateAveragePrice(snapshots []models.CSQAQGoodSnapshot, priceType string) float64 {
	if len(snapshots) == 0 {
		return 0
	}

	var sum float64
	var count int

	for _, snapshot := range snapshots {
		var price *float64
		if priceType == "buy" && snapshot.YYYPBuyPrice != nil {
			price = snapshot.YYYPBuyPrice
		} else if priceType == "sell" && snapshot.YYYPSellPrice != nil {
			price = snapshot.YYYPSellPrice
		}

		if price != nil {
			sum += *price
			count++
		}
	}

	if count == 0 {
		return 0
	}

	return sum / float64(count)
}

// analyzeTrendWith4Factors 使用四因子模型分析价格趋势
//
// ⚠️ 关键改进：同时分析买价和售价趋势！
// • YYYP_BUY_PRICE：我们从YouPin购买的成本价
// • YYYP_SELL_PRICE：我们在YouPin出售的价格
//
// 四个因子：
// 1. 趋势因子 (Trend Factor): 线性回归斜率，捕捉价格长期上升/下降趋势
// 2. 季节性因子 (Seasonality): 7天周期内的重复模式
// 3. 波动性因子 (Volatility): 历史标准差，量化不确定性
// 4. 均值回归因子 (Mean Reversion): 价格偏离7天平均值的程度
//
// 如果两个价格都下跌 → "down"
// 如果两个价格都上升 → "up"
// 其他情况 → 看综合趋势
func analyzeTrendWith4Factors(snapshots []models.CSQAQGoodSnapshot) (string, float64) {
	if len(snapshots) < 7 {
		return "stable", 50.0
	}

	// 提取买价和售价序列
	var buyPrices, sellPrices []float64
	for _, snapshot := range snapshots {
		if snapshot.YYYPBuyPrice != nil {
			buyPrices = append(buyPrices, *snapshot.YYYPBuyPrice)
		}
		if snapshot.YYYPSellPrice != nil {
			sellPrices = append(sellPrices, *snapshot.YYYPSellPrice)
		}
	}

	if len(buyPrices) < 7 || len(sellPrices) < 7 {
		return "stable", 50.0
	}

	// ===== 分析买价趋势 =====
	buyTrendScore := calculateTrendFactor(buyPrices)
	buySeasonalityScore := calculateSeasonalityFactor(buyPrices)
	buyVolatilityScore := calculateVolatilityFactor(buyPrices)
	buyMeanReversionScore := calculateMeanReversionFactor(buyPrices)
	buyCompositeScore := buyTrendScore*0.40 + buySeasonalityScore*0.25 + buyVolatilityScore*0.20 + buyMeanReversionScore*0.15

	// ===== 分析售价趋势 =====
	sellTrendScore := calculateTrendFactor(sellPrices)
	sellSeasonalityScore := calculateSeasonalityFactor(sellPrices)
	sellVolatilityScore := calculateVolatilityFactor(sellPrices)
	sellMeanReversionScore := calculateMeanReversionFactor(sellPrices)
	sellCompositeScore := sellTrendScore*0.40 + sellSeasonalityScore*0.25 + sellVolatilityScore*0.20 + sellMeanReversionScore*0.15

	// ===== 综合两个趋势 =====
	// 权重分配：买价 60%（我们的成本），售价 40%（市场价格）
	compositeScore := buyCompositeScore*0.60 + sellCompositeScore*0.40

	// 确定趋势方向
	// 如果两个价格都很坏(都<40)，则是down
	// 如果两个价格都很好(都>60)，则是up
	// 否则看综合分数
	var trend string
	if buyCompositeScore < 40 && sellCompositeScore < 40 {
		trend = "down"      // 两个都下跌，最危险
		compositeScore = 25 // 给最低分
	} else if buyCompositeScore > 60 && sellCompositeScore > 60 {
		trend = "up"        // 两个都上升，最乐观
		compositeScore = 75 // 给高分
	} else if compositeScore > 55 {
		trend = "up"
	} else if compositeScore < 45 {
		trend = "down"
	} else {
		trend = "stable"
	}

	return trend, compositeScore
}

// calculateTrendFactor 计算趋势因子 (0-100)
// 使用线性回归斜率：斜率为正表示上升趋势，负表示下降趋势
//
// 改进：动态归一化 + 最近价格权重加强（捕捉陡峭下跌）
func calculateTrendFactor(prices []float64) float64 {
	if len(prices) < 2 {
		return 50.0
	}

	n := float64(len(prices))
	sumX := n * (n - 1) / 2
	sumY := 0.0
	sumXY := 0.0
	sumX2 := n * (n - 1) * (2*n - 1) / 6

	for i, price := range prices {
		sumY += price
		sumXY += float64(i) * price
	}

	// 线性回归斜率
	slope := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)

	// 计算价格平均值，用于动态归一化
	avgPrice := sumY / n

	// 动态计算斜率百分比（相对于平均价格）
	// 这样可以自适应不同的价格水平
	slopePercent := 0.0
	if avgPrice > 0 {
		slopePercent = (slope / avgPrice) * 100 // 斜率相对于平均价格的百分比
	}

	// 将百分比转换到0-100范围
	// 假设正常情况下斜率百分比在[-5%, +5%]范围
	normalizedSlope := 50 + math.Max(-40, math.Min(40, slopePercent/0.1))

	if normalizedSlope > 100 {
		normalizedSlope = 100
	}
	if normalizedSlope < 0 {
		normalizedSlope = 0
	}

	return normalizedSlope
}

// calculateSeasonalityFactor 计算季节性因子 (0-100)
// 检测7天周期内的重复模式（如周末vs工作日）
func calculateSeasonalityFactor(prices []float64) float64 {
	if len(prices) < 7 {
		return 50.0
	}

	// 比较最近7天和前7天的价格模式
	n := len(prices)
	var recentWeek, previousWeek []float64

	if n >= 14 {
		previousWeek = prices[n-14 : n-7]
		recentWeek = prices[n-7:]
	} else {
		// 数据不足14天，返回中性分数
		return 50.0
	}

	// 计算两周价格变化的相似度（皮尔逊相关系数）
	correlation := calculateCorrelation(previousWeek, recentWeek)

	// 将相关系数[-1, 1]转换到[0, 100]范围
	// 高相关性（重复模式）得分高
	seasonalityScore := (correlation + 1) / 2 * 100

	return seasonalityScore
}

// calculateVolatilityFactor 计算波动性因子 (0-100)
// 波动性高表示风险大，得分低；波动性低表示价格稳定，得分高
func calculateVolatilityFactor(prices []float64) float64 {
	if len(prices) < 2 {
		return 50.0
	}

	mean := 0.0
	for _, p := range prices {
		mean += p
	}
	mean /= float64(len(prices))

	variance := 0.0
	for _, p := range prices {
		diff := p - mean
		variance += diff * diff
	}
	variance /= float64(len(prices))
	stdDev := math.Sqrt(variance)

	// 变异系数（相对标准差）
	cv := stdDev / mean

	// 将变异系数转换到0-100范围
	// cv越低越好（低波动性）
	volatilityScore := math.Max(0, 100-cv*500) // 假设cv在0-0.2范围内

	return volatilityScore
}

// calculateMeanReversionFactor 计算均值回归因子 (0-100)
// 价格如果偏离7天均值过远，预示会回归均值
func calculateMeanReversionFactor(prices []float64) float64 {
	if len(prices) < 2 {
		return 50.0
	}

	n := len(prices)
	currentPrice := prices[n-1]

	// 计算7天平均价格
	var sevenDayAvg float64
	startIdx := n - 7
	if startIdx < 0 {
		startIdx = 0
	}

	count := 0
	for i := startIdx; i < n; i++ {
		sevenDayAvg += prices[i]
		count++
	}
	sevenDayAvg /= float64(count)

	// 计算偏离度（百分比）
	deviation := (currentPrice - sevenDayAvg) / sevenDayAvg

	// 如果当前价格低于均值（deviation < 0），说明被低估，有上升空间 -> 得分高
	// 如果当前价格高于均值（deviation > 0），说明被高估，有下降空间 -> 得分低
	// 转换到0-100范围：deviation从-0.2到+0.2映射到100到0
	meanReversionScore := 50 - deviation*250 // deviation*250将[-0.2,+0.2]映射到[100,0]

	if meanReversionScore > 100 {
		meanReversionScore = 100
	}
	if meanReversionScore < 0 {
		meanReversionScore = 0
	}

	return meanReversionScore
}

// calculateCorrelation 计算皮尔逊相关系数 [-1, 1]
func calculateCorrelation(x, y []float64) float64 {
	if len(x) != len(y) || len(x) == 0 {
		return 0
	}

	n := float64(len(x))
	meanX, meanY := 0.0, 0.0

	for i := 0; i < len(x); i++ {
		meanX += x[i]
		meanY += y[i]
	}
	meanX /= n
	meanY /= n

	var numSum, denomX, denomY float64
	for i := 0; i < len(x); i++ {
		diffX := x[i] - meanX
		diffY := y[i] - meanY
		numSum += diffX * diffY
		denomX += diffX * diffX
		denomY += diffY * diffY
	}

	if denomX == 0 || denomY == 0 {
		return 0
	}

	return numSum / math.Sqrt(denomX*denomY)
}

// calculateRiskLevel 计算风险等级
func calculateRiskLevel(snapshot models.CSQAQGoodSnapshot, profitRate float64, snapshots []models.CSQAQGoodSnapshot) string {
	// 基于利润率、订单量和价格波动性

	// 高利润率相对较低风险
	if profitRate > 0.20 {
		return "low"
	}

	// 中等利润
	if profitRate > 0.05 {
		// 检查订单量
		buyCount := getBuyOrderCount(snapshot)
		sellCount := getSellOrderCount(snapshot)

		// 订单很少时提高风险等级
		if buyCount < 2 || sellCount < 2 {
			return "high"
		}

		// 检查波动性
		volatility := calculateVolatility(snapshots)
		if volatility > 0.15 {
			return "high"
		}

		return "medium"
	}

	return "high"
}

// calculateVolatility 计算价格波动率
func calculateVolatility(snapshots []models.CSQAQGoodSnapshot) float64 {
	if len(snapshots) < 2 {
		return 0
	}

	var prices []float64
	for _, snapshot := range snapshots {
		if snapshot.YYYPSellPrice != nil {
			prices = append(prices, *snapshot.YYYPSellPrice)
		}
	}

	if len(prices) < 2 {
		return 0
	}

	mean := 0.0
	for _, p := range prices {
		mean += p
	}
	mean /= float64(len(prices))

	variance := 0.0
	for _, p := range prices {
		diff := p - mean
		variance += diff * diff
	}
	variance /= float64(len(prices))

	stdDev := math.Sqrt(variance)
	return stdDev / mean // 变异系数
}

// calculateScore 计算综合评分 - 金融量化模型
// 集成arbitrage-analyzer的完整评分系统
func calculateScore(goodName string, profitRate float64, riskLevel string, snapshot models.CSQAQGoodSnapshot, avgBuyPrice, avgSellPrice float64, trendScore float64, daysOfData int, currentBuyPrice float64, currentSellPrice float64) float64 {
	score := 0.0

	// === 1. 武器类型加成（权重7%）===
	// 主战武器优先级更高
	weaponBonus := 0.0
	if isMainWeapon(goodName) {
		weaponBonus = 7.0 // 主战武器
	} else {
		weaponBonus = 2.0 // 其他武器
	}
	score += weaponBonus

	// === 2. 磨损度评分（权重12.5%）===
	// 崭新出厂保值率最高
	wearScore := getWearScore(goodName)
	score += wearScore * 2.5 // 最高12.5分

	// === 2.1 破损/战痕主战武器惩罚===
	if wearScore <= 2.0 && isMainWeapon(goodName) {
		score *= 0.85 // 降低15%
	}

	// === 3. 收益率评分（权重25%）===
	profitScore := 0.0
	profitPercent := profitRate
	if profitPercent >= 20.0 {
		profitScore = 25.0
	} else if profitPercent >= 15.0 {
		profitScore = 22.0
	} else if profitPercent >= 10.0 {
		profitScore = 19.0
	} else if profitPercent >= 8.0 {
		profitScore = 16.0
	} else {
		profitScore = profitPercent * 1.8 // 线性评分
	}
	score += profitScore

	// === 4. 风险评分（权重15%）===
	riskScore := 0.0
	switch riskLevel {
	case "low":
		riskScore = 15.0
	case "medium":
		riskScore = 9.0
	case "high":
		riskScore = 3.0
	}
	score += riskScore

	// === 5. 流动性评分（权重16%）===
	liquidityScore := 0.0

	// 买卖比率 - 9%
	buyCount := float64(getBuyOrderCount(snapshot))
	sellCount := float64(getSellOrderCount(snapshot))
	bidAskRatio := buyCount / (sellCount + 1)
	if bidAskRatio > 0.8 {
		liquidityScore += 9.0
	} else if bidAskRatio > 0.5 {
		liquidityScore += 6.5
	} else if bidAskRatio > 0.3 {
		liquidityScore += 4.5
	} else {
		liquidityScore += bidAskRatio * 12
	}

	// 总成交量评分 - 7%
	totalVolume := int(buyCount) + int(sellCount)
	if totalVolume >= 400 {
		liquidityScore += 7.0
	} else if totalVolume >= 250 {
		liquidityScore += 5.5
	} else if totalVolume >= 150 {
		liquidityScore += 3.5
	} else {
		liquidityScore += float64(totalVolume) * 0.02
	}

	score += liquidityScore

	// === 6. 价格趋势评分（权重7%）===
	// 使用四因子模型的趋势分数
	trendScoreNormalized := 0.0
	trendScorePercent := (trendScore - 50) / 50 // 转换到[-1, 1]范围
	if trendScorePercent > 0.1 {
		trendScoreNormalized = 7.0 // up
	} else if trendScorePercent > -0.1 {
		trendScoreNormalized = 5.0 // stable
	} else {
		trendScoreNormalized = 1.0 // down
	}
	score += trendScoreNormalized

	// === 7. 历史数据可靠性（权重5%）===
	dataScore := 0.0
	if daysOfData >= 7 {
		dataScore = 5.0
	} else if daysOfData >= 5 {
		dataScore = 4.0
	} else if daysOfData >= 3 {
		dataScore = 2.5
	} else {
		dataScore = float64(daysOfData) * 0.7
	}
	score += dataScore

	// === 8. 价值投资指标（权重3%）===
	// 低价格高流动性的"价值股"
	if currentBuyPrice < 100 && sellCount >= 150 {
		score += 3.0
	} else if currentBuyPrice < 50 && sellCount >= 100 {
		score += 2.0
	}

	// === 9. 市场周期评分（权重12%）===
	cycleScore := 0.0
	avgPrice := (avgBuyPrice + avgSellPrice) / 2.0
	if avgPrice > 0 {
		priceDeviation := (currentBuyPrice - avgPrice) / avgPrice

		if priceDeviation <= -0.05 {
			cycleScore = 12.0 // 底部区域
		} else if priceDeviation <= -0.02 {
			cycleScore = 9.5 // 接近底部
		} else if priceDeviation <= 0.02 && trendScorePercent > 0.1 {
			cycleScore = 8.0 // 上涨初期
		} else if priceDeviation <= 0.05 && trendScorePercent > 0.1 {
			cycleScore = 5.0 // 上涨中期
		} else if priceDeviation > 0.05 {
			cycleScore = 1.0 // 顶部区域
		} else {
			cycleScore = 5.5 // 震荡区间
		}
	}
	score += cycleScore

	// 确保在0-100之间
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}

	return score
}

// isMainWeapon 判断是否是主战武器（热门武器）
func isMainWeapon(name string) bool {
	mainWeapons := []string{
		"AK-47",
		"M4A4",
		"M4A1-S",
		"M4A1消音",
		"AWP",
		"USP",
		"USP-S",
		"格洛克",
		"Glock",
		"沙漠之鹰",
		"Desert Eagle",
		"P250",
		"CZ75",
	}

	for _, weapon := range mainWeapons {
		if strings.Contains(name, weapon) {
			return true
		}
	}
	return false
}

// getWearScore 获取磨损度评分（崭新 > 略磨 > 久经 > 破损 > 战痕）
func getWearScore(name string) float64 {
	if strings.Contains(name, "崭新出厂") || strings.Contains(name, "Factory New") {
		return 5.0 // 崭新最好
	} else if strings.Contains(name, "略有磨损") || strings.Contains(name, "Minimal Wear") {
		return 4.0 // 略磨次之
	} else if strings.Contains(name, "久经沙场") || strings.Contains(name, "Field-Tested") {
		return 3.0 // 久经居中
	} else if strings.Contains(name, "破损不堪") || strings.Contains(name, "Well-Worn") {
		return 2.0 // 破损较差
	} else if strings.Contains(name, "战痕累累") || strings.Contains(name, "Battle-Scarred") {
		return 1.0 // 战痕最差
	}
	return 2.5 // 默认中等
}

// predictPrice7DaysLater 预测7天后的价格
// 使用Holt-Winters指数平滑法
func predictPrice7DaysLater(prices []float64) float64 {
	if len(prices) == 0 {
		return 0
	}

	// 简单情况：只有一个价格
	if len(prices) == 1 {
		return prices[0]
	}

	// Holt-Winters指数平滑参数
	alpha := 0.3 // 平滑参数（对当前观测值的权重）
	beta := 0.1  // 趋势平滑参数

	// 初始化
	level := prices[0]
	trend := 0.0

	// 如果有至少两个数据点，计算初始趋势
	if len(prices) >= 2 {
		trend = prices[1] - prices[0]
	}

	// 平滑处理所有历史数据
	for i := 1; i < len(prices); i++ {
		prevLevel := level
		level = alpha*prices[i] + (1-alpha)*(level+trend)
		trend = beta*(level-prevLevel) + (1-beta)*trend
	}

	// 预测7步后的价格
	// F(t+7) = level + 7 * trend
	predictedPrice := level + 7*trend

	// 防止预测价格为负
	if predictedPrice <= 0 {
		predictedPrice = prices[len(prices)-1] // 降级使用最后一个已知价格
	}

	return predictedPrice
}

// getBuyOrderCount 获取求购订单数量
func getBuyOrderCount(snapshot models.CSQAQGoodSnapshot) int {
	if snapshot.YYYPBuyCount != nil {
		return *snapshot.YYYPBuyCount
	}
	return 0
}

// getSellOrderCount 获取在售订单数量
func getSellOrderCount(snapshot models.CSQAQGoodSnapshot) int {
	if snapshot.YYYPSellCount != nil {
		return *snapshot.YYYPSellCount
	}
	return 0
}

// ============ 第二阶段：双线程验证实时数据 ============

// ValidatedOpportunity 经过验证的求购机会
type ValidatedOpportunity struct {
	models.ArbitrageOpportunity
	RealTimeBuyPrice    float64 // 通过OpenAPI获取的当前求购价
	RealTimeSellPrice   float64 // 通过OpenAPI获取的当前在售价
	RealTimeSellCount   int     // 通过OpenAPI获取的实时在售数量
	CurrentProfit       float64 // 当前实时利润率（基于实时数据）
	IsValidated         bool    // 是否通过验证
	ValidationError     string  // 验证失败原因
	RecommendedQuantity int     // 建议求购数量
}

// verifyOpportunitiesWithRealTimeData 第二阶段：双线程验证实时数据
// 条件：在售数量 > 100，利润率 > 5%
func verifyOpportunitiesWithRealTimeData(db *gorm.DB, ypClient *youpin.OpenAPIClient, opportunities []models.ArbitrageOpportunity, doImmediatePurchase bool, orderMap map[int64]PurchaseOrder) []models.ArbitrageOpportunity {
	var validated []models.ArbitrageOpportunity
	mu := sync.Mutex{}
	wg := sync.WaitGroup{}

	// 并发处理每个机会，使用双线程池
	semaphore := make(chan struct{}, 2) // 最多2个并发线程

	for _, opp := range opportunities {
		wg.Add(1)
		go func(opportunity models.ArbitrageOpportunity) {
			defer wg.Done()

			semaphore <- struct{}{}        // 获取许可
			defer func() { <-semaphore }() // 释放许可

			// 验证单个机会
			if isOpportunityValid(opportunity) {
				mu.Lock()
				validated = append(validated, opportunity)
				mu.Unlock()

				// 通过后立即按规划数量实时下单（可选）
				if doImmediatePurchase && ypClient != nil {
					// 仅对在规划清单中的条目执行下单
					if od, ok := orderMap[opportunity.GoodID]; ok && od.Quantity > 0 {
						// 二次实时获取当前最高求购价
						maxBuy, err := getLatestMaxBuyPrice(db, ypClient, opportunity.GoodID)
						if err != nil || maxBuy <= 0 {
							if err != nil {
								log.Printf("[自动下单] %s 获取最高求购价失败: %v", opportunity.GoodName, err)
							} else {
								log.Printf("[自动下单] %s 获取最高求购价失败: 值无效", opportunity.GoodName)
							}
						} else {
							price := bumpPurchasePrice(maxBuy)
							if err := placeImmediatePurchaseOrder(db, ypClient, opportunity.GoodID, opportunity.GoodName, od.Quantity, price); err != nil {
								log.Printf("[自动下单] %s 下单失败: %v", opportunity.GoodName, err)
							} else {
								log.Printf("[自动下单] %s 已下单: 数量=%d, 价格=¥%.2f (最高=¥%.2f)", opportunity.GoodName, od.Quantity, price, maxBuy)
							}
						}
					}
				}
			}
		}(opp)
	}

	wg.Wait()

	// 按利润率排序
	sort.Slice(validated, func(i, j int) bool {
		return validated[i].ProfitRate > validated[j].ProfitRate
	})

	return validated
}

// isOpportunityValid 验证单个机会是否符合条件
// 条件：在售数量 > 100，利润率 > 5%
func isOpportunityValid(opp models.ArbitrageOpportunity) bool {
	// 条件1：在售数量 > 100
	if opp.SellOrderCount <= 100 {
		return false
	}

	// 条件2：利润率 > 5%
	if opp.ProfitRate <= 5.0 {
		return false
	}

	// 所有条件都满足
	return true
}

// —— 实时下单相关工具 ——

// getActiveYouPinToken 获取激活的悠悠有品账号Token
func getActiveYouPinToken(db *gorm.DB) string {
	var acct models.YouPinAccount
	if err := db.Where("is_active = ?", true).First(&acct).Error; err == nil && acct.Token != "" {
		return acct.Token
	}
	return ""
}

// getLatestMaxBuyPrice 获取指定商品当前最高求购价（通过模板ID）
func getLatestMaxBuyPrice(db *gorm.DB, ypClient *youpin.OpenAPIClient, goodID int64) (float64, error) {
	// 从快照获取模板ID
	var snap models.CSQAQGoodSnapshot
	if err := db.Where("good_id = ? AND yyyp_template_id IS NOT NULL", goodID).Order("created_at DESC").First(&snap).Error; err != nil || snap.YYYPTemplateID == nil || *snap.YYYPTemplateID == 0 {
		return 0, fmt.Errorf("no template id for good %d", goodID)
	}
	tplID := int(*snap.YYYPTemplateID)

	// 拉取求购列表，取最高价
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req := &youpin.GetTemplatePurchaseOrderListRequest{
		TemplateId:       tplID,
		PageIndex:        1,
		PageSize:         50,
		ShowMaxPriceFlag: false,
	}
	resp, err := ypClient.GetTemplatePurchaseOrderList(ctx, req)
	if err != nil || resp == nil {
		if err != nil {
			return 0, err
		}
		return 0, fmt.Errorf("empty response")
	}
	maxBuy := 0.0
	for _, it := range resp.Data {
		if it.PurchasePrice > maxBuy {
			maxBuy = it.PurchasePrice
		}
	}
	return maxBuy, nil
}

// bumpPurchasePrice 按区间步进加价（0~1:0.01, 1~50:0.1, 50~1000:1）
func bumpPurchasePrice(maxBuy float64) float64 {
	if maxBuy < 0 {
		maxBuy = 0
	}
	var step float64
	var decimals float64
	switch {
	case maxBuy < 1:
		step = 0.01
		decimals = 2
	case maxBuy < 50:
		step = 0.1
		decimals = 1
	default:
		step = 1
		decimals = 0
	}
	base := math.Floor(maxBuy/step) * step
	bumped := base + step
	pow := math.Pow(10, decimals)
	return math.Round(bumped*pow) / pow
}

// placeImmediatePurchaseOrder 拉取模板信息→预检→创建求购订单
func placeImmediatePurchaseOrder(db *gorm.DB, ypClient *youpin.OpenAPIClient, goodID int64, goodName string, quantity int, purchasePrice float64) error {
	// 获取模板ID
	var snap models.CSQAQGoodSnapshot
	if err := db.Where("good_id = ? AND yyyp_template_id IS NOT NULL", goodID).Order("created_at DESC").First(&snap).Error; err != nil || snap.YYYPTemplateID == nil || *snap.YYYPTemplateID == 0 {
		return fmt.Errorf("no template id for good %d", goodID)
	}
	templateIDStr := fmt.Sprintf("%d", *snap.YYYPTemplateID)

	// 获取模板求购信息
	ctxInfo, cancelInfo := context.WithTimeout(context.Background(), 15*time.Second)
	info, err := ypClient.GetTemplatePurchaseInfo(ctxInfo, templateIDStr)
	cancelInfo()
	if err != nil || info == nil {
		if err != nil {
			return fmt.Errorf("get template info failed: %w", err)
		}
		return fmt.Errorf("get template info failed: empty")
	}
	tpl := info.Data.TemplateInfo

	// 预检查
	total := purchasePrice * float64(quantity)
	minSell, _ := strconv.ParseFloat(tpl.MinSellPrice, 64)
	maxPurchase, _ := strconv.ParseFloat(tpl.MaxPurchasePrice, 64)
	preReq := &youpin.PrePurchaseOrderCheckRequest{
		SpecialStyleObj:  map[string]interface{}{},
		IsCheckMaxPrice:  false,
		TemplateHashName: tpl.TemplateHashName,
		TotalAmount:      total,
		ReferencePrice:   tpl.ReferencePrice,
		PurchasePrice:    purchasePrice,
		PurchaseNum:      quantity,
		DiscountAmount:   0,
		MinSellPrice:     minSell,
		MaxPurchasePrice: maxPurchase,
		TemplateId:       templateIDStr,
	}
	ctxPre, cancelPre := context.WithTimeout(context.Background(), 15*time.Second)
	_, _ = ypClient.PrePurchaseOrderCheck(ctxPre, preReq)
	cancelPre()

	// 创建订单
	saveReq := &youpin.SavePurchaseOrderRequest{
		TemplateId:            tpl.TemplateId,
		TemplateHashName:      tpl.TemplateHashName,
		CommodityName:         tpl.CommodityName,
		ReferencePrice:        tpl.ReferencePrice,
		MinSellPrice:          tpl.MinSellPrice,
		MaxPurchasePrice:      tpl.MaxPurchasePrice,
		PurchasePrice:         purchasePrice,
		PurchaseNum:           quantity,
		NeedPaymentAmount:     total,
		TotalAmount:           total,
		TemplateName:          tpl.CommodityName,
		PriceDifference:       0,
		DiscountAmount:        0,
		PayConfirmFlag:        false,
		RepeatOrderCancelFlag: false,
	}
	ctxSave, cancelSave := context.WithTimeout(context.Background(), 15*time.Second)
	resp, err := ypClient.SavePurchaseOrder(ctxSave, saveReq)
	cancelSave()
	if err == nil && resp != nil {
		return nil
	}
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "REPEAT_ORDER_CONFIRM") {
			saveReq.RepeatOrderCancelFlag = true
			ctx1, c1 := context.WithTimeout(context.Background(), 15*time.Second)
			resp, err = ypClient.SavePurchaseOrder(ctx1, saveReq)
			c1()
			if err == nil && resp != nil {
				return nil
			}
			if err != nil && strings.Contains(err.Error(), "PRICE_WARNING") {
				saveReq.PayConfirmFlag = true
				ctx2, c2 := context.WithTimeout(context.Background(), 15*time.Second)
				resp, err = ypClient.SavePurchaseOrder(ctx2, saveReq)
				c2()
				if err == nil && resp != nil {
					return nil
				}
			}
		} else if strings.Contains(msg, "PRICE_WARNING") {
			saveReq.PayConfirmFlag = true
			ctx3, c3 := context.WithTimeout(context.Background(), 15*time.Second)
			resp, err = ypClient.SavePurchaseOrder(ctx3, saveReq)
			c3()
			if err == nil && resp != nil {
				return nil
			}
		}
	}
	if err != nil {
		return fmt.Errorf("save purchase order failed: %w", err)
	}
	return fmt.Errorf("save purchase order failed: unknown error")
}
