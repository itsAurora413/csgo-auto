package main

import (
	"context"
	"csgo-trader/internal/database"
	"csgo-trader/internal/models"
	"csgo-trader/internal/services"
	"csgo-trader/internal/services/youpin"
	"flag"
	"fmt"
	"log"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"encoding/json"
	"os"
	"sync"
	"sync/atomic"

	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

var (
	minProfitRate      = flag.Float64("min-profit", 0.08, "最小利润率 (默认 8%，提高利润要求)")
	minDaysHistory     = flag.Int("min-days", 7, "最少历史天数（默认 3天，没有足够数据时按当前价差判断）")
	budget             = flag.Float64("budget", 2000, "求购总预算（默认 2000元，可自定义）")
	minSellCount       = flag.Int("min-sell-count", 90, "最少在售数量（默认 50件，阶段0已过滤>=100，这里放宽以不重复过滤）")
	minBuyCount        = flag.Int("min-buy-count", 5, "最少求购数量（默认 10件，放宽标准）")
	maxReasonablePrice = flag.Float64("max-price", 300, "最高合理价格（默认 300元，过滤异常价格）")
	maxPriceSpread     = flag.Float64("max-spread", 2.0, "最大价差倍数（默认 2.0倍，售价不超过求购价的2倍）")
	minPrice           = flag.Float64("min-price", 2, "最低价格（默认2元，低于此价格视为垃圾商品）")
	maxQuantityPerItem = flag.Int("max-qty", 2, "每种饰品最多购买数量（默认2件，增加多样性）")
	once               = flag.Bool("once", false, "只运行一次，不循环")
	dbURL              = flag.String("db", "", "数据库连接字符串")
	backtest           = flag.Bool("backtest", false, "回测模式：使用7天前的预测对比实际收益")
	backtestDays       = flag.Int("backtest-days", 7, "回测天数（默认7天）")
	ypTimeoutSec       = flag.Int("yp-timeout", 20, "YouPin接口调用超时(秒)，默认20s")
	concurrency        = flag.Int("concurrency", 10, "并发线程数（默认10，用于加速商品分析）")
	autoPurchase       = flag.Bool("auto-purchase", false, "验证通过后自动实时下单求购（默认关闭）")
	onlyBottomRebound  = flag.Bool("only-bottom", false, "只看能抄底的饰品：前期下跌+当前反弹（默认关闭，关闭时为全量分析）")
	// ===== 新增：反弹幅度控制参数（追稳而非追涨）=====
	minRebound           = flag.Float64("min-rebound", 0.50, "反弹恢复率下限（默认50%：必须恢复跌幅的50%才认为有效反弹，从30%提升）")
	maxRebound           = flag.Float64("max-rebound", 0.80, "反弹恢复率上限（默认80%：反弹不能超过跌幅的80%，防止追涨）")
	maxAbsoluteRebound1d = flag.Float64("max-rebound-1d", 0.05, "单日反弹幅度上限（默认5%：一天内反弹不超过5%，防止高位接盘）")
	minAbsoluteRebound   = flag.Float64("min-rebound-abs", 0.03, "反弹绝对幅度下限（默认3%：最少要反弹3%，从2%提升）")

	proxyURL  = flag.String("proxy-url", "hk.novproxy.io:1000", "代理服务器地址")
	proxyUser = flag.String("proxy-user", "qg3e2819-region-US", "代理用户名")
	proxyPass = flag.String("proxy-pass", "mahey33h", "代理密码")
)

// BlacklistCache 黑名单缓存（template_id -> 商品名称）
var blacklistCache map[int64]string
var blacklistLock sync.RWMutex

// loadBlacklist 从 Excel 文件加载黑名单
func loadBlacklist(filepath string) (map[int64]string, error) {
	blacklistCache = make(map[int64]string)

	// 检查文件是否存在
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		log.Printf("[黑名单] 黑名单文件不存在: %s，跳过加载", filepath)
		return blacklistCache, nil
	}

	f, err := excelize.OpenFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("打开黑名单文件失败: %w", err)
	}
	defer f.Close()

	// 获取第一个 Sheet
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return blacklistCache, nil
	}

	sheetName := sheets[0]
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("读取黑名单 Sheet 失败: %w", err)
	}

	// 跳过表头，从第2行开始读取
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if len(row) < 2 {
			continue
		}

		// 第1列是 id，第2列是 template_id，第4列是 template_name
		templateIDStr := row[1]
		var templateName string
		if len(row) > 3 {
			templateName = row[3]
		}

		// 将 template_id 转换为 int64
		if templateIDStr == "" {
			continue
		}
		templateID, err := strconv.ParseInt(templateIDStr, 10, 64)
		if err != nil {
			continue
		}

		blacklistCache[templateID] = templateName
	}

	log.Printf("[黑名单] 成功加载 %d 条黑名单记录", len(blacklistCache))
	return blacklistCache, nil
}

// isBlacklisted 检查商品是否在黑名单中
func isBlacklisted(goodID int64, db *gorm.DB) bool {
	blacklistLock.RLock()
	defer blacklistLock.RUnlock()

	// 如果黑名单为空，从数据库快照获取 template_id
	if len(blacklistCache) == 0 {
		return false
	}

	// 从 CSQAQGoodSnapshot 获取该商品的 template_id
	var snapshot models.CSQAQGoodSnapshot
	if err := db.Where("good_id = ?", goodID).Order("created_at DESC").First(&snapshot).Error; err == nil && snapshot.YYYPTemplateID != nil {
		_, exists := blacklistCache[*snapshot.YYYPTemplateID]
		return exists
	}

	return false
}

// isMainWeapon 判断是否是主战武器（热门武器）
func isMainWeapon(name string) bool {
	mainWeapons := []string{
		"AK-47",
		"M4A4",
		"M4A1",
		"M4A1-S", // 消音M4的全称
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
		return 5.0 // 崭新最好，保值率高
	} else if strings.Contains(name, "略有磨损") || strings.Contains(name, "Minimal Wear") {
		return 4.0 // 略磨次之
	} else if strings.Contains(name, "久经沙场") || strings.Contains(name, "Field-Tested") {
		return 3.0 // 久经居中
	} else if strings.Contains(name, "破损不堪") || strings.Contains(name, "Well-Worn") {
		return 2.0 // 破损较差
	} else if strings.Contains(name, "战痕累累") || strings.Contains(name, "Battle-Scarred") {
		return 1.0 // 战痕最差
	}
	return 2.5 // 默认中等（无磨损标识的物品）
}

// calculateVolatility 计算价格波动率（类似金融市场的标准差）
func calculateVolatility(prices []float64) float64 {
	if len(prices) < 2 {
		return 0.0
	}

	// 计算均值
	sum := 0.0
	for _, p := range prices {
		sum += p
	}
	mean := sum / float64(len(prices))

	// 计算标准差
	variance := 0.0
	for _, p := range prices {
		diff := p - mean
		variance += diff * diff
	}
	variance /= float64(len(prices))

	// 返回变异系数（CV = 标准差/均值），标准化的波动率
	if mean > 0 {
		stdDev := variance
		for i := 0; i < 10; i++ { // 简单的开方近似
			stdDev = (stdDev + variance/stdDev) / 2
		}
		return stdDev / mean
	}
	return 0.0
}

// calculateSharpeRatio 计算类似夏普比率的指标（收益/风险比）
func calculateSharpeRatio(profitRate float64, volatility float64) float64 {
	if volatility == 0 {
		return profitRate * 10 // 无波动的情况给高分
	}
	// 夏普比率 = 收益率 / 波动率
	sharpe := profitRate / volatility
	// 归一化到合理范围
	if sharpe > 5.0 {
		return 5.0
	}
	return sharpe
}

// MarketCycleStage 市场周期阶段
type MarketCycleStage struct {
	Stage               string  // 阶段: bottom_area(底部区域), rising(上涨), top_area(顶部区域), falling(下跌)
	Score               float64 // 周期得分(0-10分，分数越高越适合买入)
	Recommendation      string  // 建议: strong_buy(强烈买入), buy(买入), hold(观望), sell(卖出)
	PricePosition       float64 // 价格位置：当前价格相对7日均价的百分比偏离
	TrendStrength       string  // 趋势强度: strong(强), moderate(中), weak(弱)
	ReversalProbability string  // 反转概率: high(高), medium(中), low(低)
	Description         string  // 描述信息
}

// analyzeMarketCycle 分析市场周期阶段（针对短期7-14天持有策略）
func analyzeMarketCycle(currentPrice float64, avgPrice7d float64, priceTrend string, volatility float64, historicalPrices []float64) MarketCycleStage {
	// 计算价格偏离度
	priceDeviation := 0.0
	if avgPrice7d > 0 {
		priceDeviation = (currentPrice - avgPrice7d) / avgPrice7d
	}

	stage := MarketCycleStage{
		PricePosition: priceDeviation * 100,
	}

	// 判断趋势强度（基于R²和波动率）
	if volatility < 0.05 {
		stage.TrendStrength = "strong" // 低波动，趋势清晰
	} else if volatility < 0.12 {
		stage.TrendStrength = "moderate"
	} else {
		stage.TrendStrength = "weak" // 高波动，趋势不稳定
	}

	// === 核心逻辑：识别周期阶段 ===

	// 1. 底部区域（最佳买入区）
	if priceDeviation <= -0.08 { // 低于均价8%以上
		stage.Stage = "bottom_area"
		stage.Score = 9.0
		stage.Recommendation = "strong_buy"
		stage.ReversalProbability = "high"
		stage.Description = "🟢 价格处于底部区域，强烈建议买入"

		// 如果是下跌趋势快要反转，加分
		if priceTrend == "down" {
			stage.Score = 10.0
			stage.Description = "🟢🟢 底部区域+下跌趋势末期，绝佳买入机会"
		}

		// 2. 接近底部（好的买入区）
	} else if priceDeviation <= -0.03 { // 低于均价3-8%
		stage.Stage = "bottom_area"
		stage.Score = 7.5
		stage.Recommendation = "buy"
		stage.ReversalProbability = "medium"
		stage.Description = "🟢 价格接近底部，建议买入"

		if priceTrend == "stable" || priceTrend == "up" {
			stage.Score = 8.0
			stage.Description = "🟢 价格回调到位，趋势转正，适合买入"
		}

		// 3. 上涨初期（可以买入）
	} else if priceDeviation <= 0.03 && priceTrend == "up" { // 均价附近且上涨
		stage.Stage = "rising"
		stage.Score = 6.5
		stage.Recommendation = "buy"
		stage.ReversalProbability = "low"
		stage.Description = "🟡 上涨初期，可以买入"

		if stage.TrendStrength == "strong" {
			stage.Score = 7.0
			stage.Description = "🟡 强势上涨初期，可以买入"
		}

		// 4. 上涨中期（观望）
	} else if priceDeviation <= 0.06 && priceTrend == "up" { // 高于均价3-6%且上涨
		stage.Stage = "rising"
		stage.Score = 5.0
		stage.Recommendation = "hold"
		stage.ReversalProbability = "medium"
		stage.Description = "🟡 上涨中期，建议观望"

		// 5. 顶部区域（不建议买入）
	} else if priceDeviation > 0.06 { // 高于均价6%以上
		stage.Stage = "top_area"
		stage.Score = 2.0
		stage.Recommendation = "hold"
		stage.ReversalProbability = "high"
		stage.Description = "🔴 价格处于顶部区域，可能回调，不建议买入"

		if priceDeviation > 0.10 { // 高于均价10%以上
			stage.Score = 1.0
			stage.Description = "🔴🔴 价格严重高估，极可能回调，强烈不建议买入"
		}

		if priceTrend == "up" {
			stage.Description = "🔴 价格冲高，小心见顶回落"
		}

		// 6. 下跌阶段（观望）
	} else if priceTrend == "down" {
		stage.Stage = "falling"
		stage.Score = 4.0
		stage.Recommendation = "hold"
		stage.ReversalProbability = "medium"
		stage.Description = "🟡 下跌阶段，等待企稳"

		// 检查是否接近历史低点（抄底机会）
		if len(historicalPrices) >= 3 {
			minPrice := historicalPrices[0]
			for _, p := range historicalPrices {
				if p < minPrice {
					minPrice = p
				}
			}
			if currentPrice <= minPrice*1.05 { // 接近历史最低价5%以内
				stage.Score = 7.0
				stage.Recommendation = "buy"
				stage.Description = "🟢 接近历史低点，可考虑抄底"
			}
		}

		// 7. 震荡区间（稳定）
	} else {
		stage.Stage = "stable"
		stage.Score = 5.5
		stage.Recommendation = "hold"
		stage.ReversalProbability = "low"
		stage.Description = "🟡 价格震荡，可观望或小仓位试探"
	}

	// === 针对短期持有策略的特别调整 ===

	// 如果波动率过高，降低所有得分（风险太大）
	if volatility > 0.15 {
		stage.Score *= 0.7
		stage.Description += " | ⚠️高波动警告"
	}

	// 如果是7-14天周期，优先选择上涨初期和底部区域
	if stage.Stage == "rising" && priceDeviation < 0.02 {
		stage.Score += 0.5 // 小幅加分
	}

	return stage
}

// calculateTrendByLinearRegression 使用线性回归计算价格趋势
// 返回: 趋势方向(up/down/stable), 斜率, R²相关系数
func calculateTrendByLinearRegression(prices []float64) (string, float64, float64) {
	n := len(prices)
	if n < 3 {
		return "unknown", 0.0, 0.0
	}

	// 构建时间序列 x = [0, 1, 2, ..., n-1]
	// y = prices (从旧到新)

	// 计算均值
	sumX := 0.0
	sumY := 0.0
	for i := 0; i < n; i++ {
		sumX += float64(i)
		sumY += prices[i]
	}
	meanX := sumX / float64(n)
	meanY := sumY / float64(n)

	// 计算斜率和截距
	numerator := 0.0   // Σ(xi - x̄)(yi - ȳ)
	denominator := 0.0 // Σ(xi - x̄)²
	for i := 0; i < n; i++ {
		xi := float64(i)
		yi := prices[i]
		numerator += (xi - meanX) * (yi - meanY)
		denominator += (xi - meanX) * (xi - meanX)
	}

	if denominator == 0 {
		return "stable", 0.0, 0.0
	}

	slope := numerator / denominator // 斜率 β
	// intercept := meanY - slope*meanX // 截距 α (暂时不需要)

	// 计算 R² (决定系数，衡量拟合优度)
	ssTotal := 0.0 // 总平方和
	ssRes := 0.0   // 残差平方和
	for i := 0; i < n; i++ {
		yi := prices[i]
		yPred := slope*float64(i) + (meanY - slope*meanX)
		ssTotal += (yi - meanY) * (yi - meanY)
		ssRes += (yi - yPred) * (yi - yPred)
	}

	rSquared := 0.0
	if ssTotal > 0 {
		rSquared = 1 - (ssRes / ssTotal)
	}

	// 判断趋势：结合斜率大小和拟合度
	slopePercent := (slope / meanY) * 100 // 每个时间单位的变化百分比

	trend := "stable"

	// R² < 0.3 说明线性关系不明显，趋势不可靠
	if rSquared < 0.3 {
		// 数据拟合度差时，使用最高价和最低价来判断趋势
		minPrice := prices[0]
		maxPrice := prices[0]
		for _, p := range prices {
			if p < minPrice {
				minPrice = p
			}
			if p > maxPrice {
				maxPrice = p
			}
		}

		// 最近的价格（最后一个）
		recentPrice := prices[len(prices)-1]
		priceChangePercent := ((recentPrice - maxPrice) / maxPrice) * 100

		// 如果最近价格相对高点下跌超过1%，认为是下跌趋势
		if priceChangePercent < -1.0 {
			trend = "down"
		} else if priceChangePercent > 1.0 {
			trend = "up"
		} else {
			trend = "stable"
		}
	} else if slopePercent > 1.0 { // 每个时间单位涨超过1%
		trend = "up"
	} else if slopePercent < -1.0 { // 每个时间单位跌超过1%
		trend = "down"
	} else {
		trend = "stable"
	}

	return trend, slope, rSquared
}

// analyzeTrendWithBothPrices 综合分析求购价和售价的趋势
func analyzeTrendWithBothPrices(buyPrices []float64, sellPrices []float64) (string, string) {
	buyTrend, _, buyR2 := calculateTrendByLinearRegression(buyPrices)
	sellTrend, _, sellR2 := calculateTrendByLinearRegression(sellPrices)

	// 详细信息用于调试
	trendDetail := ""
	if len(buyPrices) >= 3 {
		trendDetail += "求购趋势:" + buyTrend
		if buyR2 >= 0.3 {
			trendDetail += "(可靠)"
		}
	}
	if len(sellPrices) >= 3 {
		if trendDetail != "" {
			trendDetail += " | "
		}
		trendDetail += "售价趋势:" + sellTrend
		if sellR2 >= 0.3 {
			trendDetail += "(可靠)"
		}
	}

	// 综合判断：两者都上涨才算上涨，两者都下跌才算下跌
	finalTrend := "stable"

	// 优先看售价趋势（因为卖出时参考售价）
	if sellR2 >= 0.3 && len(sellPrices) >= 3 {
		if sellTrend == "up" && buyTrend != "down" {
			finalTrend = "up"
		} else if sellTrend == "down" && buyTrend != "up" {
			finalTrend = "down"
		} else if sellTrend == buyTrend { // 两个趋势一致
			finalTrend = sellTrend
		}
	} else if buyR2 >= 0.3 && len(buyPrices) >= 3 {
		// 售价数据不可靠时，参考求购价
		finalTrend = buyTrend
	}

	return finalTrend, trendDetail
}

