package quant

import (
	"fmt"
	"math"
)

// CalculateFeatures 计算商品的所有特征
func CalculateFeatures(snapshots []GoodSnapshot) (*Features, error) {
	if len(snapshots) < 24 {
		return nil, fmt.Errorf("insufficient data: need at least 24 snapshots, got %d", len(snapshots))
	}

	n := len(snapshots)
	features := &Features{}

	// 使用最新的价格
	latest := snapshots[n-1]
	features.CurrentPrice = latest.YYYPSellPrice
	features.SellCount = latest.YYYPSellCount
	features.BuyCount = latest.YYYPBuyCount

	// 提取价格序列
	prices := make([]float64, n)
	for i, s := range snapshots {
		prices[i] = s.YYYPSellPrice
	}

	// 计算移动平均线
	if n >= 24 {
		features.MA24h = calculateMA(prices[n-24:])
	}
	if n >= 72 {
		features.MA72h = calculateMA(prices[n-72:])
	}
	if n >= 168 {
		features.MA168h = calculateMA(prices[n-168:])
	}

	// 计算EMA
	if n >= 24 {
		features.EMA24h = calculateEMA(prices, 24)
	}

	// 计算波动率（标准差）
	features.Volatility = calculateVolatility(prices)

	// 计算价格变化
	if n > 24 {
		features.PriceChange1d = (prices[n-1] - prices[n-24]) / prices[n-24] * 100
	}
	if n > 168 {
		features.PriceChange7d = (prices[n-1] - prices[n-168]) / prices[n-168] * 100
	}

	// 计算趋势
	features.TrendSlope, features.TrendStrength = calculateTrend(prices)

	// 计算价差特征
	spreads := make([]float64, n)
	for i, s := range snapshots {
		if s.YYYPBuyPrice > 0 {
			spreads[i] = s.YYYPSellPrice - s.YYYPBuyPrice
		}
	}
	
	if latest.YYYPBuyPrice > 0 {
		features.Spread = latest.YYYPSellPrice - latest.YYYPBuyPrice
		features.SpreadRatio = features.Spread / latest.YYYPBuyPrice
	}
	
	features.AvgSpread = calculateMA(spreads)
	if features.AvgSpread > 0 {
		features.SpreadDeviation = (features.Spread - features.AvgSpread) / features.AvgSpread
	}

	// 计算价格变化频率（有多少次价格发生变化）
	changeCount := 0
	for i := 1; i < n; i++ {
		if math.Abs(prices[i]-prices[i-1]) > 0.01 {
			changeCount++
		}
	}
	features.PriceChangFreq = float64(changeCount) / float64(n)

	return features, nil
}

// CalculateTrendScore 计算趋势得分 (0-1)
func CalculateTrendScore(features *Features) float64 {
	score := 0.0

	// 1. 价格高于均线 (权重: 30%)
	if features.CurrentPrice > features.MA24h {
		score += 0.3
	}
	if features.CurrentPrice > features.MA72h {
		score += 0.1
	}
	if features.CurrentPrice > features.MA168h {
		score += 0.1
	}

	// 2. 趋势强度 (权重: 30%)
	if features.TrendSlope > 0 {
		score += math.Min(features.TrendStrength*0.3, 0.3)
	}

	// 3. 短期涨幅 (权重: 20%)
	if features.PriceChange1d > 0 {
		score += math.Min(features.PriceChange1d/10*0.2, 0.2)
	}

	// 4. 波动率合理 (权重: 10%)
	if features.Volatility > 0 && features.Volatility < 10 {
		score += 0.1
	}

	return math.Min(score, 1.0)
}

// CalculateSpreadScore 计算价差得分 (0-1)
func CalculateSpreadScore(features *Features) float64 {
	score := 0.0

	// 1. 价差率合理 (权重: 40%)
	if features.SpreadRatio > 0.01 && features.SpreadRatio < 0.1 {
		score += 0.4
	} else if features.SpreadRatio >= 0.1 && features.SpreadRatio < 0.15 {
		score += 0.2
	}

	// 2. 价差低于平均 (好的买入机会) (权重: 40%)
	if features.SpreadDeviation < -0.2 {
		score += 0.4
	} else if features.SpreadDeviation < 0 {
		score += 0.2
	}

	// 3. 绝对价差合理 (权重: 20%)
	if features.Spread > 0.5 && features.Spread < 10 {
		score += 0.2
	}

	return math.Min(score, 1.0)
}

// CalculateLiquidityScore 计算流动性得分 (0-1)
func CalculateLiquidityScore(features *Features) float64 {
	score := 0.0

	// 1. 在售数量 (权重: 40%)
	if features.SellCount >= 50 {
		score += 0.4
	} else if features.SellCount >= 20 {
		score += 0.2
	}

	// 2. 求购数量 (权重: 20%)
	if features.BuyCount >= 10 {
		score += 0.2
	} else if features.BuyCount >= 5 {
		score += 0.1
	}

	// 3. 价格变化频率 (权重: 40%)
	if features.PriceChangFreq > 0.3 {
		score += 0.4
	} else if features.PriceChangFreq > 0.1 {
		score += 0.2
	}

	return math.Min(score, 1.0)
}

