package quant

import (
	"fmt"
	"log"
	"math"
	"time"
)

// BacktestEngine 回测引擎
type BacktestEngine struct {
	db     *Database
	config *StrategyConfig
}

// NewBacktestEngine 创建回测引擎
func NewBacktestEngine(db *Database, config *StrategyConfig) *BacktestEngine {
	return &BacktestEngine{
		db:     db,
		config: config,
	}
}

// RunBacktest 运行回测
func (be *BacktestEngine) RunBacktest(startDate, endDate time.Time) (*BacktestResult, error) {
	log.Printf("开始回测: %s 到 %s", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	// 获取有足够数据的商品列表
	minSnapshots := be.config.CooldownDays*24 + 168 // 至少需要7天+7天的数据
	goodIDs, err := be.db.GetAllGoodsWithSufficientData(minSnapshots, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get goods: %w", err)
	}

	log.Printf("找到 %d 个商品用于回测", len(goodIDs))

	result := &BacktestResult{
		TradeDetails:  []BacktestTrade{},
		ProfitsByGood: make(map[int64]float64),
	}

	// 对每个商品进行回测
	for i, goodID := range goodIDs {
		if i > 0 && i%100 == 0 {
			log.Printf("回测进度: %d/%d", i, len(goodIDs))
		}

		trades, err := be.backtestGood(goodID, startDate, endDate)
		if err != nil {
			// 跳过有问题的商品
			continue
		}

		for _, trade := range trades {
			result.TradeDetails = append(result.TradeDetails, trade)
			result.TotalTrades++

			if trade.ProfitRate > 0 {
				result.WinningTrades++
			} else {
				result.LosingTrades++
			}

			result.TotalReturn += trade.NetProfit
			result.ProfitsByGood[goodID] += trade.NetProfit
		}
	}

	// 计算统计指标
	be.calculateMetrics(result)

	log.Printf("回测完成: 总交易=%d, 盈利=%d, 亏损=%d, 胜率=%.1f%%, 总收益=%.2f",
		result.TotalTrades, result.WinningTrades, result.LosingTrades,
		result.WinRate, result.TotalReturn)

	return result, nil
}

// backtestGood 回测单个商品
func (be *BacktestEngine) backtestGood(goodID int64, startDate, endDate time.Time) ([]BacktestTrade, error) {
	// 加载历史数据
	snapshots, err := be.db.LoadHistoricalData(goodID, startDate, endDate)
	if err != nil {
		return nil, err
	}

	if len(snapshots) < 168+be.config.CooldownDays*24 {
		return nil, fmt.Errorf("insufficient data")
	}

	var trades []BacktestTrade
	goodName, _ := be.db.GetGoodName(goodID)

	// 滑动窗口回测
	// 从第168个快照开始（需要至少168小时的历史数据来计算特征）
	for i := 168; i < len(snapshots)-be.config.CooldownDays*24; i++ {
		// 使用前168个快照（7天）来生成信号
		historyWindow := snapshots[i-168 : i+1]

		// 尝试生成买入信号
		signal, err := GenerateBuySignal(goodID, historyWindow, be.config)
		if err != nil {
			// 没有买入信号，继续
			continue
		}

		// 模拟买入
		buySnapshot := snapshots[i]
		buyPrice := buySnapshot.YYYPBuyPrice
		buyTime := buySnapshot.CreatedAt

		// 严格执行7天冷却期
		sellIndex := i + be.config.CooldownDays*24
		if sellIndex >= len(snapshots) {
			// 没有足够的未来数据
			break
		}

		// 7天后卖出
		sellSnapshot := snapshots[sellIndex]
		sellPrice := sellSnapshot.YYYPSellPrice
		sellTime := sellSnapshot.CreatedAt

		// 计算收益（扣除手续费）
		profitRate := (sellPrice/buyPrice - 1 - be.config.FeeRate) * 100
		netProfit := (sellPrice - buyPrice) * (1 - be.config.FeeRate)

		trade := BacktestTrade{
			GoodID:         goodID,
			GoodName:       goodName,
			BuyTime:        buyTime,
			BuyPrice:       buyPrice,
			SellTime:       sellTime,
			SellPrice:      sellPrice,
			ProfitRate:     profitRate,
			NetProfit:      netProfit,
			HoldingDays:    be.config.CooldownDays,
			SignalStrength: signal.SignalStrength,
		}

		trades = append(trades, trade)

		// 跳过冷却期，避免重复交易
		i += be.config.CooldownDays*24 - 1
	}

	return trades, nil
}

// calculateMetrics 计算回测指标
func (be *BacktestEngine) calculateMetrics(result *BacktestResult) {
	if result.TotalTrades == 0 {
		return
	}

	// 胜率
	result.WinRate = float64(result.WinningTrades) / float64(result.TotalTrades) * 100

	// 总收益率（假设初始资金1000）
	initialCapital := 1000.0
	result.TotalReturnRate = result.TotalReturn / initialCapital * 100

	// 计算夏普比率
	returns := make([]float64, len(result.TradeDetails))
	for i, trade := range result.TradeDetails {
		returns[i] = trade.ProfitRate
	}
	result.SharpeRatio = calculateSharpeRatio(returns)

	// 计算最大回撤
	result.MaxDrawdown = calculateMaxDrawdown(result.TradeDetails)

	// 平均持仓天数
	totalDays := 0
	for _, trade := range result.TradeDetails {
		totalDays += trade.HoldingDays
	}
	result.AvgHoldingDays = float64(totalDays) / float64(result.TotalTrades)
}

// calculateSharpeRatio 计算夏普比率
func calculateSharpeRatio(returns []float64) float64 {
	if len(returns) == 0 {
		return 0
	}

	// 计算平均收益
	avgReturn := 0.0
	for _, r := range returns {
		avgReturn += r
	}
	avgReturn /= float64(len(returns))

	// 计算标准差
	variance := 0.0
	for _, r := range returns {
		variance += math.Pow(r-avgReturn, 2)
	}
	variance /= float64(len(returns))
	stdDev := math.Sqrt(variance)

	if stdDev == 0 {
		return 0
	}

	// 夏普比率 = (平均收益 - 无风险利率) / 标准差
	// 假设无风险利率为0
	return avgReturn / stdDev
}

// calculateMaxDrawdown 计算最大回撤
func calculateMaxDrawdown(trades []BacktestTrade) float64 {
	if len(trades) == 0 {
		return 0
	}

	capital := 1000.0
	peak := capital
	maxDrawdown := 0.0

	for _, trade := range trades {
		capital += trade.NetProfit
		if capital > peak {
			peak = capital
		}
		drawdown := (peak - capital) / peak * 100
		if drawdown > maxDrawdown {
			maxDrawdown = drawdown
		}
	}

	return maxDrawdown
}

// ValidateStrategy 验证策略是否达标
func (be *BacktestEngine) ValidateStrategy(result *BacktestResult) bool {
	if result.TotalTrades < 10 {
		log.Printf("验证失败: 交易次数太少 (%d < 10)", result.TotalTrades)
		return false
	}

	if result.WinRate < 60 {
		log.Printf("验证失败: 胜率太低 (%.1f%% < 60%%)", result.WinRate)
		return false
	}

	if result.TotalReturnRate <= 0 {
		log.Printf("验证失败: 收益率非正 (%.2f%% <= 0)", result.TotalReturnRate)
		return false
	}

	if result.MaxDrawdown > be.config.MaxDrawdown {
		log.Printf("验证失败: 最大回撤超标 (%.2f%% > %.2f%%)", result.MaxDrawdown, be.config.MaxDrawdown)
		return false
	}

	if result.SharpeRatio < 0.5 {
		log.Printf("验证失败: 夏普比率太低 (%.2f < 0.5)", result.SharpeRatio)
		return false
	}

	log.Printf("✓ 策略验证通过: 胜率=%.1f%%, 收益率=%.2f%%, 夏普比率=%.2f, 最大回撤=%.2f%%",
		result.WinRate, result.TotalReturnRate, result.SharpeRatio, result.MaxDrawdown)

	return true
}

// IdentifyLosingGoods 识别亏损的商品，添加到黑名单
func (be *BacktestEngine) IdentifyLosingGoods(result *BacktestResult) error {
	blacklistCount := 0
	for goodID, profit := range result.ProfitsByGood {
		if profit < 0 {
			// 亏损的商品加入黑名单
			goodName, _ := be.db.GetGoodName(goodID)
			reason := fmt.Sprintf("回测亏损: %.2f", profit)
			if err := be.db.AddToBlacklist(goodID, reason); err != nil {
				log.Printf("警告: 添加黑名单失败 (good_id=%d): %v", goodID, err)
			} else {
				log.Printf("已加入黑名单: %s (good_id=%d, 亏损=%.2f)", goodName, goodID, profit)
				blacklistCount++
			}
		}
	}

	log.Printf("共添加 %d 个商品到黑名单", blacklistCount)
	return nil
}

