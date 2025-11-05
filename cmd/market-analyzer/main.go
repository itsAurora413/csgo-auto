package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"csgo-trader/internal/database"
	"csgo-trader/internal/models"
	"gorm.io/gorm"
)

var (
	dbURL      = flag.String("db", "", "数据库连接URL")
	days       = flag.Int("days", 90, "分析天数，默认90天")
	outputFile = flag.String("output", "market_analysis.json", "输出文件路径")
	verbose    = flag.Bool("verbose", false, "详细输出")
)

// MarketAnalysis 市场分析结果
type MarketAnalysis struct {
	AnalysisDate  time.Time              `json:"analysis_date"`
	Period        string                 `json:"period"`
	TotalItems    int                    `json:"total_items"`
	OverallTrend  string                 `json:"overall_trend"`  // up, down, stable
	TrendStrength float64                `json:"trend_strength"` // 0-100
	PriceChange   PriceChangeStats       `json:"price_change"`
	VolumeStats   VolumeStats            `json:"volume_stats"`
	TopGainers    []ItemPerformance      `json:"top_gainers"`
	TopLosers     []ItemPerformance      `json:"top_losers"`
	RecentEvents  []MarketEvent          `json:"recent_events"`
	HistoryCompare []HistoricalComparison `json:"history_compare"`
	Insights      []string               `json:"insights"`
}

// PriceChangeStats 价格变化统计
type PriceChangeStats struct {
	AvgChangePercent float64 `json:"avg_change_percent"`
	MedianChange     float64 `json:"median_change"`
	VolatilityIndex  float64 `json:"volatility_index"`
	ItemsUp          int     `json:"items_up"`
	ItemsDown        int     `json:"items_down"`
	ItemsStable      int     `json:"items_stable"`
}

// VolumeStats 成交量统计
type VolumeStats struct {
	AvgDailyVolume   float64 `json:"avg_daily_volume"`
	VolumeGrowth     float64 `json:"volume_growth"`
	HighVolumeItems  int     `json:"high_volume_items"`
}

// ItemPerformance 物品表现
type ItemPerformance struct {
	GoodID       int     `json:"good_id"`
	GoodName     string  `json:"good_name"`
	PriceStart   float64 `json:"price_start"`
	PriceEnd     float64 `json:"price_end"`
	ChangePercent float64 `json:"change_percent"`
	VolumeChange float64 `json:"volume_change"`
}

// MarketEvent 市场事件
type MarketEvent struct {
	Date        time.Time `json:"date"`
	EventType   string    `json:"event_type"` // spike, crash, surge, holiday
	Description string    `json:"description"`
	Impact      string    `json:"impact"` // high, medium, low
}

// HistoricalComparison 历史对比
type HistoricalComparison struct {
	Period          string  `json:"period"`
	StartDate       string  `json:"start_date"`
	EndDate         string  `json:"end_date"`
	Similarity      float64 `json:"similarity"` // 0-100
	TrendPattern    string  `json:"trend_pattern"`
	AvgChange       float64 `json:"avg_change"`
	VolumePattern   string  `json:"volume_pattern"`
}

// DailyStats 每日统计
type DailyStats struct {
	Date         time.Time
	AvgPrice     float64
	TotalVolume  int64
	ItemCount    int
	PriceChange  float64
	VolumeChange float64
}

func main() {
	flag.Parse()

	// 初始化数据库
	db, err := database.Initialize(*dbURL)
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}

	log.Printf("开始市场分析（过去%d天）...", *days)

	// 执行分析
	analysis, err := analyzeMarket(db, *days)
	if err != nil {
		log.Fatalf("市场分析失败: %v", err)
	}

	// 输出结果
	if *verbose {
		printAnalysisVerbose(analysis)
	} else {
		printAnalysisSummary(analysis)
	}

	// 保存到文件
	if err := saveAnalysisToFile(analysis, *outputFile); err != nil {
		log.Printf("保存分析结果失败: %v", err)
	} else {
		log.Printf("分析结果已保存到: %s", *outputFile)
	}
}

