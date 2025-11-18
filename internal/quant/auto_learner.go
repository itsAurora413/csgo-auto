package quant

import (
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// AutoLearner 自动学习调度器
type AutoLearner struct {
	db        *Database
	optimizer *Optimizer
}

// NewAutoLearner 创建自动学习调度器
func NewAutoLearner(db *Database) *AutoLearner {
	return &AutoLearner{
		db:        db,
		optimizer: NewOptimizer(db),
	}
}

// RunDailyLearning 执行每日学习任务
func (al *AutoLearner) RunDailyLearning() error {
	log.Println("======================================")
	log.Println("开始每日自动学习")
	log.Println("======================================")

	startTime := time.Now()

	// 1. 确定训练数据范围（最近30天）
	endDate := time.Now()
	startDate := endDate.Add(-30 * 24 * time.Hour)

	log.Printf("训练数据范围: %s 到 %s", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	// 2. 获取当前活跃策略配置
	_, err := al.db.GetActiveStrategyVersion()
	if err != nil {
		log.Printf("警告: 获取当前策略版本失败: %v，使用默认配置", err)
	}

	baseConfig := DefaultStrategyConfig()

	// 3. 快速优化参数（基于当前配置微调）
	log.Println("开始参数优化...")
	bestConfig, bestResult, err := al.optimizer.QuickOptimize(startDate, endDate, baseConfig)
	if err != nil {
		return al.logLearningFailure(startDate, endDate, err)
	}

	// 4. 验证新策略
	be := NewBacktestEngine(al.db, bestConfig)
	if !be.ValidateStrategy(bestResult) {
		return al.logLearningFailure(startDate, endDate, fmt.Errorf("策略验证失败"))
	}

	// 5. 识别亏损商品，更新黑名单
	log.Println("更新黑名单...")
	if err := be.IdentifyLosingGoods(bestResult); err != nil {
		log.Printf("警告: 更新黑名单失败: %v", err)
	}

	// 6. 创建新策略版本
	newVersion := fmt.Sprintf("v%s", time.Now().Format("20060102_150405"))
	log.Printf("创建新策略版本: %s", newVersion)

	if err := al.saveStrategyVersion(newVersion, bestConfig, bestResult); err != nil {
		return al.logLearningFailure(startDate, endDate, err)
	}

	// 7. 记录学习日志
	if err := al.logLearningSuccess(startDate, endDate, bestConfig, bestResult); err != nil {
		log.Printf("警告: 记录学习日志失败: %v", err)
	}

	elapsed := time.Since(startTime)
	log.Printf("======================================")
	log.Printf("每日学习完成，耗时: %v", elapsed)
	log.Printf("新策略版本: %s (状态: validating)", newVersion)
	log.Printf("======================================")

	return nil
}

// CheckAndActivateValidatingStrategies 检查并激活验证期满的策略
func (al *AutoLearner) CheckAndActivateValidatingStrategies() error {
	// TODO: 实现策略验证和激活逻辑
	// 查询状态为 'validating' 且验证期已满3天的策略
	// 如果验证期表现良好，将状态改为 'active'
	// 将旧的 'active' 策略改为 'archived'

	log.Println("检查待激活的策略...")
	// 这里暂时简化处理，直接激活最新的 validating 策略

	return nil
}

// saveStrategyVersion 保存策略版本
func (al *AutoLearner) saveStrategyVersion(version string, config *StrategyConfig, result *BacktestResult) error {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	metricsJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}

	query := `
		INSERT INTO strategy_versions (version, config, status, backtest_metrics, validation_start_time)
		VALUES (?, ?, 'validating', ?, NOW())
	`

	_, err = al.db.db.Exec(query, version, string(configJSON), string(metricsJSON))
	if err != nil {
		return fmt.Errorf("failed to save strategy version: %w", err)
	}

	log.Printf("策略版本已保存: %s", version)
	return nil
}

// logLearningSuccess 记录学习成功日志
func (al *AutoLearner) logLearningSuccess(startDate, endDate time.Time, config *StrategyConfig, result *BacktestResult) error {
	dataRange := fmt.Sprintf("%s to %s", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	bestParams := map[string]interface{}{
		"trend_threshold":     config.TrendThreshold,
		"spread_threshold":    config.SpreadThreshold,
		"liquidity_threshold": config.LiquidityThreshold,
		"min_profit_rate":     config.MinProfitRate,
		"max_volatility":      config.MaxVolatility,
	}

	paramsJSON, _ := json.Marshal(bestParams)

	query := `
		INSERT INTO learning_logs (
			learning_time, data_range, best_params,
			backtest_sharpe_ratio, backtest_return_rate, backtest_win_rate,
			status
		) VALUES (NOW(), ?, ?, ?, ?, ?, 'success')
	`

	_, err := al.db.db.Exec(query, dataRange, string(paramsJSON),
		result.SharpeRatio, result.TotalReturnRate, result.WinRate)

	return err
}

// logLearningFailure 记录学习失败日志
func (al *AutoLearner) logLearningFailure(startDate, endDate time.Time, learningErr error) error {
	dataRange := fmt.Sprintf("%s to %s", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	query := `
		INSERT INTO learning_logs (
			learning_time, data_range, status, error_message
		) VALUES (NOW(), ?, 'failed', ?)
	`

	_, err := al.db.db.Exec(query, dataRange, learningErr.Error())
	if err != nil {
		log.Printf("记录失败日志也失败了: %v", err)
	}

	return learningErr
}

// ScheduleDailyLearning 调度每日学习任务（在凌晨2点执行）
func (al *AutoLearner) ScheduleDailyLearning() {
	go func() {
		for {
			now := time.Now()
			// 计算到下一个凌晨2点的时间
			next := time.Date(now.Year(), now.Month(), now.Day(), 2, 0, 0, 0, now.Location())
			if now.After(next) {
				next = next.Add(24 * time.Hour)
			}

			duration := next.Sub(now)
			log.Printf("下次自动学习时间: %s (还有 %v)", next.Format("2006-01-02 15:04:05"), duration)

			time.Sleep(duration)

			// 执行学习
			if err := al.RunDailyLearning(); err != nil {
				log.Printf("自动学习失败: %v", err)
			}

			// 检查并激活验证期满的策略
			if err := al.CheckAndActivateValidatingStrategies(); err != nil {
				log.Printf("激活策略失败: %v", err)
			}
		}
	}()
}