// calculateScore 计算套利机会的综合评分（金融量化模型 + 市场周期分析）
// 注意：需要传入完整的历史价格数据才能计算市场周期，否则市场周期得分为0
func calculateScore(opp models.ArbitrageOpportunity) float64 {
	score := 0.0

	// === 1. 武器类型加成（权重7%）===
	// 主战武器（AK、M4、AWP等）优先级更高，类似蓝筹股
	weaponBonus := 0.0
	if isMainWeapon(opp.GoodName) {
		weaponBonus = 7.0 // 主战武器（蓝筹股）
	} else {
		weaponBonus = 2.0 // 其他武器（小盘股）
	}
	score += weaponBonus

	// === 2. 磨损度评分（权重12.5%）===
	// 崭新出厂保值率最高，类似优质资产
	wearScore := getWearScore(opp.GoodName)
	score += wearScore * 2.5 // 最高12.5分（5.0 * 2.5）

	// === 2.1 破损/战痕主战武器惩罚（流动性和保值率差）===
	if wearScore <= 2.0 && isMainWeapon(opp.GoodName) {
		score *= 0.85 // 破损或战痕的主战武器降低15%
	}

	// === 3. 收益率评分（权重25%）===
	// 类似ROI，但要考虑风险调整后的收益
	profitScore := 0.0
	if opp.ProfitRate >= 0.20 {
		profitScore = 25.0 // 超高收益
	} else if opp.ProfitRate >= 0.15 {
		profitScore = 22.0
	} else if opp.ProfitRate >= 0.10 {
		profitScore = 19.0
	} else if opp.ProfitRate >= 0.08 {
		profitScore = 16.0
	} else {
		profitScore = opp.ProfitRate * 180 // 线性评分
	}
	score += profitScore

	// === 4. 风险评分（权重15%）===
	// 类似贝塔系数，低风险=低波动
	riskScore := 0.0
	switch opp.RiskLevel {
	case "low":
		riskScore = 15.0 // 低风险高分
	case "medium":
		riskScore = 9.0
	case "high":
		riskScore = 1.5 // 高风险低分（从3.0降至1.5，进一步降低高风险权重）
	}
	score += riskScore

	// === 5. 流动性评分（权重16%）===
	// 类似成交量指标，流动性越高越容易变现
	liquidityScore := 0.0

	// 买卖比率（Bid-Ask Ratio）- 9%
	bidAskRatio := float64(opp.BuyOrderCount) / float64(opp.SellOrderCount+1)
	if bidAskRatio > 0.8 { // 求购数接近在售数，市场活跃
		liquidityScore += 9.0
	} else if bidAskRatio > 0.5 {
		liquidityScore += 6.5
	} else if bidAskRatio > 0.3 {
		liquidityScore += 4.5
	} else {
		liquidityScore += bidAskRatio * 12
	}

	// 总成交量评分（Market Depth）- 7%
	totalVolume := opp.BuyOrderCount + opp.SellOrderCount
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
	// 类似技术分析的趋势指标
	trendScore := 0.0
	switch opp.PriceTrend {
	case "up":
		trendScore = 7.0 // 上涨趋势最好（顺势而为）
	case "stable":
		trendScore = 5.0 // 稳定次之
	case "down":
		trendScore = 1.0 // 下跌趋势风险大（逆势操作）
	default:
		trendScore = 3.5
	}
	score += trendScore

	// === 7. 历史数据可靠性（权重5%）===
	// 样本量越大，统计意义越强
	dataScore := 0.0
	if opp.DaysOfData >= 7 {
		dataScore = 5.0
	} else if opp.DaysOfData >= 5 {
		dataScore = 4.0
	} else if opp.DaysOfData >= 3 {
		dataScore = 2.5
	} else {
		dataScore = float64(opp.DaysOfData) * 0.7
	}
	score += dataScore

	// === 8. 价值投资指标（权重3%）===
	// 绝对利润潜力：高价饰品即使利润率低，绝对利润也可观
	absoluteProfit := opp.EstimatedProfit * float64(1) // 单件绝对利润
	if absoluteProfit >= 50 {                          // 单件利润≥50元
		score += 3.0
	} else if absoluteProfit >= 20 { // 单件利润≥20元
		score += 2.0
	} else if absoluteProfit >= 10 { // 单件利润≥10元
		score += 1.0
	}

	// === 8.5 热度评分（权重5%）⭐新增⭐===
	// 热度排名越低（数字越小）说明越热门，评分越高
	// 热度排名反映市场关注度：排名1-10（超热）、11-50（热门）、51-100（中等）、100以上（冷门）
	heatScore := 0.0
	if opp.RankNum != nil && *opp.RankNum > 0 {
		rankNum := *opp.RankNum
		if rankNum <= 500 {
			heatScore = 5.0 // 排名1-10：超热门商品，最高分
		} else if rankNum <= 800 {
			heatScore = 4.5 // 排名11-30：很热门
		} else if rankNum <= 1000 {
			heatScore = 4.0 // 排名31-50：热门
		} else if rankNum <= 3000 {
			heatScore = 3.5 // 排名51-100：中等热度
		} else if rankNum <= 5000 {
			heatScore = 3.0 // 排名101-200：中等热度
		} else if rankNum <= 10000 {
			heatScore = 2.5 // 排名201-500：中等热度
		} else {
			heatScore = 0.5 // 排名200以上：冷门商品
		}
	} else {
		// 无热度数据时，给予中等分数
		heatScore = 2.5
	}
	score += heatScore

	// === 9. 市场周期评分（权重12%）⭐新增⭐===
	// 短期持有策略的核心：在底部买入，上涨初期买入
	// 注意：这里无法获取历史价格数组，所以基于已有字段估算
	// 实际周期分析在生成opportunity时已完成

	// 简化版周期评分（基于价格偏离和趋势）
	cycleScore := 0.0
	avgPrice := (opp.AvgBuyPrice7d + opp.AvgSellPrice7d) / 2.0
	if avgPrice > 0 {
		priceDeviation := (opp.CurrentBuyPrice - avgPrice) / avgPrice

		// 底部区域（低于均价5%以上）
		if priceDeviation <= -0.05 {
			cycleScore = 12.0 // 满分
			if opp.PriceTrend == "down" {
				cycleScore = 12.0 // 下跌末期，保持满分
			} else if opp.PriceTrend == "up" || opp.PriceTrend == "stable" {
				cycleScore = 11.0 // 触底反弹
			}
			// 接近底部（低于均价2-5%）
		} else if priceDeviation <= -0.02 {
			cycleScore = 9.5
			// 上涨初期（均价附近且上涨）
		} else if priceDeviation <= 0.02 && opp.PriceTrend == "up" {
			cycleScore = 8.0
			// 上涨中期（高于均价2-5%且上涨）
		} else if priceDeviation <= 0.05 && opp.PriceTrend == "up" {
			cycleScore = 5.0
			// 顶部区域（高于均价5%以上）
		} else if priceDeviation > 0.05 {
			cycleScore = 2.0 // 低分
			if priceDeviation > 0.10 {
				cycleScore = 0.5 // 严重高估
			}
			// 震荡或其他
		} else {
			cycleScore = 6.0
		}
	} else {
		// 没有历史均价时，给中等分数
		cycleScore = 6.0
	}

	score += cycleScore

	return score
}

// PurchaseItem 购买项
type PurchaseItem struct {
	GoodID   int64
	GoodName string
	Quantity int
	Price    float64
	Total    float64
	Profit   float64 // 预期利润
}

// PurchasePlan 购买方案
type PurchasePlan struct {
	Items        []PurchaseItem
	TotalCost    float64
	TotalProfit  float64
	TotalItems   int
	ProfitRate   float64
	StrategyName string
}

// generatePurchasePlan 根据给定的商品列表和预算生成购买方案
func generatePurchasePlan(opportunities []models.ArbitrageOpportunity, budget float64, strategyName string) PurchasePlan {
	plan := PurchasePlan{
		Items:        []PurchaseItem{},
		StrategyName: strategyName,
	}

	remainingBudget := budget

	for i := range opportunities {
		if remainingBudget <= 10 {
			break
		}

		opp := &opportunities[i]
		buyPrice := opp.RecommendedBuyPrice

		// 计算购买数量
		quantity := calculateOptimalQuantity(opp, remainingBudget, budget, i+1, len(opportunities))
		if quantity == 0 {
			continue
		}

		total := buyPrice * float64(quantity)
		if total > remainingBudget {
			continue
		}

		// 计算预期利润：(售价*0.99 - 买价) * 数量
		profit := (opp.CurrentSellPrice*0.99 - buyPrice) * float64(quantity)

		item := PurchaseItem{
			GoodID:   opp.GoodID,
			GoodName: opp.GoodName,
			Quantity: quantity,
			Price:    buyPrice,
			Total:    total,
			Profit:   profit,
		}

		plan.Items = append(plan.Items, item)
		plan.TotalCost += total
		plan.TotalProfit += profit
		plan.TotalItems += quantity
		remainingBudget -= total

		// 限制最多100种商品
		if len(plan.Items) >= 100 {
			break
		}
	}

	// 计算总体利润率
	if plan.TotalCost > 0 {
		plan.ProfitRate = plan.TotalProfit / plan.TotalCost
	}

	return plan
}

// calculateOptimalQuantity 计算最优购买数量（多样性优先策略）
// 购买数量策略：
// - 默认1个（最大多样性）
// - 只有在利润率特别高（>=18%）且低风险时才买2个
// - 严格限制最大数量为 maxQuantityPerItem
func calculateOptimalQuantity(opp *models.ArbitrageOpportunity, remainingBudget float64, totalBudget float64, currentRank int, totalOpportunities int) int {
	buyPrice := opp.RecommendedBuyPrice

	// === 基础逻辑：默认购买1个（多样性优先）===
	quantity := 1

	// === 检查预算是否允许 ===
	maxQuantity := int(remainingBudget / buyPrice)
	if maxQuantity == 0 {
		return 0
	}
	if maxQuantity < 1 {
		return 0
	}

	// === 下跌趋势直接返回1个（最重要的风险因素） ===
	if opp.PriceTrend == "down" {
		return 1
	}

	// === 高风险商品始终只买1个 ===
	if opp.RiskLevel == "high" {
		return 1
	}

	// === 判断是否应该买2个（非常严格的条件）===
	// 条件：利润率>=18% + 低风险 + 稳定或上涨趋势
	shouldBuyTwo := opp.ProfitRate >= 0.18 &&
		opp.RiskLevel == "low" &&
		(opp.PriceTrend == "up" || opp.PriceTrend == "stable") &&
		opp.DaysOfData >= 5

	if shouldBuyTwo && maxQuantity >= 2 {
		quantity = 2
	}

	// === 严格限制：不超过配置的最大数量 ===
	if quantity > *maxQuantityPerItem {
		quantity = *maxQuantityPerItem
	}

	// === 检查预算限制 ===
	if quantity > maxQuantity {
		quantity = maxQuantity
	}

	return quantity
}

// BacktestResult 回测结果
type BacktestResult struct {
	GoodID              int64
	GoodName            string
	PredictedBuyPrice   float64 // 预测的买入价
	PredictedSellPrice  float64 // 预测的卖出价
	PredictedProfit     float64 // 预测的利润
	PredictedProfitRate float64 // 预测的利润率
	ActualBuyPrice      float64 // 实际买入价（7天前）
	ActualSellPrice     float64 // 实际卖出价（今天）
	ActualProfit        float64 // 实际利润
	ActualProfitRate    float64 // 实际利润率
	PriceChangeRate     float64 // 价格变化率
	IsSuccessful        bool    // 是否成功（实际利润>0）
	ProfitAccuracy      float64 // 利润预测准确度（实际/预测）
	Quantity            int     // 推荐数量
}

// runBacktest 回测函数：验证N天前的预测准确度
func runBacktest(db *gorm.DB) {
	log.Printf("[回测分析] ==================== 开始回测分析 ====================")
	log.Printf("[回测分析] 回测天数: %d天", *backtestDays)

	// 计算N天前的时间范围
	targetDate := time.Now().AddDate(0, 0, -*backtestDays)
	// 找到当天的分析记录（允许±12小时误差）
	startTime := targetDate.Add(-12 * time.Hour)
	endTime := targetDate.Add(12 * time.Hour)

	log.Printf("[回测分析] 查询时间范围: %s ~ %s",
		startTime.Format("2006-01-02 15:04:05"),
		endTime.Format("2006-01-02 15:04:05"))

	// 1. 从历史归档表查询N天前的套利机会（只取推荐的商品，即有推荐数量的）
	var historicalOpportunities []models.ArbitrageOpportunity
	if err := db.Table("arbitrage_opportunities_history").
		Where("analysis_time >= ? AND analysis_time <= ? AND recommended_quantity > 0", startTime, endTime).
		Order("analysis_time DESC").
		Limit(50). // 只取前50个推荐
		Find(&historicalOpportunities).Error; err != nil {
		log.Printf("[回测分析] 查询历史数据失败: %v", err)
		return
	}

	if len(historicalOpportunities) == 0 {
		log.Printf("[回测分析] 未找到%d天前的推荐数据，可能当时未运行分析", *backtestDays)
		log.Printf("[回测分析] 提示: 请确保数据库中有至少%d天前的 arbitrage_opportunities 记录", *backtestDays)
		return
	}

	log.Printf("[回测分析] 找到 %d 条历史推荐记录", len(historicalOpportunities))
	actualAnalysisTime := historicalOpportunities[0].AnalysisTime
	log.Printf("[回测分析] 实际分析时间: %s", actualAnalysisTime.Format("2006-01-02 15:04:05"))

	// 2. 获取这些商品今天的最新价格
	goodIDs := []int64{}
	for _, opp := range historicalOpportunities {
		goodIDs = append(goodIDs, opp.GoodID)
	}

	// 查询今天的最新快照
	todaySnapshots := make(map[int64]*models.CSQAQGoodSnapshot)
	var snapshots []models.CSQAQGoodSnapshot
	if err := db.Where("good_id IN ?", goodIDs).
		Order("created_at DESC").
		Find(&snapshots).Error; err != nil {
		log.Printf("[回测分析] 查询今日价格失败: %v", err)
		return
	}

	// 按商品ID分组，取最新的一条
	for i := range snapshots {
		snapshot := &snapshots[i]
		if _, exists := todaySnapshots[snapshot.GoodID]; !exists {
			todaySnapshots[snapshot.GoodID] = snapshot
		}
	}

	log.Printf("[回测分析] 成功获取 %d 个商品的今日价格", len(todaySnapshots))

	// 3. 对比预测和实际结果
	results := []BacktestResult{}
	for _, histOpp := range historicalOpportunities {
		todaySnapshot, exists := todaySnapshots[histOpp.GoodID]
		if !exists || todaySnapshot.YYYPBuyPrice == nil || todaySnapshot.YYYPSellPrice == nil {
			continue // 跳过没有今日数据的商品
		}

		// 预测值（N天前的预测）
		predictedBuyPrice := histOpp.RecommendedBuyPrice
		predictedSellPrice := histOpp.CurrentSellPrice
		predictedProfit := (predictedSellPrice*0.99 - predictedBuyPrice) * float64(histOpp.RecommendedQuantity)
		predictedProfitRate := histOpp.ProfitRate

		// 实际值（按N天前的买入价，今天的卖出价计算）
		actualBuyPrice := histOpp.RecommendedBuyPrice // 实际买入价就是当时推荐的价格
		actualSellPrice := *todaySnapshot.YYYPSellPrice
		actualProfit := (actualSellPrice*0.99 - actualBuyPrice) * float64(histOpp.RecommendedQuantity)
		actualProfitRate := 0.0
		if actualBuyPrice > 0 {
			actualProfitRate = (actualSellPrice*0.99 - actualBuyPrice) / actualBuyPrice
		}

		// 价格变化率
		priceChangeRate := 0.0
		if predictedSellPrice > 0 {
			priceChangeRate = (actualSellPrice - predictedSellPrice) / predictedSellPrice
		}

		// 利润准确度
		profitAccuracy := 0.0
		if predictedProfit > 0 {
			profitAccuracy = actualProfit / predictedProfit
		}

		result := BacktestResult{
			GoodID:              histOpp.GoodID,
			GoodName:            histOpp.GoodName,
			PredictedBuyPrice:   predictedBuyPrice,
			PredictedSellPrice:  predictedSellPrice,
			PredictedProfit:     predictedProfit,
			PredictedProfitRate: predictedProfitRate,
			ActualBuyPrice:      actualBuyPrice,
			ActualSellPrice:     actualSellPrice,
			ActualProfit:        actualProfit,
			ActualProfitRate:    actualProfitRate,
			PriceChangeRate:     priceChangeRate,
			IsSuccessful:        actualProfit > 0,
			ProfitAccuracy:      profitAccuracy,
			Quantity:            histOpp.RecommendedQuantity,
		}

		results = append(results, result)
	}

	log.Printf("[回测分析] 成功计算 %d 个商品的回测结果", len(results))

	// 4. 统计和输出报告
	printBacktestReport(results, actualAnalysisTime)
}

// printBacktestReport 打印回测报告
func printBacktestReport(results []BacktestResult, analysisTime time.Time) {
	if len(results) == 0 {
		log.Printf("[回测报告] 没有可用的回测数据")
		return
	}

	log.Printf("\n[回测报告] ==================== 回测准确度分析 ====================")
	log.Printf("[回测报告] 原始分析时间: %s", analysisTime.Format("2006-01-02 15:04:05"))
	log.Printf("[回测报告] 今日时间: %s", time.Now().Format("2006-01-02 15:04:05"))
	log.Printf("[回测报告] 回测周期: %d天", *backtestDays)
	log.Printf("")

	// 统计指标
	totalPredictedProfit := 0.0
	totalActualProfit := 0.0
	successCount := 0
	totalInvestment := 0.0
	accuracySum := 0.0

	for _, r := range results {
		totalPredictedProfit += r.PredictedProfit
		totalActualProfit += r.ActualProfit
		totalInvestment += r.ActualBuyPrice * float64(r.Quantity)
		if r.IsSuccessful {
			successCount++
		}
		if r.ProfitAccuracy > 0 {
			accuracySum += r.ProfitAccuracy
		}
	}

	successRate := float64(successCount) / float64(len(results)) * 100
	avgAccuracy := accuracySum / float64(len(results)) * 100
	predictedROI := totalPredictedProfit / totalInvestment * 100
	actualROI := totalActualProfit / totalInvestment * 100

	log.Printf("[回测统计] ==================== 总体表现 ====================")
	log.Printf("📊 样本数量: %d 个商品", len(results))
	log.Printf("💰 总投资: ¥%.2f", totalInvestment)
	log.Printf("📈 预期利润: ¥%.2f (ROI: %.1f%%)", totalPredictedProfit, predictedROI)
	log.Printf("💵 实际利润: ¥%.2f (ROI: %.1f%%)", totalActualProfit, actualROI)
	log.Printf("✅ 成功率: %.1f%% (%d/%d)", successRate, successCount, len(results))
	log.Printf("🎯 平均准确度: %.1f%%", avgAccuracy)

	// 利润差异
	profitDiff := totalActualProfit - totalPredictedProfit
	profitDiffPercent := 0.0
	if totalPredictedProfit != 0 {
		profitDiffPercent = profitDiff / totalPredictedProfit * 100
	}

	diffIcon := "="
	if profitDiff > 0 {
		diffIcon = "📈"
	} else if profitDiff < 0 {
		diffIcon = "📉"
	}

	log.Printf("%s 利润差异: ¥%.2f (%.1f%%)", diffIcon, profitDiff, profitDiffPercent)
	log.Printf("")

	// 详细列表
	log.Printf("[回测详情] ==================== 各商品表现 ====================")
	log.Printf("%-4s %-45s %8s %8s %10s %10s %8s",
		"序号", "商品名称", "预期利润", "实际利润", "准确度", "价格变化", "结果")
	log.Printf("%-4s %-45s %8s %8s %10s %10s %8s",
		"----", "---------------------------------------------", "--------", "--------", "----------", "----------", "--------")

	for i, r := range results {
		displayName := r.GoodName
		if len(displayName) > 43 {
			displayName = displayName[:40] + "..."
		}

		resultIcon := "❌"
		if r.IsSuccessful {
			resultIcon = "✅"
		}

		log.Printf("#%-3d %-45s %7.2f元 %7.2f元 %9.1f%% %9.1f%% %8s",
			i+1, displayName,
			r.PredictedProfit, r.ActualProfit,
			r.ProfitAccuracy*100, r.PriceChangeRate*100,
			resultIcon)
	}

	log.Printf("==========================================================================")
	log.Printf("[回测报告] 分析完成")

	// === 生成回测结果 JSON 文件 ===
	backtestJSON := map[string]interface{}{
		"timestamp":     time.Now().Format("2006-01-02 15:04:05"),
		"analysis_time": analysisTime.Format("2006-01-02 15:04:05"),
		"summary": map[string]interface{}{
			"sample_count":          len(results),
			"total_investment":      totalInvestment,
			"predicted_profit":      totalPredictedProfit,
			"predicted_roi_percent": predictedROI,
			"actual_profit":         totalActualProfit,
			"actual_roi_percent":    actualROI,
			"success_rate_percent":  successRate,
			"avg_accuracy_percent":  avgAccuracy,
			"profit_difference":     profitDiff,
			"profit_diff_percent":   profitDiffPercent,
		},
		"details": results,
	}

	jsonBytes, _ := json.MarshalIndent(backtestJSON, "", "  ")
	os.WriteFile("backtest_result.json", jsonBytes, 0644)
	log.Printf("[输出] 回测结果已保存到: backtest_result.json")

	// === 保存策略调整日志 ===
	SaveAdjustmentLog("strategy_adjustment_log.txt")
	log.Printf("[输出] 策略调整日志已保存到: strategy_adjustment_log.txt")
}

