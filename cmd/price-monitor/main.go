package main

import (
	"context"
	"csgo-trader/internal/database"
	"csgo-trader/internal/services/youpin"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"
)

var (
	interval      = flag.Int("interval", 1800, "监控间隔（秒，默认3600=1小时）")
	minProfitRate = flag.Float64("min-profit", 0.08, "最小利润率（默认8%）")
	token         = flag.String("token", "", "悠悠有品Token（如不指定，使用硬编码值）")
	dbURL         = flag.String("db", "", "数据库连接字符串（如不指定，使用默认值）")
	logFile       = flag.String("log", "", "日志文件路径")
	once          = flag.Bool("once", false, "只运行一次，不循环")
	// 代理相关参数（硬编码默认值）
	useProxy     = flag.Bool("use-proxy", false, "是否使用代理")
	proxyURL     = flag.String("proxy-url", "hk.novproxy.io:1000", "代理服务器地址")
	proxyUser    = flag.String("proxy-user", "xkuq4621-region-US", "代理用户名")
	proxyPass    = flag.String("proxy-pass", "58hb6rzr", "代理密码")
	proxyTimeout = flag.Int("proxy-timeout", 30, "代理请求超时时间（秒）")
)

// ===== 硬编码的配置常量 =====

// YouPinConfig 悠悠有品认证配置（硬编码）
const (
	// 悠悠有品 Token（必需）
	// 替换为您的实际 Token
	YouPinToken = "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJqdGkiOiI5Y2IyNjJkYWFlMDE0NjdkOWRkOTdkZDY2NGVmZjhmMiIsIm5hbWVpZCI6IjE2NDUyMzEiLCJJZCI6IjE2NDUyMzEiLCJ1bmlxdWVfbmFtZSI6IllQMDAwMTY0NTIzMSIsIk5hbWUiOiJZUDAwMDE2NDUyMzEiLCJ2ZXJzaW9uIjoiSTFqIiwibmJmIjoxNzYyODI3NDg3LCJleHAiOjE3NjM2OTE0ODcsImlzcyI6InlvdXBpbjg5OC5jb20iLCJkZXZpY2VJZCI6ImU3ZGYzOWQ1LTEzZjYtNDZmMS1hNDI0LTFmZDU5YjU4NTk4OCIsImF1ZCI6InVzZXIifQ.CRb9VDDtCVvJBlvzLjqTWxYH_A7hBxt8mBluB00WiRE"

	// OpenAPI 配置
	OpenAPIAppKey = "1645231"
	OpenAPISecret = "MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDU5dQfzetezfTYdELMppkeeEdlvO7iHrTU8CgfO19cOo9dfmsCnPH+VlGkvya3oqkGjJd6l3gh//V7hJevJvNxbLTKdC+o3wVm17xUhcfJCxiDJSDM7Lxop2Hw7zEO5/yOjJP3OfUhZ7vh6zGjhtaBpXvvf8PLzr5d3rhh8ihaUGhNJwf0dJ8gyWaeOvkkcwnDOzKY3AgQjgx+ZwXBIlk+eG7/pPkwA6mM5zhSOE3RwOQKGC8k5bamJC9kU+cr/kYL1QF3+matq3bmjItr3v6gLDzf/IujjWV/iaO0ePQyUsbm7PS27ICOqiE7Jk+KvEjdMY/Fz1lEOnt2mvsO7/zHAgMBAAECggEAIffTiwmHYtZ4mOqf19hC+P4W4jAtay2cC5ePxz/pXKVJR5EKkN2qrLpoB2GqU0VkM5PN/XTaaY5VxBHpQ7xyvieqhtzX19lRmtGUDmZT9ItNK2uKmrew7f+63D7FtIumG7ZpS1pXdq9+5jJo9p7mbcQSDKn1evivHfoRsCr7bkE4fHkrgRgaa+BDXBvKEaQKBIlvcZbAGHiX54QpVbygrZJhImFYKNbH8uRzBNXKrmX0CBSsoCXyiesF4w+Hk6lFBEs9bj7VFIm6mi88XN+xRrXVcU9tOSg8BuQdnr43WoRG3Xq7OAs7496Q5hzseG0x3t7vUEUNRgoJbQ6gbXA/xQKBgQDgUfij/RWip2AZE4GEVC2odGiqzk3Z7HoI3SCsn9Z/dg0QdjaUoTqFFTSHCmv8fQtCCyK/eT33m+8FYIS0L1kC5E4JdXklnVKNl3Pzkt7VaZAAa3l1cT+egtYkQKlYhouslHT6ST8waQBh2FncCfPU/wN082fosgwtr6MRpGXlNQKBgQDy9uSZ5P6P3vWw+u+O2i8JOw3TxY5lZGnKjxIOtMROXPPdoXkzZ6bu+0fzlYKD4eJEEYYJTUL8BDuChEIaleWSu5pEhH/5HQJmQcT1zu/gZJLB+n69gcyUSxP4bFwu/O8DEeG8vQNoL7CPM0IZZjpTOzSmODSAydJccJsDhmiFiwKBgQDbPoLcFOJuhVShbbUq1vOBL7WVK+pfUoe73hSvY9HL5l/CdSfHgQWnSSB71C3TK4wzUpr1tdRhDCFDiiBp09UVxHqZOB3eK7Oh/CMyL5xnzbMXFfQyWyupC4utBx39DhO566ehcLG36QicmU7Kh1ewGEcSqUwn3M2WYZHqDlog+QKBgQCJPH/flYNcjJnGv9b0n7UVx6+FwP8vsko3ShzvBZgkk0iEFaM6MAAQ2QrJQdgY2kxOGn/VXTjK1TEwBbi4/5ZPDXyT2yvV8Fbcn6W7GopP+z8SJoXUUS7XTJkZJ0vilqgC2eTiEPmNrfJS0KczZZToUnbotLKjVFxoLorAsPj1BQKBgHBI1FcXoGZNDEO4hL0PLJZJIBDFj4QvmXfIVyQJXZjFtcgC9TkyGsF4B6b4bty+2K5i9No+PYLxLdJ7X+mKPXfJdGpT+LvH8S8+mhShCbRKfklVKWlKvZWGjARyoF1yQdAocu8yECCsMWz5zOJC3Z9v9JBbVpNDv2RrbEZ1hdNX"
)

