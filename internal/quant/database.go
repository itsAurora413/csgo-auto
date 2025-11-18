package quant

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Database 数据库连接
type Database struct {
	db *sql.DB
}

// NewDatabase 创建数据库连接
func NewDatabase(dsn string) (*Database, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 测试连接
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// 设置连接池
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	log.Println("数据库连接成功")

	return &Database{db: db}, nil
}

// Close 关闭数据库连接
func (d *Database) Close() error {
	return d.db.Close()
}

// InitTables 初始化所需的表
func (d *Database) InitTables() error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS strategy_versions (
			id INT AUTO_INCREMENT PRIMARY KEY,
			version VARCHAR(50) UNIQUE NOT NULL,
			config JSON NOT NULL,
			ml_model_path VARCHAR(255),
			status ENUM('training', 'validating', 'active', 'archived') DEFAULT 'training',
			backtest_metrics JSON,
			validation_start_time DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			activated_at DATETIME,
			INDEX idx_status (status),
			INDEX idx_version (version)
		)`,
		`CREATE TABLE IF NOT EXISTS trade_records (
			id INT AUTO_INCREMENT PRIMARY KEY,
			good_id BIGINT NOT NULL,
			good_name VARCHAR(255),
			buy_price FLOAT NOT NULL,
			buy_time DATETIME NOT NULL,
			sell_price FLOAT,
			sell_time DATETIME,
			actual_profit_rate FLOAT,
			predicted_profit_rate FLOAT,
			strategy_version VARCHAR(50),
			status ENUM('holding', 'sold', 'cancelled') DEFAULT 'holding',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_good_id (good_id),
			INDEX idx_status (status),
			INDEX idx_buy_time (buy_time)
		)`,
		`CREATE TABLE IF NOT EXISTS trading_signals (
			id INT AUTO_INCREMENT PRIMARY KEY,
			good_id BIGINT NOT NULL,
			good_name VARCHAR(255),
			current_buy_price FLOAT NOT NULL,
			current_sell_price FLOAT NOT NULL,
			predicted_price_7d FLOAT NOT NULL,
			predicted_profit_rate FLOAT NOT NULL,
			confidence_score FLOAT NOT NULL,
			signal_strength FLOAT NOT NULL,
			trend_score FLOAT,
			spread_score FLOAT,
			liquidity_score FLOAT,
			volatility FLOAT,
			recommended_quantity INT,
			max_investment FLOAT,
			reason TEXT,
			strategy_version VARCHAR(50),
			signal_time DATETIME NOT NULL,
			expires_at DATETIME NOT NULL,
			INDEX idx_signal_time (signal_time),
			INDEX idx_good_id (good_id)
		)`,
		`CREATE TABLE IF NOT EXISTS learning_logs (
			id INT AUTO_INCREMENT PRIMARY KEY,
			learning_time DATETIME NOT NULL,
			data_range VARCHAR(100),
			best_params JSON,
			backtest_sharpe_ratio FLOAT,
			backtest_return_rate FLOAT,
			backtest_win_rate FLOAT,
			status ENUM('success', 'failed') DEFAULT 'success',
			error_message TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_learning_time (learning_time)
		)`,
		`CREATE TABLE IF NOT EXISTS daily_reports (
			id INT AUTO_INCREMENT PRIMARY KEY,
			report_date DATE NOT NULL UNIQUE,
			total_signals INT,
			high_confidence_signals INT,
			current_positions INT,
			positions_ready_to_sell INT,
			today_profit FLOAT,
			total_profit FLOAT,
			win_rate FLOAT,
			strategy_version VARCHAR(50),
			summary TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_report_date (report_date)
		)`,
		`CREATE TABLE IF NOT EXISTS good_blacklist (
			id INT AUTO_INCREMENT PRIMARY KEY,
			good_id BIGINT NOT NULL UNIQUE,
			reason VARCHAR(255),
			loss_count INT DEFAULT 0,
			last_loss_time DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_good_id (good_id)
		)`,
	}

	for _, table := range tables {
		if _, err := d.db.Exec(table); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	log.Println("所有表初始化成功")
	return nil
}

// Query 执行查询
func (d *Database) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return d.db.Query(query, args...)
}

// QueryRow 执行单行查询
func (d *Database) QueryRow(query string, args ...interface{}) *sql.Row {
	return d.db.QueryRow(query, args...)
}

// LoadHistoricalData 加载历史快照数据
func (d *Database) LoadHistoricalData(goodID int64, startTime, endTime time.Time) ([]GoodSnapshot, error) {
	query := `
		SELECT good_id, yyyp_sell_price, yyyp_buy_price, yyyp_sell_count, yyyp_buy_count, created_at
		FROM csqaq_good_snapshots
		WHERE good_id = ? 
		AND created_at BETWEEN ? AND ?
		AND yyyp_sell_price IS NOT NULL
		AND yyyp_buy_price IS NOT NULL
		ORDER BY created_at ASC
	`

	rows, err := d.db.Query(query, goodID, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to query snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []GoodSnapshot
	for rows.Next() {
		var s GoodSnapshot
		var sellCount, buyCount sql.NullInt64
		if err := rows.Scan(&s.GoodID, &s.YYYPSellPrice, &s.YYYPBuyPrice, &sellCount, &buyCount, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan snapshot: %w", err)
		}
		if sellCount.Valid {
			s.YYYPSellCount = int(sellCount.Int64)
		}
		if buyCount.Valid {
			s.YYYPBuyCount = int(buyCount.Int64)
		}
		snapshots = append(snapshots, s)
	}

	return snapshots, rows.Err()
}

// GetAllGoodsWithSufficientData 获取有足够历史数据的商品列表
func (d *Database) GetAllGoodsWithSufficientData(minSnapshots int, startTime, endTime time.Time) ([]int64, error) {
	query := `
		SELECT good_id
		FROM csqaq_good_snapshots
		WHERE created_at BETWEEN ? AND ?
		AND yyyp_sell_price IS NOT NULL
		AND yyyp_buy_price IS NOT NULL
		GROUP BY good_id
		HAVING COUNT(*) >= ?
		ORDER BY COUNT(*) DESC
	`

	rows, err := d.db.Query(query, startTime, endTime, minSnapshots)
	if err != nil {
		return nil, fmt.Errorf("failed to query goods: %w", err)
	}
	defer rows.Close()

	var goodIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan good_id: %w", err)
		}
		goodIDs = append(goodIDs, id)
	}

	return goodIDs, rows.Err()
}

// GetGoodName 获取商品名称
func (d *Database) GetGoodName(goodID int64) (string, error) {
	query := `
		SELECT COALESCE(market_hash_name, name, CONCAT('Good_', good_id)) as name
		FROM csqaq_goods
		WHERE good_id = ?
		LIMIT 1
	`

	var name string
	err := d.db.QueryRow(query, goodID).Scan(&name)
	if err == sql.ErrNoRows {
		return fmt.Sprintf("Good_%d", goodID), nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get good name: %w", err)
	}

	return name, nil
}

// SaveTradeSignals 保存交易信号
func (d *Database) SaveTradeSignals(signals []TradeSignal) error {
	if len(signals) == 0 {
		return nil
	}

	// 先清空过期信号
	_, err := d.db.Exec("DELETE FROM trading_signals WHERE expires_at < NOW()")
	if err != nil {
		log.Printf("警告: 清空过期信号失败: %v", err)
	}

	query := `
		INSERT INTO trading_signals (
			good_id, good_name, current_buy_price, current_sell_price,
			predicted_price_7d, predicted_profit_rate, confidence_score, signal_strength,
			trend_score, spread_score, liquidity_score, volatility,
			recommended_quantity, max_investment, reason, strategy_version,
			signal_time, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	stmt, err := d.db.Prepare(query)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, signal := range signals {
		_, err := stmt.Exec(
			signal.GoodID, signal.GoodName, signal.CurrentBuyPrice, signal.CurrentSellPrice,
			signal.PredictedPrice7d, signal.PredictedProfitRate, signal.ConfidenceScore, signal.SignalStrength,
			signal.TrendScore, signal.SpreadScore, signal.LiquidityScore, signal.Volatility,
			signal.RecommendedQuantity, signal.MaxInvestment, signal.Reason, signal.StrategyVersion,
			signal.SignalTime, signal.ExpiresAt,
		)
		if err != nil {
			log.Printf("警告: 保存信号失败 (good_id=%d): %v", signal.GoodID, err)
		}
	}

	log.Printf("成功保存 %d 个交易信号", len(signals))
	return nil
}