// VerificationResult 验证结果
type VerificationResult struct {
	GoodID            int64
	GoodName          string
	OriginalBuyPrice  float64 // 原始求购价
	OriginalSellPrice float64 // 原始售价
	VerifiedBuyPrice  float64 // 验证后的求购价
	VerifiedSellPrice float64 // 验证后的售价
	IsStillValid      bool    // 是否仍然符合套利条件
	Reason            string  // 不符合的原因
	ProfitRateChange  float64 // 利润率变化（新利润率 - 原利润率）
}

// PurchaseListItem 购买清单项
type PurchaseListItem struct {
	GoodID   int64
	GoodName string
	Quantity int
	Price    float64
	Total    float64
}

// verifyOpportunitiesPrices 第二阶段验证：再次获取实时价格，确保套利机会仍然有效
// 只验证purchaseList中的饰品（根据预算确定的最终购买清单）
// 使用NovProxy代理 + 双线程查询模式，每个线程独立的频率限制（250ms间隔）
func verifyOpportunitiesPrices(db *gorm.DB, ypClient *youpin.OpenAPIClient, opportunities []models.ArbitrageOpportunity, purchaseList interface{}, timeoutSec int) []models.ArbitrageOpportunity {
	if ypClient == nil {
		log.Printf("[二次验证] OpenAPI客户端未初始化，跳过验证")
		return opportunities
	}

	if len(opportunities) == 0 {
		log.Printf("[二次验证] 没有套利机会需要验证")
		return opportunities
	}

	// 构建购买清单中的GoodID集合（用于过滤）以及明细映射（用于获取数量）
	purchaseGoodIDSet := make(map[int64]bool)
	purchaseItemMap := make(map[int64]PurchaseListItem)
	if purchaseList != nil {
		// 尝试用反射遍历purchaseList的元素
		listVal := reflect.ValueOf(purchaseList)
		if listVal.Kind() == reflect.Slice {
			for i := 0; i < listVal.Len(); i++ {
				elem := listVal.Index(i)
				if elem.Kind() == reflect.Struct {
					// 从结构体中获取GoodID字段
					goodIDField := elem.FieldByName("GoodID")
					if goodIDField.IsValid() && goodIDField.Kind() == reflect.Int64 {
						gid := goodIDField.Int()
						purchaseGoodIDSet[gid] = true
						// 额外收集数量、名称、价格等信息
						var item PurchaseListItem
						item.GoodID = gid
						if nameField := elem.FieldByName("GoodName"); nameField.IsValid() {
							if nameField.Kind() == reflect.String {
								item.GoodName = nameField.String()
							}
						}
						if qtyField := elem.FieldByName("Quantity"); qtyField.IsValid() {
							switch qtyField.Kind() {
							case reflect.Int, reflect.Int32, reflect.Int64:
								item.Quantity = int(qtyField.Int())
							}
						}
						if priceField := elem.FieldByName("Price"); priceField.IsValid() && priceField.Kind() == reflect.Float64 {
							item.Price = priceField.Float()
						}
						if totalField := elem.FieldByName("Total"); totalField.IsValid() && totalField.Kind() == reflect.Float64 {
							item.Total = totalField.Float()
						}
						purchaseItemMap[item.GoodID] = item
					}
				}
			}
		}
	}

	// 如果purchaseList为空，则不进行验证
	if len(purchaseGoodIDSet) == 0 {
		log.Printf("[二次验证] 购买清单为空，跳过二次验证")
		return []models.ArbitrageOpportunity{}
	}

	// 过滤出在购买清单中的套利机会
	toVerify := []models.ArbitrageOpportunity{}
	for _, opp := range opportunities {
		if purchaseGoodIDSet[opp.GoodID] {
			toVerify = append(toVerify, opp)
		}
	}

	log.Printf("[二次验证] 开始验证 %d 个套利机会的实时价格... (双线程，从购买清单中筛选)", len(toVerify))

	// 双线程处理
	var wg sync.WaitGroup
	taskChan := make(chan models.ArbitrageOpportunity, 2)
	resultChan := make(chan VerificationResult, len(toVerify))

	validCount := int64(0)
	invalidCount := int64(0)
	processedCount := int64(0)

	// 两个处理线程（每个线程独立的速率限制）
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()

			// 每个线程独立的速率限制：500ms/请求
			threadRateLimiter := time.NewTicker(2 * time.Second)
			defer threadRateLimiter.Stop()

			for opp := range taskChan {
				// 等待当前线程的速率限制
				<-threadRateLimiter.C

				// 获取该商品的TemplateID
				var snapshot models.CSQAQGoodSnapshot
				if err := db.Where("good_id = ? AND yyyp_template_id IS NOT NULL", opp.GoodID).
					Order("created_at DESC").
					First(&snapshot).Error; err != nil || snapshot.YYYPTemplateID == nil || *snapshot.YYYPTemplateID == 0 {
					log.Printf("[二次验证] [Worker-%d] 商品 %d 缺少TemplateID，跳过验证", wid, opp.GoodID)
					result := VerificationResult{
						GoodID:            opp.GoodID,
						GoodName:          opp.GoodName,
						OriginalBuyPrice:  opp.CurrentBuyPrice,
						OriginalSellPrice: opp.CurrentSellPrice,
						VerifiedBuyPrice:  opp.CurrentBuyPrice,
						VerifiedSellPrice: opp.CurrentSellPrice,
						IsStillValid:      true,
					}
					resultChan <- result
					atomic.AddInt64(&processedCount, 1)
					continue
				}

				// 获取最新的实时价格
				rp, reason := fetchRealtimePrice(db, ypClient, nil, opp.GoodID, opp.GoodName, timeoutSec)

				// 构建验证结果
				result := VerificationResult{
					GoodID:            opp.GoodID,
					GoodName:          opp.GoodName,
					OriginalBuyPrice:  opp.CurrentBuyPrice,
					OriginalSellPrice: opp.CurrentSellPrice,
				}

				// 检查是否获取成功
				if !rp.ok {
					log.Printf("[二次验证] [Worker-%d] 商品 %s 获取实时价格失败: %s，保持原价", wid, opp.GoodName, reason)
					result.VerifiedBuyPrice = opp.CurrentBuyPrice
					result.VerifiedSellPrice = opp.CurrentSellPrice
					result.IsStillValid = true // 获取失败时保持原有机会
				} else {
					result.VerifiedBuyPrice = rp.buy
					result.VerifiedSellPrice = rp.sell

					// 验证是否仍然满足套利条件
					if result.VerifiedSellPrice <= 0 || result.VerifiedBuyPrice <= 0 {
						result.IsStillValid = false
						result.Reason = "实时价格无效"
						atomic.AddInt64(&invalidCount, 1)
					} else if result.VerifiedSellPrice <= result.VerifiedBuyPrice {
						result.IsStillValid = false
						result.Reason = "售价不高于求购价"
						atomic.AddInt64(&invalidCount, 1)
					} else {
						// 计算新利润率
						feeRate := 0.01
						netSellPrice := result.VerifiedSellPrice * (1 - feeRate)
						estimatedProfit := netSellPrice - result.VerifiedBuyPrice
						newProfitRate := estimatedProfit / result.VerifiedBuyPrice

						if newProfitRate < *minProfitRate {
							result.IsStillValid = false
							result.Reason = fmt.Sprintf("利润率从 %.2f%% 下降到 %.2f%%，不满足最小 %.2f%%",
								opp.ProfitRate*100, newProfitRate*100, *minProfitRate*100)
							atomic.AddInt64(&invalidCount, 1)
						} else {
							result.IsStillValid = true
							result.ProfitRateChange = newProfitRate - opp.ProfitRate
							atomic.AddInt64(&validCount, 1)

							// 若开启自动下单，则在验证通过后立即二次获取最高求购价、加价并下单
							if *autoPurchase && ypClient != nil {
								if item, ok := purchaseItemMap[opp.GoodID]; ok && item.Quantity > 0 {
									// 第二次获取最新最高求购价（尽可能贴近下单时刻）
									latestMax, _ := getLatestMaxBuyPrice(db, ypClient, opp.GoodID, timeoutSec)
									// 如果获取失败或为0，则回退使用本次验证的买价
									if latestMax <= 0 {
										latestMax = result.VerifiedBuyPrice
									}
									bumped := bumpPurchasePrice(latestMax)
									// 执行下单
									if err := placeImmediatePurchaseOrder(db, ypClient, opp.GoodID, item.GoodName, item.Quantity, bumped, timeoutSec); err != nil {
										log.Printf("[自动下单] [Worker-%d] %s 下单失败: %v", wid, opp.GoodName, err)
									} else {
										log.Printf("[自动下单] [Worker-%d] %s 已创建求购订单: 数量=%d, 价格=¥%.2f (最高=¥%.2f)", wid, opp.GoodName, item.Quantity, bumped, latestMax)
									}
								}
							}
						}
					}
				}

				resultChan <- result
				atomic.AddInt64(&processedCount, 1)

				// 输出进度
				processed := atomic.LoadInt64(&processedCount)
				if processed%50 == 0 || processed == int64(len(opportunities)) {
					log.Printf("[二次验证] 进度: %d/%d", processed, len(opportunities))
				}
			}
		}(w)
	}

	// 分发任务
	go func() {
		for _, opp := range toVerify {
			taskChan <- opp
		}
		close(taskChan)
	}()

	// 等待所有线程完成
	wg.Wait()
	close(resultChan)

	// 收集结果
	verificationResults := []VerificationResult{}
	verifiedOpps := []models.ArbitrageOpportunity{}

	resultMap := make(map[int64]VerificationResult)
	for result := range resultChan {
		resultMap[result.GoodID] = result
		verificationResults = append(verificationResults, result)
	}

	// 构建验证通过的机会列表
	for _, opp := range toVerify {
		if result, ok := resultMap[opp.GoodID]; ok && result.IsStillValid {
			opp.CurrentBuyPrice = result.VerifiedBuyPrice
			opp.CurrentSellPrice = result.VerifiedSellPrice

			// 重新计算利润率
			feeRate := 0.01
			netSellPrice := result.VerifiedSellPrice * (1 - feeRate)
			estimatedProfit := netSellPrice - result.VerifiedBuyPrice
			opp.ProfitRate = estimatedProfit / result.VerifiedBuyPrice

			verifiedOpps = append(verifiedOpps, opp)
		}
	}

	// 输出验证结果
	log.Printf("[二次验证] ==================== 验证结果汇总 ====================")
	log.Printf("[二次验证] 总计验证: %d 个", len(toVerify))
	log.Printf("[二次验证] 验证通过: %d 个 ✅", atomic.LoadInt64(&validCount))
	log.Printf("[二次验证] 验证失败: %d 个 ❌", atomic.LoadInt64(&invalidCount))

	// 输出验证失败的机会
	if atomic.LoadInt64(&invalidCount) > 0 {
		log.Printf("[二次验证] ==================== 验证失败的机会 ====================")
		for _, result := range verificationResults {
			if !result.IsStillValid {
				log.Printf("[❌] %s", result.GoodName)
				log.Printf("     原价: 求购 ¥%.2f → 在售 ¥%.2f", result.OriginalBuyPrice, result.OriginalSellPrice)
				log.Printf("     新价: 求购 ¥%.2f → 在售 ¥%.2f", result.VerifiedBuyPrice, result.VerifiedSellPrice)
				log.Printf("     原因: %s", result.Reason)
			}
		}
	}

	// 输出验证通过但利润率变化的机会
	var profitRateChanges []VerificationResult
	for _, result := range verificationResults {
		if result.IsStillValid && result.ProfitRateChange != 0 {
			profitRateChanges = append(profitRateChanges, result)
		}
	}

	if len(profitRateChanges) > 0 {
		log.Printf("[二次验证] ==================== 利润率有变化的机会 ====================")
		for _, result := range profitRateChanges {
			changeIcon := "📈"
			if result.ProfitRateChange < 0 {
				changeIcon = "📉"
			}
			log.Printf("[%s] %s: 利润率变化 %+.2f%%", changeIcon, result.GoodName, result.ProfitRateChange*100)
		}
	}

	log.Printf("[二次验证] ===================================================================")

	return verifiedOpps
}

// GoodProcessingTask 单个商品处理任务
type GoodProcessingTask struct {
	goodID int64
	good   models.CSQAQGood
}

// processGoodsInParallel 并发处理商品列表（第一阶段）
func processGoodsInParallel(
	db *gorm.DB,
	ypClient *youpin.OpenAPIClient,
	goodIDs []int64,
	goodsCache map[int64]models.CSQAQGood,
	numWorkers int,
) []struct {
	good                models.CSQAQGood
	currentBuyPrice     float64
	currentSellPrice    float64
	avgBuyPrice7d       float64
	avgSellPrice7d      float64
	buyOrderCount       int
	sellOrderCount      int
	daysOfData          int
	hasEnoughHistory    bool
	rankNum             *int // 热度排名
	historicalSnapshots []models.CSQAQGoodSnapshot
} {
	// 创建任务队列和结果队列
	taskChan := make(chan GoodProcessingTask, len(goodIDs))
	resultChan := make(chan struct {
		good                models.CSQAQGood
		currentBuyPrice     float64
		currentSellPrice    float64
		avgBuyPrice7d       float64
		avgSellPrice7d      float64
		buyOrderCount       int
		sellOrderCount      int
		daysOfData          int
		hasEnoughHistory    bool
		rankNum             *int // 热度排名
		historicalSnapshots []models.CSQAQGoodSnapshot
	})

	// 启动工作线程
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for task := range taskChan {
				// 处理单个商品
				processOneGood(db, ypClient, task, resultChan)
			}
		}(i)
	}

	// 发送任务
	go func() {
		for _, goodID := range goodIDs {
			if good, exists := goodsCache[goodID]; exists {
				taskChan <- GoodProcessingTask{
					goodID: goodID,
					good:   good,
				}
			}
		}
		close(taskChan)
	}()

	// 在后台等待所有工作线程完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 收集结果
	var results []struct {
		good                models.CSQAQGood
		currentBuyPrice     float64
		currentSellPrice    float64
		avgBuyPrice7d       float64
		avgSellPrice7d      float64
		buyOrderCount       int
		sellOrderCount      int
		daysOfData          int
		hasEnoughHistory    bool
		rankNum             *int // 热度排名
		historicalSnapshots []models.CSQAQGoodSnapshot
	}

	processedCount := 0
	for result := range resultChan {
		if result.currentBuyPrice > 0 && result.currentSellPrice > 0 {
			results = append(results, result)
		}
		processedCount++
		if processedCount%100 == 0 {
			log.Printf("[并发处理] 已处理 %d 个商品，已筛选 %d 个候选项", processedCount, len(results))
		}
	}

	return results
}