// DatabaseConfig 数据库配置（硬编码）
const (
	// MySQL 连接字符串
	// 格式: username:password@tcp(host:port)/database?charset=utf8mb4&parseTime=True&loc=Local
	DefaultDBURL = "root:Wyj250413.@tcp(192.3.81.194:3306)/csgo_trader?charset=utf8mb4&parseTime=True&loc=Local"
)

// MonitorState 监控状态
type MonitorState struct {
	mu              sync.RWMutex
	LastUpdateTime  time.Time
	ActiveBuyOrders map[string]*BuyOrderInfo // OrderNo -> BuyOrderInfo
	ActiveSales     map[int64]*SaleOrderInfo // CommodityID -> SaleOrderInfo
	ErrorCount      int
	UpdateCount     int
}

// BuyOrderInfo 求购订单信息
type BuyOrderInfo struct {
	OrderNo       string    `json:"order_no"`
	TemplateID    int       `json:"template_id"`
	CommodityName string    `json:"commodity_name"`
	Price         float64   `json:"price"`
	Quantity      int       `json:"quantity"`
	Rank          string    `json:"rank"`         // 排名
	BuyQuantity   int       `json:"buy_quantity"` // 已收货数量
	CostPrice     float64   `json:"cost_price"`   // 成本价
	TargetPrice   float64   `json:"target_price"` // 目标售价
	CreatedAt     time.Time `json:"created_at"`
	LastUpdated   time.Time `json:"last_updated"`
}

// SaleOrderInfo 出售订单信息
type SaleOrderInfo struct {
	CommodityID    int64     `json:"commodity_id"`
	TemplateID     int       `json:"template_id"`
	CommodityName  string    `json:"commodity_name"`
	Price          float64   `json:"price"`
	CostPrice      float64   `json:"cost_price"`
	MinMarketPrice float64   `json:"min_market_price"` // 市场最低售价
	Status         string    `json:"status"`           // listed/sold
	CreatedAt      time.Time `json:"created_at"`
	LastUpdated    time.Time `json:"last_updated"`
}

// PriceMonitor 价格监控器
type PriceMonitor struct {
	client        *youpin.Client
	openAPIClient *youpin.OpenAPIClient
	db            *gorm.DB
	state         *MonitorState
	ctx           context.Context
	cancel        context.CancelFunc
	logger        *log.Logger
}

