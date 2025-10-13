package main

import (
	"context"
	"csgo-trader/internal/database"
	"csgo-trader/internal/models"
	"csgo-trader/internal/services/youpin"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"time"

	"gorm.io/gorm"
)

var (
	token          = flag.String("token", "", "YouPin API token")
	interval       = flag.Int("interval", 300, "检查间隔(秒)，默认5分钟")
	once           = flag.Bool("once", false, "只运行一次，不循环")
	minProfitRate  = flag.Float64("min-profit", 0.05, "最小利润率，默认5%")
	priceDecrement = flag.Float64("decrement", 0.005, "降价幅度，默认0.5%")
	minRankGap     = flag.Float64("min-rank-gap", 0.02, "第一名与第二名最小差距，默认2%")
	dryRun         = flag.Bool("dry-run", false, "演练模式，不实际修改求购")
	logFile        = flag.String("log", "logs/purchase_monitor.log", "日志文件路径")
)

// PurchaseMonitor 求购监控器
type PurchaseMonitor struct {
	client *youpin.Client
	db     *gorm.DB
	config MonitorConfig
	logger *log.Logger
}

// MonitorConfig 监控配置
type MonitorConfig struct {
	MinProfitRate  float64
	PriceDecrement float64
	MinRankGap     float64
	DryRun         bool
}

// PurchaseCheckResult 求购检查结果
type PurchaseCheckResult struct {
	OrderNo          string
	TemplateID       int
	CommodityName    string
	CurrentPrice     float64
	CurrentRank      string
	Quantity         int
	Action           string  // keep, delete, increase, decrease
	ActionReason     string
	NewPrice         float64
	MarketSellPrice  float64
	MarketBuyPrice   float64
	EstimatedProfit  float64
	ProfitRate       float64
	RankPosition     int // 1=第一名, 2=第二名, etc
	FirstRankPrice   float64
	SecondRankPrice  float64
}

func main() {
	flag.Parse()

	// 检查必需参数
	if *token == "" {
		*token = os.Getenv("YOUPIN_TOKEN")
		if *token == "" {
			log.Fatal("请提供YouPin token（使用 -token 或设置环境变量 YOUPIN_TOKEN）")
		}
	}

	// 初始化日志
	logDir := "logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Fatalf("创建日志目录失败: %v", err)
	}

	logF, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("打开日志文件失败: %v", err)
	}
	defer logF.Close()

	logger := log.New(logF, "[PURCHASE-MONITOR] ", log.LstdFlags)
	logger.Println("================== 求购监控脚本启动 ==================")

	// 初始化YouPin客户端
	client, err := youpin.NewClient(*token)
	if err != nil {
		log.Fatalf("初始化YouPin客户端失败: %v", err)
	}
	logger.Printf("YouPin账户: %s", client.GetUserNickname())

	// 初始化数据库
	db, err := database.InitDB("")
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	logger.Println("数据库连接成功")

	// 创建监控器
	monitor := &PurchaseMonitor{
		client: client,
		db:     db,
		config: MonitorConfig{
			MinProfitRate:  *minProfitRate,
			PriceDecrement: *priceDecrement,
			MinRankGap:     *minRankGap,
			DryRun:         *dryRun,
		},
		logger: logger,
	}

	if *dryRun {
		logger.Println("[DRY-RUN] 演练模式已启用，不会实际修改求购")
	}

	// 运行监控
	if *once {
		logger.Println("单次运行模式")
		monitor.runOnce()
	} else {
		logger.Printf("循环监控模式，间隔 %d 秒", *interval)
		monitor.runLoop(*interval)
	}
}