// processOneGood 处理单个商品
func processOneGood(
	db *gorm.DB,
	ypClient *youpin.OpenAPIClient,
	task GoodProcessingTask,
	resultChan chan struct {
		good                models.CSQAQGood
		currentBuyPrice     float64
		currentSellPrice    float64
		avgBuyPrice7d       float64
		avgSellPrice7d      float64
		buyOrderCount       int
		sellOrderCount      int
		daysOfData          int
		hasEnoughHistory    bool
		rankNum             *int // 热度排名
		historicalSnapshots []models.CSQAQGoodSnapshot
	},
) {
	good := task.good
	goodID := task.goodID

	// 初始化结果结构体（0值）
	result := struct {
		good                models.CSQAQGood
		currentBuyPrice     float64
		currentSellPrice    float64
		avgBuyPrice7d       float64
		avgSellPrice7d      float64
		buyOrderCount       int
		sellOrderCount      int
		daysOfData          int
		hasEnoughHistory    bool
		rankNum             *int // 热度排名
		historicalSnapshots []models.CSQAQGoodSnapshot
	}{
		good: good,
	}

	// 类型过滤
	name := good.Name
	lowerName := strings.ToLower(name)

	hasGuajian := strings.Contains(name, "挂件")
	hasJinianpin := strings.Contains(name, "纪念品")

	if strings.Contains(name, "★") ||
		strings.Contains(name, "手套") ||
		strings.Contains(name, "贴纸") ||
		strings.Contains(name, "印花") ||
		strings.Contains(name, "胶囊") ||
		strings.Contains(name, "探员") ||
		strings.Contains(name, "音乐盒") ||
		strings.Contains(name, "钥匙") ||
		strings.Contains(name, "通行证") ||
		strings.Contains(name, "涂鸦") ||
		strings.Contains(name, "收藏品") ||
		strings.Contains(name, "武器箱") ||
		strings.Contains(name, "布章") ||
		strings.Contains(name, "特工") ||
		strings.Contains(name, "徽章") ||
		strings.Contains(name, "挂饰") ||
		strings.Contains(name, "缀饰") ||
		strings.Contains(name, "徽记") ||
		strings.Contains(name, "补丁") ||
		strings.Contains(name, "人偶") ||
		strings.Contains(name, "人形") ||
		strings.Contains(name, "代理人") ||
		strings.Contains(name, "人质") ||
		strings.Contains(name, "徽章包") ||
		strings.Contains(name, "补章") ||
		strings.Contains(name, "德拉戈米尔 | 军刀勇士") ||
		strings.Contains(name, "纪念包") ||
		strings.Contains(lowerName, "sticker") ||
		strings.Contains(lowerName, "patch") ||
		strings.Contains(lowerName, "agent") ||
		strings.Contains(lowerName, "music kit") ||
		strings.Contains(lowerName, "souvenir") ||
		strings.Contains(lowerName, "case") ||
		strings.Contains(lowerName, "capsule") ||
		strings.Contains(lowerName, "graffiti") ||
		strings.Contains(lowerName, "key") ||
		strings.Contains(lowerName, "pass") ||
		hasGuajian ||
		hasJinianpin {
		log.Printf("[第一阶段] ✗ ID=%d, 名称=%s, 被排除: 类型不符 (非枪械饰品)", goodID, name)
		resultChan <- result
		return
	}

	// 黑名单检查
	if isBlacklisted(goodID, db) {
		log.Printf("[第一阶段] ✗ ID=%d, 名称=%s, 被排除: 黑名单商品", goodID, name)
		resultChan <- result
		return
	}

	// 获取历史数据
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)
	var historicalSnapshots []models.CSQAQGoodSnapshot
	if err := db.Where("good_id = ? AND created_at >= ?", goodID, sevenDaysAgo).
		Order("created_at DESC").
		Find(&historicalSnapshots).Error; err != nil || len(historicalSnapshots) == 0 {
		log.Printf("[第一阶段] ✗ ID=%d, 名称=%s, 被排除: 无历史数据 (过去7天)", goodID, name)
		resultChan <- result
		return
	}

	// 获取实时价格
	var currentBuyPrice, currentSellPrice float64
	var rtBuyCount, rtSellCount int
	var usedRealtime bool

	if rp, _ := fetchRealtimePrice(db, ypClient, nil, goodID, good.Name, *ypTimeoutSec); rp.ok {
		currentBuyPrice = rp.buy
		currentSellPrice = rp.sell
		rtBuyCount = rp.buyCount
		rtSellCount = rp.sellCount
		usedRealtime = true
	} else {
		// 回退快照
		if len(historicalSnapshots) == 0 {
			log.Printf("[第一阶段] ✗ ID=%d, 名称=%s, 被排除: 无有效价格数据", goodID, name)
			resultChan <- result
			return
		}
		latestSnapshot := historicalSnapshots[0]
		if latestSnapshot.YYYPBuyPrice == nil || latestSnapshot.YYYPSellPrice == nil ||
			*latestSnapshot.YYYPBuyPrice <= 0 || *latestSnapshot.YYYPSellPrice <= 0 {
			log.Printf("[第一阶段] ✗ ID=%d, 名称=%s, 被排除: 快照价格无效", goodID, name)
			resultChan <- result
			return
		}
		currentBuyPrice = *latestSnapshot.YYYPBuyPrice
		currentSellPrice = *latestSnapshot.YYYPSellPrice
	}

	// 基础价格检查
	if currentBuyPrice <= 0 || currentSellPrice <= 0 ||
		currentBuyPrice > *maxReasonablePrice || currentSellPrice > *maxReasonablePrice ||
		currentBuyPrice < *minPrice || currentSellPrice < *minPrice ||
		currentSellPrice > currentBuyPrice*(*maxPriceSpread) {
		log.Printf("[第一阶段] ✗ ID=%d, 名称=%s, 被排除: 价格异常 (买:%.2f, 卖:%.2f)", goodID, name, currentBuyPrice, currentSellPrice)
		resultChan <- result
		return
	}

	// 套利空间检查
	feeRate := 0.01
	netSellPrice := currentSellPrice * (1 - feeRate)
	if netSellPrice <= currentBuyPrice {
		log.Printf("[第一阶段] ✗ ID=%d, 名称=%s, 被排除: 无套利空间 (净卖价:%.2f <= 买价:%.2f)", goodID, name, netSellPrice, currentBuyPrice)
		resultChan <- result
		return
	}

	// 获取订单数量
	buyOrderCount := 0
	sellOrderCount := 0

	if usedRealtime {
		buyOrderCount = rtBuyCount
		sellOrderCount = rtSellCount
	} else if len(historicalSnapshots) > 0 && historicalSnapshots[0].YYYPSellCount != nil && *historicalSnapshots[0].YYYPSellCount > 0 {
		// 只关心真实的在售数量，如果没有就不推荐
		sellOrderCount = *historicalSnapshots[0].YYYPSellCount
		// 买单数量可选，没有就估算
		if historicalSnapshots[0].YYYPBuyCount != nil && *historicalSnapshots[0].YYYPBuyCount > 0 {
			buyOrderCount = *historicalSnapshots[0].YYYPBuyCount
		} else {
			buyOrderCount = int(float64(sellOrderCount) * 0.35) // 估算为在售数量的35%
		}
	} else {
		// 没有真实的在售数量就不推荐
		log.Printf("[第一阶段] ✗ ID=%d, 名称=%s, 被排除: 无有效在售数量", goodID, name)
		resultChan <- result
		return
	}

	// 流动性检查
	if sellOrderCount < *minSellCount || buyOrderCount < *minBuyCount {
		log.Printf("[第一阶段] ✗ ID=%d, 名称=%s, 被排除: 流动性不足 (在售:%d<%d, 求购:%d<%d)",
			goodID, name, sellOrderCount, *minSellCount, buyOrderCount, *minBuyCount)
		resultChan <- result
		return
	}

	// 计算平均价格
	var avgBuyPrice7d, avgSellPrice7d float64
	hasEnoughHistory := len(historicalSnapshots) >= *minDaysHistory

	if hasEnoughHistory {
		var totalBuyPrice, totalSellPrice float64
		validBuyCount, validSellCount := 0, 0
		for _, snapshot := range historicalSnapshots {
			if snapshot.YYYPBuyPrice != nil && *snapshot.YYYPBuyPrice > 0 {
				totalBuyPrice += *snapshot.YYYPBuyPrice
				validBuyCount++
			}
			if snapshot.YYYPSellPrice != nil && *snapshot.YYYPSellPrice > 0 {
				totalSellPrice += *snapshot.YYYPSellPrice
				validSellCount++
			}
		}

		if validBuyCount > 0 && validSellCount > 0 {
			avgBuyPrice7d = totalBuyPrice / float64(validBuyCount)
			avgSellPrice7d = totalSellPrice / float64(validSellCount)
		}
	}

	// 利润率检查
	estimatedProfit := netSellPrice - currentBuyPrice
	profitRate := estimatedProfit / currentBuyPrice

	if profitRate < *minProfitRate {
		log.Printf("[第一阶段] ✗ ID=%d, 名称=%s, 被排除: 利润率不足 (%.2f%% < %.2f%%)",
			goodID, name, profitRate*100, *minProfitRate*100)
		resultChan <- result
		return
	}

	// 通过了所有检查
	log.Printf("[第一阶段] ✓ ID=%d, 名称=%s, 通过所有检查 (在售:%d, 求购:%d, 买价:%.2f, 卖价:%.2f, 利润率:%.2f%%)",
		goodID, name, sellOrderCount, buyOrderCount, currentBuyPrice, currentSellPrice, profitRate*100)

	// === 抄底策略检查（第一阶段）===
	// 在第一阶段就识别底部反弹特征，避免遗漏抄底机会
	// 条件：3-7天下跌 + 1-3天反弹 = 底部反弹信号
	if len(historicalSnapshots) >= 2 {
		now := time.Now()
		var price1d, price2d, price3d, price7d, price30d float64
		var has1d, has2d, has3d, has7d, has30d bool

		// 从历史快照中获取对应时间点的价格
		for _, snapshot := range historicalSnapshots {
			if snapshot.YYYPSellPrice != nil && *snapshot.YYYPSellPrice > 0 {
				age := now.Sub(snapshot.CreatedAt)

				// 1天前的价格（23-25小时）
				if age >= 23*time.Hour && age <= 25*time.Hour && !has1d {
					price1d = *snapshot.YYYPSellPrice
					has1d = true
				}

				// 2天前的价格（47-49小时）
				if age >= 47*time.Hour && age <= 49*time.Hour && !has2d {
					price2d = *snapshot.YYYPSellPrice
					has2d = true
				}

				// 3天前的价格（71-73小时）
				if age >= 71*time.Hour && age <= 73*time.Hour && !has3d {
					price3d = *snapshot.YYYPSellPrice
					has3d = true
				}

				// 7天前的价格（6.5-7.5天）
				if age >= 156*time.Hour && age <= 180*time.Hour && !has7d {
					price7d = *snapshot.YYYPSellPrice
					has7d = true
				}

				// 30天前的价格（28-32天）
				if age >= 672*time.Hour && age <= 768*time.Hour && !has30d {
					price30d = *snapshot.YYYPSellPrice
					has30d = true
				}
			}
		}

		// 计算涨跌幅
		var rate1d, rate2d, rate3d, rate7d, rate30d float64
		_ = rate2d  // 可能未使用，但保留作为完整的多周期分析框架
		_ = rate3d  // 可能未使用
		_ = rate30d // 30天涨跌幅在当前版本未使用，但保留框架
		if has1d && price1d > 0 {
			rate1d = (currentSellPrice - price1d) / price1d
		}
		if has2d && price2d > 0 {
			rate2d = (currentSellPrice - price2d) / price2d
		}
		if has3d && price3d > 0 {
			rate3d = (currentSellPrice - price3d) / price3d
		}
		if has7d && price7d > 0 {
			rate7d = (currentSellPrice - price7d) / price7d
		}
		if has30d && price30d > 0 {
			rate30d = (currentSellPrice - price30d) / price30d
		}

		// 抄底策略判断：支持3-7天灵活周期
		isBottomRebound := false

		// 情况1：连续上涨中（1天↑ AND 7天↑）- 避免高位接盘，不符合抄底
		if has1d && has7d && rate1d > 0 && rate7d > 0 {
			isBottomRebound = false
		} else if has1d && has7d && has30d && rate1d < 0 && rate7d < 0 && rate30d < 0 {
			// 情况2：所有周期都在下跌 - 避免继续下跌
			isBottomRebound = false
		} else {
			// 情况3：底部反弹 - 支持3-7天的灵活周期

			// 灵活周期检查：3天、4天、5天、6天、7天中的任何一个满足下跌条件
			hasValidDecline := false
			var declineDays int
			var declineRate float64
			_ = declineDays

			// 检查7天下跌
			if has7d && rate7d < -0.05 {
				hasValidDecline = true
				declineDays = 7
				declineRate = rate7d
			}
			// 检查6-7天下跌 - 稍微放宽一点
			if !hasValidDecline && has7d && rate7d < -0.04 {
				hasValidDecline = true
				declineDays = 7
				declineRate = rate7d
			}
			// 检查5天下跌 - 从3天数据推断
			if !hasValidDecline && has3d && rate3d < -0.04 {
				hasValidDecline = true
				declineDays = 5
				declineRate = rate3d * 1.5
			}
			// 检查4天下跌
			if !hasValidDecline && has2d && rate2d < -0.03 {
				hasValidDecline = true
				declineDays = 4
				declineRate = rate2d
			}
			// 检查3天下跌 - 需要最强的下跌
			if !hasValidDecline && has1d && rate1d < -0.05 {
				hasValidDecline = true
				declineDays = 3
				declineRate = rate1d
			}

			if hasValidDecline {
				// 找最近的反弹点（1天、2天或3天内）
				var latestRebound float64
				var hasRebound bool

				if has1d && rate1d > 0 {
					latestRebound = rate1d
					hasRebound = true
				} else if has2d && rate2d > 0 {
					latestRebound = rate2d
					hasRebound = true
				} else if has3d && rate3d > 0 {
					latestRebound = rate3d
					hasRebound = true
				}

				// 有反弹且在3天内
				if hasRebound && latestRebound > 0 {
					// 计算反弹恢复率：反弹幅度 / 跌幅
					recoveryRate := latestRebound / (-declineRate)

					// ⭐ 改进：反弹判断逻辑（追稳而非追涨）=====
					// 确保选中的是"底部稳定反弹"而非"高位追涨"

					// 反弹恢复率范围：必须在minRebound% ~ maxRebound%之间
					// - 最低：必须恢复至少50%的跌幅（原来30%太低）
					// - 最高：不超过跌幅的80%（防止追涨过度）
					minRecoveryRate := *minRebound        // 从0.30提升至0.50
					maxRecoveryRate := *maxRebound        // 新增：最高0.80
					minAbsoluteReb := *minAbsoluteRebound // 从0.02提升至0.03

					// 检查反弹是否在合理范围内
					recoveryRateOK := recoveryRate >= minRecoveryRate && recoveryRate <= maxRecoveryRate
					absoluteReOK := latestRebound >= minAbsoluteReb && latestRebound <= *maxAbsoluteRebound1d

					// 单日反弹不能超过5%
					if has1d && latestRebound == rate1d && rate1d > *maxAbsoluteRebound1d {
						// 1天反弹超过5%，标记为风险较高但仍保留
					}

					if recoveryRateOK || absoluteReOK {
						// 反弹在合理范围内，这是底部反弹信号！✅
						isBottomRebound = true
					}
				}
			}
		}
		// 其他情况：既不是连续上涨，也不是底部反弹，按普通情况处理
		_ = isBottomRebound // 标记使用，为第二阶段预留
	}

	// 构建并返回结果
	result.currentBuyPrice = currentBuyPrice
	result.currentSellPrice = currentSellPrice
	result.avgBuyPrice7d = avgBuyPrice7d
	result.avgSellPrice7d = avgSellPrice7d
	result.buyOrderCount = buyOrderCount
	result.sellOrderCount = sellOrderCount
	result.daysOfData = len(historicalSnapshots)
	result.hasEnoughHistory = hasEnoughHistory
	result.historicalSnapshots = historicalSnapshots

	// 获取热度排名：从最新的快照中读取
	if len(historicalSnapshots) > 0 && historicalSnapshots[0].RankNum != nil {
		result.rankNum = historicalSnapshots[0].RankNum
	}

	resultChan <- result
}

func main() {
	flag.Parse()

	log.Printf("[套利分析器] 启动中...")
	log.Printf("[套利分析器] 配置:")
	log.Printf("  - 最小利润率: %.2f%% ⬆️", *minProfitRate*100)
	log.Printf("  - 求购预算: ¥%.2f", *budget)
	log.Printf("  - 价格范围: ¥%.2f ~ ¥%.2f", *minPrice, *maxReasonablePrice)
	log.Printf("  - 最少在售数量: %d件", *minSellCount)
	log.Printf("  - 最少求购数量: %d件", *minBuyCount)
	log.Printf("  - 每种饰品最多: %d件 🎯", *maxQuantityPerItem)
	log.Printf("  - 最大价差倍数: %.1f倍", *maxPriceSpread)
	log.Printf("  - 最少历史天数: %d天", *minDaysHistory)
	log.Printf("  - 并发线程数: %d", *concurrency)
	log.Printf("[多样性策略] 优先广度而非深度:")
	log.Printf("  - 默认每种只买1件（最大化品种数量）")
	log.Printf("  - 利润率≥18%% + 低风险时可买2件")
	log.Printf("  - 严格限制: 每种最多%d件", *maxQuantityPerItem)
	// ===== 新增：反弹参数说明 =====
	log.Printf("[反弹控制] 追稳策略参数:")
	log.Printf("  - 反弹恢复率下限: %.0f%%（必须恢复至少%.0f%%的跌幅）", *minRebound*100, *minRebound*100)
	log.Printf("  - 反弹恢复率上限: %.0f%%（防止追涨，不超过%.0f%%的跌幅）", *maxRebound*100, *maxRebound*100)
	log.Printf("  - 绝对反弹下限: %.2f%%（最少要反弹%.2f%%）", *minAbsoluteRebound*100, *minAbsoluteRebound*100)
	log.Printf("  - 单日反弹上限: %.2f%%（一天反弹不超过%.2f%%）", *maxAbsoluteRebound1d*100, *maxAbsoluteRebound1d*100)
	if *onlyBottomRebound {
		log.Printf("[抄底模式] 🟢 仅抄底模式已激活: 只保留 \"7天跌幅≥5%% + 1-3天内反弹\" 的饰品")
	} else {
		log.Printf("  - 📊 模式: 全量分析模式")
	}

	// 初始化数据库
	db, err := database.Initialize(*dbURL)
	if err != nil {
		log.Fatalf("[套利分析器] 数据库初始化失败: %v", err)
	}

	// 加载黑名单
	blacklistPath := "/root/goods_black_note.xlsx"
	if _, err := loadBlacklist(blacklistPath); err != nil {
		log.Printf("[黑名单] ⚠️ 加载黑名单失败: %v", err)
	}

	// 自动迁移：套利机会、历史归档、求购计划与明细表
	if err := db.AutoMigrate(
		&models.ArbitrageOpportunity{},
		&models.ArbitrageOpportunityHistory{},
		&models.PurchasePlan{},
		&models.PurchasePlanItem{},
	); err != nil {
		log.Fatalf("[套利分析器] 表迁移失败: %v", err)
	}

	// 初始化预测客户端
	predictionClient := services.NewPredictionClient("http://localhost:5000")
	ok, err := predictionClient.Health()
	if !ok || err != nil {
		log.Printf("[套利分析器] ⚠️ 预测服务不可用: %v，将继续使用传统分析方法", err)
	} else {
		log.Printf("[套利分析器] ✓ 预测服务连接成功，已启用集成预测模型")
	}

	if *backtest {
		// 回测模式
		runBacktest(db)
	} else if *once {
		// 只运行一次
		runAnalysis(db, predictionClient)
	} else {
		// 持续循环运行：每次运行完立即开始下一次
		for {
			runAnalysis(db, predictionClient)
			log.Printf("[套利分析器] 本轮分析完成，立即开始下一轮分析...")
		}
	}
}