func main() {
	flag.Parse()

	// 初始化日志
	var logWriter *os.File
	var err error
	if *logFile != "" {
		logWriter, err = os.OpenFile(*logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatalf("无法打开日志文件: %v", err)
		}
		defer logWriter.Close()
	} else {
		logWriter = os.Stdout
	}

	logger := log.New(logWriter, "[PriceMonitor] ", log.LstdFlags|log.Lshortfile)

	// ===== 获取认证配置 =====
	// 优先级：命令行参数 > 环境变量 > 硬编码配置

	// 获取数据库连接字符串
	currentDBURL := *dbURL
	if currentDBURL == "" {
		// 检查环境变量
		if envDB := os.Getenv("DATABASE_URL"); envDB != "" {
			currentDBURL = envDB
			logger.Printf("✓ 从环境变量 DATABASE_URL 获取数据库连接字符串")
		} else {
			// 使用硬编码的默认数据库连接
			currentDBURL = DefaultDBURL
			logger.Printf("✓ 使用硬编码的默认数据库连接")
		}
	} else {
		logger.Printf("✓ 使用命令行参数数据库连接字符串")
	}

	// ===== 初始化客户端 =====
	// 初始化悠悠有品客户端（Token认证）
	var client *youpin.Client
	var openAPIClient *youpin.OpenAPIClient
	var proxyURLWithAuth string

	if *useProxy {
		proxyURLWithAuth = fmt.Sprintf("http://%s:%s@%s", *proxyUser, *proxyPass, *proxyURL)
		initTimeout := time.Duration(*proxyTimeout) * time.Second
		if initTimeout < 30*time.Second {
			initTimeout = 30 * time.Second // 初始化至少使用30秒超时
		}

		// 初始化 Token 认证客户端（支持代理）
		openAPIClient, err = youpin.NewOpenAPIClientWithDefaultKeysAndTokenAndProxy(YouPinToken, proxyURLWithAuth, initTimeout)
		if err != nil {
			logger.Fatalf("初始化悠悠有品客户端（代理）失败: %v", err)
		}
		logger.Printf("✓ 悠悠有品客户端初始化成功（使用代理）")
	} else {
		// 不使用代理初始化
		openAPIClient, err = youpin.NewOpenAPIClientWithDefaultKeysAndToken(YouPinToken)
		if err != nil {
			logger.Fatalf("初始化悠悠有品客户端失败: %v", err)
		}
		logger.Printf("✓ 悠悠有品客户端初始化成功")
	}

	// 初始化数据库
	db, err := database.Initialize(currentDBURL)
	if err != nil {
		logger.Fatalf("数据库初始化失败: %v", err)
	}

	logger.Printf("✓ 数据库连接成功")

	// 创建监控器
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	monitor := &PriceMonitor{
		client:        client,
		openAPIClient: openAPIClient,
		db:            db,
		state: &MonitorState{
			ActiveBuyOrders: make(map[string]*BuyOrderInfo),
			ActiveSales:     make(map[int64]*SaleOrderInfo),
		},
		ctx:    ctx,
		cancel: cancel,
		logger: logger,
	}

	logger.Printf("🚀 价格监控器启动成功")
	logger.Printf("配置: 监控间隔=%ds, 最小利润率=%.2f%%, 价格步长=智能规则", *interval, *minProfitRate*100)
	logger.Printf("  价格步长规则: ¥0-1 → 0.01 | ¥1-50 → 0.1 | ¥50-1000 → 1.0")

	// 运行监控循环
	if *once {
		// 只运行一次
		monitor.runOnce()
	} else {
		// 循环运行
		monitor.runLoop()
	}
}

// runOnce 运行一次监控
func (pm *PriceMonitor) runOnce() {
	pm.logger.Printf("执行单次监控...")
	err := pm.Monitor()
	if err != nil {
		pm.logger.Printf("❌ 监控错误: %v", err)
		pm.state.ErrorCount++
	}
	pm.printStatus()
}

// runLoop 循环运行监控
func (pm *PriceMonitor) runLoop() {
	ticker := time.NewTicker(time.Duration(*interval) * time.Second)
	defer ticker.Stop()

	// 首次立即执行
	err := pm.Monitor()
	if err != nil {
		pm.logger.Printf("❌ 监控错误: %v", err)
		pm.state.ErrorCount++
	}
	pm.printStatus()

	for {
		select {
		case <-ticker.C:
			err := pm.Monitor()
			if err != nil {
				pm.logger.Printf("❌ 监控错误: %v", err)
				pm.state.ErrorCount++
			}
			pm.printStatus()
		case <-pm.ctx.Done():
			pm.logger.Printf("监控器已停止")
			return
		}
	}
}