// GenerateBuySignal 生成买入信号
func GenerateBuySignal(goodID int64, snapshots []GoodSnapshot, config *StrategyConfig) (*TradeSignal, error) {
	if len(snapshots) < 168 {
		return nil, fmt.Errorf("insufficient data for signal generation")
	}

	// 计算特征
	features, err := CalculateFeatures(snapshots)
	if err != nil {
		return nil, err
	}

	// 计算各项得分
	trendScore := CalculateTrendScore(features)
	spreadScore := CalculateSpreadScore(features)
	liquidityScore := CalculateLiquidityScore(features)

	// 检查是否满足基本条件
	if trendScore < config.TrendThreshold {
		return nil, fmt.Errorf("trend score too low: %.2f < %.2f", trendScore, config.TrendThreshold)
	}
	if spreadScore < config.SpreadThreshold {
		return nil, fmt.Errorf("spread score too low: %.2f < %.2f", spreadScore, config.SpreadThreshold)
	}
	if liquidityScore < config.LiquidityThreshold {
		return nil, fmt.Errorf("liquidity score too low: %.2f < %.2f", liquidityScore, config.LiquidityThreshold)
	}
	if features.Volatility > config.MaxVolatility {
		return nil, fmt.Errorf("volatility too high: %.2f > %.2f", features.Volatility, config.MaxVolatility)
	}

	// 预测7天后价格（简单线性外推）
	predicted7dPrice := features.CurrentPrice * (1 + features.TrendSlope*7)
	
	// 计算预期收益率（扣除手续费）
	buyPrice := snapshots[len(snapshots)-1].YYYPBuyPrice
	if buyPrice <= 0 {
		return nil, fmt.Errorf("invalid buy price: %.2f", buyPrice)
	}
	
	predictedProfitRate := (predicted7dPrice/buyPrice - 1 - config.FeeRate) * 100

	// 检查收益率是否达标
	if predictedProfitRate < config.MinProfitRate {
		return nil, fmt.Errorf("predicted profit rate too low: %.2f < %.2f", predictedProfitRate, config.MinProfitRate)
	}

	// 计算综合信号强度 (0-100)
	signalStrength := (trendScore*0.35 + spreadScore*0.30 + liquidityScore*0.35) * 100

	// 计算置信度（基于各项得分的一致性）
	scores := []float64{trendScore, spreadScore, liquidityScore}
	avgScore := (trendScore + spreadScore + liquidityScore) / 3
	variance := 0.0
	for _, s := range scores {
		variance += math.Pow(s-avgScore, 2)
	}
	variance /= float64(len(scores))
	confidenceScore := avgScore * (1 - math.Sqrt(variance))

	// 决定推荐数量
	recommendedQuantity := config.DefaultQuantity
	if confidenceScore >= config.HighConfidenceThreshold && predictedProfitRate >= config.HighConfidenceMinProfit {
		// 高置信度且高收益，可以购买更多
		recommendedQuantity = 2
		if confidenceScore >= 0.98 {
			recommendedQuantity = config.MaxMultipleQuantity
		}
	}

	// 生成推荐理由
	reason := fmt.Sprintf("趋势得分:%.0f%%, 价差得分:%.0f%%, 流动性得分:%.0f%%, 预期收益:%.1f%%",
		trendScore*100, spreadScore*100, liquidityScore*100, predictedProfitRate)

	signal := &TradeSignal{
		GoodID:              goodID,
		CurrentBuyPrice:     buyPrice,
		CurrentSellPrice:    features.CurrentPrice,
		PredictedPrice7d:    predicted7dPrice,
		PredictedProfitRate: predictedProfitRate,
		ConfidenceScore:     confidenceScore,
		SignalStrength:      signalStrength,
		TrendScore:          trendScore,
		SpreadScore:         spreadScore,
		LiquidityScore:      liquidityScore,
		Volatility:          features.Volatility,
		RecommendedQuantity: recommendedQuantity,
		MaxInvestment:       buyPrice * float64(recommendedQuantity),
		Reason:              reason,
	}

	return signal, nil
}

// 辅助函数

func calculateMA(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func calculateEMA(values []float64, period int) float64 {
	if len(values) < period {
		return calculateMA(values)
	}

	multiplier := 2.0 / float64(period+1)
	ema := calculateMA(values[:period])

	for i := period; i < len(values); i++ {
		ema = (values[i] * multiplier) + (ema * (1 - multiplier))
	}

	return ema
}

func calculateVolatility(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}

	mean := calculateMA(values)
	variance := 0.0
	for _, v := range values {
		variance += math.Pow(v-mean, 2)
	}
	variance /= float64(len(values))
	stdDev := math.Sqrt(variance)

	// 返回变异系数（百分比）
	if mean > 0 {
		return (stdDev / mean) * 100
	}
	return 0
}

func calculateTrend(values []float64) (slope, strength float64) {
	n := len(values)
	if n < 2 {
		return 0, 0
	}

	// 线性回归
	sumX := 0.0
	sumY := 0.0
	sumXY := 0.0
	sumX2 := 0.0

	for i, v := range values {
		x := float64(i)
		sumX += x
		sumY += v
		sumXY += x * v
		sumX2 += x * x
	}

	nFloat := float64(n)
	slope = (nFloat*sumXY - sumX*sumY) / (nFloat*sumX2 - sumX*sumX)

	// 计算R²（决定系数）作为趋势强度
	meanY := sumY / nFloat
	ssTotal := 0.0
	ssResidual := 0.0

	for i, v := range values {
		x := float64(i)
		predicted := slope*x + (sumY-slope*sumX)/nFloat
		ssTotal += math.Pow(v-meanY, 2)
		ssResidual += math.Pow(v-predicted, 2)
	}

	if ssTotal > 0 {
		strength = 1 - (ssResidual / ssTotal)
	}

	return slope, math.Max(0, strength)
}

