package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"csgo-trader/internal/database"
	"csgo-trader/internal/models"
	"csgo-trader/internal/services/youpin"
)

// PriceRule 定义不同价格档位的最小差值规则
type PriceRule struct {
	MaxPrice float64 // 此价格以下适用此规则
	MinDiff  float64 // 最小差值（与最低在售价的差）
	RuleDesc string  // 规则描述
}

var priceRules = []PriceRule{
	{MaxPrice: 10, MinDiff: 1, RuleDesc: "10块以下的饰品最小差值1元"},
	{MaxPrice: 50, MinDiff: 3, RuleDesc: "10-50块的饰品最小差值3元"},
	{MaxPrice: 150, MinDiff: 20, RuleDesc: "50-150块的饰品最小差值20元"},
	{MaxPrice: math.MaxFloat64, MinDiff: 0, RuleDesc: "150块以上的饰品无最小差值要求"},
}

// MonitorConfig 监听配置
type MonitorConfig struct {
	Token         string        // 用户Token
	CheckInterval time.Duration // 检查间隔
	MaxRetries    int           // 最大重试次数
	RetryDelay    time.Duration // 重试延迟
	AutoDelete    bool          // 是否自动删除不符合条件的求购
	DryRun        bool          // 是否干运行（只输出，不执行删除）
}

// PurchaseChecker 求购检查器
type PurchaseChecker struct {
	apiClient *youpin.OpenAPIClient
	config    MonitorConfig
	logger    *log.Logger
}

// NewPurchaseChecker 创建新的求购检查器
func NewPurchaseChecker(config MonitorConfig) (*PurchaseChecker, error) {
	// 创建OpenAPI客户端
	apiClient, err := youpin.NewOpenAPIClientWithDefaultKeysAndToken(config.Token)
	if err != nil {
		return nil, fmt.Errorf("创建API客户端失败: %w", err)
	}

	logger := log.New(os.Stdout, "[PurchaseMonitor] ", log.LstdFlags|log.Lshortfile)

	return &PurchaseChecker{
		apiClient: apiClient,
		config:    config,
		logger:    logger,
	}, nil
}

// GetMinDiffForPrice 根据价格获取最小差值要求
func (pc *PurchaseChecker) GetMinDiffForPrice(price float64) (float64, string) {
	for _, rule := range priceRules {
		if price < rule.MaxPrice {
			return rule.MinDiff, rule.RuleDesc
		}
	}
	return 0, ""
}

// CheckAndMonitor 检查并监听求购（双线程同步执行版本）
func (pc *PurchaseChecker) CheckAndMonitor(ctx context.Context) error {
	ticker := time.NewTicker(10 * time.Second) // 固定间隔10秒
	defer ticker.Stop()

	pc.logger.Println("=== 求购监听开始 ===")
	pc.logger.Println("执行模式: 双线程同步执行")
	pc.logger.Printf("检查间隔: 10s\n")
	pc.logger.Printf("自动删除: %v\n", pc.config.AutoDelete)
	pc.logger.Printf("干运行模式: %v\n", pc.config.DryRun)

	for {
		select {
		case <-ctx.Done():
			pc.logger.Println("监听已停止")
			return ctx.Err()
		case <-ticker.C:
			// 使用两个线程同步执行检查
			if err := pc.checkWithDualThreads(ctx); err != nil {
				pc.logger.Printf("检查失败: %v\n", err)
			}
		}
	}
}