// GetCurrentHoldingCount 获取某商品当前持仓数量
func (d *Database) GetCurrentHoldingCount(goodID int64) (int, error) {
	query := `
		SELECT COUNT(*) 
		FROM trade_records 
		WHERE good_id = ? AND status = 'holding'
	`

	var count int
	err := d.db.QueryRow(query, goodID).Scan(&count)
	return count, err
}

// IsBlacklisted 检查商品是否在黑名单中
func (d *Database) IsBlacklisted(goodID int64) (bool, error) {
	query := `SELECT COUNT(*) FROM good_blacklist WHERE good_id = ?`
	var count int
	err := d.db.QueryRow(query, goodID).Scan(&count)
	return count > 0, err
}

// AddToBlacklist 添加到黑名单
func (d *Database) AddToBlacklist(goodID int64, reason string) error {
	query := `
		INSERT INTO good_blacklist (good_id, reason, loss_count, last_loss_time)
		VALUES (?, ?, 1, NOW())
		ON DUPLICATE KEY UPDATE 
			loss_count = loss_count + 1,
			last_loss_time = NOW(),
			reason = ?
	`
	_, err := d.db.Exec(query, goodID, reason, reason)
	return err
}

// SaveTradeRecord 保存交易记录
func (d *Database) SaveTradeRecord(record *TradeRecord) error {
	query := `
		INSERT INTO trade_records (
			good_id, good_name, buy_price, buy_time, predicted_profit_rate,
			strategy_version, status
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	result, err := d.db.Exec(query,
		record.GoodID, record.GoodName, record.BuyPrice, record.BuyTime,
		record.PredictedProfitRate, record.StrategyVersion, record.Status,
	)
	if err != nil {
		return fmt.Errorf("failed to save trade record: %w", err)
	}

	id, _ := result.LastInsertId()
	record.ID = int(id)
	return nil
}

// GetActiveStrategyVersion 获取当前活跃的策略版本
func (d *Database) GetActiveStrategyVersion() (string, error) {
	query := `SELECT version FROM strategy_versions WHERE status = 'active' ORDER BY activated_at DESC LIMIT 1`
	var version string
	err := d.db.QueryRow(query).Scan(&version)
	if err == sql.ErrNoRows {
		return "v1.0.0", nil // 默认版本
	}
	return version, err
}
