package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"csgo-trader/internal/quant"
)

const (
	defaultDSN = "root:Wyj250413.@tcp(23.254.215.66:3306)/csgo_trader?charset=utf8mb4&parseTime=True&loc=Local"
)

func main() {
	// 命令行参数
	mode := flag.String("mode", "daemon", "运行模式: daemon(守护进程), train(手动训练), query(查询), backtest(回测)")
	action := flag.String("action", "", "查询操作: buy-signals(买入信号), positions(持仓), performance(性能)")
	limit := flag.Int("limit", 10, "查询结果数量限制")
	dsn := flag.String("dsn", defaultDSN, "数据库连接字符串")
	flag.Parse()

	// 设置日志
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// 连接数据库
	db, err := quant.NewDatabase(*dsn)
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	defer db.Close()

	// 初始化表
	if err := db.InitTables(); err != nil {
		log.Fatalf("初始化表失败: %v", err)
	}

	switch *mode {
	case "daemon":
		runDaemon(db)
	case "train":
		runTraining(db)
	case "query":
		runQuery(db, *action, *limit)
	case "backtest":
		runBacktest(db)
	default:
		log.Fatalf("未知模式: %s", *mode)
	}
}

// runDaemon 守护进程模式
func runDaemon(db *quant.Database) {
	log.Println("======================================")
	log.Println("量化交易策略系统启动 (守护进程模式)")
	log.Println("======================================")

	// 获取或创建默认策略配置
	config := quant.DefaultStrategyConfig()
	version, err := db.GetActiveStrategyVersion()
	if err != nil {
		log.Printf("警告: 获取活跃策略版本失败: %v, 使用默认版本", err)
		version = "v1.0.0"
	}

	// 创建策略执行器
	strategy := quant.NewStrategy(db, config, version)

	// 创建自动学习调度器
	learner := quant.NewAutoLearner(db)

	// 启动每日自动学习调度
	learner.ScheduleDailyLearning()

	// 启动信号更新定时任务（每小时）
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		// 立即执行一次
		log.Println("执行首次信号更新...")
		if err := strategy.UpdateSignals(); err != nil {
			log.Printf("信号更新失败: %v", err)
		}

		for range ticker.C {
			log.Println("定时信号更新触发...")
			if err := strategy.UpdateSignals(); err != nil {
				log.Printf("信号更新失败: %v", err)
			}
		}
	}()

	log.Println("守护进程已启动")
	log.Println("- 每小时自动更新交易信号")
	log.Println("- 每天凌晨2点自动学习优化")
	log.Println("按 Ctrl+C 停止")

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("收到停止信号，正在关闭...")
}

// runTraining 手动训练模式
func runTraining(db *quant.Database) {
	log.Println("======================================")
	log.Println("手动训练模式")
	log.Println("======================================")

	learner := quant.NewAutoLearner(db)
	if err := learner.RunDailyLearning(); err != nil {
		log.Fatalf("训练失败: %v", err)
	}

	log.Println("训练完成!")
}

// runQuery 查询模式
func runQuery(db *quant.Database, action string, limit int) {
	switch action {
	case "buy-signals":
		queryBuySignals(db, limit)
	case "positions":
		queryPositions(db)
	case "performance":
		queryPerformance(db)
	default:
		log.Fatalf("未知查询操作: %s", action)
	}
}

// queryBuySignals 查询买入信号
func queryBuySignals(db *quant.Database, limit int) {
	query := `
		SELECT good_id, good_name, current_buy_price, predicted_profit_rate,
		       confidence_score, signal_strength, recommended_quantity, reason
		FROM trading_signals
		WHERE expires_at > NOW()
		ORDER BY signal_strength DESC
		LIMIT ?
	`

	rows, err := db.Query(query, limit)
	if err != nil {
		log.Fatalf("查询失败: %v", err)
	}
	defer rows.Close()

	fmt.Println("\n=== 当前买入信号（前", limit, "个）===")
	fmt.Println()

	count := 0
	for rows.Next() {
		var goodID int64
		var goodName string
		var buyPrice, profitRate, confidence, strength float64
		var quantity int
		var reason string

		if err := rows.Scan(&goodID, &goodName, &buyPrice, &profitRate, &confidence, &strength, &quantity, &reason); err != nil {
			log.Printf("扫描行失败: %v", err)
			continue
		}

		count++
		fmt.Printf("%d. %s (ID:%d)\n", count, goodName, goodID)
		fmt.Printf("   买入价: ¥%.2f\n", buyPrice)
		fmt.Printf("   预期收益率: %.2f%%\n", profitRate)
		fmt.Printf("   置信度: %.2f\n", confidence)
		fmt.Printf("   信号强度: %.0f\n", strength)
		fmt.Printf("   推荐数量: %d件\n", quantity)
		fmt.Printf("   理由: %s\n", reason)
		fmt.Println()
	}

	if count == 0 {
		fmt.Println("当前没有买入信号")
	}
}