func runAnalysis(db *gorm.DB, predictionClient *services.PredictionClient) {
	startTime := time.Now()
	analysisTime := startTime
	log.Printf("[套利分析] ==================== 开始新一轮分析 ====================")
	log.Printf("[套利分析] 分析时间: %s", analysisTime.Format("2006-01-02 15:04:05"))
	log.Printf("[套利分析] 分析方法: 集成预测模型 (Prophet + XGBoost + LinearRegression)")

	// === 市场风险检测（自适应策略） ===
	marketRisk := DetectMarketRisk(db)
	log.Printf("[市场风险检测] %s", marketRisk.Recommendation)
	log.Printf("  - 历史成功率: %.1f%%", marketRisk.SuccessRate*100)
	log.Printf("  - 平均ROI: %.2f%%", marketRisk.AvgROI*100)

	// 应用自适应策略调整
	ApplyAdaptiveAdjustment(marketRisk)

	// 预备：尝试构建YouPin OpenAPI客户端
	var ypClient *youpin.OpenAPIClient
	{
		// 首先尝试获取一个有效的Token用于求购查询
		var accountToken string
		var account models.YouPinAccount
		if err := db.Where("is_active = ?", true).First(&account).Error; err == nil && account.Token != "" {
			accountToken = account.Token
		}

		// 如果有Token，使用带Token的OpenAPI客户端；否则只使用OpenAPI认证（求购查询会失败，但可进行价格验证）
		if accountToken != "" {
			proxyURLWithAuth := fmt.Sprintf("http://%s:%s@%s", *proxyUser, *proxyPass, *proxyURL)
			if c, err := youpin.NewOpenAPIClientWithDefaultKeysAndTokenAndProxy(accountToken, proxyURLWithAuth, time.Duration(100*time.Second)); err == nil {
				ypClient = c
				log.Printf("[套利分析] YouPin OpenAPI客户端初始化成功（OpenAPI + Token双认证）")
			} else {
				log.Printf("[套利分析] Token客户端初始化失败: %v，尝试仅使用OpenAPI", err)
				if c, err := youpin.NewOpenAPIClientWithDefaultKeys(); err == nil {
					ypClient = c
					log.Printf("[套利分析] YouPin OpenAPI客户端初始化成功（仅OpenAPI认证，求购查询不可用）")
				} else {
					log.Printf("[套利分析] YouPin OpenAPI客户端初始化失败: %v", err)
				}
			}
		} else {
			if c, err := youpin.NewOpenAPIClientWithDefaultKeys(); err == nil {
				ypClient = c
				log.Printf("[套利分析] YouPin OpenAPI客户端初始化成功（仅OpenAPI认证，求购查询不可用）")
			} else {
				log.Printf("[套利分析] YouPin OpenAPI客户端初始化失败: %v", err)
			}
		}
	}

	// 获取所有有价格快照的商品ID
	log.Printf("[套利分析] 开始查询所有商品ID...")
	var goodIDs []int64
	if err := db.Model(&models.CSQAQGoodSnapshot{}).
		Distinct("good_id").
		Pluck("good_id", &goodIDs).Error; err != nil {
		log.Printf("[套利分析] 查询失败: %v", err)
		return
	}
	log.Printf("[套利分析] 共找到 %d 个商品ID", len(goodIDs))

	// 批量获取所有商品信息并缓存到map中
	log.Printf("[套利分析] 开始批量加载商品信息...")
	var allGoods []models.CSQAQGood
	if err := db.Where("good_id IN ?", goodIDs).Find(&allGoods).Error; err != nil {
		log.Printf("[套利分析] 加载商品信息失败: %v", err)
		return
	}

	// 构建商品ID到商品信息的映射
	goodsCache := make(map[int64]models.CSQAQGood, len(allGoods))
	for _, good := range allGoods {
		goodsCache[good.GoodID] = good
	}
	log.Printf("[套利分析] 商品信息加载完成，共 %d 个商品", len(goodsCache))

	// === 阶段0：历史数据预测过滤 ===
	// 使用历史数据快速预测，过滤出有潜力的商品，避免浪费时间在无机会的商品上
	log.Printf("[套利分析] ==================== 阶段0：历史数据预测过滤 ====================")
	filteredGoodIDs, filterStats := filterByHistoricalPrediction(goodIDs, goodsCache, predictionClient)
	log.Printf("[套利分析] 阶段0 统计: 总计 %d → 筛选后 %d (保留率 %.1f%%)",
		filterStats["total"], len(filteredGoodIDs), float64(len(filteredGoodIDs))/float64(filterStats["total"])*100)

	// 打印过滤通过的饰品信息
	if len(filteredGoodIDs) > 0 {
		log.Printf("[套利分析] ==================== 阶段0 通过的 %d 个饰品 ====================", len(filteredGoodIDs))
		for i, goodID := range filteredGoodIDs {
			if good, exists := goodsCache[goodID]; exists {
				log.Printf("[通过饰品 %d/%d] ID=%d, 名称=%s", i+1, len(filteredGoodIDs), goodID, good.Name)
			} else {
				log.Printf("[通过饰品 %d/%d] ID=%d (缓存中不存在)", i+1, len(filteredGoodIDs), goodID)
			}
		}
		log.Printf("[套利分析] ==================== 阶段0 通过饰品列表结束 ====================")
	}

	// === 第一阶段：仅对筛选后的商品获取最新价格 ===
	log.Printf("[套利分析] ==================== 第一阶段：获取筛选商品的最新价格（并发 %d 线程） ====================", *concurrency)

	phaseStartTime := time.Now()
	var candidateItems []struct {
		good                models.CSQAQGood
		currentBuyPrice     float64
		currentSellPrice    float64
		avgBuyPrice7d       float64
		avgSellPrice7d      float64
		buyOrderCount       int
		sellOrderCount      int
		daysOfData          int
		hasEnoughHistory    bool
		rankNum             *int // 热度排名
		historicalSnapshots []models.CSQAQGoodSnapshot
	}

	// 使用并发处理（仅处理筛选后的商品）
	candidateItems = processGoodsInParallel(db, ypClient, filteredGoodIDs, goodsCache, *concurrency)

	log.Printf("[套利分析] 第一阶段耗时: %.2f 秒，筛选完成! 候选项: %d 个",
		time.Since(phaseStartTime).Seconds(), len(candidateItems))

	// === 第二阶段：使用最新价格重新预测（分批并发） ===
	log.Printf("[套利分析] ==================== 第二阶段：基于最新价格的分批并发预测 ====================")
	phaseStartTime = time.Now()

	// 提取候选商品的ID列表用于二次预测
	goodIDsForFinalPrediction := make([]int64, 0, len(candidateItems))
	for _, candidate := range candidateItems {
		goodIDsForFinalPrediction = append(goodIDsForFinalPrediction, candidate.good.GoodID)
	}

	if len(goodIDsForFinalPrediction) == 0 {
		log.Printf("[套利分析] 没有符合条件的商品，分析结束")
		return
	}

	log.Printf("[二次预测] 开始对 %d 个候选商品进行二次预测（基于最新价格）...", len(goodIDsForFinalPrediction))

	// 使用小批量预测 + 高并发的方式（每批10个，20个线程，避免超时）
	predictions, successCount, errorCount := smallBatchPredictWithConcurrency(
		goodIDsForFinalPrediction,
		10, // 每批10个商品
		20, // 20个并发线程
		predictionClient,
		7,
	)

	log.Printf("[二次预测] 完成! 总计 %d，成功 %d，失败 %d，耗时: %.2f 秒",
		len(goodIDsForFinalPrediction), successCount, errorCount, time.Since(phaseStartTime).Seconds())

	// 以下代码为了兼容性保留，但不再使用原循环
	processedCount := len(goodIDs)
	skippedCount := len(goodIDs) - len(candidateItems)

	// 统计各种跳过原因（由于并发处理，这里无法精确统计各类原因，但可以给出总体统计）
	skipReasons := map[string]int{
		"类型过滤":  0,
		"无历史数据": 0,
		"价格无效":  0,
		"价格过高":  0,
		"价差异常":  0,
		"价格过低":  0,
		"无套利空间": 0,
		"流动性不足": 0,
	}
	realDataCount := len(candidateItems)
	estimatedDataCount := 0

	// 以下原有的 for 循环已被并发处理替代
	if false { // 保留代码结构用于参考
		for i, goodID := range goodIDs {
			time.Sleep(time.Millisecond * 100)
			// 每处理100个商品输出一次进度
			if i%100 == 0 && i > 0 {
				log.Printf("[第一阶段] 进度: %d/%d (%.1f%%), 已筛选 %d 个候选项, 跳过 %d 个",
					i, len(goodIDs), float64(i)/float64(len(goodIDs))*100, len(candidateItems), skippedCount)
			}
			processedCount++

			// 从缓存中获取商品信息
			good, exists := goodsCache[goodID]
			if !exists {
				skippedCount++
				continue
			}

			// 过滤掉非枪械饰品（刀、手套、贴纸/布章、角色/探员/特工、印花、挂件、纪念品、胶囊、音乐盒、钥匙、通行证、涂鸦等）
			name := good.Name
			lowerName := strings.ToLower(name)

			// 检测是否包含"挂件"或"纪念品"
			hasGuajian := strings.Contains(name, "挂件")
			hasJinianpin := strings.Contains(name, "纪念品")

			if strings.Contains(name, "★") || // 刀具（带星标）
				strings.Contains(name, "手套") ||
				strings.Contains(name, "贴纸") ||
				strings.Contains(name, "印花") ||
				strings.Contains(name, "胶囊") ||
				strings.Contains(name, "探员") ||
				strings.Contains(name, "音乐盒") ||
				strings.Contains(name, "钥匙") ||
				strings.Contains(name, "通行证") ||
				strings.Contains(name, "涂鸦") ||
				strings.Contains(name, "收藏品") ||
				strings.Contains(name, "武器箱") ||
				// 额外英文/别名过滤
				strings.Contains(name, "布章") ||
				strings.Contains(name, "特工") ||
				strings.Contains(name, "徽章") ||
				strings.Contains(name, "挂饰") ||
				strings.Contains(name, "缀饰") ||
				strings.Contains(name, "徽记") ||
				strings.Contains(name, "补丁") ||
				strings.Contains(name, "人偶") ||
				strings.Contains(name, "人形") ||
				strings.Contains(name, "代理人") ||
				strings.Contains(name, "人质") ||
				strings.Contains(name, "徽章包") ||
				strings.Contains(name, "补章") ||
				strings.Contains(name, "德拉戈米尔 | 军刀勇士") ||
				strings.Contains(name, "纪念包") ||
				// 英文小写匹配
				strings.Contains(lowerName, "sticker") ||
				strings.Contains(lowerName, "patch") ||
				strings.Contains(lowerName, "agent") ||
				strings.Contains(lowerName, "music kit") ||
				strings.Contains(lowerName, "souvenir") ||
				strings.Contains(lowerName, "case") ||
				strings.Contains(lowerName, "capsule") ||
				strings.Contains(lowerName, "graffiti") ||
				strings.Contains(lowerName, "key") ||
				strings.Contains(lowerName, "pass") ||
				hasGuajian ||
				hasJinianpin {
				skippedCount++
				skipReasons["类型过滤"]++
				continue
			}

			// 一次性获取7天内的所有历史快照（包含最新的）
			sevenDaysAgo := time.Now().AddDate(0, 0, -7)
			var historicalSnapshots []models.CSQAQGoodSnapshot
			if err := db.Where("good_id = ? AND created_at >= ?", goodID, sevenDaysAgo).
				Order("created_at DESC").
				Find(&historicalSnapshots).Error; err != nil {
				skippedCount++
				skipReasons["无历史数据"]++
				continue
			}
			// 计算当前价：优先使用YouPin实时价，失败再回退快照
			var currentBuyPrice, currentSellPrice float64
			var rtBuyCount, rtSellCount int
			var usedRealtime bool
			// 顺序获取实时价
			if rp, reason := fetchRealtimePrice(db, ypClient, nil, goodID, good.Name, *ypTimeoutSec); rp.ok {
				currentBuyPrice = rp.buy
				currentSellPrice = rp.sell
				rtBuyCount = rp.buyCount
				rtSellCount = rp.sellCount
				usedRealtime = true
			} else {
				// 回退快照
				if len(historicalSnapshots) == 0 {
					skippedCount++
					skipReasons["无历史数据"]++
					log.Printf("[套利分析][RT失败] good_id=%d name=%s reason=%s", goodID, good.Name, reason)
					continue
				}
				latestSnapshot := historicalSnapshots[0]
				if latestSnapshot.YYYPBuyPrice == nil || latestSnapshot.YYYPSellPrice == nil ||
					*latestSnapshot.YYYPBuyPrice <= 0 || *latestSnapshot.YYYPSellPrice <= 0 {
					skippedCount++
					skipReasons["价格无效"]++
					log.Printf("[套利分析][RT失败] good_id=%d name=%s reason=%s (fallback invalid)", goodID, good.Name, reason)
					continue
				}
				currentBuyPrice = *latestSnapshot.YYYPBuyPrice
				currentSellPrice = *latestSnapshot.YYYPSellPrice
			}

			// 已在上方声明 rtBuyCount/rtSellCount 统计实时数量

			// === 第一步：基础价格有效性检查 ===
			if currentBuyPrice <= 0 || currentSellPrice <= 0 {
				skippedCount++
				skipReasons["价格无效"]++
				continue
			}

			// === 第二步：价格上限检查（过滤天价商品）===
			if currentBuyPrice > *maxReasonablePrice || currentSellPrice > *maxReasonablePrice {
				skippedCount++
				skipReasons["价格过高"]++
				continue
			}

			// === 第三步：价格合理性检查（售价不应该远高于求购价）===
			// 正常情况下售价应该略高于求购价，如果售价是求购价的2倍以上，说明数据异常
			if currentSellPrice > currentBuyPrice*(*maxPriceSpread) {
				skippedCount++
				skipReasons["价差异常"]++
				continue
			}

			// === 第四步：价格下限检查（过滤过于便宜的商品，可能是垃圾）===
			if currentBuyPrice < 0.5 || currentSellPrice < 0.5 {
				skippedCount++
				skipReasons["价格过低"]++
				continue
			}

			// === 第五步：基本套利空间检查（必须有利润空间）===
			// 扣除1%手续费后，售价必须高于求购价
			var feeRate float64 = 0.01
			var netSellPrice float64 = currentSellPrice * (1 - feeRate)
			if netSellPrice <= currentBuyPrice {
				skippedCount++
				skipReasons["无套利空间"]++
				continue
			}

			// 获取求购和在售订单数量（优先使用实时数量；否则从快照读取；再否则估算）
			buyOrderCount := 0  // 求购数量
			sellOrderCount := 0 // 在售数量
			usingRealData := false

			if usedRealtime {
				buyOrderCount = rtBuyCount
				sellOrderCount = rtSellCount
				usingRealData = true
			} else if len(historicalSnapshots) > 0 && historicalSnapshots[0].YYYPSellCount != nil && *historicalSnapshots[0].YYYPSellCount > 0 {
				// 只关心真实的在售数量，如果没有就跳过
				sellOrderCount = *historicalSnapshots[0].YYYPSellCount
				// 买单数量可选，没有就估算为在售的35%
				if historicalSnapshots[0].YYYPBuyCount != nil && *historicalSnapshots[0].YYYPBuyCount > 0 {
					buyOrderCount = *historicalSnapshots[0].YYYPBuyCount
				} else {
					buyOrderCount = int(float64(sellOrderCount) * 0.35)
				}
				usingRealData = true
			} else {
				// 没有真实的在售数量就跳过，不用估算值
				skippedCount++
				skipReasons["无流动性数据"]++
				continue
			}

			// 跟踪数据来源
			if usingRealData {
				realDataCount++
			}
			if usingRealData {
				realDataCount++
			} else {
				estimatedDataCount++
			}

			// === 第六步：流动性检查（在售数量和求购数量）===
			if sellOrderCount < *minSellCount {
				skippedCount++
				skipReasons["流动性不足"]++
				continue
			}
			if buyOrderCount < *minBuyCount {
				skippedCount++
				skipReasons["流动性不足"]++
				continue
			}

			// 计算平均价格（如果历史数据足够）
			var avgBuyPrice7d, avgSellPrice7d float64
			hasEnoughHistory := len(historicalSnapshots) >= *minDaysHistory

			if hasEnoughHistory {
				var totalBuyPrice, totalSellPrice float64
				validBuyCount, validSellCount := 0, 0
				for _, snapshot := range historicalSnapshots {
					if snapshot.YYYPBuyPrice != nil && *snapshot.YYYPBuyPrice > 0 {
						totalBuyPrice += *snapshot.YYYPBuyPrice
						validBuyCount++
					}
					if snapshot.YYYPSellPrice != nil && *snapshot.YYYPSellPrice > 0 {
						totalSellPrice += *snapshot.YYYPSellPrice
						validSellCount++
					}
				}

				if validBuyCount > 0 && validSellCount > 0 {
					avgBuyPrice7d = totalBuyPrice / float64(validBuyCount)
					avgSellPrice7d = totalSellPrice / float64(validSellCount)
				}
			}

			// 计算利润率（考虑交易费用，假设1%手续费）
			feeRate = 0.01 // 1%手续费
			netSellPrice = currentSellPrice * (1 - feeRate)
			estimatedProfit := netSellPrice - currentBuyPrice
			profitRate := estimatedProfit / currentBuyPrice

			// 只记录有基本利润的商品（阈值可以放宽，留到后面筛选）
			if profitRate < *minProfitRate {
				skippedCount++
				continue
			}

			// 将符合基本条件的商品添加到候选列表
			candidateItems = append(candidateItems, struct {
				good                models.CSQAQGood
				currentBuyPrice     float64
				currentSellPrice    float64
				avgBuyPrice7d       float64
				avgSellPrice7d      float64
				buyOrderCount       int
				sellOrderCount      int
				daysOfData          int
				hasEnoughHistory    bool
				rankNum             *int // 热度排名
				historicalSnapshots []models.CSQAQGoodSnapshot
			}{
				good:                good,
				currentBuyPrice:     currentBuyPrice,
				currentSellPrice:    currentSellPrice,
				avgBuyPrice7d:       avgBuyPrice7d,
				avgSellPrice7d:      avgSellPrice7d,
				buyOrderCount:       buyOrderCount,
				sellOrderCount:      sellOrderCount,
				daysOfData:          len(historicalSnapshots),
				hasEnoughHistory:    hasEnoughHistory,
				rankNum:             nil,
				historicalSnapshots: historicalSnapshots,
			})
		}
	} // 关闭 if false 条件块

	// 输出统计信息
	log.Printf("[第一阶段] 筛选完成! 总计处理: %d, 候选项: %d, 跳过: %d",
		processedCount, len(candidateItems), skippedCount)
	log.Printf("[第一阶段] 数据来源: 真实数据 %d 个, 估算数据 %d 个", realDataCount, estimatedDataCount)

	// === 第三阶段：使用最新预测结果进行分析和评分 ===
	log.Printf("[套利分析] ==================== 第三阶段：最终分析与决策 ====================")
	var opportunities []models.ArbitrageOpportunity

	// 统计
	predictionCount := 0
	skipCount := 0

	for i, candidate := range candidateItems {
		if i%50 == 0 && i > 0 {
			log.Printf("[第三阶段] 进度: %d/%d (%.1f%%)",
				i, len(candidateItems), float64(i)/float64(len(candidateItems))*100)
		}

		currentBuyPrice := candidate.currentBuyPrice
		currentSellPrice := candidate.currentSellPrice
		historicalSnapshots := candidate.historicalSnapshots
		goodID := candidate.good.GoodID

		// 重新计算利润率
		var feeRate2 float64 = 0.01
		var netSellPrice2 float64 = currentSellPrice * (1 - feeRate2)
		estimatedProfit := netSellPrice2 - currentBuyPrice
		profitRate := estimatedProfit / currentBuyPrice

		// === 集成预测模型分析 ===
		// 获取该商品的预测结果
		prediction, hasPrediction := predictions[goodID]
		var forecastedPrice7d float64
		var predictionConfidence float64

		if hasPrediction && prediction != nil {
			// 有预测结果
			if ensemble, err := prediction.GetEnsembleForecast(); err == nil && len(ensemble) >= 7 {
				forecastedPrice7d = ensemble[6] // 第7天的预测价格
				if rec, err := prediction.GetRecommendation(); err == nil {
					predictionConfidence = rec.Confidence
				}
			}
		}

		// 如果有预测且预测未来价格会下跌，则跳过
		if hasPrediction && forecastedPrice7d > 0 && forecastedPrice7d < currentBuyPrice {
			log.Printf("[跳过] ID:%d 名称:%s | 预测价格下跌: 当前%.2f -> 7天后%.2f",
				goodID, candidate.good.Name, currentBuyPrice, forecastedPrice7d)
			skipCount++
			continue
		}

		// === 第二阶段：基础有效性检查 ===

		// 价格上限检查
		if currentBuyPrice > *maxReasonablePrice || currentSellPrice > *maxReasonablePrice {
			continue
		}

		// 价格下限检查
		if currentBuyPrice < 0.5 || currentSellPrice < 0.5 {
			continue
		}

		// 价差合理性检查
		if currentSellPrice > currentBuyPrice*(*maxPriceSpread) {
			continue
		}

		// 必须有实际利润
		if estimatedProfit <= 0 || profitRate <= 0 {
			continue
		}

		// === 使用预测模型确定价格趋势 ===
		priceTrend := "unknown"
		if hasPrediction && forecastedPrice7d > 0 {
			// 基于预测价格判断趋势
			priceDiff := (forecastedPrice7d - currentBuyPrice) / currentBuyPrice
			if priceDiff > 0.05 { // 预测上涨5%以上
				priceTrend = "up"
			} else if priceDiff < -0.05 { // 预测下跌5%以上
				priceTrend = "down"
			} else { // 预测变化在±5%以内
				priceTrend = "stable"
			}
		} else if candidate.hasEnoughHistory && len(historicalSnapshots) >= 3 {
			// 备用方案：使用历史数据的线性回归
			sellPrices := []float64{}
			for _, snapshot := range historicalSnapshots {
				if snapshot.YYYPSellPrice != nil && *snapshot.YYYPSellPrice > 0 {
					sellPrices = append(sellPrices, *snapshot.YYYPSellPrice)
				}
			}
			if len(sellPrices) >= 3 {
				priceTrend, _, _ = calculateTrendByLinearRegression(sellPrices)
			}
		}

		// === 简化的预测模型过滤 ===
		// 如果有预测结果，可以使用预测的置信度作为额外的过滤依据
		// 低置信度的预测结果应该更谨慎地对待
		if hasPrediction && predictionConfidence < 0.5 {
			log.Printf("[低置信度] ID:%d 名称:%s | 置信度: %.0f%%，谨慎对待",
				goodID, candidate.good.Name, predictionConfidence*100)
		}

		// === 风险评估（使用金融波动率模型）===
		riskLevel := "medium"
		priceVolatility := 0.0

		if candidate.hasEnoughHistory && len(historicalSnapshots) > 1 {
			// 有足够历史数据时，计算波动性
			prices := []float64{}
			for _, snapshot := range historicalSnapshots {
				if snapshot.YYYPSellPrice != nil && *snapshot.YYYPSellPrice > 0 {
					prices = append(prices, *snapshot.YYYPSellPrice)
				}
			}

			if len(prices) > 1 {
				// 使用变异系数（CV）评估波动性
				priceVolatility = calculateVolatility(prices)

				// 根据波动率分级（类似VIX指数）
				if priceVolatility < 0.08 { // 变异系数<8%，低波动
					riskLevel = "low"
				} else if priceVolatility > 0.15 { // 变异系数>15%，高波动
					riskLevel = "high"
				} else {
					riskLevel = "medium"
				}
			}
		} else {
			// 历史数据不足时，根据市场指标判断风险
			// 类似使用Beta系数评估相对风险
			marketScore := 0.0

			// 流动性指标（流动性好=风险低）
			if candidate.buyOrderCount >= 50 {
				marketScore += 2.0
			}
			if candidate.sellOrderCount >= 150 {
				marketScore += 2.0
			}

			// 利润率指标（利润高但要合理）
			if profitRate >= 0.1 && profitRate <= 0.25 {
				marketScore += 2.0
			}

			// 主战武器降低风险
			if isMainWeapon(candidate.good.Name) {
				marketScore += 1.0
			}

			// 崭新磨损降低风险
			if getWearScore(candidate.good.Name) >= 4.0 {
				marketScore += 1.0
			}

			if marketScore >= 6.0 {
				riskLevel = "low"
			} else if marketScore <= 3.0 {
				riskLevel = "high"
			} else {
				riskLevel = "medium"
			}
		}

		// 异常利润率风险调整（类似过高PE的股票）
		if profitRate > 0.30 {
			// 利润率过高可能是数据异常或高风险机会
			riskLevel = "high"
		}

		// StatTrak™ 风险调整（波动更大，流动性相对较差）
		if strings.Contains(candidate.good.Name, "StatTrak") || strings.Contains(candidate.good.Name, "StatTrak™") {
			// StatTrak版本提升风险等级
			if riskLevel == "low" {
				riskLevel = "medium"
			} else if riskLevel == "medium" {
				// 如果同时是破损/战痕，则升高风险
				wearScoreForRisk := getWearScore(candidate.good.Name)
				if wearScoreForRisk <= 2.0 {
					riskLevel = "high"
				}
			}
		}

		// 计算推荐求购价格（略高于当前最高求购价以提高成交率）
		recommendedBuyPrice := currentBuyPrice * 1.01 // 比当前最高求购高1%

		// 计算推荐求购数量（不在这里计算，后面统一分配预算）
		recommendedQuantity := 0

		opportunity := models.ArbitrageOpportunity{
			GoodID:              candidate.good.GoodID,
			GoodName:            candidate.good.Name,
			CurrentBuyPrice:     currentBuyPrice,
			CurrentSellPrice:    currentSellPrice,
			ProfitRate:          profitRate,
			EstimatedProfit:     estimatedProfit,
			AvgBuyPrice7d:       candidate.avgBuyPrice7d,
			AvgSellPrice7d:      candidate.avgSellPrice7d,
			PriceTrend:          priceTrend,
			DaysOfData:          candidate.daysOfData,
			RiskLevel:           riskLevel,
			BuyOrderCount:       candidate.buyOrderCount,
			SellOrderCount:      candidate.sellOrderCount,
			RankNum:             candidate.rankNum, // 热度排名
			RecommendedBuyPrice: recommendedBuyPrice,
			RecommendedQuantity: recommendedQuantity,
			AnalysisTime:        analysisTime,
		}

		// 计算并保存综合评分（四舍五入到1位小数，确保非负）
		s := calculateScore(opportunity)
		if s < 0 {
			s = 0
		}
		opportunity.Score = math.Round(s*10) / 10

		// 打印包含评分的关键信息，便于观察每个候选项
		logMsg := fmt.Sprintf("[评分] ID:%d 名称:%s | 利润率:%.1f%% | 趋势:%s | 风险:%s | 分数:%.1f",
			opportunity.GoodID,
			opportunity.GoodName,
			opportunity.ProfitRate*100,
			opportunity.PriceTrend,
			opportunity.RiskLevel,
			opportunity.Score,
		)
		if hasPrediction && forecastedPrice7d > 0 {
			logMsg += fmt.Sprintf(" | 预测7天价: %.2f元 (置信度:%.0f%%)", forecastedPrice7d, predictionConfidence*100)
			predictionCount++
		}
		log.Printf(logMsg)

		opportunities = append(opportunities, opportunity)
	}

	log.Printf("[第三阶段] 分析完成! 共计算出 %d 个套利机会，其中 %d 个使用了最新预测", len(opportunities), predictionCount)
	if skipCount > 0 {
		log.Printf("[第三阶段] 过滤统计: 预测价格下跌而跳过 %d 个", skipCount)
	}

	// 第四阶段：智能算法优化求购清单
	log.Printf("[套利分析] ==================== 第四阶段：优化求购清单 ====================")

	if len(opportunities) == 0 {
		log.Printf("[套利分析] 未发现符合条件的套利机会")
		return
	}

	// 按价格区间分组，确保各价格段都有代表
	priceRangeGroups := map[string][]models.ArbitrageOpportunity{
		"0-10":    {},
		"10-50":   {},
		"50-100":  {},
		"100-300": {},
		"300-500": {},
		"500+":    {},
	}

	for _, opp := range opportunities {
		price := opp.CurrentBuyPrice
		var rangeKey string
		if price < 10 {
			rangeKey = "0-10"
		} else if price < 50 {
			rangeKey = "10-50"
		} else if price < 100 {
			rangeKey = "50-100"
		} else if price < 300 {
			rangeKey = "100-300"
		} else if price < 500 {
			rangeKey = "300-500"
		} else {
			rangeKey = "500+"
		}
		priceRangeGroups[rangeKey] = append(priceRangeGroups[rangeKey], opp)
	}

	// 对每个价格区间内部按评分排序
	for rangeKey := range priceRangeGroups {
		sort.Slice(priceRangeGroups[rangeKey], func(i, j int) bool {
			scoreI := calculateScore(priceRangeGroups[rangeKey][i])
			scoreJ := calculateScore(priceRangeGroups[rangeKey][j])
			if scoreI == scoreJ {
				return priceRangeGroups[rangeKey][i].ProfitRate > priceRangeGroups[rangeKey][j].ProfitRate
			}
			return scoreI > scoreJ
		})
	}

	// 重新组合：采用轮询策略，确保各价格段都有机会
	rebalancedOpportunities := []models.ArbitrageOpportunity{}
	rangeOrder := []string{"100-300", "300-500", "500+", "50-100", "10-50", "0-10"} // 优先高价
	maxPerRound := 5                                                                // 每轮每个区间最多取5个

	for round := 0; round < 20; round++ { // 最多20轮
		addedThisRound := false
		for _, rangeKey := range rangeOrder {
			group := priceRangeGroups[rangeKey]
			startIdx := round * maxPerRound
			endIdx := startIdx + maxPerRound
			if endIdx > len(group) {
				endIdx = len(group)
			}
			if startIdx < len(group) {
				rebalancedOpportunities = append(rebalancedOpportunities, group[startIdx:endIdx]...)
				addedThisRound = true
			}
		}
		if !addedThisRound {
			break
		}
	}

	opportunities = rebalancedOpportunities

	log.Printf("[价格区间分布] 各价格段商品数量:")
	for _, rangeKey := range rangeOrder {
		log.Printf("  - %s元: %d个", rangeKey, len(priceRangeGroups[rangeKey]))
	}

	// 输出所有评分的商品（用于详细分析）
	log.Printf("[套利分析] ==================== 量化评分详情 (共 %d 个) ====================", len(opportunities))
	displayCount := len(opportunities) // 显示所有找到的机会

	log.Printf("%-4s %-50s %8s %6s %6s %8s %8s %6s",
		"排名", "商品名称", "综合评分", "类型", "磨损", "利润率", "趋势", "风险")
	log.Printf("%-4s %-50s %8s %6s %6s %8s %8s %6s",
		"----", "--------------------------------------------------", "--------", "------", "------", "--------", "--------", "------")

	for i := 0; i < displayCount; i++ {
		opp := opportunities[i]
		score := calculateScore(opp)
		weaponType := "普通"
		if isMainWeapon(opp.GoodName) {
			weaponType = "⭐主战"
		}
		wearScore := getWearScore(opp.GoodName)
		bidAskRatio := float64(opp.BuyOrderCount) / float64(opp.SellOrderCount+1)

		// 截断过长的商品名称
		displayName := opp.GoodName
		if len(displayName) > 48 {
			displayName = displayName[:45] + "..."
		}

		// 趋势图标
		trendIcon := ""
		switch opp.PriceTrend {
		case "up":
			trendIcon = "📈上涨"
		case "down":
			trendIcon = "📉下跌"
		case "stable":
			trendIcon = "━稳定"
		default:
			trendIcon = "？未知"
		}

		// 风险颜色标记
		riskIcon := ""
		switch opp.RiskLevel {
		case "low":
			riskIcon = "🟢低"
		case "medium":
			riskIcon = "🟡中"
		case "high":
			riskIcon = "🔴高"
		}

		// 计算市场周期阶段
		avgPrice := (opp.AvgBuyPrice7d + opp.AvgSellPrice7d) / 2.0
		cycleStageIcon := "━"
		priceDeviation := 0.0
		if avgPrice > 0 {
			priceDeviation = (opp.CurrentBuyPrice - avgPrice) / avgPrice
			if priceDeviation <= -0.05 {
				cycleStageIcon = "🟢底部" // 底部区域，最佳买入
			} else if priceDeviation <= -0.02 {
				cycleStageIcon = "🟢近底" // 接近底部
			} else if priceDeviation <= 0.02 && opp.PriceTrend == "up" {
				cycleStageIcon = "🟡初涨" // 上涨初期
			} else if priceDeviation > 0.06 {
				cycleStageIcon = "🔴高位" // 顶部区域
			} else {
				cycleStageIcon = "🟡震荡" // 震荡
			}
		}

		log.Printf("#%-3d %-50s %7.1f分 %6s %5.1f分 %6.1f%% %8s %6s",
			i+1, displayName, score, weaponType, wearScore, opp.ProfitRate*100, trendIcon, riskIcon)

		// 详细信息（第二行）- 新增市场周期信息
		log.Printf("     ID:%d | 求购:¥%.2f | 售价:¥%.2f | 买卖比:%.2f | 在售:%d | 求购:%d | 周期:%s(%.1f%%)",
			opp.GoodID, opp.CurrentBuyPrice, opp.CurrentSellPrice,
			bidAskRatio, opp.SellOrderCount, opp.BuyOrderCount, cycleStageIcon, priceDeviation*100)
	}
	log.Printf("==========================================================================")

	log.Printf("[求购计划] 总预算: ¥%.2f", *budget)
	log.Printf("[组合优化] 开始计算最优求购组合...")

	// ==================== 新增：组合优化算法 ====================
	// 尝试多种策略，选择利润最大的组合

	// 生成多个候选方案
	plans := []PurchasePlan{}

	// 方案1: 按评分排序（当前策略）
	plan1 := generatePurchasePlan(opportunities, *budget, "按评分优先")
	plans = append(plans, plan1)

	// 方案2: 按利润率排序
	sortedByProfitRate := make([]models.ArbitrageOpportunity, len(opportunities))
	copy(sortedByProfitRate, opportunities)
	sort.Slice(sortedByProfitRate, func(i, j int) bool {
		return sortedByProfitRate[i].ProfitRate > sortedByProfitRate[j].ProfitRate
	})
	plan2 := generatePurchasePlan(sortedByProfitRate, *budget, "按利润率优先")
	plans = append(plans, plan2)

	// 方案3: 按绝对利润排序
	sortedByAbsProfit := make([]models.ArbitrageOpportunity, len(opportunities))
	copy(sortedByAbsProfit, opportunities)
	sort.Slice(sortedByAbsProfit, func(i, j int) bool {
		profitI := sortedByAbsProfit[i].EstimatedProfit
		profitJ := sortedByAbsProfit[j].EstimatedProfit
		return profitI > profitJ
	})
	plan3 := generatePurchasePlan(sortedByAbsProfit, *budget, "按绝对利润优先")
	plans = append(plans, plan3)

	// 方案4: 性价比优先（利润率 * 价格，倾向于高价高利润率）
	sortedByValueRatio := make([]models.ArbitrageOpportunity, len(opportunities))
	copy(sortedByValueRatio, opportunities)
	sort.Slice(sortedByValueRatio, func(i, j int) bool {
		valueI := sortedByValueRatio[i].ProfitRate * sortedByValueRatio[i].CurrentBuyPrice
		valueJ := sortedByValueRatio[j].ProfitRate * sortedByValueRatio[j].CurrentBuyPrice
		return valueI > valueJ
	})
	plan4 := generatePurchasePlan(sortedByValueRatio, *budget, "按性价比优先")
	plans = append(plans, plan4)

	// 输出所有方案对比
	log.Printf("[方案对比] ==================== 共生成 %d 个方案 ====================", len(plans))
	for i, plan := range plans {
		log.Printf("[方案%d] %s:", i+1, plan.StrategyName)
		log.Printf("  - 总成本: ¥%.2f", plan.TotalCost)
		log.Printf("  - 预期利润: ¥%.2f", plan.TotalProfit)
		log.Printf("  - 利润率: %.2f%%", plan.ProfitRate*100)
		log.Printf("  - 商品种类: %d种", len(plan.Items))
		log.Printf("  - 总件数: %d件", plan.TotalItems)
	}

	// 选择利润最大的方案
	bestPlan := plans[0]
	bestPlanIndex := 0
	for i, plan := range plans {
		if plan.TotalProfit > bestPlan.TotalProfit {
			bestPlan = plan
			bestPlanIndex = i
		}
	}

	log.Printf("[最优方案] ✅ 方案%d（%s）利润最高: ¥%.2f", bestPlanIndex+1, bestPlan.StrategyName, bestPlan.TotalProfit)
	log.Printf("==========================================================================")

	// 使用最优方案
	purchaseList := []struct {
		GoodID   int64
		GoodName string
		Quantity int
		Price    float64
		Total    float64
	}{}

	for _, item := range bestPlan.Items {
		purchaseList = append(purchaseList, struct {
			GoodID   int64
			GoodName string
			Quantity int
			Price    float64
			Total    float64
		}{
			GoodID:   item.GoodID,
			GoodName: item.GoodName,
			Quantity: item.Quantity,
			Price:    item.Price,
			Total:    item.Total,
		})

		// 同时更新opportunities中的推荐数量
		for i := range opportunities {
			if opportunities[i].GoodID == item.GoodID {
				opportunities[i].RecommendedQuantity = item.Quantity
				break
			}
		}
	}

	totalBudget := *budget
	totalItems := bestPlan.TotalItems
	budgetUtilization := bestPlan.TotalCost / totalBudget
	log.Printf("[预算优化] 预算使用率: %.1f%%", budgetUtilization*100)
	log.Printf("[求购计划] 已分配: ¥%.2f / ¥%.2f (剩余: ¥%.2f)",
		bestPlan.TotalCost, *budget, *budget-bestPlan.TotalCost)
	log.Printf("[求购计划] 共计划求购 %d 个饰品，总计 %d 件", len(purchaseList), totalItems)

	// 保存所有套利机会到数据库（不只是前50个）
	log.Printf("[套利分析] 开始保存 %d 条套利机会到数据库...", len(opportunities))

	// 归档上一轮数据（使用结构体映射），归档成功后再清空当前表
	if err := archiveCurrentOpportunities(db); err != nil {
		log.Printf("[套利分析] 归档失败，跳过清空以避免数据丢失: %v", err)
	} else {
		log.Printf("[套利分析] 已归档上一轮数据到历史表")
		if err := db.Exec("TRUNCATE TABLE arbitrage_opportunities").Error; err != nil {
			log.Printf("[套利分析] TRUNCATE 失败，尝试 Delete All: %v", err)
			res := db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.ArbitrageOpportunity{})
			if res.Error != nil {
				log.Printf("[套利分析] 删除全部记录失败: %v", res.Error)
			} else {
				log.Printf("[套利分析] 已删除全部旧记录: %d 条", res.RowsAffected)
			}
		} else {
			log.Printf("[套利分析] 已清空表 arbitrage_opportunities")
		}
	}

	// 批量插入所有套利机会记录
	if err := db.CreateInBatches(opportunities, 100).Error; err != nil {
		log.Printf("[套利分析] 保存套利机会失败: %v", err)
	} else {
		log.Printf("[套利分析] 成功保存 %d 条套利机会", len(opportunities))
	}

	// ==================== 第三阶段：二次验证价格 ====================
	// 在生成套利清单后，通过OpenAPI再次获取实时价格，确保机会仍然有效
	log.Printf("[套利分析] ==================== 第三阶段：二次验证套利清单价格 ====================")
	// 只验证购买清单中的饰品（根据预算确定）
	verifiedOpportunities := verifyOpportunitiesPrices(db, ypClient, opportunities, purchaseList, *ypTimeoutSec)
	log.Printf("[套利分析] 验证完成! 原始 %d 个，购买清单 %d 个，验证通过 %d 个", len(opportunities), len(purchaseList), len(verifiedOpportunities))

	// 保留原始opportunities用于后续输出清单查询
	originalOpportunities := opportunities

	// 用验证后的机会清单替换原清单
	opportunities = verifiedOpportunities

	// 如果验证后没有有效的机会，则退出
	if len(opportunities) == 0 {
		log.Printf("[验证结果] 验证后没有符合条件的套利机会，本轮分析停止")
		return
	}

	// 创建最优求购计划（清单）
	if len(purchaseList) > 0 {
		totalCost := bestPlan.TotalCost
		plan := models.PurchasePlan{
			Budget:     *budget,
			TotalItems: totalItems,
			TotalCost:  totalCost,
			Status:     "pending",
		}

		// 保存计划主表
		if err := db.Create(&plan).Error; err != nil {
			log.Printf("[求购计划] 保存计划失败: %v", err)
		} else {
			log.Printf("[求购计划] 成功创建求购计划 #%d", plan.ID)

			// 获取YouPin账户用于搜索template_id
			var account models.YouPinAccount
			if err := db.Where("is_active = ?", true).First(&account).Error; err != nil {
				log.Printf("[求购计划] 未找到激活的悠悠账户，无法获取template_id: %v", err)
				log.Printf("[求购计划] 将保存good_id，执行时再获取template_id")
			}

			// 保存计划明细
			planItems := []models.PurchasePlanItem{}

			for _, item := range purchaseList {
				// 从opportunities中找到对应的风险等级和利润率
				var profitRate float64
				var riskLevel string

				// 首先从验证后的opportunities中查找
				found := false
				for _, opp := range opportunities {
					if opp.GoodID == item.GoodID {
						profitRate = opp.ProfitRate
						riskLevel = opp.RiskLevel
						found = true
						break
					}
				}

				// 如果在验证后列表中找不到，从原始opportunities中查找（作为备用）
				if !found {
					for _, opp := range originalOpportunities {
						if opp.GoodID == item.GoodID {
							profitRate = opp.ProfitRate
							riskLevel = opp.RiskLevel
							// 记录这个商品在验证后列表中缺失的情况
							log.Printf("[⚠️ 警告] 商品 %s (ID:%d) 在验证后的opportunities中缺失，使用原始数据。利润率:%.2f%% 可能已变化", item.GoodName, item.GoodID, profitRate*100)
							break
						}
					}
				}

				// 注意：OpenAPI不支持搜索功能，template_id需要从snapshot表获取或执行时再查询
				var yyypTemplateID *int64
				// 尝试从CSQAQ商品快照表获取template_id
				var snapshot models.CSQAQGoodSnapshot
				if err := db.Where("good_id = ? AND yyyp_template_id IS NOT NULL", item.GoodID).
					Order("created_at DESC").
					First(&snapshot).Error; err == nil && snapshot.YYYPTemplateID != nil {
					yyypTemplateID = snapshot.YYYPTemplateID
					log.Printf("[求购计划] 从快照获取商品 %s 的YouPin TemplateID: %d", item.GoodName, *yyypTemplateID)
				}

				planItems = append(planItems, models.PurchasePlanItem{
					PlanID:         plan.ID,
					GoodID:         item.GoodID,
					YYYPTemplateID: yyypTemplateID,
					GoodName:       item.GoodName,
					BuyPrice:       item.Price,
					Quantity:       item.Quantity,
					Subtotal:       item.Total,
					ProfitRate:     profitRate,
					RiskLevel:      riskLevel,
				})

				// 避免请求过快
				time.Sleep(200 * time.Millisecond)
			}

			if err := db.CreateInBatches(planItems, 100).Error; err != nil {
				log.Printf("[求购计划] 保存计划明细失败: %v", err)
			} else {
				log.Printf("[求购计划] 成功保存 %d 条计划明细", len(planItems))
			}
		}

		// 输出完整的求购清单（格式化表格）
		log.Printf("\n[求购清单] ==================== 最优求购清单 ====================")
		log.Printf("%-4s %-50s %8s %6s %10s %8s %6s",
			"序号", "商品名称", "ID", "数量", "单价", "小计", "风险")
		log.Printf("%-4s %-50s %8s %6s %10s %8s %6s",
			"----", "--------------------------------------------------", "--------", "------", "----------", "--------", "------")

		for i, item := range purchaseList {
			// 找到对应的机会详情
			var profitRate float64
			var riskLevel string
			var priceTrend string
			var avgBuyPrice7d float64
			var avgSellPrice7d float64
			var currentSellPrice float64

			// 首先从验证后的opportunities中查找
			found := false
			for _, opp := range opportunities {
				if opp.GoodID == item.GoodID {
					profitRate = opp.ProfitRate
					riskLevel = opp.RiskLevel
					priceTrend = opp.PriceTrend
					avgBuyPrice7d = opp.AvgBuyPrice7d
					avgSellPrice7d = opp.AvgSellPrice7d
					currentSellPrice = opp.CurrentSellPrice
					found = true
					break
				}
			}

			// 如果在验证后列表中找不到，从原始opportunities中查找（作为备用）
			if !found {
				for _, opp := range originalOpportunities {
					if opp.GoodID == item.GoodID {
						profitRate = opp.ProfitRate
						riskLevel = opp.RiskLevel
						priceTrend = opp.PriceTrend
						avgBuyPrice7d = opp.AvgBuyPrice7d
						avgSellPrice7d = opp.AvgSellPrice7d
						currentSellPrice = opp.CurrentSellPrice
						// 记录这个商品在验证后列表中缺失的情况
						log.Printf("[⚠️ 警告] 商品 %s (ID:%d) 在验证后的opportunities中缺失，使用原始数据。利润率:%.2f%% 可能已变化", item.GoodName, item.GoodID, profitRate*100)
						break
					}
				}
			}

			// 截断商品名称
			displayName := item.GoodName
			if len(displayName) > 48 {
				displayName = displayName[:45] + "..."
			}

			// 风险图标
			riskIcon := ""
			switch riskLevel {
			case "low":
				riskIcon = "🟢低"
			case "medium":
				riskIcon = "🟡中"
			case "high":
				riskIcon = "🔴高"
			}

			// 趋势图标
			trendIcon := ""
			switch priceTrend {
			case "up":
				trendIcon = "📈"
			case "down":
				trendIcon = "📉"
			case "stable":
				trendIcon = "━"
			}

			// 计算周期阶段
			cycleStage := "━"
			avgPrice := (avgBuyPrice7d + avgSellPrice7d) / 2.0
			priceDeviation := 0.0
			if avgPrice > 0 {
				priceDeviation = (item.Price - avgPrice) / avgPrice
				if priceDeviation <= -0.05 {
					cycleStage = "🟢底部买入"
				} else if priceDeviation <= -0.02 {
					cycleStage = "🟢近底买入"
				} else if priceDeviation <= 0.02 && priceTrend == "up" {
					cycleStage = "🟡初涨买入"
				} else {
					cycleStage = "🟡正常买入"
				}
			}

			log.Printf("#%-3d %-50s %8d %5d件 %9.2f元 %7.2f元 %6s",
				i+1, displayName, item.GoodID, item.Quantity, item.Price, item.Total, riskIcon)

			// 计算单品预期利润: (售价*0.99 - 买价) * 数量
			singleItemProfit := (currentSellPrice*0.99 - item.Price) * float64(item.Quantity)

			log.Printf("     利润率:%.1f%% | 趋势:%s | 周期:%s(%.1f%%) | 预期利润:¥%.2f",
				profitRate*100, trendIcon+priceTrend, cycleStage, priceDeviation*100, singleItemProfit)
		}

		log.Printf("==========================================================================")
		log.Printf("💰 总投入: ¥%.2f | 📦 总件数: %d 件 | 📊 平均单价: ¥%.2f",
			totalCost, totalItems, totalCost/float64(totalItems))

		// 计算总预期利润
		totalExpectedProfit := 0.0
		for _, item := range purchaseList {
			for _, opp := range opportunities {
				if opp.GoodID == item.GoodID {
					// 预期利润 = (售价*(1-手续费) - 求购价) * 数量
					// 手续费率1%，扣除后为99%
					profit := (opp.CurrentSellPrice*0.99 - item.Price) * float64(item.Quantity)
					totalExpectedProfit += profit
					break
				}
			}
		}

		totalProfitRate := totalExpectedProfit / totalCost * 100
		log.Printf("📈 预期总利润: ¥%.2f | 预期收益率: %.1f%%",
			totalExpectedProfit, totalProfitRate)
		log.Printf("==========================================================================")
	}

	elapsed := time.Since(startTime)
	log.Printf("[套利分析] 本轮分析耗时: %v", elapsed)
	log.Printf("[套利分析] ==================== 分析完成 ====================")
}