// checkWithDualThreads 使用两个线程同步执行检查
func (pc *PurchaseChecker) checkWithDualThreads(ctx context.Context) error {
	pc.logger.Println("\n--- 开始检查（双线程同步执行） ---")
	startTime := time.Now()

	// 创建通道用于线程间同步
	type checkResult struct {
		purchases map[int][]youpin.MyPurchaseOrderItem
		prices    map[int]float64
		err       error
	}

	resultChan := make(chan checkResult, 1)

	// 线程1：获取求购列表
	go func() {
		pc.logger.Println("[线程1] 启动 - 获取求购列表")

		purchaseList, err := pc.getPurchaseOrderList(ctx)
		if err != nil {
			pc.logger.Printf("[线程1] 获取求购列表失败: %v\n", err)
			resultChan <- checkResult{err: err}
			return
		}

		pc.logger.Printf("[线程1] 成功获取 %d 个求购订单\n", len(purchaseList))

		if len(purchaseList) == 0 {
			resultChan <- checkResult{
				purchases: make(map[int][]youpin.MyPurchaseOrderItem),
				prices:    make(map[int]float64),
			}
			return
		}

		// 按TemplateId分组求购
		templateMap := make(map[int][]youpin.MyPurchaseOrderItem)
		for _, order := range purchaseList {
			templateMap[order.TemplateId] = append(templateMap[order.TemplateId], order)
		}

		resultChan <- checkResult{purchases: templateMap}
	}()

	// 等待线程1获取求购列表
	result1 := <-resultChan
	if result1.err != nil {
		return result1.err
	}

	if len(result1.purchases) == 0 {
		pc.logger.Println("当前没有求购订单，跳过检查")
		elapsed := time.Since(startTime)
		pc.logger.Printf("检查完成，耗时: %v\n", elapsed)
		return nil
	}

	// 线程2：获取在售价格
	go func() {
		pc.logger.Println("[线程2] 启动 - 获取在售价格")

		prices, err := pc.getOnSalePrices(ctx, result1.purchases)
		if err != nil {
			pc.logger.Printf("[线程2] 获取在售价格失败: %v\n", err)
			result1.prices = make(map[int]float64) // 使用空的价格列表继续
		} else {
			pc.logger.Printf("[线程2] 成功获取 %d 个模板的在售价格\n", len(prices))
			result1.prices = prices
		}

		resultChan <- result1
	}()

	// 等待线程2完成
	result2 := <-resultChan

	// 检查每个求购
	toDelete := []string{}
	for templateId, orders := range result2.purchases {
		minSalePrice := result2.prices[templateId]
		if minSalePrice == 0 {
			pc.logger.Printf("模板ID %d: 无在售商品\n", templateId)
			continue
		}

		for _, order := range orders {
			shouldDelete := pc.checkOrder(order, minSalePrice)

			if shouldDelete {
				toDelete = append(toDelete, order.OrderNo)
			}
		}
	}

	// 执行删除
	if len(toDelete) > 0 {
		pc.logger.Printf("\n需要删除的求购: %d 个\n", len(toDelete))
		if pc.config.DryRun {
			pc.logger.Println("干运行模式，不执行删除")
			for _, orderNo := range toDelete {
				pc.logger.Printf("  - 将删除: %s\n", orderNo)
			}
		} else if pc.config.AutoDelete {
			if err := pc.deletePurchaseOrders(ctx, toDelete); err != nil {
				pc.logger.Printf("删除求购失败: %v\n", err)
				return err
			}
			pc.logger.Printf("成功删除 %d 个求购\n", len(toDelete))
		} else {
			pc.logger.Println("自动删除已禁用，不执行删除")
			for _, orderNo := range toDelete {
				pc.logger.Printf("  - 建议删除: %s\n", orderNo)
			}
		}
	} else {
		pc.logger.Println("所有求购都符合条件，无需删除")
	}

	elapsed := time.Since(startTime)
	pc.logger.Printf("检查完成（双线程），耗时: %v\n", elapsed)
	return nil
}

// checkOrder 检查单个求购是否需要删除
func (pc *PurchaseChecker) checkOrder(order youpin.MyPurchaseOrderItem, minSalePrice float64) bool {
	purchasePrice, err := strconv.ParseFloat(order.UnitPrice, 64)
	if err != nil {
		pc.logger.Printf("  ✗ [%s] %s: 解析求购价格失败: %v\n", order.OrderNo, order.CommodityName, err)
		return false
	}

	minDiff, ruleDesc := pc.GetMinDiffForPrice(purchasePrice)
	priceDiff := purchasePrice - minSalePrice

	// 根据情况输出详细信息
	if purchasePrice >= minSalePrice {
		// 求购价 ≥ 最低在售价，符合条件
		pc.logger.Printf("  ✓ [%s] %s\n", order.OrderNo, order.CommodityName)
		pc.logger.Printf("      我的求购价: %.2f元\n", purchasePrice)
		pc.logger.Printf("      当前最低售价: %.2f元\n", minSalePrice)
		pc.logger.Printf("      价格差值: %.2f元 (求购价≥在售价)\n", priceDiff)
		pc.logger.Printf("      状态: 满足差价要求 ✓\n")
		return false
	}

	// 求购价 < 最低在售价，检查差值是否满足规则
	if priceDiff >= -minDiff {
		// 差值满足要求，符合条件
		pc.logger.Printf("  ✓ [%s] %s\n", order.OrderNo, order.CommodityName)
		pc.logger.Printf("      我的求购价: %.2f元\n", purchasePrice)
		pc.logger.Printf("      当前最低售价: %.2f元\n", minSalePrice)
		pc.logger.Printf("      价格差值: %.2f元 (允许范围: %.2f~%.2f)\n", priceDiff, -minDiff, 0.0)
		pc.logger.Printf("      规则: %s\n", ruleDesc)
		pc.logger.Printf("      状态: 满足差价要求 ✓\n")
		return false
	}

	// 差值不满足要求，需要删除
	pc.logger.Printf("  ✗ [%s] %s\n", order.OrderNo, order.CommodityName)
	pc.logger.Printf("      我的求购价: %.2f元\n", purchasePrice)
	pc.logger.Printf("      当前最低售价: %.2f元\n", minSalePrice)
	pc.logger.Printf("      价格差值: %.2f元 (允许范围: %.2f~%.2f)\n", priceDiff, -minDiff, 0.0)
	pc.logger.Printf("      规则: %s\n", ruleDesc)
	pc.logger.Printf("      状态: 超出差价范围，需删除 ✗\n")
	return true
}