// runOnce 执行一次监控
func (m *PurchaseMonitor) runOnce() {
	ctx := context.Background()

	m.logger.Println("========== 开始检查求购订单 ==========")
	startTime := time.Now()

	// 获取当前所有求购中的订单
	orders, err := m.getPurchaseOrders(ctx)
	if err != nil {
		m.logger.Printf("ERROR: 获取求购订单失败: %v", err)
		return
	}

	if len(orders) == 0 {
		m.logger.Println("当前没有求购订单")
		return
	}

	m.logger.Printf("找到 %d 个求购订单，开始检查...", len(orders))

	// 统计
	stats := struct {
		Total     int
		Kept      int
		Deleted   int
		Increased int
		Decreased int
		Errors    int
	}{Total: len(orders)}

	// 检查每个订单
	for i, order := range orders {
		m.logger.Printf("[%d/%d] 检查订单: %s - %s", i+1, len(orders), order.OrderNo, order.CommodityName)

		result, err := m.checkPurchaseOrder(ctx, order)
		if err != nil {
			m.logger.Printf("  ERROR: 检查失败: %v", err)
			stats.Errors++
			continue
		}

		// 执行操作
		if err := m.executeAction(ctx, result); err != nil {
			m.logger.Printf("  ERROR: 执行操作失败: %v", err)
			stats.Errors++
			continue
		}

		// 更新统计
		switch result.Action {
		case "keep":
			stats.Kept++
		case "delete":
			stats.Deleted++
		case "increase":
			stats.Increased++
		case "decrease":
			stats.Decreased++
		}

		// 避免请求过快，休息一下
		time.Sleep(2 * time.Second)
	}

	elapsed := time.Since(startTime)
	m.logger.Printf("========== 检查完成 ==========")
	m.logger.Printf("总订单数: %d", stats.Total)
	m.logger.Printf("保持不变: %d", stats.Kept)
	m.logger.Printf("删除订单: %d", stats.Deleted)
	m.logger.Printf("加价调整: %d", stats.Increased)
	m.logger.Printf("降价调整: %d", stats.Decreased)
	m.logger.Printf("处理错误: %d", stats.Errors)
	m.logger.Printf("耗时: %v", elapsed)
}

// runLoop 循环监控
func (m *PurchaseMonitor) runLoop(intervalSec int) {
	ticker := time.NewTicker(time.Duration(intervalSec) * time.Second)
	defer ticker.Stop()

	// 先执行一次
	m.runOnce()

	// 然后循环
	for range ticker.C {
		m.runOnce()
	}
}

// getPurchaseOrders 获取所有求购中的订单
func (m *PurchaseMonitor) getPurchaseOrders(ctx context.Context) ([]youpin.MyPurchaseOrderItem, error) {
	// status=20 表示求购中
	resp, err := m.client.SearchPurchaseOrderList(ctx, 1, 100, 20)
	if err != nil {
		return nil, fmt.Errorf("获取求购列表失败: %w", err)
	}

	return resp.Data, nil
}

// checkPurchaseOrder 检查单个求购订单
func (m *PurchaseMonitor) checkPurchaseOrder(ctx context.Context, order youpin.MyPurchaseOrderItem) (*PurchaseCheckResult, error) {
	result := &PurchaseCheckResult{
		OrderNo:       order.OrderNo,
		TemplateID:    order.TemplateId,
		CommodityName: order.CommodityName,
		CurrentRank:   order.Rank,
		Quantity:      order.Quantity,
	}

	// 解析当前求购价格
	currentPrice, err := strconv.ParseFloat(order.UnitPrice, 64)
	if err != nil {
		return nil, fmt.Errorf("解析求购价格失败: %w", err)
	}
	result.CurrentPrice = currentPrice

	// 获取市场信息
	marketInfo, err := m.getMarketInfo(ctx, order.TemplateId)
	if err != nil {
		return nil, fmt.Errorf("获取市场信息失败: %w", err)
	}

	result.MarketSellPrice = marketInfo.LowestSellPrice
	result.MarketBuyPrice = marketInfo.HighestBuyPrice
	result.FirstRankPrice = marketInfo.FirstRankBuyPrice
	result.SecondRankPrice = marketInfo.SecondRankBuyPrice
	result.RankPosition = marketInfo.MyRankPosition

	// 计算预期利润
	if result.MarketSellPrice > 0 {
		// 考虑悠悠有品的手续费（买方无手续费，卖方2%）
		sellFee := result.MarketSellPrice * 0.02
		result.EstimatedProfit = result.MarketSellPrice - sellFee - currentPrice
		result.ProfitRate = result.EstimatedProfit / currentPrice
	}

	// 决策逻辑
	m.makeDecision(result)

	return result, nil
}

// MarketInfo 市场信息
type MarketInfo struct {
	LowestSellPrice    float64
	HighestBuyPrice    float64
	FirstRankBuyPrice  float64
	SecondRankBuyPrice float64
	MyRankPosition     int
}