// analyzeMarket 执行市场分析
func analyzeMarket(db *gorm.DB, days int) (*MarketAnalysis, error) {
	ctx := context.Background()
	_ = ctx

	startDate := time.Now().AddDate(0, 0, -days)
	endDate := time.Now()

	analysis := &MarketAnalysis{
		AnalysisDate: time.Now(),
		Period:       fmt.Sprintf("%d天", days),
		Insights:     []string{},
	}

	// 获取价格快照数据
	log.Println("正在获取价格快照数据...")
	snapshots, err := getSnapshots(db, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("获取快照数据失败: %w", err)
	}

	if len(snapshots) == 0 {
		return nil, fmt.Errorf("没有找到快照数据")
	}

	log.Printf("找到 %d 条快照记录", len(snapshots))

	// 计算每日统计
	dailyStats := calculateDailyStats(snapshots)

	// 分析整体趋势
	analysis.OverallTrend, analysis.TrendStrength = analyzeTrend(dailyStats)

	// 分析价格变化
	analysis.PriceChange = analyzePriceChange(snapshots, dailyStats)

	// 分析成交量
	analysis.VolumeStats = analyzeVolume(dailyStats)

	// 找出涨跌幅前10
	analysis.TopGainers, analysis.TopLosers = findTopPerformers(db, snapshots, startDate, endDate)

	// 识别市场事件
	analysis.RecentEvents = identifyMarketEvents(dailyStats, startDate, endDate)

	// 历史对比
	analysis.HistoryCompare = compareWithHistory(db, dailyStats, days)

	// 生成洞察
	analysis.Insights = generateInsights(analysis)

	analysis.TotalItems = countUniqueGoods(snapshots)

	return analysis, nil
}

// getSnapshots 获取快照数据
func getSnapshots(db *gorm.DB, startDate, endDate time.Time) ([]models.CSQAQGoodSnapshot, error) {
	var snapshots []models.CSQAQGoodSnapshot

	// 只获取有价格数据的快照
	err := db.Where("created_at >= ? AND created_at <= ?", startDate, endDate).
		Where("(yyyp_sell_price IS NOT NULL OR buff_sell_price IS NOT NULL)").
		Order("created_at ASC").
		Find(&snapshots).Error

	return snapshots, err
}

// calculateDailyStats 计算每日统计
func calculateDailyStats(snapshots []models.CSQAQGoodSnapshot) []DailyStats {
	// 按日期分组
	dailyMap := make(map[string]*DailyStats)

	for _, snap := range snapshots {
		dateKey := snap.CreatedAt.Format("2006-01-02")

		if _, exists := dailyMap[dateKey]; !exists {
			dailyMap[dateKey] = &DailyStats{
				Date: snap.CreatedAt.Truncate(24 * time.Hour),
			}
		}

		stats := dailyMap[dateKey]

		// 优先使用悠悠有品价格，其次BUFF价格
		price := 0.0
		if snap.YYYPSellPrice != nil && *snap.YYYPSellPrice > 0 {
			price = *snap.YYYPSellPrice
		} else if snap.BuffSellPrice != nil && *snap.BuffSellPrice > 0 {
			price = *snap.BuffSellPrice
		}

		volume := 0
		if snap.YYYPSellCount != nil {
			volume = *snap.YYYPSellCount
		}

		stats.AvgPrice += price
		stats.TotalVolume += int64(volume)
		stats.ItemCount++
	}

	// 计算平均值
	for _, stats := range dailyMap {
		if stats.ItemCount > 0 {
			stats.AvgPrice /= float64(stats.ItemCount)
		}
	}

	// 转换为切片并排序
	dailyStatsList := make([]DailyStats, 0, len(dailyMap))
	for _, stats := range dailyMap {
		dailyStatsList = append(dailyStatsList, *stats)
	}

	sort.Slice(dailyStatsList, func(i, j int) bool {
		return dailyStatsList[i].Date.Before(dailyStatsList[j].Date)
	})

	// 计算环比变化
	for i := 1; i < len(dailyStatsList); i++ {
		prev := dailyStatsList[i-1]
		curr := &dailyStatsList[i]

		if prev.AvgPrice > 0 {
			curr.PriceChange = ((curr.AvgPrice - prev.AvgPrice) / prev.AvgPrice) * 100
		}

		if prev.TotalVolume > 0 {
			curr.VolumeChange = ((float64(curr.TotalVolume) - float64(prev.TotalVolume)) / float64(prev.TotalVolume)) * 100
		}
	}

	return dailyStatsList
}