// getPurchaseOrderList 获取求购列表（重试）
func (pc *PurchaseChecker) getPurchaseOrderList(ctx context.Context) ([]youpin.MyPurchaseOrderItem, error) {
	var result []youpin.MyPurchaseOrderItem
	pageIndex := 1
	pageSize := 50

	for {
		var retries int
		var resp *youpin.SearchPurchaseOrderListResponse
		var err error

		// 重试逻辑
		for retries = 0; retries < pc.config.MaxRetries; retries++ {
			req := &youpin.SearchPurchaseOrderListRequest{
				PageIndex: pageIndex,
				PageSize:  pageSize,
				Status:    20, // 20表示求购中
			}

			resp, err = pc.apiClient.SearchPurchaseOrderList(ctx, req)
			if err == nil {
				break
			}

			if retries < pc.config.MaxRetries-1 {
				pc.logger.Printf("获取求购列表失败，%v后重试: %v\n", pc.config.RetryDelay, err)
				time.Sleep(pc.config.RetryDelay)
			}
		}

		if err != nil {
			return nil, fmt.Errorf("获取求购列表失败 (重试%d次): %w", retries, err)
		}

		result = append(result, resp.Data...)

		// 如果没有更多数据就结束
		if len(resp.Data) < pageSize {
			break
		}

		pageIndex++
	}

	return result, nil
}

// getOnSalePrices 批量获取在售价格
func (pc *PurchaseChecker) getOnSalePrices(ctx context.Context, templateMap map[int][]youpin.MyPurchaseOrderItem) (map[int]float64, error) {
	result := make(map[int]float64)

	if len(templateMap) == 0 {
		return result, nil
	}

	// 构建请求列表
	requestList := make([]youpin.BatchPriceQueryItem, 0, len(templateMap))
	for templateId := range templateMap {
		id := templateId
		requestList = append(requestList, youpin.BatchPriceQueryItem{
			TemplateID: &id,
		})
	}

	// 分批获取（每次最多200个）
	for i := 0; i < len(requestList); i += 200 {
		end := i + 200
		if end > len(requestList) {
			end = len(requestList)
		}

		var retries int
		var resp *youpin.BatchGetOnSaleCommodityInfoResponse
		var err error

		// 重试逻辑
		for retries = 0; retries < pc.config.MaxRetries; retries++ {
			resp, err = pc.apiClient.BatchGetOnSaleCommodityInfo(ctx, requestList[i:end])
			if err == nil {
				break
			}

			if retries < pc.config.MaxRetries-1 {
				pc.logger.Printf("获取在售价格失败，%v后重试: %v\n", pc.config.RetryDelay, err)
				time.Sleep(pc.config.RetryDelay)
			}
		}

		if err != nil {
			return nil, fmt.Errorf("获取在售价格失败 (重试%d次): %w", retries, err)
		}

		for _, item := range resp.Data {
			priceStr := strings.TrimSpace(item.SaleCommodityResponse.MinSellPrice)
			if priceStr != "" && priceStr != "0" {
				price, err := strconv.ParseFloat(priceStr, 64)
				if err == nil {
					result[item.SaleTemplateResponse.TemplateId] = price
				}
			}
		}
	}

	return result, nil
}

// deletePurchaseOrders 删除求购订单
func (pc *PurchaseChecker) deletePurchaseOrders(ctx context.Context, orderNoList []string) error {
	if len(orderNoList) == 0 {
		return nil
	}

	var lastErr error
	deleted := 0

	// 分批删除（为了安全起见，每次最多删除10个）
	for i := 0; i < len(orderNoList); i += 10 {
		end := i + 10
		if end > len(orderNoList) {
			end = len(orderNoList)
		}

		batchOrders := orderNoList[i:end]
		var retries int
		var err error

		// 重试逻辑
		for retries = 0; retries < pc.config.MaxRetries; retries++ {
			_, err = pc.apiClient.DeletePurchaseOrder(ctx, batchOrders, "")
			if err == nil {
				deleted += len(batchOrders)
				pc.logger.Printf("成功删除批次 %d/%d (共%d个)\n", i/10+1, (len(orderNoList)+9)/10, len(batchOrders))
				break
			}

			if retries < pc.config.MaxRetries-1 {
				pc.logger.Printf("删除求购失败，%v后重试: %v\n", pc.config.RetryDelay, err)
				time.Sleep(pc.config.RetryDelay)
			}
		}

		if err != nil {
			pc.logger.Printf("删除求购失败 (重试%d次): %v\n", retries, err)
			lastErr = err
		}
	}

	return lastErr
}

