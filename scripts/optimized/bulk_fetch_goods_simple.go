package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// 配置结构体
type Config struct {
	// API配置
	APIToken   string
	APIBaseURL string
	APITimeout time.Duration

	// 数据库配置
	DatabaseDSN string

	// 处理配置
	RangeStart int
	RangeEnd   int
	BatchSize  int
	MaxRetries int

	// 价格过滤配置
	MinPrice float64
	MaxPrice float64
}

// 统计信息
type Stats struct {
	mu              sync.RWMutex
	TotalProcessed  int
	SuccessCount    int
	ErrorCount      int
	PriceRangeCount int
	SkippedCount    int
	StartTime       time.Time
}

func (s *Stats) IncrementSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SuccessCount++
	s.TotalProcessed++
}

func (s *Stats) IncrementError() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ErrorCount++
	s.TotalProcessed++
}

func (s *Stats) IncrementPriceRange() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PriceRangeCount++
}

func (s *Stats) IncrementSkipped() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SkippedCount++
	s.TotalProcessed++
}

func (s *Stats) GetStats() (int, int, int, int, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TotalProcessed, s.SuccessCount, s.ErrorCount, s.PriceRangeCount, s.SkippedCount
}

func (s *Stats) GetElapsedTime() time.Duration {
	return time.Since(s.StartTime)
}

// API响应结构体
type GoodResponse struct {
	Code int64 `json:"code"`
	Data struct {
		GoodsInfo struct {
			ID             int64   `json:"id"`
			MarketHashName string  `json:"market_hash_name"`
			Name           string  `json:"name"`
			YyypSellPrice  float64 `json:"yyyp_sell_price"`
			BuffSellPrice  float64 `json:"buff_sell_price"`
		} `json:"goods_info"`
	} `json:"data"`
}

// 商品信息结构体
type GoodInfo struct {
	ID             int64
	MarketHashName string
	Name           string
	YyypSellPrice  float64
	BuffSellPrice  float64
}

// 数据库管理器
type DatabaseManager struct {
	db *sql.DB
}

func NewDatabaseManager(dsn string) (*DatabaseManager, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("数据库连接失败: %v", err)
	}

	// 设置连接池参数
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// 测试连接
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("数据库ping失败: %v", err)
	}

	dm := &DatabaseManager{db: db}
	if err := dm.createTables(); err != nil {
		return nil, err
	}

	return dm, nil
}

func (dm *DatabaseManager) createTables() error {
	createTables := []string{
		`CREATE TABLE IF NOT EXISTS csqaq_goods (
			id INT AUTO_INCREMENT PRIMARY KEY,
			good_id BIGINT UNIQUE NOT NULL,
			market_hash_name TEXT,
			name TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			INDEX idx_good_id (good_id),
			INDEX idx_updated_at (updated_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`,
		`CREATE TABLE IF NOT EXISTS csqaq_good_snapshots (
			id INT AUTO_INCREMENT PRIMARY KEY,
			good_id BIGINT NOT NULL,
			yyyp_sell_price DOUBLE,
			buff_sell_price DOUBLE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_good_id (good_id),
			INDEX idx_created_at (created_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`,
	}

	for _, sql := range createTables {
		if _, err := dm.db.Exec(sql); err != nil {
			return fmt.Errorf("创建表失败: %v", err)
		}
	}
	return nil
}