// Monitor 执行一次完整的监控
func (pm *PriceMonitor) Monitor() error {
	pm.logger.Printf("--- 开始监控周期 ---")

	ctx, cancel := context.WithTimeout(pm.ctx, 30*time.Second)
	defer cancel()

	// 1. 获取我的求购订单列表
	pm.logger.Printf("📋 获取求购订单列表...")
	buyOrders, err := pm.fetchMyBuyOrders(ctx)
	if err != nil {
		return fmt.Errorf("获取求购订单失败: %w", err)
	}
	pm.logger.Printf("✓ 获取了 %d 个求购订单", len(buyOrders))

	// 2. 处理每个求购订单
	for _, buyOrder := range buyOrders {
		err := pm.processBuyOrder(ctx, &buyOrder)
		if err != nil {
			pm.logger.Printf("❌ 处理求购订单 %s 失败: %v", buyOrder.OrderNo, err)
		}
	}

	pm.state.mu.Lock()
	pm.state.LastUpdateTime = time.Now()
	pm.state.UpdateCount++
	pm.state.mu.Unlock()

	pm.logger.Printf("--- 监控周期完成 ---")
	return nil
}

// processBuyOrder 处理求购订单
// 流程：
// 1. 检查排名，如果是第一位，检查第二位价格是否差很多，如果是则减价后检查利润
// 2. 如果不是第一位，检查加价后利润是否满足要求，如果满足则加价
// 3. 如果利润不满足，检查是否还有库存没出售，如果有则删除求购
func (pm *PriceMonitor) processBuyOrder(ctx context.Context, buyOrder *youpin.MyPurchaseOrderItem) error {
	templateID := buyOrder.TemplateId
	price := parseFloat(buyOrder.UnitPrice)
	rank := buyOrder.Rank
	quantity := buyOrder.Quantity
	_ = quantity - buyOrder.BuyQuantity

	pm.logger.Printf("  📌 求购: %s(ID:%d) 价格:¥%.2f 排名:%s 库存:%d/%d",
		buyOrder.CommodityName, templateID, price, rank, buyOrder.BuyQuantity, quantity)

	// 获取市场信息
	marketInfo, err := pm.getMarketInfo(ctx, templateID)
	if err != nil {
		return fmt.Errorf("获取市场信息失败: %w", err)
	}

	pm.logger.Printf("    市场最低售价: ¥%.2f", marketInfo.LowestPrice)

	// 检查排名
	isFirst := rank == "1"

	if isFirst {
		pm.logger.Printf("    ✅ 当前为第一位求购")
		return pm.handleFirstRankBuyOrder(ctx, buyOrder, marketInfo)
	} else {
		pm.logger.Printf("    ❌ 当前非第一位 (排名:%s)", rank)
		return pm.handleNonFirstRankBuyOrder(ctx, buyOrder, marketInfo)
	}
}

// handleFirstRankBuyOrder 处理第一位求购
// 检查第二位价格是否差很多，如果是则减价；否则维持当前价格
func (pm *PriceMonitor) handleFirstRankBuyOrder(ctx context.Context, buyOrder *youpin.MyPurchaseOrderItem, marketInfo *MarketInfo) error {
	currentPrice := parseFloat(buyOrder.UnitPrice)

	// 获取其他求购订单信息
	otherOrders, err := pm.getOtherBuyOrders(ctx, buyOrder.TemplateId)
	if err != nil {
		pm.logger.Printf("    ⚠️  获取其他求购信息失败: %v", err)
		return nil // 不中断流程
	}

	if len(otherOrders) == 0 {
		pm.logger.Printf("    ℹ️  只有我的求购单，无需比较")
		return nil
	}

	// 获取第二位的价格
	secondPrice := otherOrders[0].Price
	priceDiff := currentPrice - secondPrice
	priceDiffPercent := priceDiff / secondPrice

	pm.logger.Printf("    第二位价格: ¥%.2f, 价差: ¥%.2f (%.2f%%)", secondPrice, priceDiff, priceDiffPercent*100)

	// 如果价差超过5%，则进行减价处理
	if priceDiffPercent > 0.05 {
		pm.logger.Printf("    📉 价差超过5%%，准备减价")

		// 计算新价格：降到第二位价格以上最小步长
		newPrice := getNextPrice(secondPrice)

		// 计算新价格下的利润
		profit := (marketInfo.LowestPrice - newPrice) / newPrice
		pm.logger.Printf("    新价格: ¥%.2f (步长: ¥%.2f), 新利润率: %.2f%%", newPrice, calculatePriceStep(newPrice), profit*100)

		if profit >= *minProfitRate {
			pm.logger.Printf("    ✅ 新价格仍满足最小利润率 %.2f%%，进行减价", *minProfitRate*100)
			return pm.updateBuyOrderPrice(ctx, buyOrder.OrderNo, newPrice, buyOrder.TemplateId, buyOrder.CommodityName)
		} else {
			pm.logger.Printf("    ❌ 新价格利润 %.2f%% < 最小要求 %.2f%%，准备删除求购", profit*100, *minProfitRate*100)
			pm.logger.Printf("    ❌ 新价格利润 %.2f%% < 最小要求 %.2f%%，删除求购", profit*100, *minProfitRate*100)
			return pm.deleteBuyOrder(ctx, buyOrder.OrderNo)
		}
	} else {
		pm.logger.Printf("    ℹ️  价差 %.2f%% ≤ 5%%，维持当前价格", priceDiffPercent*100)
	}

	return nil
}