func main() {
	// 命令行参数
	token := flag.String("token", os.Getenv("YOUPIN_TOKEN"), "用户Token (可选，如不提供则从数据库获取)")
	interval := flag.Duration("interval", 30*time.Second, "检查间隔")
	maxRetries := flag.Int("retries", 3, "最大重试次数")
	autoDelete := flag.Bool("delete", false, "是否自动删除不符合条件的求购")
	dryRun := flag.Bool("dry-run", false, "干运行模式（只输出，不执行删除）")
	dbURL := flag.String("db", "", "数据库连接字符串（可选）")
	help := flag.Bool("help", false, "显示帮助信息")

	flag.Parse()

	if *help {
		fmt.Print(`
求购监听脚本

用法: ./purchase-monitor [选项]

选项:
  -token string       用户Token (可选，如不提供则从数据库获取)
  -interval duration  检查间隔 (默认: 30s)
  -retries int        最大重试次数 (默认: 3)
  -delete             是否自动删除不符合条件的求购 (默认: false)
  -dry-run            干运行模式，只输出不执行删除 (默认: false)
  -db string          数据库连接字符串 (可选，不提供则使用默认)
  -help               显示此帮助信息

价格规则:
  - 10块以下的饰品: 求购价最多比最低在售价低1元
  - 10-50块的饰品: 求购价最多比最低在售价低3元
  - 50-150块的饰品: 求购价最多比最低在售价低20元
  - 150块以上的饰品: 无最小差值限制

示例:
  # 干运行模式（只查看，不删除）
  ./purchase-monitor -dry-run

  # 启用自动删除
  ./purchase-monitor -delete

  # 自定义检查间隔为1分钟
  ./purchase-monitor -interval 1m -delete

  # 指定Token而不使用数据库
  ./purchase-monitor -token YOUR_TOKEN -delete
`)
		return
	}

	// 获取Token
	var accountToken string

	// 第一优先级：命令行参数
	if *token != "" {
		accountToken = *token
		log.Printf("[求购监听] 使用命令行参数提供的Token")
	} else {
		// 第二优先级：从数据库获取
		log.Printf("[求购监听] 尝试从数据库获取Token...")

		// 初始化数据库
		db, err := database.Initialize(*dbURL)
		if err == nil {
			var account models.YouPinAccount
			if err := db.Where("is_active = ?", true).First(&account).Error; err == nil && account.Token != "" {
				accountToken = account.Token
				log.Printf("[求购监听] 从数据库获取Token成功，账户: %s", account.Nickname)
			} else {
				log.Printf("[求购监听] 数据库中没有有效的Token: %v", err)
			}
		} else {
			log.Printf("[求购监听] 数据库初始化失败: %v", err)
		}
	}

	// 如果还是没有Token，则提示用户
	if accountToken == "" {
		fmt.Print(`
错误：没有有效的Token

请使用以下方式之一提供Token：
  1. 命令行参数: ./purchase-monitor -token YOUR_TOKEN -delete
  2. 环境变量: export YOUPIN_TOKEN=YOUR_TOKEN && ./purchase-monitor -delete
  3. 数据库: 确保数据库中存在有效的YouPin账户

查看帮助: ./purchase-monitor -help
`)
		os.Exit(1)
	}

	config := MonitorConfig{
		Token:         accountToken,
		CheckInterval: *interval,
		MaxRetries:    *maxRetries,
		RetryDelay:    2 * time.Second,
		AutoDelete:    *autoDelete,
		DryRun:        *dryRun,
	}

	checker, err := NewPurchaseChecker(config)
	if err != nil {
		log.Fatalf("初始化失败: %v", err)
	}

	// 运行监听
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 处理信号用于优雅关闭
	go func() {
		sigChan := make(chan os.Signal, 1)
		// 在实际应用中应该导入signal包来处理SIGINT和SIGTERM
		// 这里简化处理
		<-sigChan
		cancel()
	}()

	if err := checker.CheckAndMonitor(ctx); err != nil {
		log.Fatalf("监听错误: %v", err)
	}
}
