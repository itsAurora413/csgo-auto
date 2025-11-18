package quant

import (
	"fmt"
	"log"
	"time"
)

// Optimizer 参数优化器
type Optimizer struct {
	db             *Database
	backtestEngine *BacktestEngine
}

// NewOptimizer 创建优化器
func NewOptimizer(db *Database) *Optimizer {
	return &Optimizer{
		db: db,
	}
}

// OptimizeParameters 网格搜索优化参数
func (o *Optimizer) OptimizeParameters(startDate, endDate time.Time) (*StrategyConfig, *BacktestResult, error) {
	log.Println("开始参数优化...")

	// 定义参数搜索空间
	trendThresholds := []float64{0.5, 0.6, 0.7}
	spreadThresholds := []float64{0.4, 0.5, 0.6}
	liquidityThresholds := []float64{0.6, 0.7, 0.8}
	minProfitRates := []float64{2.5, 3.0, 3.5}
	maxVolatilities := []float64{12.0, 15.0, 18.0}

	bestConfig := DefaultStrategyConfig()
	var bestResult *BacktestResult
	bestScore := -999999.0

	totalCombinations := len(trendThresholds) * len(spreadThresholds) * len(liquidityThresholds) *
		len(minProfitRates) * len(maxVolatilities)
	currentCombination := 0

	// 网格搜索
	for _, trendThreshold := range trendThresholds {
		for _, spreadThreshold := range spreadThresholds {
			for _, liquidityThreshold := range liquidityThresholds {
				for _, minProfitRate := range minProfitRates {
					for _, maxVolatility := range maxVolatilities {
						currentCombination++

						// 创建测试配置
						testConfig := DefaultStrategyConfig()
						testConfig.TrendThreshold = trendThreshold
						testConfig.SpreadThreshold = spreadThreshold
						testConfig.LiquidityThreshold = liquidityThreshold
						testConfig.MinProfitRate = minProfitRate
						testConfig.MaxVolatility = maxVolatility

						// 运行回测
						be := NewBacktestEngine(o.db, testConfig)
						result, err := be.RunBacktest(startDate, endDate)
						if err != nil {
							log.Printf("回测失败: %v", err)
							continue
						}

						// 计算综合得分：夏普比率 * 100 + 收益率 - 最大回撤
						// 优先考虑夏普比率（风险调整收益）
						score := result.SharpeRatio*100 + result.TotalReturnRate - result.MaxDrawdown*2

						// 必须满足基本条件
						if result.TotalReturnRate > 0 && result.MaxDrawdown <= testConfig.MaxDrawdown && result.WinRate >= 60 {
							if score > bestScore {
								bestScore = score
								bestConfig = testConfig
								bestResult = result
								log.Printf("[%d/%d] 找到更好的配置: 得分=%.2f, 夏普=%.2f, 收益率=%.2f%%, 回撤=%.2f%%, 胜率=%.1f%%",
									currentCombination, totalCombinations, score,
									result.SharpeRatio, result.TotalReturnRate, result.MaxDrawdown, result.WinRate)
							}
						}

						if currentCombination%10 == 0 {
							log.Printf("优化进度: %d/%d (%.1f%%)", currentCombination, totalCombinations,
								float64(currentCombination)/float64(totalCombinations)*100)
						}
					}
				}
			}
		}
	}

	if bestResult == nil {
		return nil, nil, fmt.Errorf("未找到合格的参数组合")
	}

	log.Printf("参数优化完成! 最佳得分=%.2f", bestScore)
	log.Printf("最佳参数: 趋势=%.2f, 价差=%.2f, 流动性=%.2f, 收益率=%.1f%%, 波动率=%.1f%%",
		bestConfig.TrendThreshold, bestConfig.SpreadThreshold, bestConfig.LiquidityThreshold,
		bestConfig.MinProfitRate, bestConfig.MaxVolatility)

	return bestConfig, bestResult, nil
}

// QuickOptimize 快速优化（用于日常自动学习）
func (o *Optimizer) QuickOptimize(startDate, endDate time.Time, baseConfig *StrategyConfig) (*StrategyConfig, *BacktestResult, error) {
	log.Println("开始快速优化...")

	// 基于当前配置进行微调
	bestConfig := baseConfig
	var bestResult *BacktestResult
	bestScore := -999999.0

	// 只在当前参数附近搜索
	adjustments := []float64{-0.1, 0, 0.1}

	for _, trendAdj := range adjustments {
		for _, spreadAdj := range adjustments {
			for _, liquidityAdj := range adjustments {
				testConfig := *baseConfig // 复制配置
				testConfig.TrendThreshold += trendAdj
				testConfig.SpreadThreshold += spreadAdj
				testConfig.LiquidityThreshold += liquidityAdj

				// 确保在合理范围内
				testConfig.TrendThreshold = clamp(testConfig.TrendThreshold, 0.3, 0.9)
				testConfig.SpreadThreshold = clamp(testConfig.SpreadThreshold, 0.3, 0.9)
				testConfig.LiquidityThreshold = clamp(testConfig.LiquidityThreshold, 0.3, 0.9)

				// 运行回测
				be := NewBacktestEngine(o.db, &testConfig)
				result, err := be.RunBacktest(startDate, endDate)
				if err != nil {
					continue
				}

				// 计算得分
				score := result.SharpeRatio*100 + result.TotalReturnRate - result.MaxDrawdown*2

				if result.TotalReturnRate > 0 && result.MaxDrawdown <= testConfig.MaxDrawdown && result.WinRate >= 60 {
					if score > bestScore {
						bestScore = score
						bestConfig = &testConfig
						bestResult = result
					}
				}
			}
		}
	}

	if bestResult == nil {
		// 如果微调失败，返回原配置
		log.Println("快速优化未找到更好的配置，保持原配置")
		be := NewBacktestEngine(o.db, baseConfig)
		result, err := be.RunBacktest(startDate, endDate)
		if err != nil {
			return nil, nil, err
		}
		return baseConfig, result, nil
	}

	log.Printf("快速优化完成! 得分=%.2f, 夏普=%.2f, 收益率=%.2f%%, 回撤=%.2f%%",
		bestScore, bestResult.SharpeRatio, bestResult.TotalReturnRate, bestResult.MaxDrawdown)

	return bestConfig, bestResult, nil
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