// queryPositions 查询持仓
func queryPositions(db *quant.Database) {
	query := `
		SELECT t.good_id, t.good_name, t.buy_price, t.buy_time,
		       DATEDIFF(NOW(), t.buy_time) as holding_days,
		       s.yyyp_sell_price as current_price
		FROM trade_records t
		LEFT JOIN (
			SELECT good_id, yyyp_sell_price,
			       ROW_NUMBER() OVER (PARTITION BY good_id ORDER BY created_at DESC) as rn
			FROM csqaq_good_snapshots
		) s ON t.good_id = s.good_id AND s.rn = 1
		WHERE t.status = 'holding'
		ORDER BY t.buy_time DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Fatalf("查询失败: %v", err)
	}
	defer rows.Close()

	fmt.Println("\n=== 当前持仓 ===")
	fmt.Println()

	count := 0
	totalValue := 0.0
	totalProfit := 0.0

	for rows.Next() {
		var goodID int64
		var goodName string
		var buyPrice, currentPrice float64
		var buyTime time.Time
		var holdingDays int

		if err := rows.Scan(&goodID, &goodName, &buyPrice, &buyTime, &holdingDays, &currentPrice); err != nil {
			log.Printf("扫描行失败: %v", err)
			continue
		}

		count++
		profitRate := (currentPrice/buyPrice - 1 - 0.01) * 100 // 扣除1%手续费
		profit := (currentPrice - buyPrice) * 0.99

		totalValue += currentPrice
		totalProfit += profit

		status := "冷却中"
		if holdingDays >= 7 {
			if profitRate >= 2 {
				status = "✓ 建议卖出"
			} else {
				status = "可卖出（利润未达标）"
			}
		} else {
			status = fmt.Sprintf("冷却中（还需%d天）", 7-holdingDays)
		}

		fmt.Printf("%d. %s (ID:%d)\n", count, goodName, goodID)
		fmt.Printf("   买入价: ¥%.2f | 当前价: ¥%.2f\n", buyPrice, currentPrice)
		fmt.Printf("   持有天数: %d天 | 状态: %s\n", holdingDays, status)
		fmt.Printf("   利润率: %.2f%% | 利润: ¥%.2f\n", profitRate, profit)
		fmt.Println()
	}

	if count == 0 {
		fmt.Println("当前没有持仓")
	} else {
		fmt.Printf("=== 汇总 ===\n")
		fmt.Printf("总持仓数: %d件\n", count)
		fmt.Printf("总价值: ¥%.2f\n", totalValue)
		fmt.Printf("总利润: ¥%.2f\n", totalProfit)
	}
}

// queryPerformance 查询性能统计
func queryPerformance(db *quant.Database) {
	// 查询已卖出的交易统计
	query := `
		SELECT COUNT(*) as total,
		       SUM(CASE WHEN actual_profit_rate > 0 THEN 1 ELSE 0 END) as wins,
		       AVG(actual_profit_rate) as avg_profit_rate,
		       SUM((sell_price - buy_price) * 0.99) as total_profit
		FROM trade_records
		WHERE status = 'sold'
	`

	var total, wins int
	var avgProfitRate, totalProfit float64

	err := db.QueryRow(query).Scan(&total, &wins, &avgProfitRate, &totalProfit)
	if err != nil {
		log.Fatalf("查询失败: %v", err)
	}

	fmt.Println("\n=== 策略表现统计 ===")
	fmt.Println()
	
	if total > 0 {
		winRate := float64(wins) / float64(total) * 100
		fmt.Printf("总交易数: %d\n", total)
		fmt.Printf("盈利交易: %d\n", wins)
		fmt.Printf("胜率: %.1f%%\n", winRate)
		fmt.Printf("平均收益率: %.2f%%\n", avgProfitRate)
		fmt.Printf("累计利润: ¥%.2f\n", totalProfit)
	} else {
		fmt.Println("暂无已完成的交易记录")
	}
}

// runBacktest 回测模式
func runBacktest(db *quant.Database) {
	log.Println("======================================")
	log.Println("回测模式")
	log.Println("======================================")

	config := quant.DefaultStrategyConfig()
	be := quant.NewBacktestEngine(db, config)

	// 回测最近30天
	endDate := time.Now()
	startDate := endDate.Add(-30 * 24 * time.Hour)

	result, err := be.RunBacktest(startDate, endDate)
	if err != nil {
		log.Fatalf("回测失败: %v", err)
	}

	// 打印回测结果
	fmt.Println("\n=== 回测结果 ===")
	fmt.Printf("时间范围: %s 到 %s\n", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	fmt.Printf("总交易数: %d\n", result.TotalTrades)
	fmt.Printf("盈利交易: %d\n", result.WinningTrades)
	fmt.Printf("亏损交易: %d\n", result.LosingTrades)
	fmt.Printf("胜率: %.1f%%\n", result.WinRate)
	fmt.Printf("总收益: ¥%.2f\n", result.TotalReturn)
	fmt.Printf("收益率: %.2f%%\n", result.TotalReturnRate)
	fmt.Printf("夏普比率: %.2f\n", result.SharpeRatio)
	fmt.Printf("最大回撤: %.2f%%\n", result.MaxDrawdown)
	fmt.Printf("平均持仓天数: %.1f天\n", result.AvgHoldingDays)

	// 验证策略
	if be.ValidateStrategy(result) {
		fmt.Println("\n✓ 策略验证通过")
	} else {
		fmt.Println("\n✗ 策略验证失败")
	}
}