// handleNonFirstRankBuyOrder 处理非第一位求购
// 检查加价后利润是否满足要求，如果满足则加价
// 首先检查账户余额，确保有足够的余额进行加价
func (pm *PriceMonitor) handleNonFirstRankBuyOrder(ctx context.Context, buyOrder *youpin.MyPurchaseOrderItem, marketInfo *MarketInfo) error {
	// 获取第一位的价格
	firstOrders, err := pm.getFirstBuyOrder(ctx, buyOrder.TemplateId)
	if err != nil {
		pm.logger.Printf("    ⚠️  获取第一位求购价格失败: %v", err)
		return nil
	}

	if len(firstOrders) == 0 {
		pm.logger.Printf("    ℹ️  无其他求购，保持当前价格")
		return nil
	}

	firstPrice := firstOrders[0].Price
	pm.logger.Printf("    第一位求购价格: ¥%.2f", firstPrice)

	// 计算加价后的新价格（超过第一位最小步长）
	newPrice := getNextPrice(firstPrice)

	// 验证加价的利润是否满足要求
	// 利润 = (市场最低售价 - 新求购价) / 新求购价
	profit := (marketInfo.LowestPrice - newPrice) / newPrice

	pm.logger.Printf("    📈 加价后: ¥%.2f (步长: ¥%.2f), 对应利润率: %.2f%%", newPrice, calculatePriceStep(newPrice), profit*100)

	if profit >= *minProfitRate {
		pm.logger.Printf("    ✅ 加价后利润 %.2f%% ≥ 最小要求 %.2f%%，检查账户余额", profit*100, *minProfitRate*100)

		// 检查账户余额
		balances, err := pm.openAPIClient.GetBalances(ctx)
		if err != nil {
			pm.logger.Printf("    ⚠️  获取账户余额失败: %v，跳过加价", err)
			return nil
		}

		// 计算加价需要增加的金额
		originalPrice := parseFloat(buyOrder.UnitPrice)
		quantity := buyOrder.Quantity
		priceDifference := newPrice - originalPrice
		additionalAmount := priceDifference * float64(quantity)

		pm.logger.Printf("    💰 原始价格: ¥%.2f, 新价格: ¥%.2f, 数量: %d", originalPrice, newPrice, quantity)
		pm.logger.Printf("    💰 求购余额: ¥%.2f, 需要增加: ¥%.2f", balances.PurchaseBalance, additionalAmount)

		// 检查余额是否充足
		if balances.PurchaseBalance >= additionalAmount {
			pm.logger.Printf("    ✅ 余额充足，进行加价")
			return pm.updateBuyOrderPrice(ctx, buyOrder.OrderNo, newPrice, buyOrder.TemplateId, buyOrder.CommodityName)
		} else {
			pm.logger.Printf("    ❌ 余额不足 (缺少 ¥%.2f)，不进行加价", additionalAmount-balances.PurchaseBalance)
			return nil
		}
	} else {
		pm.logger.Printf("    ❌ 加价后利润 %.2f%% < 最小要求 %.2f%%，不进行加价", profit*100, *minProfitRate*100)
		return nil
	}
}