// analyzeTrend 分析趋势
func analyzeTrend(dailyStats []DailyStats) (string, float64) {
	if len(dailyStats) < 2 {
		return "stable", 0
	}

	// 计算线性回归斜率
	n := float64(len(dailyStats))
	var sumX, sumY, sumXY, sumX2 float64

	for i, stats := range dailyStats {
		x := float64(i)
		y := stats.AvgPrice
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	slope := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
	avgPrice := sumY / n

	// 计算趋势强度（标准化斜率）
	strength := math.Abs((slope / avgPrice) * 100)

	// 判断趋势方向
	trend := "stable"
	if slope > 0.01 {
		trend = "up"
	} else if slope < -0.01 {
		trend = "down"
	}

	return trend, math.Min(strength, 100)
}

// analyzePriceChange 分析价格变化
func analyzePriceChange(snapshots []models.CSQAQGoodSnapshot, dailyStats []DailyStats) PriceChangeStats {
	stats := PriceChangeStats{}

	if len(dailyStats) == 0 {
		return stats
	}

	// 计算价格变化
	changes := []float64{}
	for _, daily := range dailyStats {
		if daily.PriceChange != 0 {
			changes = append(changes, daily.PriceChange)

			if daily.PriceChange > 0.5 {
				stats.ItemsUp++
			} else if daily.PriceChange < -0.5 {
				stats.ItemsDown++
			} else {
				stats.ItemsStable++
			}
		}
	}

	if len(changes) > 0 {
		// 平均变化
		var sum float64
		for _, c := range changes {
			sum += c
		}
		stats.AvgChangePercent = sum / float64(len(changes))

		// 中位数
		sort.Float64s(changes)
		mid := len(changes) / 2
		if len(changes)%2 == 0 {
			stats.MedianChange = (changes[mid-1] + changes[mid]) / 2
		} else {
			stats.MedianChange = changes[mid]
		}

		// 波动率（标准差）
		var variance float64
		for _, c := range changes {
			variance += math.Pow(c-stats.AvgChangePercent, 2)
		}
		stats.VolatilityIndex = math.Sqrt(variance / float64(len(changes)))
	}

	return stats
}

// analyzeVolume 分析成交量
func analyzeVolume(dailyStats []DailyStats) VolumeStats {
	stats := VolumeStats{}

	if len(dailyStats) == 0 {
		return stats
	}

	// 平均每日成交量
	var totalVolume int64
	for _, daily := range dailyStats {
		totalVolume += daily.TotalVolume
	}
	stats.AvgDailyVolume = float64(totalVolume) / float64(len(dailyStats))

	// 成交量增长率（前后对比）
	if len(dailyStats) >= 2 {
		firstHalf := dailyStats[:len(dailyStats)/2]
		secondHalf := dailyStats[len(dailyStats)/2:]

		var firstVol, secondVol int64
		for _, daily := range firstHalf {
			firstVol += daily.TotalVolume
		}
		for _, daily := range secondHalf {
			secondVol += daily.TotalVolume
		}

		if firstVol > 0 {
			stats.VolumeGrowth = ((float64(secondVol) - float64(firstVol)) / float64(firstVol)) * 100
		}
	}

	return stats
}

// findTopPerformers 找出表现最好和最差的物品
func findTopPerformers(db *gorm.DB, snapshots []models.CSQAQGoodSnapshot, startDate, endDate time.Time) ([]ItemPerformance, []ItemPerformance) {
	// 按商品分组
	goodsMap := make(map[int64][]models.CSQAQGoodSnapshot)
	for _, snap := range snapshots {
		goodsMap[snap.GoodID] = append(goodsMap[snap.GoodID], snap)
	}

	// 获取商品名称映射
	goodNames := make(map[int64]string)
	var goods []models.CSQAQGood
	if err := db.Find(&goods).Error; err == nil {
		for _, good := range goods {
			goodNames[good.GoodID] = good.Name
		}
	}

	// 计算每个商品的表现
	performances := []ItemPerformance{}
	for goodID, snaps := range goodsMap {
		if len(snaps) < 2 {
			continue
		}

		// 排序
		sort.Slice(snaps, func(i, j int) bool {
			return snaps[i].CreatedAt.Before(snaps[j].CreatedAt)
		})

		first := snaps[0]
		last := snaps[len(snaps)-1]

		// 获取价格
		priceStart := 0.0
		if first.YYYPSellPrice != nil && *first.YYYPSellPrice > 0 {
			priceStart = *first.YYYPSellPrice
		} else if first.BuffSellPrice != nil && *first.BuffSellPrice > 0 {
			priceStart = *first.BuffSellPrice
		}

		priceEnd := 0.0
		if last.YYYPSellPrice != nil && *last.YYYPSellPrice > 0 {
			priceEnd = *last.YYYPSellPrice
		} else if last.BuffSellPrice != nil && *last.BuffSellPrice > 0 {
			priceEnd = *last.BuffSellPrice
		}

		if priceStart > 0 && priceEnd > 0 {
			changePercent := ((priceEnd - priceStart) / priceStart) * 100

			goodName := goodNames[goodID]
			if goodName == "" {
				goodName = fmt.Sprintf("商品ID:%d", goodID)
			}

			perf := ItemPerformance{
				GoodID:       int(goodID),
				GoodName:     goodName,
				PriceStart:   priceStart,
				PriceEnd:     priceEnd,
				ChangePercent: changePercent,
			}

			performances = append(performances, perf)
		}
	}

	// 排序
	sort.Slice(performances, func(i, j int) bool {
		return performances[i].ChangePercent > performances[j].ChangePercent
	})

	// 取前10和后10
	topGainers := []ItemPerformance{}
	topLosers := []ItemPerformance{}

	if len(performances) > 0 {
		n := 10
		if len(performances) < n {
			n = len(performances)
		}

		topGainers = performances[:n]

		if len(performances) >= n {
			topLosers = performances[len(performances)-n:]
			// 反转顺序
			for i, j := 0, len(topLosers)-1; i < j; i, j = i+1, j-1 {
				topLosers[i], topLosers[j] = topLosers[j], topLosers[i]
			}
		}
	}

	return topGainers, topLosers
}

// identifyMarketEvents 识别市场事件
func identifyMarketEvents(dailyStats []DailyStats, startDate, endDate time.Time) []MarketEvent {
	events := []MarketEvent{}

	// 检查十月一假期效应
	octFirstStart := time.Date(time.Now().Year(), 10, 1, 0, 0, 0, 0, time.Local)
	octFirstEnd := time.Date(time.Now().Year(), 10, 7, 23, 59, 59, 0, time.Local)

	if (startDate.Before(octFirstEnd) && endDate.After(octFirstStart)) {
		events = append(events, MarketEvent{
			Date:        octFirstStart,
			EventType:   "holiday",
			Description: "十月一黄金周假期，市场流动性增加",
			Impact:      "high",
		})
	}

	// 检测价格剧烈波动
	for i, stats := range dailyStats {
		if i == 0 {
			continue
		}

		// 单日涨幅超过5%
		if stats.PriceChange > 5 {
			events = append(events, MarketEvent{
				Date:        stats.Date,
				EventType:   "spike",
				Description: fmt.Sprintf("市场单日上涨%.2f%%", stats.PriceChange),
				Impact:      "high",
			})
		}

		// 单日跌幅超过5%
		if stats.PriceChange < -5 {
			events = append(events, MarketEvent{
				Date:        stats.Date,
				EventType:   "crash",
				Description: fmt.Sprintf("市场单日下跌%.2f%%", stats.PriceChange),
				Impact:      "high",
			})
		}

		// 成交量激增（超过平均2倍）
		if i > 10 && stats.VolumeChange > 100 {
			events = append(events, MarketEvent{
				Date:        stats.Date,
				EventType:   "surge",
				Description: fmt.Sprintf("成交量激增%.2f%%", stats.VolumeChange),
				Impact:      "medium",
			})
		}
	}

	return events
}

// compareWithHistory 与历史时期对比
func compareWithHistory(db *gorm.DB, currentStats []DailyStats, days int) []HistoricalComparison {
	comparisons := []HistoricalComparison{}

	if len(currentStats) == 0 {
		return comparisons
	}

	// 当前时期的特征
	_, _ = analyzeTrend(currentStats)
	currentAvgChange := 0.0
	for _, stats := range currentStats {
		currentAvgChange += stats.PriceChange
	}
	if len(currentStats) > 0 {
		currentAvgChange /= float64(len(currentStats))
	}

	// 对比历史同期（去年同期）
	lastYearStart := time.Now().AddDate(-1, 0, -days)
	lastYearEnd := time.Now().AddDate(-1, 0, 0)

	lastYearSnaps, err := getSnapshots(db, lastYearStart, lastYearEnd)
	if err == nil && len(lastYearSnaps) > 0 {
		lastYearStats := calculateDailyStats(lastYearSnaps)
		lastYearTrend, _ := analyzeTrend(lastYearStats)

		lastYearAvgChange := 0.0
		for _, stats := range lastYearStats {
			lastYearAvgChange += stats.PriceChange
		}
		if len(lastYearStats) > 0 {
			lastYearAvgChange /= float64(len(lastYearStats))
		}

		// 计算相似度
		similarity := 100 - math.Abs(currentAvgChange-lastYearAvgChange)*10
		if similarity < 0 {
			similarity = 0
		}

		comparisons = append(comparisons, HistoricalComparison{
			Period:       "去年同期",
			StartDate:    lastYearStart.Format("2006-01-02"),
			EndDate:      lastYearEnd.Format("2006-01-02"),
			Similarity:   similarity,
			TrendPattern: lastYearTrend,
			AvgChange:    lastYearAvgChange,
		})
	}

	// 对比6个月前
	sixMonthsStart := time.Now().AddDate(0, -6, -days)
	sixMonthsEnd := time.Now().AddDate(0, -6, 0)

	sixMonthsSnaps, err := getSnapshots(db, sixMonthsStart, sixMonthsEnd)
	if err == nil && len(sixMonthsSnaps) > 0 {
		sixMonthsStats := calculateDailyStats(sixMonthsSnaps)
		sixMonthsTrend, _ := analyzeTrend(sixMonthsStats)

		sixMonthsAvgChange := 0.0
		for _, stats := range sixMonthsStats {
			sixMonthsAvgChange += stats.PriceChange
		}
		if len(sixMonthsStats) > 0 {
			sixMonthsAvgChange /= float64(len(sixMonthsStats))
		}

		similarity := 100 - math.Abs(currentAvgChange-sixMonthsAvgChange)*10
		if similarity < 0 {
			similarity = 0
		}

		comparisons = append(comparisons, HistoricalComparison{
			Period:       "6个月前",
			StartDate:    sixMonthsStart.Format("2006-01-02"),
			EndDate:      sixMonthsEnd.Format("2006-01-02"),
			Similarity:   similarity,
			TrendPattern: sixMonthsTrend,
			AvgChange:    sixMonthsAvgChange,
		})
	}

	return comparisons
}

// generateInsights 生成洞察
func generateInsights(analysis *MarketAnalysis) []string {
	insights := []string{}

	// 趋势洞察
	switch analysis.OverallTrend {
	case "up":
		insights = append(insights, fmt.Sprintf("市场整体呈上涨趋势，趋势强度%.1f%%", analysis.TrendStrength))
		if analysis.TrendStrength > 50 {
			insights = append(insights, "当前是强势上涨行情，建议关注高涨幅品种")
		}
	case "down":
		insights = append(insights, fmt.Sprintf("市场整体呈下跌趋势，趋势强度%.1f%%", analysis.TrendStrength))
		if analysis.TrendStrength > 50 {
			insights = append(insights, "当前是弱势下跌行情，建议谨慎入场")
		}
	case "stable":
		insights = append(insights, "市场整体保持稳定，横盘震荡")
	}

	// 价格变化洞察
	if analysis.PriceChange.ItemsUp > analysis.PriceChange.ItemsDown {
		ratio := float64(analysis.PriceChange.ItemsUp) / float64(analysis.PriceChange.ItemsDown)
		insights = append(insights, fmt.Sprintf("上涨品种数量是下跌品种的%.1f倍，市场情绪偏多", ratio))
	}

	// 波动率洞察
	if analysis.PriceChange.VolatilityIndex > 5 {
		insights = append(insights, fmt.Sprintf("市场波动率较高(%.2f%%)，存在较多交易机会但风险也较大", analysis.PriceChange.VolatilityIndex))
	} else {
		insights = append(insights, fmt.Sprintf("市场波动率较低(%.2f%%)，价格相对稳定", analysis.PriceChange.VolatilityIndex))
	}

	// 成交量洞察
	if analysis.VolumeStats.VolumeGrowth > 20 {
		insights = append(insights, fmt.Sprintf("成交量环比增长%.1f%%，市场活跃度提升", analysis.VolumeStats.VolumeGrowth))
		insights = append(insights, "可能的原因：节假日效应、新玩家入场、大额资本流入")
	} else if analysis.VolumeStats.VolumeGrowth < -20 {
		insights = append(insights, fmt.Sprintf("成交量环比下降%.1f%%，市场观望情绪浓厚", math.Abs(analysis.VolumeStats.VolumeGrowth)))
	}

	// 事件洞察
	for _, event := range analysis.RecentEvents {
		if event.EventType == "holiday" {
			insights = append(insights, "十月一黄金周期间，国内玩家活跃度提升，带动市场交易量增加")
			insights = append(insights, "假期效应通常会持续1-2周，随后市场会逐步回归常态")
		}
	}

	// 历史对比洞察
	for _, comp := range analysis.HistoryCompare {
		if comp.Similarity > 70 {
			insights = append(insights, fmt.Sprintf("当前走势与%s相似度%.1f%%，可参考当时的市场表现", comp.Period, comp.Similarity))
		}
	}

	// 涨幅榜洞察
	if len(analysis.TopGainers) > 0 {
		top := analysis.TopGainers[0]
		insights = append(insights, fmt.Sprintf("涨幅榜第一名：%s，涨幅%.2f%%", top.GoodName, top.ChangePercent))
	}

	return insights
}

// countUniqueGoods 统计唯一商品数量
func countUniqueGoods(snapshots []models.CSQAQGoodSnapshot) int {
	goodsSet := make(map[int64]bool)
	for _, snap := range snapshots {
		goodsSet[snap.GoodID] = true
	}
	return len(goodsSet)
}

// printAnalysisSummary 打印分析摘要
func printAnalysisSummary(analysis *MarketAnalysis) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("市场分析报告")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("分析日期: %s\n", analysis.AnalysisDate.Format("2006-01-02 15:04:05"))
	fmt.Printf("分析周期: %s\n", analysis.Period)
	fmt.Printf("商品数量: %d\n", analysis.TotalItems)
	fmt.Println()

	// 整体趋势
	fmt.Println("【整体趋势】")
	trendChinese := map[string]string{"up": "上涨", "down": "下跌", "stable": "稳定"}
	fmt.Printf("  趋势方向: %s\n", trendChinese[analysis.OverallTrend])
	fmt.Printf("  趋势强度: %.2f%%\n", analysis.TrendStrength)
	fmt.Println()

	// 价格变化
	fmt.Println("【价格变化统计】")
	fmt.Printf("  平均涨跌幅: %.2f%%\n", analysis.PriceChange.AvgChangePercent)
	fmt.Printf("  中位数涨跌幅: %.2f%%\n", analysis.PriceChange.MedianChange)
	fmt.Printf("  市场波动率: %.2f%%\n", analysis.PriceChange.VolatilityIndex)
	fmt.Printf("  上涨品种: %d  下跌品种: %d  稳定品种: %d\n",
		analysis.PriceChange.ItemsUp, analysis.PriceChange.ItemsDown, analysis.PriceChange.ItemsStable)
	fmt.Println()

	// 成交量统计
	fmt.Println("【成交量统计】")
	fmt.Printf("  日均成交量: %.0f\n", analysis.VolumeStats.AvgDailyVolume)
	fmt.Printf("  成交量增长: %.2f%%\n", analysis.VolumeStats.VolumeGrowth)
	fmt.Println()

	// 涨幅榜
	fmt.Println("【涨幅榜 TOP 5】")
	n := 5
	if len(analysis.TopGainers) < n {
		n = len(analysis.TopGainers)
	}
	for i := 0; i < n; i++ {
		g := analysis.TopGainers[i]
		fmt.Printf("  %d. %s  %.2f%% (%.2f → %.2f)\n",
			i+1, g.GoodName, g.ChangePercent, g.PriceStart, g.PriceEnd)
	}
	fmt.Println()

	// 跌幅榜
	fmt.Println("【跌幅榜 TOP 5】")
	if len(analysis.TopLosers) < n {
		n = len(analysis.TopLosers)
	}
	for i := 0; i < n; i++ {
		l := analysis.TopLosers[i]
		fmt.Printf("  %d. %s  %.2f%% (%.2f → %.2f)\n",
			i+1, l.GoodName, l.ChangePercent, l.PriceStart, l.PriceEnd)
	}
	fmt.Println()

	// 市场事件
	if len(analysis.RecentEvents) > 0 {
		fmt.Println("【重要市场事件】")
		for _, event := range analysis.RecentEvents {
			fmt.Printf("  [%s] %s - %s\n",
				event.Date.Format("01-02"), event.Description, event.Impact)
		}
		fmt.Println()
	}

	// 历史对比
	if len(analysis.HistoryCompare) > 0 {
		fmt.Println("【历史对比】")
		for _, comp := range analysis.HistoryCompare {
			fmt.Printf("  %s (%s ~ %s)\n", comp.Period, comp.StartDate, comp.EndDate)
			fmt.Printf("    相似度: %.1f%%  趋势: %s  平均涨跌: %.2f%%\n",
				comp.Similarity, comp.TrendPattern, comp.AvgChange)
		}
		fmt.Println()
	}

	// 市场洞察
	if len(analysis.Insights) > 0 {
		fmt.Println("【市场洞察】")
		for i, insight := range analysis.Insights {
			fmt.Printf("  %d. %s\n", i+1, insight)
		}
		fmt.Println()
	}

	fmt.Println(strings.Repeat("=", 80))
}

// printAnalysisVerbose 打印详细分析
func printAnalysisVerbose(analysis *MarketAnalysis) {
	printAnalysisSummary(analysis)

	// 完整涨幅榜
	if len(analysis.TopGainers) > 5 {
		fmt.Println("\n【完整涨幅榜】")
		for i, g := range analysis.TopGainers {
			fmt.Printf("  %d. %s  %.2f%% (%.2f → %.2f)\n",
				i+1, g.GoodName, g.ChangePercent, g.PriceStart, g.PriceEnd)
		}
	}

	// 完整跌幅榜
	if len(analysis.TopLosers) > 5 {
		fmt.Println("\n【完整跌幅榜】")
		for i, l := range analysis.TopLosers {
			fmt.Printf("  %d. %s  %.2f%% (%.2f → %.2f)\n",
				i+1, l.GoodName, l.ChangePercent, l.PriceStart, l.PriceEnd)
		}
	}
}

// saveAnalysisToFile 保存分析结果到文件
func saveAnalysisToFile(analysis *MarketAnalysis, filename string) error {
	data, err := json.MarshalIndent(analysis, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}