// fetchRealtimePrice 获取YouPin实时最高求购价与最低在售价（带可选限速）
// 现在使用OpenAPI接口获取更准确的价格信息
func fetchRealtimePrice(db *gorm.DB, ypClient *youpin.OpenAPIClient, limiter <-chan time.Time, goodID int64, goodName string, timeoutSec int) (out struct {
	buy       float64
	sell      float64
	buyCount  int
	sellCount int
	ok        bool
}, reason string) {
	if ypClient == nil {
		reason = "ypClient is nil"
		return
	}
	// 获取模板ID：优先快照，其次不再需要搜索（OpenAPI接口需要准确的模板ID）
	var templateID int64
	var snap models.CSQAQGoodSnapshot
	if err := db.Where("good_id = ?", goodID).Order("created_at DESC").First(&snap).Error; err == nil && snap.YYYPTemplateID != nil && *snap.YYYPTemplateID > 0 {
		templateID = *snap.YYYPTemplateID
	} else {
		// 如果没有快照中的模板ID，返回失败
		reason = "no template id in snapshot"
		return
	}
	if templateID == 0 {
		reason = "no template id"
		return
	}

	// 最高求购价 - 使用Token认证的求购接口
	maxBuy := 0.0
	if limiter != nil {
		<-limiter
	}
	ctx1, cancel1 := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel1()

	// 使用OpenAPIClient的求购接口（内部使用Token认证）
	purchaseReq := &youpin.GetTemplatePurchaseOrderListRequest{
		TemplateId:       int(templateID),
		PageIndex:        1,
		PageSize:         50,
		ShowMaxPriceFlag: false,
	}
	if po, err := ypClient.GetTemplatePurchaseOrderList(ctx1, purchaseReq); err == nil && po != nil {
		for _, item := range po.Data {
			if item.PurchasePrice > maxBuy {
				maxBuy = item.PurchasePrice
			}
		}
		out.buyCount = len(po.Data)
	} else if err != nil {
		reason = "get purchase list failed: " + err.Error()
	}

	// 最低在售价 - 使用OpenAPI签名认证接口
	lowestSell := 0.0
	if limiter != nil {
		<-limiter
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel2()

	// 使用BatchGetOnSaleCommodityInfo获取在售价格
	tempID := int(templateID)
	requestList := []youpin.BatchPriceQueryItem{
		{TemplateID: &tempID},
	}
	if priceResp, err := ypClient.BatchGetOnSaleCommodityInfo(ctx2, requestList); err == nil && priceResp != nil && len(priceResp.Data) > 0 {
		// 解析最低在售价
		if minPrice, parseErr := strconv.ParseFloat(priceResp.Data[0].SaleCommodityResponse.MinSellPrice, 64); parseErr == nil {
			lowestSell = minPrice
		}
		// 解析在售数量
		out.sellCount = priceResp.Data[0].SaleCommodityResponse.SellNum
	} else if err != nil {
		if reason != "" {
			reason += "; "
		}
		reason += "get market price failed: " + err.Error()
	}

	if maxBuy > 0 && lowestSell > 0 {
		out.buy = maxBuy
		out.sell = lowestSell
		out.ok = true
		reason = ""
	} else {
		if reason == "" {
			reason = "invalid realtime prices"
		}
	}
	return
}

// getLatestMaxBuyPrice 仅获取指定商品模板的最新最高求购价
func getLatestMaxBuyPrice(db *gorm.DB, ypClient *youpin.OpenAPIClient, goodID int64, timeoutSec int) (float64, error) {
	if ypClient == nil {
		return 0, fmt.Errorf("ypClient is nil")
	}
	// 获取模板ID
	var snap models.CSQAQGoodSnapshot
	if err := db.Where("good_id = ?", goodID).Order("created_at DESC").First(&snap).Error; err != nil || snap.YYYPTemplateID == nil || *snap.YYYPTemplateID == 0 {
		return 0, fmt.Errorf("no template id for good %d", goodID)
	}
	templateID := int(*snap.YYYPTemplateID)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	req := &youpin.GetTemplatePurchaseOrderListRequest{
		TemplateId:       templateID,
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

// bumpPurchasePrice 按区间步进规则，将最高求购价加一个最小步进并保留对应精度
// 区间：
//
//	0～1: 步进0.01；1～50: 0.1；50～1000: 1
//
// 示例：39 => 39.1；51 => 52
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
	// 向下对齐到步进倍数，再+一步
	base := math.Floor(maxBuy/step) * step
	bumped := base + step
	// 规整小数位
	pow := math.Pow(10, decimals)
	return math.Round(bumped*pow) / pow
}

// placeImmediatePurchaseOrder 根据模板ID信息发起立即求购下单
// 流程：获取模板求购信息 -> 预检查 -> 创建订单（处理价格警告/重复订单确认）
func placeImmediatePurchaseOrder(db *gorm.DB, ypClient *youpin.OpenAPIClient, goodID int64, goodName string, quantity int, purchasePrice float64, timeoutSec int) error {
	if ypClient == nil {
		return fmt.Errorf("ypClient is nil")
	}
	// 获取模板ID
	var snap models.CSQAQGoodSnapshot
	if err := db.Where("good_id = ?", goodID).Order("created_at DESC").First(&snap).Error; err != nil || snap.YYYPTemplateID == nil || *snap.YYYPTemplateID == 0 {
		return fmt.Errorf("no template id for good %d", goodID)
	}
	templateIDStr := fmt.Sprintf("%d", *snap.YYYPTemplateID)

	// 获取模板求购信息（包含hashName、参考价、最小在售价、最大求购价）
	ctxInfo, cancelInfo := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancelInfo()
	info, err := ypClient.GetTemplatePurchaseInfo(ctxInfo, templateIDStr)
	if err != nil || info == nil {
		if err != nil {
			return fmt.Errorf("get template purchase info failed: %w", err)
		}
		return fmt.Errorf("get template purchase info failed: empty response")
	}
	tpl := info.Data.TemplateInfo

	// 预检查
	totalAmount := purchasePrice * float64(quantity)
	minSell, _ := strconv.ParseFloat(tpl.MinSellPrice, 64)
	maxPurchase, _ := strconv.ParseFloat(tpl.MaxPurchasePrice, 64)

	preReq := &youpin.PrePurchaseOrderCheckRequest{
		SpecialStyleObj:      map[string]interface{}{},
		IsCheckMaxPrice:      false,
		TemplateHashName:     tpl.TemplateHashName,
		TotalAmount:          totalAmount,
		ReferencePrice:       tpl.ReferencePrice,
		PurchasePrice:        purchasePrice,
		PurchaseNum:          quantity,
		DiscountAmount:       0,
		MinSellPrice:         minSell,
		MaxPurchasePrice:     maxPurchase,
		TemplateId:           templateIDStr,
		IncrementServiceCode: nil,
	}

	ctxPre, cancelPre := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	_, _ = ypClient.PrePurchaseOrderCheck(ctxPre, preReq) // 失败也尝试走保存逻辑，由保存接口处理确认
	cancelPre()

	// 首次创建订单
	templateIDInt := tpl.TemplateId
	saveReq := &youpin.SavePurchaseOrderRequest{
		TemplateId:            templateIDInt,
		TemplateHashName:      tpl.TemplateHashName,
		CommodityName:         tpl.CommodityName,
		ReferencePrice:        tpl.ReferencePrice,
		MinSellPrice:          tpl.MinSellPrice,
		MaxPurchasePrice:      tpl.MaxPurchasePrice,
		PurchasePrice:         purchasePrice,
		PurchaseNum:           quantity,
		NeedPaymentAmount:     totalAmount,
		TotalAmount:           totalAmount,
		TemplateName:          tpl.CommodityName,
		PriceDifference:       0,
		DiscountAmount:        0,
		PayConfirmFlag:        false,
		RepeatOrderCancelFlag: false,
	}

	ctxSave, cancelSave := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	resp, err := ypClient.SavePurchaseOrder(ctxSave, saveReq)
	cancelSave()
	if err == nil && resp != nil {
		return nil
	}

	// 错误处理：尝试处理重复订单确认与价格警告
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "REPEAT_ORDER_CONFIRM") {
			saveReq.RepeatOrderCancelFlag = true
			ctx1, c1 := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
			resp, err = ypClient.SavePurchaseOrder(ctx1, saveReq)
			c1()
			if err == nil && resp != nil {
				return nil
			}
			if err != nil && strings.Contains(err.Error(), "PRICE_WARNING") {
				saveReq.PayConfirmFlag = true
				ctx2, c2 := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
				resp, err = ypClient.SavePurchaseOrder(ctx2, saveReq)
				c2()
				if err == nil && resp != nil {
					return nil
				}
			}
		} else if strings.Contains(msg, "PRICE_WARNING") {
			saveReq.PayConfirmFlag = true
			ctx3, c3 := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
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

// archiveCurrentOpportunities copies current arbitrage_opportunities rows
// into arbitrage_opportunities_history using struct mapping to avoid
// column mismatch issues. It runs in a transaction and only truncation
// should happen after successful archive elsewhere.
func archiveCurrentOpportunities(db *gorm.DB) error {
	var curr []models.ArbitrageOpportunity
	if err := db.Find(&curr).Error; err != nil {
		return err
	}
	if len(curr) == 0 {
		return nil
	}
	// Map to history slice
	hist := make([]models.ArbitrageOpportunityHistory, 0, len(curr))
	for _, r := range curr {
		hist = append(hist, models.ArbitrageOpportunityHistory{
			GoodID:              r.GoodID,
			GoodName:            r.GoodName,
			CurrentBuyPrice:     r.CurrentBuyPrice,
			CurrentSellPrice:    r.CurrentSellPrice,
			ProfitRate:          r.ProfitRate,
			EstimatedProfit:     r.EstimatedProfit,
			AvgBuyPrice7d:       r.AvgBuyPrice7d,
			AvgSellPrice7d:      r.AvgSellPrice7d,
			PriceTrend:          r.PriceTrend,
			DaysOfData:          r.DaysOfData,
			RiskLevel:           r.RiskLevel,
			BuyOrderCount:       r.BuyOrderCount,
			SellOrderCount:      r.SellOrderCount,
			RecommendedBuyPrice: r.RecommendedBuyPrice,
			RecommendedQuantity: r.RecommendedQuantity,
			Score:               r.Score,
			AnalysisTime:        r.AnalysisTime,
			CreatedAt:           r.CreatedAt,
			UpdatedAt:           r.UpdatedAt,
		})
	}
	tx := db.Begin()
	if err := tx.Error; err != nil {
		return err
	}
	if err := tx.CreateInBatches(hist, 200).Error; err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

// ============ 新增：高级技术指标计算模块 ============

// CalculateBollingerBands 计算布林带 (Bollinger Bands)
// 返回: (upper, mid, lower)
func CalculateBollingerBands(prices []float64, period int, numStdDev float64) (float64, float64, float64) {
	if len(prices) < period {
		return 0, 0, 0
	}

	// 使用最后 period 个价格计算
	recentPrices := prices[len(prices)-period:]

	// 计算中线（SMA）
	sum := 0.0
	for _, p := range recentPrices {
		sum += p
	}
	mid := sum / float64(period)

	// 计算标准差
	variance := 0.0
	for _, p := range recentPrices {
		diff := p - mid
		variance += diff * diff
	}
	variance /= float64(period)
	stdDev := math.Sqrt(variance)

	upper := mid + numStdDev*stdDev
	lower := mid - numStdDev*stdDev

	return upper, mid, lower
}

// CalculateRSI 计算相对强弱指标 (RSI)
func CalculateRSI(prices []float64, period int) float64 {
	if len(prices) < period+1 {
		return 50.0 // 数据不足，返回中性值
	}

	// 计算价格变化
	gains := 0.0
	losses := 0.0

	for i := len(prices) - period; i < len(prices); i++ {
		change := prices[i] - prices[i-1]
		if change > 0 {
			gains += change
		} else {
			losses += -change
		}
	}

	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	if avgLoss == 0 {
		if avgGain > 0 {
			return 100.0
		}
		return 50.0
	}

	rs := avgGain / avgLoss
	rsi := 100 - (100 / (1 + rs))

	return rsi
}

// CalculateMACD 计算MACD指标
// 返回: (macd_line, signal_line, histogram)
func CalculateMACD(prices []float64) (float64, float64, float64) {
	if len(prices) < 26 {
		return 0, 0, 0
	}

	// EMA12
	ema12 := calculateEMA(prices, 12)

	// EMA26
	ema26 := calculateEMA(prices, 26)

	// MACD = EMA12 - EMA26
	macLine := ema12 - ema26

	// Signal Line = EMA(MACD, 9)
	// 简化：使用最近的EMA值作为signal
	signalLine := (macLine + ema12) / 2

	histogram := macLine - signalLine

	return macLine, signalLine, histogram
}

// calculateEMA 计算指数移动平均线
func calculateEMA(prices []float64, period int) float64 {
	if len(prices) < period {
		return 0
	}

	// 初始SMA
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += prices[i]
	}
	ema := sum / float64(period)

	// 计算乘数
	multiplier := 2.0 / float64(period+1)

	// 计算后续EMA
	for i := period; i < len(prices); i++ {
		ema = prices[i]*multiplier + ema*(1-multiplier)
	}

	return ema
}

// MultiIndicatorSignal 5信号投票融合系统
type MultiIndicatorSignal struct {
	BollingerBand bool // 信号1: price < BB_lower
	RSIExtreme    bool // 信号2: RSI < 30
	MAXover       bool // 信号3: MA5 > MA20（上涨信号）
	ConsecutiveMA bool // 信号4: 连续3天 price < MA20
	MACDCross     bool // 信号5: MACD > Signal（动能转正）
}

// EvaluateMultiIndicators 综合评估5个技术指标，返回是否满足买入条件
func EvaluateMultiIndicators(prices []float64, sellPrices []float64) MultiIndicatorSignal {
	signal := MultiIndicatorSignal{
		BollingerBand: false,
		RSIExtreme:    false,
		MAXover:       false,
		ConsecutiveMA: false,
		MACDCross:     false,
	}

	if len(prices) < 20 || len(sellPrices) < 20 {
		return signal
	}

	// 信号1: 布林带策略 - price < BB_lower
	if len(sellPrices) >= 20 {
		_, _, lower := CalculateBollingerBands(sellPrices, 20, 2.0)
		currentPrice := sellPrices[len(sellPrices)-1]
		if currentPrice < lower && lower > 0 {
			signal.BollingerBand = true
		}
	}

	// 信号2: RSI极端值 - RSI < 30
	if len(sellPrices) >= 14 {
		rsi := CalculateRSI(sellPrices, 14)
		if rsi < 30 {
			signal.RSIExtreme = true
		}
	}

	// 信号3: MA交叉 - MA5 > MA20
	if len(sellPrices) >= 20 {
		ma5 := calculateSimpleMA(sellPrices, 5)
		ma20 := calculateSimpleMA(sellPrices, 20)
		if ma5 > ma20 {
			signal.MAXover = true
		}
	}

	// 信号4: 连续3天低于MA20
	if len(sellPrices) >= 20 {
		ma20 := calculateSimpleMA(sellPrices, 20)
		consecutiveCount := 0
		for i := len(sellPrices) - 3; i < len(sellPrices); i++ {
			if i >= 0 && sellPrices[i] < ma20 {
				consecutiveCount++
			}
		}
		if consecutiveCount >= 3 {
			signal.ConsecutiveMA = true
		}
	}

	// 信号5: MACD金叉
	if len(sellPrices) >= 26 {
		// 获取当前和前一个时间点的MACD
		currMACD, currSignal, _ := CalculateMACD(sellPrices)
		// 前一个时间点（简化：使用历史数据的最后一个）
		prevPrices := sellPrices[:len(sellPrices)-1]
		if len(prevPrices) >= 26 {
			prevMACD, prevSignal, _ := CalculateMACD(prevPrices)
			// 金叉：前一个MACD < Signal，当前MACD > Signal
			if prevMACD < prevSignal && currMACD > currSignal {
				signal.MACDCross = true
			}
		}
	}

	return signal
}

// calculateSimpleMA 计算简单移动平均
func calculateSimpleMA(prices []float64, period int) float64 {
	if len(prices) < period {
		return 0
	}
	sum := 0.0
	for i := len(prices) - period; i < len(prices); i++ {
		sum += prices[i]
	}
	return sum / float64(period)
}

// CheckBuySignal 根据投票融合规则判断是否应该买入
// 硬性条件：必须同时满足信号1和信号2
// 额外确认：信号3/4/5中至少2个满足
// 返回：(应该买入, 原因)
func CheckBuySignal(signal MultiIndicatorSignal) (bool, string) {
	// 硬性条件：必须同时满足布林带和RSI
	if !signal.BollingerBand || !signal.RSIExtreme {
		return false, "硬性条件不满足（需要布林带下轨+RSI超卖）"
	}

	// 额外确认：信号3/4/5中至少2个满足
	additionalSignals := 0
	if signal.MAXover {
		additionalSignals++
	}
	if signal.ConsecutiveMA {
		additionalSignals++
	}
	if signal.MACDCross {
		additionalSignals++
	}

	if additionalSignals < 2 {
		return false, fmt.Sprintf("额外确认不足（需要3/4/5中至少2个，当前满足%d个）", additionalSignals)
	}

	return true, "所有条件满足，推荐买入"
}

// ============ 市场风险检测模块 ============

// MarketRiskInfo 市场风险信息
type MarketRiskInfo struct {
	SuccessRate    float64 // 历史成功率
	AvgROI         float64 // 平均ROI
	RiskLevel      string  // 风险等级: "green", "yellow", "red"
	Recommendation string  // 建议
}

// DetectMarketRisk 检测市场整体风险
func DetectMarketRisk(db *gorm.DB) MarketRiskInfo {
	info := MarketRiskInfo{
		SuccessRate:    0.5,
		AvgROI:         0.0,
		RiskLevel:      "green",
		Recommendation: "市场状况正常，按标准策略执行",
	}

	// 查询最近7天的推荐记录
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)
	var recommendations []models.ArbitrageOpportunity

	if err := db.Where("analysis_time >= ? AND recommended_quantity > 0", sevenDaysAgo).
		Find(&recommendations).Error; err != nil || len(recommendations) == 0 {
		return info
	}

	// 统计成功率和ROI
	successCount := 0
	totalROI := 0.0

	for _, rec := range recommendations {
		// 检查该商品现在是否仍在推荐清单中（简化检查：是否还有正ROI预期）
		if rec.EstimatedProfit > 0 {
			successCount++
		}
		totalROI += rec.ProfitRate
	}

	info.SuccessRate = float64(successCount) / float64(len(recommendations))
	info.AvgROI = totalROI / float64(len(recommendations))

	// 风险等级判断
	if info.AvgROI < 0 {
		info.RiskLevel = "red"
		info.Recommendation = "🔴市场风险警告：平均ROI为负，自动提升推荐阈值"
	} else if info.SuccessRate < 0.4 {
		info.RiskLevel = "yellow"
		info.Recommendation = "🟡市场警告：成功率低于40%，建议提升利润率阈值"
	} else if info.AvgROI > 0.10 {
		info.RiskLevel = "green"
		info.Recommendation = "✅市场状况良好，平均ROI>10%"
	}

	return info
}

// ============ 自适应策略调整模块 ============

// StrategyAdjustment 策略调整记录
type StrategyAdjustment struct {
	Time      time.Time
	Reason    string
	OldParams map[string]interface{}
	NewParams map[string]interface{}
}

var strategyAdjustmentLog []StrategyAdjustment

// ApplyAdaptiveAdjustment 根据市场风险应用自适应策略调整
func ApplyAdaptiveAdjustment(marketRisk MarketRiskInfo) {
	adjustment := StrategyAdjustment{
		Time:      time.Now(),
		OldParams: make(map[string]interface{}),
		NewParams: make(map[string]interface{}),
	}

	shouldAdjust := false

	// 黄色预警：提升利润率阈值
	if marketRisk.RiskLevel == "yellow" && *minProfitRate < 0.10 {
		adjustment.OldParams["minProfitRate"] = *minProfitRate
		*minProfitRate = 0.10
		adjustment.NewParams["minProfitRate"] = 0.10
		adjustment.Reason = fmt.Sprintf("黄色预警 - 成功率%.1f%% < 40%%", marketRisk.SuccessRate*100)
		shouldAdjust = true
	}

	// 红色预警：大幅提升阈值
	if marketRisk.RiskLevel == "red" {
		adjustment.OldParams["minProfitRate"] = *minProfitRate
		*minProfitRate = 0.12
		adjustment.NewParams["minProfitRate"] = 0.12
		shouldAdjust = true

		// 降低波动率阈值（选择更稳定的商品）
		adjustment.Reason = fmt.Sprintf("红色预警 - 市场平均ROI%.2f%% < 0", marketRisk.AvgROI*100)
	}

	if shouldAdjust {
		strategyAdjustmentLog = append(strategyAdjustmentLog, adjustment)
		log.Printf("[自适应调整] %s", adjustment.Reason)
		log.Printf("  - 利润率阈值: %.2f%% -> %.2f%%",
			adjustment.OldParams["minProfitRate"],
			adjustment.NewParams["minProfitRate"])
	}
}

// SaveAdjustmentLog 保存策略调整日志
func SaveAdjustmentLog(filepath string) error {
	if len(strategyAdjustmentLog) == 0 {
		return nil
	}

	var output strings.Builder
	output.WriteString("策略自适应调整日志\n")
	output.WriteString("==========================================\n")

	for _, adj := range strategyAdjustmentLog {
		output.WriteString(fmt.Sprintf("\n【%s】\n", adj.Time.Format("2006-01-02 15:04:05")))
		output.WriteString(fmt.Sprintf("原因: %s\n", adj.Reason))
		output.WriteString("参数变更:\n")
		for key, oldVal := range adj.OldParams {
			newVal := adj.NewParams[key]
			output.WriteString(fmt.Sprintf("  %s: %v -> %v\n", key, oldVal, newVal))
		}
	}

	return os.WriteFile(filepath, []byte(output.String()), 0644)
}

// batchPredictWithConcurrency 分批并发预测，每批最多50个商品，控制并发量
// 参数说明：
// - goodIDs: 要预测的商品ID列表
// - batchSize: 每批的大小（建议50-100，平衡API效率和实时性）
// - concurrency: 并发批数（建议2-4，避免过多并发导致服务压力和实时性问题）
// - predictionClient: 预测客户端
// - days: 预测天数
func batchPredictWithConcurrency(
	goodIDs []int64,
	batchSize int,
	concurrency int,
	predictionClient *services.PredictionClient,
	days int,
) (map[int64]*services.PredictionResult, int, int) {
	if batchSize < 1 || batchSize > 100 {
		batchSize = 50 // 默认50
	}
	if concurrency < 1 || concurrency > 10 {
		concurrency = 2 // 默认2个并发
	}

	totalGoodIDs := len(goodIDs)
	if totalGoodIDs == 0 {
		return make(map[int64]*services.PredictionResult), 0, 0
	}

	// 计算需要多少批
	numBatches := (totalGoodIDs + batchSize - 1) / batchSize
	log.Printf("[分批并发预测] 共 %d 个商品，批大小 %d，共需 %d 批，并发数 %d",
		totalGoodIDs, batchSize, numBatches, concurrency)

	// 准备批次
	type batchJob struct {
		batchIdx  int
		batchGIDs []int64
		startIdx  int
		endIdx    int
	}
	batches := make([]batchJob, 0, numBatches)
	for i := 0; i < totalGoodIDs; i += batchSize {
		end := i + batchSize
		if end > totalGoodIDs {
			end = totalGoodIDs
		}
		batches = append(batches, batchJob{
			batchIdx:  len(batches),
			batchGIDs: goodIDs[i:end],
			startIdx:  i,
			endIdx:    end,
		})
	}

	// 使用信道处理并发
	type resultJob struct {
		batchIdx int
		results  map[int64]*services.PredictionResult
		err      error
	}

	jobsChan := make(chan batchJob, numBatches)
	resultsChan := make(chan resultJob, numBatches)

	// 启动并发预测工作者
	var wg sync.WaitGroup
	for w := 0; w < concurrency && w < numBatches; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for job := range jobsChan {
				batchStartTime := time.Now()
				log.Printf("[批次 %d/%d] Worker-%d: 预测 %d 个商品 (IDs: %d-%d)...",
					job.batchIdx+1, numBatches, workerID, len(job.batchGIDs), job.startIdx+1, job.endIdx)

				results, err := predictionClient.BatchPredict(job.batchGIDs, days)
				if err != nil {
					log.Printf("[批次 %d] ⚠️ 预测失败: %v", job.batchIdx+1, err)
					resultsChan <- resultJob{
						batchIdx: job.batchIdx,
						results:  make(map[int64]*services.PredictionResult),
						err:      err,
					}
				} else {
					log.Printf("[批次 %d] ✓ 完成，耗时 %.2f 秒，成功 %d 个",
						job.batchIdx+1, time.Since(batchStartTime).Seconds(), len(results))
					resultsChan <- resultJob{
						batchIdx: job.batchIdx,
						results:  results,
						err:      nil,
					}
				}

				// 控制请求速率，避免过快
				time.Sleep(100 * time.Millisecond)
			}
		}(w)
	}

	// 发送批次任务到信道
	go func() {
		for _, batch := range batches {
			jobsChan <- batch
		}
		close(jobsChan)
	}()

	// 收集结果
	allResults := make(map[int64]*services.PredictionResult)
	successCount := 0
	errorCount := 0

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	for result := range resultsChan {
		if result.err == nil {
			for goodID, predResult := range result.results {
				allResults[goodID] = predResult
				successCount++
			}
		} else {
			errorCount++
		}
	}

	return allResults, successCount, errorCount
}

