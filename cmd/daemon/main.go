package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// 硬编码配置
const (
	STEAM_ID       = "76561199078507841"
	YOUPIN_APP_KEY = "12919014"
)

var (
	checkInterval = flag.Duration("interval", 5*time.Minute, "检查间隔时间")
	isBacktest    = flag.Bool("backtest", true, "是否启用回测模式")
	backtestDays  = flag.Int("days", 7, "回测天数")
)

func main() {
	flag.Parse()

	log.Printf("╔════════════════════════════════════════════════════════════════╗\n")
	log.Printf("║              【后台守护进程】- 回测分析与策略反馈             ║\n")
	log.Printf("║                                                                ║\n")
	log.Printf("║ 功能: 按趋势分类历史机会 → 回测分析 → 输出策略反馈            ║\n")
	log.Printf("║ 执行: 长期后台运行 (不kill会一直运行)                         ║\n")
	log.Printf("║ 检查间隔: %v                                                  ║\n", *checkInterval)
	log.Printf("║ 回测模式: %v                                                  ║\n", *isBacktest)
	log.Printf("║                                                                ║\n")
	log.Printf("╚════════════════════════════════════════════════════════════════╝\n\n")

	// 数据库连接
	dsn := "root:Wyj250413.@tcp(23.254.215.66:3306)/csgo_trader?charset=utf8mb4&parseTime=True&loc=Local"
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ 数据库连接失败: %v\n", err)
	}

	log.Printf("✅ 数据库连接成功\n")
	log.Printf("✅ 后台守护进程已启动 (PID: %d)\n\n", os.Getpid())

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 主循环
	ticker := time.NewTicker(*checkInterval)
	defer ticker.Stop()

	iteration := 0

	for {
		select {
		case <-sigChan:
			log.Printf("\n\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			log.Printf("🛑 收到关闭信号，正在优雅关闭...\n")
			log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			return

		case <-ticker.C:
			iteration++
			log.Printf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			log.Printf("[迭代 #%d] ⏰ %s\n", iteration, time.Now().Format("2006-01-02 15:04:05"))
			log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

			// 回测分析
			if *isBacktest {
				log.Printf("\n[回测模块] 📊 趋势分类回测 + 策略反馈\n")
				runBacktest(db, ctx, *backtestDays)
			}

			cancel()

			log.Printf("\n✅ 本轮检查完成，下次检查在 %v 后\n", *checkInterval)
		}
	}
}