// getMarketInfo 获取市场信息
func (m *PurchaseMonitor) getMarketInfo(ctx context.Context, templateID int) (*MarketInfo, error) {
	info := &MarketInfo{}

	// 获取在售商品（最低售价）
	sellList, err := m.client.GetCommodityList(ctx, templateID, 1, 10, 1)
	if err != nil {
		return nil, fmt.Errorf("获取在售列表失败: %w", err)
	}

	if len(sellList.Data.CommodityList) > 0 {
		lowestPrice, _ := strconv.ParseFloat(sellList.Data.CommodityList[0].Price, 64)
		info.LowestSellPrice = lowestPrice
	}

	// 获取求购列表（最高求购价和排名）
	buyList, err := m.client.GetTemplatePurchaseOrderList(ctx, templateID, 1, 10)
	if err != nil {
		return nil, fmt.Errorf("获取求购列表失败: %w", err)
	}

	if len(buyList.Data) > 0 {
		info.HighestBuyPrice = buyList.Data[0].PurchasePrice
		info.FirstRankBuyPrice = buyList.Data[0].PurchasePrice

		if len(buyList.Data) > 1 {
			info.SecondRankBuyPrice = buyList.Data[1].PurchasePrice
		}

		// 查找自己的排名
		for i, item := range buyList.Data {
			// 注意：YouPin API中，自己的订单可能有特殊标识
			// 这里简化处理，假设价格相同的就是自己的
			if item.PurchasePrice == info.HighestBuyPrice {
				info.MyRankPosition = i + 1
				break
			}
		}
	}

	return info, nil
}

// calculateIncreasedPrice 根据价格区间计算加价后的价格
// 加价规则：
// - 0~1元：增量为0.01的倍数
// - 1~50元：增量为0.1的倍数
// - 50~1000元：增量为1的倍数
// - 1000元以上：增量为10的倍数
func calculateIncreasedPrice(currentPrice float64) float64 {
	var increment float64
	var newPrice float64

	if currentPrice < 1.0 {
		// 0~1元：增量为0.01
		increment = 0.01
		newPrice = math.Ceil(currentPrice/increment)*increment + increment
	} else if currentPrice < 50.0 {
		// 1~50元：增量为0.1
		increment = 0.1
		newPrice = math.Ceil(currentPrice/increment)*increment + increment
	} else if currentPrice < 1000.0 {
		// 50~1000元：增量为1
		increment = 1.0
		newPrice = math.Ceil(currentPrice/increment)*increment + increment
	} else {
		// 1000元以上：增量为10
		increment = 10.0
		newPrice = math.Ceil(currentPrice/increment)*increment + increment
	}

	// 保留两位小数
	return math.Round(newPrice*100) / 100
}

// makeDecision 决策逻辑
func (m *PurchaseMonitor) makeDecision(result *PurchaseCheckResult) {
	// 规则1: 检查利润率是否符合要求
	if result.ProfitRate < m.config.MinProfitRate {
		result.Action = "delete"
		result.ActionReason = fmt.Sprintf("利润率%.2f%%低于最小要求%.2f%%",
			result.ProfitRate*100, m.config.MinProfitRate*100)
		return
	}

	// 规则2: 我不是第一名
	if result.RankPosition > 1 {
		// 检查第一名的价格与卖价差值
		if result.FirstRankPrice > 0 && result.MarketSellPrice > 0 {
			firstProfit := result.MarketSellPrice - result.MarketSellPrice*0.02 - result.FirstRankPrice
			firstProfitRate := firstProfit / result.FirstRankPrice

			// 如果第一名的利润率太小，不值得竞争
			if firstProfitRate < m.config.MinProfitRate * 0.5 {
				result.Action = "keep"
				result.ActionReason = fmt.Sprintf("排名第%d，但第一名利润率%.2f%%太低，不竞争",
					result.RankPosition, firstProfitRate*100)
				return
			}

			// 如果利润率还可以，尝试加价超过第一名
			// 使用阶梯式加价策略
			newPrice := calculateIncreasedPrice(result.FirstRankPrice)

			// 确保加价后仍有足够利润
			newProfit := result.MarketSellPrice - result.MarketSellPrice*0.02 - newPrice
			newProfitRate := newProfit / newPrice

			if newProfitRate >= m.config.MinProfitRate {
				result.Action = "increase"
				result.NewPrice = newPrice
				increaseAmount := newPrice - result.FirstRankPrice
				result.ActionReason = fmt.Sprintf("排名第%d，加价¥%.2f抢第一（新利润率%.2f%%）",
					result.RankPosition, increaseAmount, newProfitRate*100)
			} else {
				result.Action = "keep"
				result.ActionReason = fmt.Sprintf("排名第%d，但加价后利润率不足，保持观望", result.RankPosition)
			}
			return
		}
	}

	// 规则3: 我是第一名，检查与第二名的差距
	if result.RankPosition == 1 && result.SecondRankPrice > 0 {
		priceGap := (result.CurrentPrice - result.SecondRankPrice) / result.SecondRankPrice

		// 如果差距过大，可以适当降价节省成本，但保持第一
		if priceGap > m.config.MinRankGap*2 {
			// 降到比第二名高一点即可
			newPrice := result.SecondRankPrice * (1 + m.config.MinRankGap/2)

			// 确保降价后仍有足够利润
			newProfit := result.MarketSellPrice - result.MarketSellPrice*0.02 - newPrice
			newProfitRate := newProfit / newPrice

			if newProfitRate >= m.config.MinProfitRate {
				result.Action = "decrease"
				result.NewPrice = math.Round(newPrice*100) / 100
				result.ActionReason = fmt.Sprintf("第一名但差距过大(%.2f%%)，降价节省成本（新利润率%.2f%%）",
					priceGap*100, newProfitRate*100)
				return
			}
		}
	}

	// 默认：保持不变
	result.Action = "keep"
	result.ActionReason = fmt.Sprintf("排名第%d，利润率%.2f%%，保持现状",
		result.RankPosition, result.ProfitRate*100)
}