// smallBatchPredictWithConcurrency 使用小批量预测 API (每批10个商品) + 高并发的方式预测多个商品
// 这种方式平衡了单个预测的慢和大批量预测的超时问题
func smallBatchPredictWithConcurrency(
	goodIDs []int64,
	batchSize int,
	numWorkers int,
	predictionClient *services.PredictionClient,
	days int,
) (map[int64]*services.PredictionResult, int, int) {
	if len(goodIDs) == 0 {
		return make(map[int64]*services.PredictionResult), 0, 0
	}

	if batchSize < 1 || batchSize > 50 {
		batchSize = 10 // 默认10个
	}
	if numWorkers < 1 || numWorkers > 100 {
		numWorkers = 20 // 默认20个并发
	}

	log.Printf("[小批预测并发] 开始预测 %d 个商品，批大小 %d，使用 %d 个并发线程...", len(goodIDs), batchSize, numWorkers)
	startTime := time.Now()

	// 准备小批次
	type batchJob struct {
		batchIdx  int
		batchGIDs []int64
	}
	batches := make([]batchJob, 0)
	for i := 0; i < len(goodIDs); i += batchSize {
		end := i + batchSize
		if end > len(goodIDs) {
			end = len(goodIDs)
		}
		batches = append(batches, batchJob{
			batchIdx:  len(batches),
			batchGIDs: goodIDs[i:end],
		})
	}

	jobsChan := make(chan batchJob, len(batches))
	type resultJob struct {
		results map[int64]*services.PredictionResult
		err     error
	}
	resultsChan := make(chan resultJob, len(batches))

	// 启动并发预测工作者
	var wg sync.WaitGroup
	for w := 0; w < numWorkers && w < len(batches); w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for job := range jobsChan {
				results, err := predictionClient.BatchPredict(job.batchGIDs, days)
				if err == nil {
					for _, goodID := range job.batchGIDs {
						if _, ok := results[goodID]; ok {
							log.Printf("[小批预测并发] ✓ good_id=%d 预测成功", goodID)
						} else {
							log.Printf("[小批预测并发] ✗ good_id=%d 预测失败: 无结果", goodID)
						}
					}
				} else {
					for _, goodID := range job.batchGIDs {
						log.Printf("[小批预测并发] ✗ good_id=%d 预测失败: %v", goodID, err)
					}
				}
				resultsChan <- resultJob{
					results: results,
					err:     err,
				}
			}
		}(w)
	}

	// 发送任务到信道
	go func() {
		for _, batch := range batches {
			jobsChan <- batch
		}
		close(jobsChan)
	}()

	// 等待所有工作者完成并关闭结果通道
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// 收集结果
	allResults := make(map[int64]*services.PredictionResult)
	successCount := 0
	errorCount := 0

	for result := range resultsChan {
		if result.err == nil {
			for goodID, predResult := range result.results {
				allResults[goodID] = predResult
				successCount++
			}
		} else {
			errorCount++
		}
	}

	log.Printf("[小批预测并发] 完成! 耗时 %.2f 秒，总计 %d，成功 %d，失败 %d",
		time.Since(startTime).Seconds(), len(goodIDs), successCount, errorCount)

	return allResults, successCount, errorCount
}