func (dm *DatabaseManager) SaveGoodsBatch(goods []GoodInfo) error {
	if len(goods) == 0 {
		return nil
	}

	tx, err := dm.db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %v", err)
	}
	defer tx.Rollback()

	// 准备批量插入语句
	goodsStmt, err := tx.Prepare(`
		INSERT INTO csqaq_goods (good_id, market_hash_name, name, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON DUPLICATE KEY UPDATE
			market_hash_name = VALUES(market_hash_name),
			name = VALUES(name),
			updated_at = CURRENT_TIMESTAMP
	`)
	if err != nil {
		return fmt.Errorf("准备商品插入语句失败: %v", err)
	}
	defer goodsStmt.Close()

	snapshotStmt, err := tx.Prepare(`
		INSERT INTO csqaq_good_snapshots (good_id, yyyp_sell_price, buff_sell_price, created_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		return fmt.Errorf("准备快照插入语句失败: %v", err)
	}
	defer snapshotStmt.Close()

	for _, good := range goods {
		// 插入商品信息
		if _, err := goodsStmt.Exec(good.ID, good.MarketHashName, good.Name); err != nil {
			log.Printf("保存商品 %d 失败: %v", good.ID, err)
			continue
		}

		// 如果有价格信息，插入快照
		if good.YyypSellPrice > 0 || good.BuffSellPrice > 0 {
			if _, err := snapshotStmt.Exec(good.ID, good.YyypSellPrice, good.BuffSellPrice); err != nil {
				log.Printf("保存快照 %d 失败: %v", good.ID, err)
			}
		}
	}

	return tx.Commit()
}

func (dm *DatabaseManager) Close() error {
	return dm.db.Close()
}

// 全局API调用控制器
type APIController struct {
	lastRequest time.Time
	mu          sync.Mutex
}

var apiController = &APIController{}

// 等待1秒间隔（所有API调用都要等待）
func (ac *APIController) waitInterval() {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	elapsed := time.Since(ac.lastRequest)
	if elapsed < time.Second {
		waitTime := time.Second - elapsed
		log.Printf("等待 %v", waitTime.Truncate(time.Millisecond))
		time.Sleep(waitTime)
	}
	ac.lastRequest = time.Now()
}

// API客户端
type APIClient struct {
	client *http.Client
	config *Config
}

func NewAPIClient(config *Config) *APIClient {
	client := &http.Client{
		Timeout: config.APITimeout,
	}

	return &APIClient{
		client: client,
		config: config,
	}
}

// 绑定本地IP到CSQAQ API
func (ac *APIClient) BindLocalIP(ctx context.Context) error {
	// 等待1秒间隔
	apiController.waitInterval()

	url := fmt.Sprintf("%s/sys/bind_local_ip", ac.config.APIBaseURL)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return fmt.Errorf("创建绑定IP请求失败: %v", err)
	}

	req.Header.Set("ApiToken", ac.config.APIToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := ac.client.Do(req)
	if err != nil {
		return fmt.Errorf("绑定IP请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取绑定IP响应失败: %v", err)
	}

	// 检查HTTP状态码
	if resp.StatusCode == 429 {
		log.Printf("⚠️ IP绑定被限流（最近已绑定）")
		return nil // 限流不算错误
	}

	if resp.StatusCode != 200 {
		log.Printf("❌ IP绑定失败，HTTP状态: %d，响应: %s", resp.StatusCode, string(body))
		return fmt.Errorf("IP绑定失败，HTTP状态: %d", resp.StatusCode)
	}

	log.Printf("✅ 成功绑定本地IP到CSQAQ API")
	return nil
}

func (ac *APIClient) FetchGood(ctx context.Context, id int) (*GoodInfo, error) {
	// 等待1秒间隔
	apiController.waitInterval()

	url := fmt.Sprintf("%s/info/good?id=%d", ac.config.APIBaseURL, id)

	for retry := 0; retry <= ac.config.MaxRetries; retry++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("创建请求失败: %v", err)
		}

		req.Header.Set("ApiToken", ac.config.APIToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "CSGO-Auto-Trader/1.0")

		resp, err := ac.client.Do(req)
		if err != nil {
			if retry < ac.config.MaxRetries {
				log.Printf("请求 good_id %d 失败，重试 %d/%d: %v",
					id, retry+1, ac.config.MaxRetries, err)
				// 重试也要等待1秒间隔
				apiController.waitInterval()
				continue
			}
			return nil, fmt.Errorf("请求失败: %v", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			if retry < ac.config.MaxRetries {
				log.Printf("读取响应失败，重试 %d/%d: %v", retry+1, ac.config.MaxRetries, err)
				apiController.waitInterval()
				continue
			}
			return nil, fmt.Errorf("读取响应失败: %v", err)
		}

		// 检查HTTP状态码
		if resp.StatusCode == 429 {
			if retry < ac.config.MaxRetries {
				log.Printf("限流 good_id %d, 重试 %d/%d", id, retry+1, ac.config.MaxRetries)
				// 限流时也只等待1秒间隔，不额外等待
				apiController.waitInterval()
				continue
			}
			return nil, fmt.Errorf("请求被限流")
		}

		if resp.StatusCode != 200 {
			if retry < ac.config.MaxRetries {
				log.Printf("HTTP错误 %d，good_id %d，重试 %d/%d", resp.StatusCode, id, retry+1, ac.config.MaxRetries)
				apiController.waitInterval()
				continue
			}
			return nil, fmt.Errorf("HTTP错误: %d", resp.StatusCode)
		}

		// 检查是否返回HTML
		if strings.Contains(string(body), "<!DOCTYPE html>") {
			if retry < ac.config.MaxRetries {
				log.Printf("Good ID %d 返回HTML, 重试 %d/%d", id, retry+1, ac.config.MaxRetries)
				apiController.waitInterval()
				continue
			}
			return nil, fmt.Errorf("API返回HTML页面")
		}

		var goodResp GoodResponse
		if err := json.Unmarshal(body, &goodResp); err != nil {
			if retry < ac.config.MaxRetries {
				log.Printf("JSON解析错误，good_id %d，重试 %d/%d: %v", id, retry+1, ac.config.MaxRetries, err)
				apiController.waitInterval()
				continue
			}
			return nil, fmt.Errorf("JSON解析错误: %v", err)
		}

		if goodResp.Code != 200 || goodResp.Data.GoodsInfo.ID == 0 {
			return nil, nil // 商品不存在
		}

		goodInfo := &GoodInfo{
			ID:             goodResp.Data.GoodsInfo.ID,
			MarketHashName: goodResp.Data.GoodsInfo.MarketHashName,
			Name:           goodResp.Data.GoodsInfo.Name,
			YyypSellPrice:  goodResp.Data.GoodsInfo.YyypSellPrice,
			BuffSellPrice:  goodResp.Data.GoodsInfo.BuffSellPrice,
		}

		return goodInfo, nil
	}

	return nil, fmt.Errorf("达到最大重试次数")
}

// 主处理器
type Processor struct {
	config *Config
	client *APIClient
	db     *DatabaseManager
	stats  *Stats
}

func NewProcessor(config *Config) (*Processor, error) {
	client := NewAPIClient(config)

	db, err := NewDatabaseManager(config.DatabaseDSN)
	if err != nil {
		return nil, err
	}

	stats := &Stats{StartTime: time.Now()}

	return &Processor{
		config: config,
		client: client,
		db:     db,
		stats:  stats,
	}, nil
}

func (p *Processor) isInPriceRange(good *GoodInfo) bool {
	return (good.YyypSellPrice >= p.config.MinPrice && good.YyypSellPrice <= p.config.MaxPrice) ||
		(good.BuffSellPrice >= p.config.MinPrice && good.BuffSellPrice <= p.config.MaxPrice)
}

func (p *Processor) ProcessRange(ctx context.Context) error {
	totalItems := p.config.RangeEnd - p.config.RangeStart + 1
	log.Printf("开始处理范围: %d - %d (%d 个商品)",
		p.config.RangeStart, p.config.RangeEnd, totalItems)

	var batch []GoodInfo

	for id := p.config.RangeStart; id <= p.config.RangeEnd; id++ {
		select {
		case <-ctx.Done():
			// 保存剩余批次
			if len(batch) > 0 {
				p.saveBatch(batch)
			}
			return ctx.Err()
		default:
		}

		good, err := p.client.FetchGood(ctx, id)
		if err != nil {
			log.Printf("获取 good_id %d 失败: %v", id, err)
			p.stats.IncrementError()
			continue
		}

		if good == nil {
			p.stats.IncrementSkipped()
			continue
		}

		// 检查价格范围
		if !p.isInPriceRange(good) && (good.YyypSellPrice > 0 || good.BuffSellPrice > 0) {
			log.Printf("跳过 good_id %d (%s), 价格超出范围: YYYP=%.2f, Buff=%.2f",
				good.ID, good.Name, good.YyypSellPrice, good.BuffSellPrice)
			p.stats.IncrementSkipped()
			continue
		}

		// 添加到批次
		batch = append(batch, *good)

		if p.isInPriceRange(good) {
			p.stats.IncrementPriceRange()
		}

		// 批次已满，保存到数据库
		if len(batch) >= p.config.BatchSize {
			p.saveBatch(batch)
			batch = batch[:0] // 重置切片
		}

		// 每100个显示进度
		if (id-p.config.RangeStart+1)%100 == 0 {
			processed, success, errors, priceRange, skipped := p.stats.GetStats()
			progress := float64(id-p.config.RangeStart+1) / float64(totalItems) * 100
			elapsed := p.stats.GetElapsedTime()
			rate := float64(processed) / elapsed.Seconds()

			log.Printf("进度: %.1f%% (%d/%d), 成功: %d, 错误: %d, 价格范围内: %d, 跳过: %d, 速率: %.2f/s",
				progress, id-p.config.RangeStart+1, totalItems, success, errors, priceRange, skipped, rate)
		}
	}

	// 保存剩余批次
	if len(batch) > 0 {
		p.saveBatch(batch)
	}

	return nil
}

func (p *Processor) saveBatch(batch []GoodInfo) {
	if err := p.db.SaveGoodsBatch(batch); err != nil {
		log.Printf("批量保存失败: %v", err)
		for range batch {
			p.stats.IncrementError()
		}
	} else {
		for _, good := range batch {
			if p.isInPriceRange(&good) {
				log.Printf("✓ 保存 good_id %d (%s), 价格: YYYP=%.2f, Buff=%.2f",
					good.ID, good.Name, good.YyypSellPrice, good.BuffSellPrice)
			} else {
				log.Printf("✓ 保存 good_id %d (%s), 无价格数据", good.ID, good.Name)
			}
			p.stats.IncrementSuccess()
		}
	}
}

func (p *Processor) StartProgressReporter(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	totalItems := p.config.RangeEnd - p.config.RangeStart + 1

	for {
		select {
		case <-ticker.C:
			processed, success, errors, priceRange, skipped := p.stats.GetStats()
			elapsed := p.stats.GetElapsedTime()

			progress := float64(processed) / float64(totalItems) * 100
			rate := float64(processed) / elapsed.Seconds()

			log.Printf("进度: %.1f%% (%d/%d), 成功: %d, 错误: %d, 价格范围内: %d, 跳过: %d, 速率: %.2f/s, 用时: %v",
				progress, processed, totalItems, success, errors, priceRange, skipped, rate, elapsed.Truncate(time.Second))

		case <-ctx.Done():
			return
		}
	}
}

// 启动IP绑定定时器
func (p *Processor) StartIPBindingTimer(ctx context.Context) {
	// 立即绑定一次
	if err := p.client.BindLocalIP(ctx); err != nil {
		log.Printf("初始IP绑定失败: %v", err)
	}

	// 每30秒重新绑定
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			log.Printf("🔗 开始重新绑定IP...")
			if err := p.client.BindLocalIP(ctx); err != nil {
				log.Printf("重新绑定IP失败: %v", err)
			}

		case <-ctx.Done():
			return
		}
	}
}

func (p *Processor) Close() error {
	return p.db.Close()
}

// 默认配置
func getDefaultConfig() *Config {
	return &Config{
		APIToken:    "UAXMU177X578K1Q9E1G0N5M8",
		APIBaseURL:  "https://api.csqaq.com/api/v1",
		APITimeout:  15 * time.Second,
		DatabaseDSN: "root:Wyj250413.@tcp(23.254.215.66:3306)/csgo_trader?charset=utf8mb4&parseTime=True&loc=Local",
		RangeStart:  0,
		RangeEnd:    24041,
		BatchSize:   50,
		MaxRetries:  3,
		MinPrice:    3.0,
		MaxPrice:    50.0,
	}
}

func main() {
	// 设置日志格式
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	config := getDefaultConfig()

	// 解析命令行参数
	if len(os.Args) > 1 {
		if interval, err := strconv.Atoi(os.Args[1]); err == nil {
			log.Printf("自定义间隔时间: %d毫秒", interval)
		}
	}

	log.Printf("开始获取饰品数据: ID %d 到 %d", config.RangeStart, config.RangeEnd)
	log.Printf("工作模式: 绑定IP -> 等待1秒 -> 查询商品 -> 重复")
	log.Printf("IP重绑定: 每30秒自动重新绑定")

	// 创建处理器
	processor, err := NewProcessor(config)
	if err != nil {
		log.Fatalf("创建处理器失败: %v", err)
	}
	defer processor.Close()

	// 设置信号处理
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("收到停止信号，正在优雅关闭...")
		cancel()
	}()

	// 启动进度报告器
	go processor.StartProgressReporter(ctx)

	// 启动IP绑定定时器
	go processor.StartIPBindingTimer(ctx)

	// 开始处理
	startTime := time.Now()
	if err := processor.ProcessRange(ctx); err != nil {
		log.Fatalf("处理失败: %v", err)
	}

	// 输出最终统计
	processed, success, errors, priceRange, skipped := processor.stats.GetStats()
	elapsed := time.Since(startTime)

	log.Printf("\n=== 处理完成！===")
	log.Printf("总计处理: %d, 成功: %d, 错误: %d, 价格范围内: %d, 跳过: %d",
		processed, success, errors, priceRange, skipped)
	log.Printf("总用时: %v, 平均速率: %.2f/s",
		elapsed.Truncate(time.Second), float64(processed)/elapsed.Seconds())
}