// executeAction 执行操作
func (m *PurchaseMonitor) executeAction(ctx context.Context, result *PurchaseCheckResult) error {
	m.logger.Printf("  排名: %d | 价格: ¥%.2f | 最低售价: ¥%.2f | 利润率: %.2f%%",
		result.RankPosition, result.CurrentPrice, result.MarketSellPrice, result.ProfitRate*100)
	m.logger.Printf("  决策: %s - %s", result.Action, result.ActionReason)

	if m.config.DryRun {
		m.logger.Printf("  [DRY-RUN] 跳过实际操作")
		return nil
	}

	switch result.Action {
	case "delete":
		return m.deletePurchaseOrder(ctx, result.OrderNo)
	case "increase", "decrease":
		return m.updatePurchasePrice(ctx, result)
	case "keep":
		// 不需要操作
		return nil
	default:
		return fmt.Errorf("未知操作: %s", result.Action)
	}
}

// deletePurchaseOrder 删除求购订单
func (m *PurchaseMonitor) deletePurchaseOrder(ctx context.Context, orderNo string) error {
	m.logger.Printf("  >> 删除求购订单: %s", orderNo)

	resp, err := m.client.DeletePurchaseOrder(ctx, []string{orderNo})
	if err != nil {
		return fmt.Errorf("删除失败: %w", err)
	}

	if !resp.Data.Status {
		return fmt.Errorf("删除失败: API返回失败状态")
	}

	m.logger.Printf("  >> 删除成功")
	return nil
}

// updatePurchasePrice 更新求购价格
func (m *PurchaseMonitor) updatePurchasePrice(ctx context.Context, result *PurchaseCheckResult) error {
	m.logger.Printf("  >> 更新求购价格: ¥%.2f -> ¥%.2f", result.CurrentPrice, result.NewPrice)

	// 获取订单详情
	detail, err := m.client.GetPurchaseOrderDetail(ctx, result.OrderNo)
	if err != nil {
		return fmt.Errorf("获取订单详情失败: %w", err)
	}

	// 获取模板信息
	templateInfo, err := m.client.GetTemplatePurchaseInfo(ctx, strconv.Itoa(result.TemplateID))
	if err != nil {
		return fmt.Errorf("获取模板信息失败: %w", err)
	}

	// 构建更新请求
	totalAmount := result.NewPrice * float64(result.Quantity)
	req := youpin.UpdatePurchaseOrderRequest{
		TemplateId:        result.TemplateID,
		TemplateHashName:  templateInfo.Data.TemplateInfo.TemplateHashName,
		CommodityName:     result.CommodityName,
		ReferencePrice:    templateInfo.Data.TemplateInfo.ReferencePrice,
		MinSellPrice:      templateInfo.Data.TemplateInfo.MinSellPrice,
		MaxPurchasePrice:  templateInfo.Data.TemplateInfo.MaxPurchasePrice,
		PurchasePrice:     result.NewPrice,
		PurchaseNum:       result.Quantity,
		NeedPaymentAmount: totalAmount,
		TotalAmount:       totalAmount,
		TemplateName:      result.CommodityName,
		PriceDifference:   0,
		OrderNo:           result.OrderNo,
		DiscountAmount:    0,
		SupplyQuantity:    detail.Data.BuyQuantity,
		PayConfirmFlag:    true, // 直接确认支付
		RepeatOrderCancelFlag: false,
	}

	// 发送更新请求
	resp, err := m.client.UpdatePurchaseOrder(ctx, req)
	if err != nil {
		return fmt.Errorf("更新失败: %w", err)
	}

	if !resp.Data.UpdateFlag {
		return fmt.Errorf("更新失败: API返回更新标志为false")
	}

	m.logger.Printf("  >> 更新成功")
	return nil
}