// filterByHistoricalPrediction 使用历史数据进行首轮预测，快速过滤有潜力的商品
// 这个函数在获取最新价格之前执行，可以快速筛选出有机会的商品，避免浪费时间获取无机会商品的最新价格
// 参数说明：
// - goodIDs: 所有待分析的商品ID
// - goodsCache: 商品信息缓存
// - predictionClient: 预测客户端
// 返回值：
// - 筛选后的商品ID列表（这些商品的7天后预测价格有潜力）
// - 过滤统计信息
func filterByHistoricalPrediction(
	goodIDs []int64,
	goodsCache map[int64]models.CSQAQGood,
	predictionClient *services.PredictionClient,
) ([]int64, map[string]int) {
	if len(goodIDs) == 0 {
		return []int64{}, map[string]int{}
	}

	log.Printf("[历史预测过滤] 开始用历史数据预测 %d 个商品...", len(goodIDs))
	filterStartTime := time.Now()

	// 使用小批量预测 + 高并发的方式（每批10个，20个线程，避免超时）
	predictions, successCount, errorCount := smallBatchPredictWithConcurrency(
		goodIDs,
		10, // 每批10个商品
		20, // 20个并发线程
		predictionClient,
		7,
	)

	log.Printf("[历史预测过滤] 历史预测完成: 耗时 %.2f 秒，成功 %d，失败 %d",
		time.Since(filterStartTime).Seconds(), successCount, errorCount)

	stats := map[string]int{
		"total":              len(goodIDs),
		"prediction_success": successCount,
		"prediction_error":   errorCount,
		"filtered_passed":    0,
		"filtered_rejected":  0,
	}

	// 根据预测结果过滤（只保留预测成功且7天后能够盈利的商品）
	filteredGoodIDs := make([]int64, 0, len(goodIDs))

	for _, goodID := range goodIDs {
		pred, hasPred := predictions[goodID]
		if !hasPred || pred == nil {
			// 预测失败，拒绝该商品
			stats["filtered_rejected"]++
			continue
		}

		// 获取7天后的预测价格
		ensemble, err := pred.GetEnsembleForecast()
		if err != nil || len(ensemble) < 7 {
			// 预测结果无效，拒绝该商品
			stats["filtered_rejected"]++
			continue
		}

		forecastedPrice := ensemble[6] // 第7天价格
		currentPrice := pred.CurrentPrice

		// 过滤条件：预测价格上涨 >= 3% 才值得获取最新价格重新预测
		priceDiff := (forecastedPrice - currentPrice) / currentPrice
		if priceDiff >= 0.03 {
			// 预测成功且能盈利，保留
			filteredGoodIDs = append(filteredGoodIDs, goodID)
			stats["filtered_passed"]++
		} else {
			// 预测下跌或涨幅不足，拒绝
			stats["filtered_rejected"]++
		}
	}

	log.Printf("[历史预测过滤] 过滤完成: 通过 %d 个，拒绝 %d 个，保留率 %.1f%%",
		stats["filtered_passed"],
		stats["filtered_rejected"],
		float64(stats["filtered_passed"])/float64(len(goodIDs))*100)

	return filteredGoodIDs, stats
}