func runBacktest(db *gorm.DB, ctx context.Context, days int) {
	// 查询N天前的分析结果
	startDate := time.Now().AddDate(0, 0, -days)

	var backtestData []map[string]interface{}
	if err := db.Raw(`
		SELECT
			DATE(analysis_time) as date,
			good_name,
			current_buy_price,
			current_sell_price,
			price_trend,
			risk_level,
			score,
			(current_sell_price * 0.99 - current_buy_price) / current_buy_price * 100 as profit_rate
		FROM arbitrage_opportunities
		WHERE analysis_time >= ?
		ORDER BY analysis_time DESC
		LIMIT 500
	`, startDate).Scan(&backtestData).Error; err != nil {
		log.Printf("   ⚠️ 查询回测数据失败: %v\n", err)
		return
	}

	if len(backtestData) == 0 {
		log.Printf("   ℹ️ 暂无 %d 天的历史数据\n", days)
		return
	}

	log.Printf("   • 分析 %d 天前 (%s) 的 %d 笔交易\n", days, startDate.Format("2006-01-02"), len(backtestData))

	// 🆕 按趋势分类回测结果
	type TrendStats struct {
		Count           int
		TotalProfit     float64
		MaxProfit       float64
		MinProfit       float64
		ProfitableCount int
	}

	trendMap := make(map[string]*TrendStats)
	trendMap["up"] = &TrendStats{MaxProfit: -100, MinProfit: 100}
	trendMap["down"] = &TrendStats{MaxProfit: -100, MinProfit: 100}
	trendMap["stable"] = &TrendStats{MaxProfit: -100, MinProfit: 100}
	trendMap["unknown"] = &TrendStats{MaxProfit: -100, MinProfit: 100}

	// 总体统计
	totalProfit := 0.0
	totalMaxProfit := -100.0
	totalMinProfit := 100.0
	totalProfitableCount := 0

	// 分类处理
	for _, record := range backtestData {
		var profit float64
		var trend string

		if p, ok := record["profit_rate"].(float64); ok {
			profit = p
		}
		if t, ok := record["price_trend"].(string); ok {
			trend = t
		} else {
			trend = "unknown"
		}

		// 确保map中有这个趋势
		if _, exists := trendMap[trend]; !exists {
			trendMap[trend] = &TrendStats{MaxProfit: -100, MinProfit: 100}
		}

		stats := trendMap[trend]
		stats.Count++
		stats.TotalProfit += profit

		if profit > stats.MaxProfit {
			stats.MaxProfit = profit
		}
		if profit < stats.MinProfit {
			stats.MinProfit = profit
		}
		if profit > 0 {
			stats.ProfitableCount++
		}

		// 总体统计
		totalProfit += profit
		if profit > totalMaxProfit {
			totalMaxProfit = profit
		}
		if profit < totalMinProfit {
			totalMinProfit = profit
		}
		if profit > 0 {
			totalProfitableCount++
		}
	}

	// 计算总体指标
	totalAvgProfit := totalProfit / float64(len(backtestData))
	totalWinRate := float64(totalProfitableCount) / float64(len(backtestData)) * 100

	log.Printf("\n   📊 【总体回测结果】\n")
	log.Printf("      • 平均利润率: %.2f%%\n", totalAvgProfit)
	log.Printf("      • 最高利润: %.2f%%\n", totalMaxProfit)
	log.Printf("      • 最低利润: %.2f%%\n", totalMinProfit)
	log.Printf("      • 胜率: %.1f%% (%d/%d)\n", totalWinRate, totalProfitableCount, len(backtestData))

	// 🆕 按趋势分别输出统计
	log.Printf("\n   📈 【按价格趋势分类的回测分析】\n")

	var upStats, downStats *TrendStats

	for trend, stats := range trendMap {
		if stats.Count == 0 {
			continue
		}

		avgProfit := stats.TotalProfit / float64(stats.Count)
		winRate := float64(stats.ProfitableCount) / float64(stats.Count) * 100
		percent := float64(stats.Count) / float64(len(backtestData)) * 100

		switch trend {
		case "up":
			upStats = stats
			log.Printf("\n      📈 向上趋势: %.0f%% (%.0f个交易)\n", percent, float64(stats.Count))
		case "down":
			downStats = stats
			log.Printf("\n      📉 向下趋势: %.0f%% (%.0f个交易)\n", percent, float64(stats.Count))
		case "stable":
			log.Printf("\n      → 稳定趋势: %.0f%% (%.0f个交易)\n", percent, float64(stats.Count))
		default:
			log.Printf("\n      ❓ 未知趋势: %.0f%% (%.0f个交易)\n", percent, float64(stats.Count))
		}

		log.Printf("         • 平均利润: %.2f%%\n", avgProfit)
		log.Printf("         • 最高利润: %.2f%%\n", stats.MaxProfit)
		log.Printf("         • 最低利润: %.2f%%\n", stats.MinProfit)
		log.Printf("         • 胜率: %.1f%% (%d/%d)\n", winRate, stats.ProfitableCount, stats.Count)
	}

	// 🆕 【策略反馈】
	log.Printf("\n   💡 【策略反馈与分析】\n")

	if upStats != nil && downStats != nil {
		upWinRate := float64(upStats.ProfitableCount) / float64(upStats.Count) * 100
		downWinRate := float64(downStats.ProfitableCount) / float64(downStats.Count) * 100
		upAvgProfit := upStats.TotalProfit / float64(upStats.Count)
		downAvgProfit := downStats.TotalProfit / float64(downStats.Count)

		log.Printf("\n      🎯 关键对比:\n")
		log.Printf("         • 向上趋势胜率: %.1f%% vs 向下趋势胜率: %.1f%%\n", upWinRate, downWinRate)
		log.Printf("         • 向上趋势平均利润: %.2f%% vs 向下趋势平均利润: %.2f%%\n", upAvgProfit, downAvgProfit)

		// 🆕 【生成可实施的建议】
		if downWinRate < 30 && upWinRate > 70 {
			log.Printf("\n      🔴 【严重问题】\n")
			log.Printf("         下跌趋势物品的胜率仅 %.1f%%，而上升趋势物品胜率 %.1f%%\n", downWinRate, upWinRate)
			log.Printf("         这表明 analyzer 对下跌趋势的识别不够激进\n\n")
			log.Printf("      ✅ 【建议的 analyzer 改进方案】\n")
			log.Printf("         1. 修改趋势评分惩罚:\n")
			log.Printf("            现在: down趋势 = -6 分\n")
			log.Printf("            建议: down趋势 = -12 到 -15 分\n\n")
			log.Printf("         2. 添加双价格下跌检测:\n")
			log.Printf("            当 YYYP_BUY_PRICE 和 YYYP_SELL_PRICE 都下跌时\n")
			log.Printf("            直接评分为极低 (< 20 分)\n")
			log.Printf("            或直接排除这些物品\n\n")
			log.Printf("         3. 风险等级优化:\n")
			log.Printf("            HIGH风险 + down趋势 = 自动过滤\n")
			log.Printf("            MEDIUM风险 + down趋势 = 评分-10分\n\n")
			log.Printf("         4. 市场环境感知:\n")
			log.Printf("            如果>40%%的机会是down趋势\n")
			log.Printf("            则全局降低所有评分 (乘以0.8)\n")
		} else if upWinRate > 60 && downWinRate > 50 {
			log.Printf("\n      🟢 【正常状态】\n")
			log.Printf("         向上趋势胜率: %.1f%% (可接受)\n", upWinRate)
			log.Printf("         向下趋势胜率: %.1f%% (可接受)\n", downWinRate)
			log.Printf("         建议: 维持当前 analyzer 策略\n")
		} else if downWinRate > upWinRate {
			log.Printf("\n      ⚠️ 【异常情况】\n")
			log.Printf("         向下趋势胜率 (%.1f%%) 高于向上趋势 (%.1f%%)\n", downWinRate, upWinRate)
			log.Printf("         这可能表示:\n")
			log.Printf("         • 价格反转期间，下跌物品有反弹潜力\n")
			log.Printf("         • 或者 analyzer 趋势判断过于保守\n")
			log.Printf("         建议人工审查最近的下跌物品数据\n")
		}
	}

	// 🆕 【用户行动项】
	log.Printf("\n   📋 【后续行动】\n")
	log.Printf("      1. 如果上述建议已实施，请重新编译 analyzer\n")
	log.Printf("      2. 在 dist/ 目录中查看 STRATEGY_FEEDBACK_REPORT.md 获取详细建议\n")
	log.Printf("      3. 下一轮回测时对比改进前后的效果\n")

	// 回测分析
	log.Printf("\n   💬 【总体回测分析】\n")
	if totalWinRate > 80 {
		log.Printf("      • ✅ 胜率很高 (%.1f%%), 当前 analyzer 策略有效\n", totalWinRate)
	} else if totalWinRate > 60 {
		log.Printf("      • ⚡ 胜率中等偏上 (%.1f%%), 策略基本有效，可以进一步优化\n", totalWinRate)
	} else if totalWinRate > 50 {
		log.Printf("      • ⚠️ 胜率中等 (%.1f%%), 需要针对性优化\n", totalWinRate)
	} else {
		log.Printf("      • ❌ 胜率较低 (%.1f%%), 需要重新评估 analyzer 策略\n", totalWinRate)
	}
}
