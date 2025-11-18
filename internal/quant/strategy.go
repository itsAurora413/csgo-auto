package quant

import (
	"fmt"
	"log"
	"time"
)

// Strategy 策略执行器
type Strategy struct {
	db          *Database
	config      *StrategyConfig
	riskManager *RiskManager
	version     string
}

// NewStrategy 创建策略执行器
func NewStrategy(db *Database, config *StrategyConfig, version string) *Strategy {
	return &Strategy{
		db:          db,
		config:      config,
		riskManager: NewRiskManager(db, config),
		version:     version,
	}
}

// GenerateSignals 生成所有交易信号
func (s *Strategy) GenerateSignals() ([]TradeSignal, error) {
	log.Println("开始生成交易信号...")

	now := time.Now()
	endTime := now
	startTime := now.Add(-time.Duration(s.config.TrainDays+7) * 24 * time.Hour)

	// 获取有足够数据的商品
	minSnapshots := 168 // 至少需要7天数据
	goodIDs, err := s.db.GetAllGoodsWithSufficientData(minSnapshots, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get goods: %w", err)
	}

	log.Printf("找到 %d 个商品，开始分析...", len(goodIDs))

	var signals []TradeSignal
	validSignalCount := 0

	for i, goodID := range goodIDs {
		if i > 0 && i%100 == 0 {
			log.Printf("分析进度: %d/%d, 有效信号: %d", i, len(goodIDs), validSignalCount)
		}

		// 加载最近的历史数据
		recentStart := now.Add(-time.Duration(s.config.TrainDays) * 24 * time.Hour)
		snapshots, err := s.db.LoadHistoricalData(goodID, recentStart, endTime)
		if err != nil || len(snapshots) < 168 {
			continue
		}

		// 生成买入信号
		signal, err := GenerateBuySignal(goodID, snapshots, s.config)
		if err != nil {
			// 没有买入信号
			continue
		}

		// 获取商品名称
		goodName, err := s.db.GetGoodName(goodID)
		if err != nil {
			goodName = fmt.Sprintf("Good_%d", goodID)
		}
		signal.GoodName = goodName
		signal.StrategyVersion = s.version
		signal.SignalTime = now
		signal.ExpiresAt = now.Add(1 * time.Hour) // 信号1小时有效

		// 风险验证
		valid, validateReason := s.riskManager.ValidateSignal(signal)
		if !valid {
			log.Printf("跳过 good_id=%d: %s", goodID, validateReason)
			continue
		}

		// 检查仓位限制
		allowed, checkReason, err := s.riskManager.CheckPositionLimit(goodID, signal)
		if err != nil {
			log.Printf("警告: 检查仓位失败 (good_id=%d): %v", goodID, err)
			continue
		}
		if !allowed {
			// 不允许购买 (reason: %s)
			log.Printf("跳过 good_id=%d: %s", goodID, checkReason)
			continue
		}

		signals = append(signals, *signal)
		validSignalCount++
	}

	log.Printf("信号生成完成: 共 %d 个有效信号", len(signals))

	// 按信号强度排序
	sortSignalsByStrength(signals)

	return signals, nil
}

// sortSignalsByStrength 按信号强度降序排序
func sortSignalsByStrength(signals []TradeSignal) {
	// 简单的冒泡排序
	n := len(signals)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if signals[j].SignalStrength < signals[j+1].SignalStrength {
				signals[j], signals[j+1] = signals[j+1], signals[j]
			}
		}
	}
}

// SaveSignals 保存交易信号到数据库
func (s *Strategy) SaveSignals(signals []TradeSignal) error {
	return s.db.SaveTradeSignals(signals)
}

// GetTopSignals 获取前N个最强信号
func (s *Strategy) GetTopSignals(n int) ([]TradeSignal, error) {
	signals, err := s.GenerateSignals()
	if err != nil {
		return nil, err
	}

	if len(signals) > n {
		signals = signals[:n]
	}

	return signals, nil
}

// UpdateSignals 更新交易信号（定时任务）
func (s *Strategy) UpdateSignals() error {
	log.Println("=== 开始更新交易信号 ===")
	start := time.Now()

	signals, err := s.GenerateSignals()
	if err != nil {
		return fmt.Errorf("failed to generate signals: %w", err)
	}

	if err := s.SaveSignals(signals); err != nil {
		return fmt.Errorf("failed to save signals: %w", err)
	}

	elapsed := time.Since(start)
	log.Printf("=== 信号更新完成，耗时: %v ===", elapsed)

	// 打印前10个最佳信号
	topN := 10
	if len(signals) < topN {
		topN = len(signals)
	}

	log.Println("=== 前10个最佳交易机会 ===")
	for i := 0; i < topN; i++ {
		sig := signals[i]
		log.Printf("%d. %s (ID:%d) - 预期收益:%.2f%%, 置信度:%.2f, 信号强度:%.0f, 推荐数量:%d",
			i+1, sig.GoodName, sig.GoodID, sig.PredictedProfitRate, sig.ConfidenceScore,
			sig.SignalStrength, sig.RecommendedQuantity)
	}

	return nil
}
