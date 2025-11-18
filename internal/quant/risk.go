package quant

import (
	"fmt"
	"log"
)

// RiskManager 风险管理器
type RiskManager struct {
	db     *Database
	config *StrategyConfig
}

// NewRiskManager 创建风险管理器
func NewRiskManager(db *Database, config *StrategyConfig) *RiskManager {
	return &RiskManager{
		db:     db,
		config: config,
	}
}

// CheckPositionLimit 检查仓位限制
func (rm *RiskManager) CheckPositionLimit(goodID int64, signal *TradeSignal) (bool, string, error) {
	// 1. 检查是否在黑名单
	isBlacklisted, err := rm.db.IsBlacklisted(goodID)
	if err != nil {
		return false, "", fmt.Errorf("failed to check blacklist: %w", err)
	}
	if isBlacklisted {
		return false, "商品在黑名单中", nil
	}

	// 2. 检查当前持仓数量
	currentHolding, err := rm.db.GetCurrentHoldingCount(goodID)
	if err != nil {
		return false, "", fmt.Errorf("failed to get holding count: %w", err)
	}

	// 3. 根据推荐数量和当前持仓决定是否购买
	if currentHolding >= rm.config.MaxItemsPerGood {
		return false, fmt.Sprintf("已达到最大持仓数量 (%d/%d)", currentHolding, rm.config.MaxItemsPerGood), nil
	}

	// 4. 调整推荐数量（不能超过最大持仓限制）
	maxCanBuy := rm.config.MaxItemsPerGood - currentHolding
	if signal.RecommendedQuantity > maxCanBuy {
		signal.RecommendedQuantity = maxCanBuy
		signal.MaxInvestment = signal.CurrentBuyPrice * float64(maxCanBuy)
		log.Printf("调整购买数量: good_id=%d, 原推荐=%d, 调整后=%d (当前持仓=%d)",
			goodID, signal.RecommendedQuantity+currentHolding-maxCanBuy, maxCanBuy, currentHolding)
	}

	// 5. 严格控制：默认只买1件
	if currentHolding > 0 {
		// 如果已经有持仓，只有在高置信度且高收益时才继续买入
		if signal.ConfidenceScore < rm.config.HighConfidenceThreshold ||
			signal.PredictedProfitRate < rm.config.HighConfidenceMinProfit {
			return false, fmt.Sprintf("已有持仓且置信度不够高 (持仓=%d, 置信度=%.2f, 预期收益=%.2f%%)",
				currentHolding, signal.ConfidenceScore, signal.PredictedProfitRate), nil
		}
	}

	// 6. 检查单笔投资额度
	if signal.MaxInvestment > rm.config.MaxPositionSize {
		// 调整数量以符合最大投资额度
		adjustedQuantity := int(rm.config.MaxPositionSize / signal.CurrentBuyPrice)
		if adjustedQuantity < 1 {
			return false, fmt.Sprintf("单价太高，超过最大投资额度 (%.2f > %.2f)",
				signal.CurrentBuyPrice, rm.config.MaxPositionSize), nil
		}
		signal.RecommendedQuantity = adjustedQuantity
		signal.MaxInvestment = signal.CurrentBuyPrice * float64(adjustedQuantity)
	}

	return true, "", nil
}

// ValidateSignal 验证信号的有效性
func (rm *RiskManager) ValidateSignal(signal *TradeSignal) (bool, string) {
	// 1. 检查价格合理性
	if signal.CurrentBuyPrice <= 0 || signal.CurrentSellPrice <= 0 {
		return false, "价格无效"
	}

	// 2. 检查买卖价顺序
	if signal.CurrentBuyPrice > signal.CurrentSellPrice {
		return false, "买价高于卖价，数据异常"
	}

	// 3. 检查预测价格合理性
	if signal.PredictedPrice7d <= 0 {
		return false, "预测价格无效"
	}

	// 4. 检查价格变化是否过于极端（防止异常数据）
	priceChangeRatio := signal.PredictedPrice7d / signal.CurrentSellPrice
	if priceChangeRatio > 1.5 || priceChangeRatio < 0.7 {
		return false, fmt.Sprintf("预测价格变化过大 (%.2f -> %.2f)", signal.CurrentSellPrice, signal.PredictedPrice7d)
	}

	// 5. 检查置信度
	if signal.ConfidenceScore < 0 || signal.ConfidenceScore > 1 {
		return false, "置信度超出范围"
	}

	// 6. 检查波动率
	if signal.Volatility > rm.config.MaxVolatility {
		return false, fmt.Sprintf("波动率过高 (%.2f > %.2f)", signal.Volatility, rm.config.MaxVolatility)
	}

	return true, ""
}

// CalculatePositionSize 计算建议仓位大小
func (rm *RiskManager) CalculatePositionSize(signal *TradeSignal) int {
	// 基础数量
	quantity := rm.config.DefaultQuantity

	// 高置信度且高收益，增加数量
	if signal.ConfidenceScore >= rm.config.HighConfidenceThreshold &&
		signal.PredictedProfitRate >= rm.config.HighConfidenceMinProfit {

		// 根据置信度和预期收益决定数量
		if signal.ConfidenceScore >= 0.98 && signal.PredictedProfitRate >= 8.0 {
			quantity = rm.config.MaxMultipleQuantity // 3件
		} else if signal.ConfidenceScore >= 0.95 && signal.PredictedProfitRate >= 5.0 {
			quantity = 2 // 2件
		}
	}

	// 确保不超过最大限制
	if quantity > rm.config.MaxItemsPerGood {
		quantity = rm.config.MaxItemsPerGood
	}

	return quantity
}

// MonitorRisk 监控整体风险（用于实时监控）
func (rm *RiskManager) MonitorRisk() (*RiskReport, error) {
	report := &RiskReport{}

	// TODO: 查询当前所有持仓，计算总体风险指标
	// 这里可以扩展更多风险监控逻辑

	return report, nil
}

// RiskReport 风险报告
type RiskReport struct {
	TotalPositions    int
	TotalValue        float64
	DiversificationOK bool
	HighRiskGoods     []int64
	Warnings          []string
}

// ShouldUpdateBlacklist 判断是否应该更新黑名单
func (rm *RiskManager) ShouldUpdateBlacklist(goodID int64, lossCount int, totalLoss float64) bool {
	// 连续亏损3次或累计亏损超过100元，加入黑名单
	if lossCount >= 3 {
		return true
	}
	if totalLoss < -100 {
		return true
	}
	return false
}
