package main

import (
    "context"
    "csgo-trader/internal/database"
    "csgo-trader/internal/models"
    "csgo-trader/internal/services/youpin"
    "flag"
    "math"
    "log"
    "sort"
    "strings"
    "time"

    "gorm.io/gorm"
    "strconv"
)

var (
	minProfitRate      = flag.Float64("min-profit", 0.05, "最小利润率 (默认 5%)")
	minDaysHistory     = flag.Int("min-days", 3, "最少历史天数（默认 3天，没有足够数据时按当前价差判断）")
	budget             = flag.Float64("budget", 2000, "求购总预算（默认 2000元，可自定义）")
	minSellCount       = flag.Int("min-sell-count", 100, "最少在售数量（默认 100件，确保流动性）")
	minBuyCount        = flag.Int("min-buy-count", 10, "最少求购数量（默认 10件，确保需求）")
	maxReasonablePrice = flag.Float64("max-price", 10000, "最高合理价格（默认 10000元，过滤异常价格）")
	maxPriceSpread     = flag.Float64("max-spread", 2.0, "最大价差倍数（默认 2.0倍，售价不超过求购价的2倍）")
	once               = flag.Bool("once", false, "只运行一次，不循环")
	dbURL              = flag.String("db", "", "数据库连接字符串")
	backtest           = flag.Bool("backtest", false, "回测模式：使用7天前的预测对比实际收益")
	backtestDays       = flag.Int("backtest-days", 7, "回测天数（默认7天）")
)

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
	Stage              string  // 阶段: bottom_area(底部区域), rising(上涨), top_area(顶部区域), falling(下跌)
	Score              float64 // 周期得分(0-10分，分数越高越适合买入)
	Recommendation     string  // 建议: strong_buy(强烈买入), buy(买入), hold(观望), sell(卖出)
	PricePosition      float64 // 价格位置：当前价格相对7日均价的百分比偏离
	TrendStrength      string  // 趋势强度: strong(强), moderate(中), weak(弱)
	ReversalProbability string  // 反转概率: high(高), medium(中), low(低)
	Description        string  // 描述信息
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
		trend = "stable"
	} else if slopePercent > 1.5 { // 每天涨超过1.5%
		trend = "up"
	} else if slopePercent < -1.5 { // 每天跌超过1.5%
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
		} else if sellTrend == sellTrend { // 两个趋势一致
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
		riskScore = 3.0 // 高风险低分
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
	// 低价格高流动性的"价值股"
	if opp.CurrentBuyPrice < 100 && opp.SellOrderCount >= 150 {
		score += 3.0 // 价值投资机会
	} else if opp.CurrentBuyPrice < 50 && opp.SellOrderCount >= 100 {
		score += 2.0
	}

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

// calculateOptimalQuantity 计算最优购买数量（凯利公式思想）
func calculateOptimalQuantity(opp *models.ArbitrageOpportunity, remainingBudget float64) int {
	buyPrice := opp.RecommendedBuyPrice

	// === 1. 基于风险的仓位控制（类似凯利公式）===
	var baseQuantity int
	switch opp.RiskLevel {
	case "low":
		baseQuantity = 3 // 低风险可重仓
	case "medium":
		baseQuantity = 2 // 中风险中等仓位
	case "high":
		baseQuantity = 1 // 高风险轻仓
	}

	// === 2. 蓝筹股加仓（主战武器）===
	if isMainWeapon(opp.GoodName) {
		if baseQuantity < 3 {
			baseQuantity += 1
		}
	}

	// === 3. 优质资产加仓（崭新磨损）===
	wearScore := getWearScore(opp.GoodName)
	if wearScore >= 4.0 && baseQuantity < 3 { // 崭新或略磨
		baseQuantity += 1
	}

	// === 4. 流动性调整（做市商思维）===
	bidAskRatio := float64(opp.BuyOrderCount) / float64(opp.SellOrderCount+1)
	if bidAskRatio > 0.5 && opp.SellOrderCount >= 150 {
		// 买卖活跃，流动性好
		baseQuantity = 3
	} else if opp.SellOrderCount < 100 {
		// 流动性差，减仓
		baseQuantity = 1
	}

	// === 5. 价格区间调整（市值因子）===
	if buyPrice > 500 {
		baseQuantity = 1 // 大市值股票（高价），少买几只
	} else if buyPrice < 50 {
		if baseQuantity < 3 {
			baseQuantity += 1 // 小市值股票（低价），可以多买
		}
	}

	// === 6. 趋势跟随策略===
	if opp.PriceTrend == "up" && opp.RiskLevel == "low" {
		// 上涨趋势+低风险，可以加仓
		if baseQuantity < 3 {
			baseQuantity += 1
		}
	} else if opp.PriceTrend == "down" {
		// 下跌趋势减仓
		if baseQuantity > 1 {
			baseQuantity -= 1
		}
	}

	// === 7. 预算约束检查===
	maxQuantity := int(remainingBudget / buyPrice)
	if maxQuantity == 0 {
		return 0
	}

	quantity := baseQuantity
	if quantity > maxQuantity {
		quantity = maxQuantity
	}

	// === 8. 仓位上限（风险控制）===
	if quantity > 3 {
		quantity = 3
	}

	return quantity
}

// BacktestResult 回测结果
type BacktestResult struct {
	GoodID               int64
	GoodName             string
	PredictedBuyPrice    float64 // 预测的买入价
	PredictedSellPrice   float64 // 预测的卖出价
	PredictedProfit      float64 // 预测的利润
	PredictedProfitRate  float64 // 预测的利润率
	ActualBuyPrice       float64 // 实际买入价（7天前）
	ActualSellPrice      float64 // 实际卖出价（今天）
	ActualProfit         float64 // 实际利润
	ActualProfitRate     float64 // 实际利润率
	PriceChangeRate      float64 // 价格变化率
	IsSuccessful         bool    // 是否成功（实际利润>0）
	ProfitAccuracy       float64 // 利润预测准确度（实际/预测）
	Quantity             int     // 推荐数量
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
}

func main() {
	flag.Parse()

	log.Printf("[套利分析器] 启动中...")
	log.Printf("[套利分析器] 配置:")
	log.Printf("  - 最小利润率: %.2f%%", *minProfitRate*100)
	log.Printf("  - 求购预算: ¥%.2f", *budget)
	log.Printf("  - 最少在售数量: %d件", *minSellCount)
	log.Printf("  - 最少求购数量: %d件", *minBuyCount)
	log.Printf("  - 最高合理价格: ¥%.2f", *maxReasonablePrice)
	log.Printf("  - 最大价差倍数: %.1f倍", *maxPriceSpread)
	log.Printf("  - 最少历史天数: %d天", *minDaysHistory)

	// 初始化数据库
	db, err := database.Initialize(*dbURL)
	if err != nil {
		log.Fatalf("[套利分析器] 数据库初始化失败: %v", err)
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

	if *backtest {
		// 回测模式
		runBacktest(db)
	} else if *once {
		// 只运行一次
		runAnalysis(db)
	} else {
		// 持续循环运行：每次运行完立即开始下一次
		for {
			runAnalysis(db)
			log.Printf("[套利分析器] 本轮分析完成，立即开始下一轮分析...")
		}
	}
}

func runAnalysis(db *gorm.DB) {
    startTime := time.Now()
    analysisTime := startTime
    log.Printf("[套利分析] ==================== 开始新一轮分析 ====================")
    log.Printf("[套利分析] 分析时间: %s", analysisTime.Format("2006-01-02 15:04:05"))

    // 预备：尝试构建YouPin实时客户端（用于在无7天内快照时实时获取价差）
    var ypClient *youpin.Client
    {
        var account models.YouPinAccount
        if err := db.Where("is_active = ?", true).First(&account).Error; err == nil && account.Token != "" {
            if c, err := youpin.NewClient(account.Token); err == nil {
                ypClient = c
            } else {
                log.Printf("[套利分析] YouPin客户端初始化失败（实时价兜底不可用）: %v", err)
            }
        } else {
            log.Printf("[套利分析] 未找到激活的悠悠有品账户（实时价兜底不可用）")
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

	// 第一阶段：收集所有符合条件的商品数据
	log.Printf("[套利分析] ==================== 第一阶段：筛选符合条件的商品 ====================")
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
		historicalSnapshots []models.CSQAQGoodSnapshot
	}

	processedCount := 0
	skippedCount := 0

	// 统计各种跳过原因
	skipReasons := map[string]int{
		"类型过滤":   0,
		"无历史数据": 0,
		"价格无效":   0,
		"价格过高":   0,
		"价差异常":   0,
		"价格过低":   0,
		"无套利空间": 0,
		"流动性不足": 0,
	}
	realDataCount := 0
	estimatedDataCount := 0

	for i, goodID := range goodIDs {
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
        // 计算当前价：优先使用悠悠有品实时价，失败再回退快照
        var currentBuyPrice, currentSellPrice float64
        var rtBuyCount, rtSellCount int
        var usedRealtime bool

        // 实时获取（无论是否有历史，都优先尝试）
        if ypClient != nil {
            // 解析模板ID
            var templateID int64
            if len(historicalSnapshots) > 0 && historicalSnapshots[0].YYYPTemplateID != nil && *historicalSnapshots[0].YYYPTemplateID > 0 {
                templateID = *historicalSnapshots[0].YYYPTemplateID
            } else {
                var anySnap models.CSQAQGoodSnapshot
                if err := db.Where("good_id = ?", goodID).Order("created_at DESC").First(&anySnap).Error; err == nil && anySnap.YYYPTemplateID != nil && *anySnap.YYYPTemplateID > 0 {
                    templateID = *anySnap.YYYPTemplateID
                } else {
                    ctx := context.Background()
                    if searchResp, err := ypClient.SearchItems(ctx, good.Name, 1, 1, 0); err == nil && searchResp != nil && len(searchResp.Data.CommodityTemplateList) > 0 {
                        templateID = int64(searchResp.Data.CommodityTemplateList[0].ID)
                    }
                }
            }
            if templateID > 0 {
                ctx := context.Background()
                // 最高求购价
                maxBuy := 0.0
                if po, err := ypClient.GetTemplatePurchaseOrderList(ctx, int(templateID), 1, 50); err == nil && po != nil {
                    for _, item := range po.Data {
                        if item.PurchasePrice > maxBuy {
                            maxBuy = item.PurchasePrice
                        }
                    }
                    rtBuyCount = len(po.Data)
                }
                // 最低在售价
                lowestSell := 0.0
                if mp, err := ypClient.GetMarketSalePrice(ctx, strconv.FormatInt(templateID, 10)); err == nil && mp != nil && len(mp) > 0 {
                    rtSellCount = len(mp)
                    for i, p := range mp {
                        if i == 0 || p.Price < lowestSell {
                            lowestSell = p.Price
                        }
                    }
                }
                if maxBuy > 0 && lowestSell > 0 {
                    currentBuyPrice = maxBuy
                    currentSellPrice = lowestSell
                    usedRealtime = true
                }
            }
        }

        // 回退快照
        if !usedRealtime {
            if len(historicalSnapshots) == 0 {
                skippedCount++
                skipReasons["无历史数据"]++
                continue
            }
            latestSnapshot := historicalSnapshots[0]
            if latestSnapshot.YYYPBuyPrice == nil || latestSnapshot.YYYPSellPrice == nil {
                skippedCount++
                skipReasons["价格无效"]++
                continue
            }
            if *latestSnapshot.YYYPBuyPrice <= 0 || *latestSnapshot.YYYPSellPrice <= 0 {
                skippedCount++
                skipReasons["价格无效"]++
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
        buyOrderCount := 0   // 求购数量
        sellOrderCount := 0  // 在售数量
        usingRealData := false

        if usedRealtime {
            buyOrderCount = rtBuyCount
            sellOrderCount = rtSellCount
            usingRealData = true
        } else if len(historicalSnapshots) > 0 && historicalSnapshots[0].YYYPBuyCount != nil && historicalSnapshots[0].YYYPSellCount != nil {
            buyOrderCount = *historicalSnapshots[0].YYYPBuyCount
            sellOrderCount = *historicalSnapshots[0].YYYPSellCount
            usingRealData = true
        } else {
            // 如果快照中没有数量数据，使用估算值（兼容旧数据）
            // 根据价格估算热度（价格低通常热度高，但要避免垃圾货）
            if currentBuyPrice >= 1 && currentBuyPrice < 50 {
                buyOrderCount = 80
                sellOrderCount = 120
			} else if currentBuyPrice >= 50 && currentBuyPrice < 200 {
				buyOrderCount = 50
				sellOrderCount = 100
			} else if currentBuyPrice >= 200 && currentBuyPrice < 500 {
				buyOrderCount = 30
				sellOrderCount = 80
			} else if currentBuyPrice >= 500 {
				buyOrderCount = 15
				sellOrderCount = 60
			}
		}

		// 跟踪数据来源
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
			historicalSnapshots: historicalSnapshots,
		})
	}

	log.Printf("[第一阶段] 筛选完成! 总计处理: %d, 候选项: %d, 跳过: %d",
		processedCount, len(candidateItems), skippedCount)
	log.Printf("[第一阶段] 数据来源: 真实数据 %d 个, 估算数据 %d 个", realDataCount, estimatedDataCount)
	log.Printf("[第一阶段] 跳过原因统计:")
	log.Printf("  - 类型过滤: %d", skipReasons["类型过滤"])
	log.Printf("  - 无历史数据: %d", skipReasons["无历史数据"])
	log.Printf("  - 价格无效: %d", skipReasons["价格无效"])
	log.Printf("  - 价格过高: %d", skipReasons["价格过高"])
	log.Printf("  - 价差异常: %d", skipReasons["价差异常"])
	log.Printf("  - 价格过低: %d", skipReasons["价格过低"])
	log.Printf("  - 无套利空间: %d", skipReasons["无套利空间"])
	log.Printf("  - 流动性不足: %d", skipReasons["流动性不足"])

	// 第二阶段：对所有候选商品进行详细分析和评分
	log.Printf("[套利分析] ==================== 第二阶段：计算套利机会和风险评估 ====================")
	var opportunities []models.ArbitrageOpportunity

	// 第二阶段过滤统计
	secondStageFiltered := 0
	multiPeriodWeakFiltered := 0

	for i, candidate := range candidateItems {
		if i%100 == 0 && i > 0 {
			log.Printf("[第二阶段] 进度: %d/%d (%.1f%%)",
				i, len(candidateItems), float64(i)/float64(len(candidateItems))*100)
		}

		currentBuyPrice := candidate.currentBuyPrice
		currentSellPrice := candidate.currentSellPrice
		historicalSnapshots := candidate.historicalSnapshots

		// 重新计算利润率
		var feeRate2 float64 = 0.01
		var netSellPrice2 float64 = currentSellPrice * (1 - feeRate2)
		estimatedProfit := netSellPrice2 - currentBuyPrice
		profitRate := estimatedProfit / currentBuyPrice

		// === 第二阶段：严格的二次验证 ===

		// 价格上限检查
		if currentBuyPrice > *maxReasonablePrice || currentSellPrice > *maxReasonablePrice {
			continue
		}

		// 价格下限检查
		if currentBuyPrice < 0.5 || currentSellPrice < 0.5 {
			continue
		}

		// 价差合理性检查（更严格）
		if currentSellPrice > currentBuyPrice*(*maxPriceSpread) {
			continue
		}

		// 必须有实际利润
		if estimatedProfit <= 0 || profitRate <= 0 {
			continue
		}

		// === 分析价格趋势（使用线性回归）===
		priceTrend := "unknown"

		if candidate.hasEnoughHistory && len(historicalSnapshots) >= 3 {
			// 收集求购价和售价的历史数据（按时间从旧到新排序）
			buyPrices := []float64{}
			sellPrices := []float64{}

			for _, snapshot := range historicalSnapshots {
				if snapshot.YYYPBuyPrice != nil && *snapshot.YYYPBuyPrice > 0 {
					buyPrices = append(buyPrices, *snapshot.YYYPBuyPrice)
				}
				if snapshot.YYYPSellPrice != nil && *snapshot.YYYPSellPrice > 0 {
					sellPrices = append(sellPrices, *snapshot.YYYPSellPrice)
				}
			}

			// 使用线性回归综合分析趋势
			if len(sellPrices) >= 3 || len(buyPrices) >= 3 {
				priceTrend, _ = analyzeTrendWithBothPrices(buyPrices, sellPrices)
			}
		} else {
			// 历史数据不足时，根据当前价差判断稳定性
			priceDiff := currentSellPrice - currentBuyPrice
			diffRatio := priceDiff / currentBuyPrice
			if diffRatio < 0.15 { // 价差小于15%认为相对稳定
				priceTrend = "stable"
			}
		}

		// === 短期操作：多周期涨跌幅检查（过滤多周期走弱的商品）===
		// 计算1天、7天、30天的涨跌幅
		if len(historicalSnapshots) >= 2 {
			// 获取最新价格和历史价格
			latestPrice := currentSellPrice

			// 1天前价格（假设每1.6秒采样一次，1天约54000次采样，取最近第54次）
			var price1d, price7d, price30d float64
			var has1d, has7d, has30d bool

			// 简化：直接从历史快照中取对应时间点
			now := time.Now()
			for _, snapshot := range historicalSnapshots {
				if snapshot.YYYPSellPrice != nil && *snapshot.YYYPSellPrice > 0 {
					age := now.Sub(snapshot.CreatedAt)

					// 1天前的价格（23-25小时）
					if age >= 23*time.Hour && age <= 25*time.Hour && !has1d {
						price1d = *snapshot.YYYPSellPrice
						has1d = true
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
			var rate1d, rate7d, rate30d float64
			if has1d && price1d > 0 {
				rate1d = (latestPrice - price1d) / price1d
			}
			if has7d && price7d > 0 {
				rate7d = (latestPrice - price7d) / price7d
			}
			if has30d && price30d > 0 {
				rate30d = (latestPrice - price30d) / price30d
			}

			// 短期操作策略：过滤多周期走弱的商品
			// 如果1天和7天都在跌，认为短期风险大，跳过
			if has1d && has7d && rate1d < 0 && rate7d < 0 {
				multiPeriodWeakFiltered++
				secondStageFiltered++
				continue // 跳过多周期下跌的商品
			}

			// 如果1天、7天、30天都在跌，更要避免（防止接飞刀）
			if has1d && has7d && has30d && rate1d < 0 && rate7d < 0 && rate30d < 0 {
				multiPeriodWeakFiltered++
				secondStageFiltered++
				continue // 跳过多周期走弱的商品
			}
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
        log.Printf("[评分] ID:%d 名称:%s | 利润率:%.1f%% | 趋势:%s | 风险:%s | 分数:%.1f",
            opportunity.GoodID,
            opportunity.GoodName,
            opportunity.ProfitRate*100,
            opportunity.PriceTrend,
            opportunity.RiskLevel,
            opportunity.Score,
        )

		opportunities = append(opportunities, opportunity)
	}

	log.Printf("[第二阶段] 分析完成! 共计算出 %d 个套利机会", len(opportunities))
	if secondStageFiltered > 0 {
		log.Printf("[第二阶段] 过滤统计: 总过滤 %d 个, 其中多周期走弱 %d 个", secondStageFiltered, multiPeriodWeakFiltered)
	}

	// 第三阶段：智能算法优化求购清单
	log.Printf("[套利分析] ==================== 第三阶段：优化求购清单 ====================")

	if len(opportunities) == 0 {
		log.Printf("[套利分析] 未发现符合条件的套利机会")
		return
	}

	// 按综合评分排序（利润率、风险、流动性、历史数据、价格趋势）
	sort.Slice(opportunities, func(i, j int) bool {
		scoreI := calculateScore(opportunities[i])
		scoreJ := calculateScore(opportunities[j])

		// 如果评分相同，按利润率排序
		if scoreI == scoreJ {
			return opportunities[i].ProfitRate > opportunities[j].ProfitRate
		}
		return scoreI > scoreJ
	})

	// 输出评分最高的前20个商品（用于详细分析）
	log.Printf("[套利分析] ==================== 量化评分 TOP 20 ====================")
	displayCount := 20
	if len(opportunities) < displayCount {
		displayCount = len(opportunities)
	}

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

	remainingBudget := *budget
	totalItems := 0
	purchaseList := []struct {
		GoodID   int64
		GoodName string
		Quantity int
		Price    float64
		Total    float64
	}{}

	// 使用贪心算法分配预算：优先选择性价比最高的商品
	for i := range opportunities {
		if remainingBudget <= 10 { // 剩余预算太少则停止
			break
		}

		opp := &opportunities[i]
		buyPrice := opp.RecommendedBuyPrice

		// 智能计算购买数量
		quantity := calculateOptimalQuantity(opp, remainingBudget)
		if quantity == 0 {
			continue
		}

		// 更新记录
		opp.RecommendedQuantity = quantity
		total := buyPrice * float64(quantity)
		remainingBudget -= total
		totalItems += quantity

		purchaseList = append(purchaseList, struct {
			GoodID   int64
			GoodName string
			Quantity int
			Price    float64
			Total    float64
		}{
			GoodID:   opp.GoodID,
			GoodName: opp.GoodName,
			Quantity: quantity,
			Price:    buyPrice,
			Total:    total,
		})

		// 限制购买清单长度，确保多样化
		if len(purchaseList) >= 50 {
			break
		}
	}

	log.Printf("[求购计划] 已分配: ¥%.2f / ¥%.2f (剩余: ¥%.2f)",
		*budget-remainingBudget, *budget, remainingBudget)
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

	// 创建最优求购计划（清单）
	if len(purchaseList) > 0 {
		totalCost := *budget - remainingBudget
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

			var ypClient *youpin.Client
			if account.ID > 0 {
				ypClient, _ = youpin.NewClient(account.Token)
			}

			// 保存计划明细
			planItems := []models.PurchasePlanItem{}
			ctx := context.Background()

			for _, item := range purchaseList {
				// 从opportunities中找到对应的风险等级和利润率
				var profitRate float64
				var riskLevel string
				for _, opp := range opportunities {
					if opp.GoodID == item.GoodID {
						profitRate = opp.ProfitRate
						riskLevel = opp.RiskLevel
						break
					}
				}

				var yyypTemplateID *int64
				// 如果有YouPin客户端，通过商品名称搜索获取template_id
				if ypClient != nil {
					searchResp, err := ypClient.SearchItems(ctx, item.GoodName, 1, 1, 0)
					if err == nil && searchResp != nil && len(searchResp.Data.CommodityTemplateList) > 0 {
						templateID := int64(searchResp.Data.CommodityTemplateList[0].ID)
						yyypTemplateID = &templateID
						log.Printf("[求购计划] 商品 %s 对应的YouPin TemplateID: %d", item.GoodName, templateID)
					} else {
						log.Printf("[求购计划] 未能获取商品 %s 的YouPin TemplateID: %v", item.GoodName, err)
					}
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
			for _, opp := range opportunities {
				if opp.GoodID == item.GoodID {
					profitRate = opp.ProfitRate
					riskLevel = opp.RiskLevel
					priceTrend = opp.PriceTrend
					avgBuyPrice7d = opp.AvgBuyPrice7d
					avgSellPrice7d = opp.AvgSellPrice7d
					break
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
			var currentSellPrice float64
			for _, opp := range opportunities {
				if opp.GoodID == item.GoodID {
					currentSellPrice = opp.CurrentSellPrice
					break
				}
			}
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