// ===== 辅助方法 =====

// calculatePriceStep 根据价格区间计算合适的价格步长
// 规则：
// - 0 < price <= 1: 步长 0.01
// - 1 < price <= 50: 步长 0.1
// - 50 < price <= 1000: 步长 1
func calculatePriceStep(price float64) float64 {
	switch {
	case price > 0 && price <= 1:
		return 0.01
	case price > 1 && price <= 50:
		return 0.1
	case price > 50 && price <= 1000:
		return 1.0
	default:
		// 超出范围，默认按1元步长
		return 1.0
	}
}

// roundPriceByStep 根据价格所在区间对价格进行舍入
// 确保价格是对应步长的整数倍
func roundPriceByStep(price float64) float64 {
	step := calculatePriceStep(price)
	// 四舍五入到最近的步长倍数
	return math.Round(price/step) * step
}

// getNextPrice 获取下一个合法的价格
// 根据价格区间的规则，计算应该增加的最小价格
func getNextPrice(currentPrice float64) float64 {
	step := calculatePriceStep(currentPrice)
	nextPrice := currentPrice + step
	// 确保新价格也符合相应区间的规则
	// 如果跨越了区间边界，需要重新计算步长
	newStep := calculatePriceStep(nextPrice)
	if newStep != step {
		nextPrice = roundPriceByStep(nextPrice)
	}
	return nextPrice
}

// getReducedPrice 获取降低后的价格
// 根据价格区间的规则，计算应该减少的最小价格
func getReducedPrice(currentPrice float64) float64 {
	step := calculatePriceStep(currentPrice)
	reducedPrice := currentPrice - step
	if reducedPrice < 0 {
		return 0
	}
	// 确保新价格也符合相应区间的规则
	newStep := calculatePriceStep(reducedPrice)
	if newStep != step {
		reducedPrice = roundPriceByStep(reducedPrice)
	}
	return reducedPrice
}

// fetchMyBuyOrders 获取我的求购订单列表
func (pm *PriceMonitor) fetchMyBuyOrders(ctx context.Context) ([]youpin.MyPurchaseOrderItem, error) {
	// 调用 Client 中的 SearchPurchaseOrderList（通过 Token 认证）
	response, err := pm.openAPIClient.SearchPurchaseOrderList(ctx, &youpin.SearchPurchaseOrderListRequest{
		// TODO: 分页
		PageIndex: 1,
		PageSize:  40,
		Status:    20,
		Sessionid: "aNbW21QU7cUDAJB4bK22q1rk", // 设备Token用作Sessionid
	})
	if err != nil {
		return nil, fmt.Errorf("获取求购列表失败: %w", err)
	}

	pm.logger.Printf("✓ 获取了 %d 个求购订单", len(response.Data))
	return response.Data, nil
}

// updateBuyOrderPrice 更新求购订单价格
func (pm *PriceMonitor) updateBuyOrderPrice(ctx context.Context, orderNo string, newPrice float64, templateID int, commodityName string) error {
	pm.logger.Printf("    🔄 更新求购价格: ¥%.2f", newPrice)

	// 先获取订单详情
	detail, err := pm.openAPIClient.GetPurchaseOrderDetail(ctx, orderNo)
	if err != nil {
		return fmt.Errorf("获取订单详情失败: %w", err)
	}

	// 准备更新请求
	req := youpin.UpdatePurchaseOrderRequest{
		OrderNo:           orderNo,
		TemplateId:        templateID,
		TemplateHashName:  "", // 需要从其他来源获取
		CommodityName:     commodityName,
		PurchasePrice:     newPrice,
		PurchaseNum:       detail.Data.Quantity,
		ReferencePrice:    "", // 需要从其他来源获取
		MinSellPrice:      "", // 需要从其他来源获取
		MaxPurchasePrice:  detail.Data.MaxPurchasePrice,
		TemplateName:      commodityName,
		NeedPaymentAmount: newPrice * float64(detail.Data.Quantity),
		TotalAmount:       newPrice * float64(detail.Data.Quantity),
	}

	// 调用更新 API
	response, err := pm.openAPIClient.UpdatePurchaseOrder(ctx, &req)
	if err != nil {
		return fmt.Errorf("更新求购价格失败: %w", err)
	}

	if response.Code != 0 {
		return fmt.Errorf("API返回错误: %s", response.Msg)
	}

	pm.logger.Printf("    ✅ 求购订单 %s 已更新为 ¥%.2f", orderNo, newPrice)
	return nil
}

// deleteBuyOrder 删除求购订单
func (pm *PriceMonitor) deleteBuyOrder(ctx context.Context, orderNo string) error {
	pm.logger.Printf("    🗑️  删除求购订单")

	// 准备删除请求（支持批量，这里只删除一个）
	orderNoList := []string{orderNo}

	// 调用删除 API
	response, err := pm.openAPIClient.DeletePurchaseOrder(ctx, orderNoList, "aNbW21QU7cUDAJB4bK22q1rk")
	if err != nil {
		return fmt.Errorf("删除求购订单失败: %w", err)
	}

	if response.Code != 0 {
		return fmt.Errorf("API返回错误: %s", response.Msg)
	}

	pm.logger.Printf("    ✅ 求购订单 %s 已删除", orderNo)
	return nil
}

// getMarketInfo 获取市场信息
type MarketInfo struct {
	LowestPrice float64
	HighestBuy  float64
	SellCount   int
	BuyCount    int
}

func (pm *PriceMonitor) getMarketInfo(ctx context.Context, templateID int) (*MarketInfo, error) {
	// 使用 OpenAPI 调用批量查询在售商品信息
	requestList := []youpin.BatchPriceQueryItem{
		{
			TemplateID: &templateID,
		},
	}

	response, err := pm.openAPIClient.BatchGetOnSaleCommodityInfo(ctx, requestList)
	if err != nil {
		return nil, fmt.Errorf("查询在售商品信息失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}

	if len(response.Data) == 0 {
		return nil, fmt.Errorf("未找到商品在售信息")
	}

	// 解析响应数据
	commodity := response.Data[0]
	minPrice, _ := strconv.ParseFloat(commodity.SaleCommodityResponse.MinSellPrice, 64)
	sellNum := commodity.SaleCommodityResponse.SellNum

	return &MarketInfo{
		LowestPrice: minPrice,
		SellCount:   sellNum,
		HighestBuy:  0,
		BuyCount:    0,
	}, nil
}

// getOtherBuyOrders 获取其他用户的求购订单（用于获取排名）
type BuyOrderItem struct {
	Price    float64
	Quantity int
	Username string
}

func (pm *PriceMonitor) getOtherBuyOrders(ctx context.Context, templateID int) ([]BuyOrderItem, error) {
	// 调用获取商品求购列表 API
	response, err := pm.openAPIClient.GetTemplatePurchaseOrderList(ctx, &youpin.GetTemplatePurchaseOrderListRequest{
		TemplateId: templateID,
		PageIndex:  1,
		PageSize:   100,
	})
	if err != nil {
		return nil, fmt.Errorf("获取求购订单列表失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}

	var result []BuyOrderItem
	for _, item := range response.Data {
		result = append(result, BuyOrderItem{
			Price:    item.PurchasePrice,
			Quantity: item.SurplusQuantity,
		})
	}

	pm.logger.Printf("✓ 获取了 %d 个其他用户求购订单", len(result))
	return result, nil
}

// getFirstBuyOrder 获取排名第一的求购订单
func (pm *PriceMonitor) getFirstBuyOrder(ctx context.Context, templateID int) ([]BuyOrderItem, error) {
	// 调用获取所有订单，然后取第一个
	orders, err := pm.getOtherBuyOrders(ctx, templateID)
	if err != nil {
		return nil, err
	}

	if len(orders) > 0 {
		return orders[:1], nil
	}

	return []BuyOrderItem{}, nil
}

// parseFloat 安全的字符串转浮点数
func parseFloat(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// printStatus 打印监控状态
func (pm *PriceMonitor) printStatus() {
	pm.state.mu.RLock()
	defer pm.state.mu.RUnlock()

	activeBuys := len(pm.state.ActiveBuyOrders)
	activeSales := len(pm.state.ActiveSales)

	pm.logger.Printf("\n📊 监控状态统计")
	pm.logger.Printf("├─ 活跃求购订单: %d", activeBuys)
	pm.logger.Printf("├─ 活跃出售商品: %d", activeSales)
	pm.logger.Printf("├─ 更新次数: %d", pm.state.UpdateCount)
	pm.logger.Printf("├─ 错误次数: %d", pm.state.ErrorCount)
	pm.logger.Printf("└─ 上次更新: %v\n", pm.state.LastUpdateTime.Format("2006-01-02 15:04:05"))
}
