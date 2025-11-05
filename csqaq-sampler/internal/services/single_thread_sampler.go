package services

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"csqaq-sampler/internal/models"
	youpin "csqaq-sampler/internal/services/youpin"

	"gorm.io/gorm"
)

// SingleThreadSampler 单线程采样器 - 单线程请求、1秒间隔、不使用代理
type SingleThreadSampler struct {
	db          *gorm.DB
	ypClient    *youpin.OpenAPIClient
	tokenClient *youpin.OpenAPIClient
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.RWMutex
	running     bool
	stats       SingleThreadStats
	interval    time.Duration // 1秒间隔
}

// SingleThreadStats 统计信息
type SingleThreadStats struct {
	TotalProcessed  int64
	SuccessRequests int64
	FailedRequests  int64
	ValidSnapshots  int64
	LastRun         time.Time
	TotalDuration   time.Duration
	AvgResponseTime float64
}

// NewSingleThreadSampler 创建单线程采样器
func NewSingleThreadSampler(
	db *gorm.DB,
	ypClient *youpin.OpenAPIClient,
	tokenClient *youpin.OpenAPIClient,
) (*SingleThreadSampler, error) {
	ctx, cancel := context.WithCancel(context.Background())
	return &SingleThreadSampler{
		db:          db,
		ypClient:    ypClient,
		tokenClient: tokenClient,
		ctx:         ctx,
		cancel:      cancel,
		interval:    200 * time.Millisecond, // 1秒间隔
	}, nil
}

// Start 开始采样
func (s *SingleThreadSampler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	log.Println("[单线程采样器] 启动采样器 (0.2秒间隔，无代理)")

	// 启动单线程采样循环
	go s.samplingLoop()
}

// samplingLoop 采样主循环
func (s *SingleThreadSampler) samplingLoop() {
	time.Sleep(5 * time.Second)
	log.Println("[单线程采样器] 初始延迟完成，开始采样")

	for {
		select {
		case <-s.ctx.Done():
			log.Println("[单线程采样器] 已停止")
			return
		default:
			if err := s.runSamplingCycle(); err != nil {
				log.Printf("[单线程采样器] 错误: %v\n", err)
				time.Sleep(5 * time.Second)
			}
		}
	}
}

// runSamplingCycle 采样周期 - 单线程处理每个商品
func (s *SingleThreadSampler) runSamplingCycle() error {
	startTime := time.Now()

	var goods []models.CSQAQGood
	if err := s.db.Order("updated_at desc").Find(&goods).Error; err != nil {
		return fmt.Errorf("加载商品失败: %w", err)
	}

	if len(goods) == 0 {
		log.Println("[单线程采样器] 没有商品，跳过")
		return nil
	}

	log.Printf("[单线程采样器] 开始采样周期 (%d 个商品)\n", len(goods))

	// 单线程处理每个商品
	successCount := int64(0)
	failureCount := int64(0)
	validSnapshotCount := int64(0)

	for i, good := range goods {
		// 1秒间隔
		time.Sleep(s.interval)

		// 检查商品是否有YYYP ID
		if good.YYYPTemplateID == nil || *good.YYYPTemplateID == 0 {
			log.Printf("[单线程采样器] [%d/%d] 跳过 - %s (ID:%d，无YYYP ID)\n", i+1, len(goods), good.Name, good.ID)
			failureCount++
			continue
		}

		// 获取在售价格（OpenAPI）
		onSalePrice, err := s.getOnSalePrice(*good.YYYPTemplateID)
		if err != nil {
			log.Printf("[单线程采样器] [%d/%d] ✗ %s - 获取在售价格失败: %v\n", i+1, len(goods), good.Name, err)
			failureCount++
			continue
		}

		// 获取求购价（Token认证）
		purchasePrice, err := s.getPurchasePrice(*good.YYYPTemplateID)
		if err != nil {
			log.Printf("[单线程采样器] [%d/%d] ✗ %s - 获取求购价失败: %v\n", i+1, len(goods), good.Name, err)
			failureCount++
			continue
		}

		// 保存快照
		if err := s.saveSnapshot(good, onSalePrice, purchasePrice); err != nil {
			log.Printf("[单线程采样器] [%d/%d] ✗ %s - 保存快照失败: %v\n", i+1, len(goods), good.Name, err)
			failureCount++
			continue
		}

		// 成功采样 - 打印详细信息
		log.Printf("[单线程采样器] [%d/%d] ✓ %s | 在售:%.2f元(%d个) | 求购:%.2f元\n",
			i+1, len(goods), good.Name,
			onSalePrice.MinSellPrice, onSalePrice.SellCount,
			purchasePrice.MaxBuyPrice)

		successCount++
		validSnapshotCount++
	}

	duration := time.Since(startTime)
	s.mu.Lock()
	s.stats.TotalProcessed += int64(len(goods))
	s.stats.SuccessRequests += successCount
	s.stats.FailedRequests += failureCount
	s.stats.ValidSnapshots += validSnapshotCount
	s.stats.LastRun = time.Now()
	s.stats.TotalDuration = duration
	if s.stats.TotalProcessed > 0 {
		s.stats.AvgResponseTime = float64(duration.Milliseconds()) / float64(len(goods))
	}
	s.mu.Unlock()

	log.Printf("[单线程采样器] 采样周期完成 (成功:%d, 失败:%d, 耗时:%v)\n", successCount, failureCount, duration)

	return nil
}

// getOnSalePrice 获取在售价格（使用OpenAPI）
func (s *SingleThreadSampler) getOnSalePrice(templateID int64) (*OnSalePriceData, error) {
	// 使用BatchGetOnSaleCommodityInfo查询在售价格
	requestList := []youpin.BatchPriceQueryItem{
		{
			TemplateID: func(v int) *int { return &v }(int(templateID)),
		},
	}

	resp, err := s.ypClient.BatchGetOnSaleCommodityInfo(context.Background(), requestList)
	if err != nil {
		return nil, fmt.Errorf("OpenAPI调用失败: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("API未返回数据")
	}

	item := resp.Data[0]
	// 解析最低在售价格
	minPrice, _ := strconv.ParseFloat(item.SaleCommodityResponse.MinSellPrice, 64)
	sellCount := item.SaleCommodityResponse.SellNum

	return &OnSalePriceData{
		TemplateID:   templateID,
		MinSellPrice: minPrice,
		SellCount:    sellCount,
	}, nil
}

// getPurchasePrice 获取求购价格（使用Token认证）
func (s *SingleThreadSampler) getPurchasePrice(templateID int64) (*PurchasePriceData, error) {
	// 使用GetTemplatePurchaseInfo查询求购信息，包括最高求购价
	resp, err := s.tokenClient.GetTemplatePurchaseInfo(context.Background(), strconv.FormatInt(templateID, 10))
	if err != nil {
		return nil, fmt.Errorf("Token接口调用失败: %w", err)
	}

	// 解析最高求购价格
	maxBuyPrice, _ := strconv.ParseFloat(resp.Data.TemplateInfo.MaxPurchasePrice, 64)

	return &PurchasePriceData{
		TemplateID:  templateID,
		MaxBuyPrice: maxBuyPrice,
	}, nil
}

// saveSnapshot 保存快照
func (s *SingleThreadSampler) saveSnapshot(good models.CSQAQGood, onSalePrice *OnSalePriceData, purchasePrice *PurchasePriceData) error {
	snapshot := models.CSQAQGoodSnapshot{
		GoodID:         good.GoodID, // 使用 GoodID 而不是 ID
		YYYPTemplateID: good.YYYPTemplateID,
		YYYPSellPrice:  &onSalePrice.MinSellPrice,
		YYYPBuyPrice:   &purchasePrice.MaxBuyPrice,
		YYYPSellCount:  &onSalePrice.SellCount,
	}

	if err := s.db.Create(&snapshot).Error; err != nil {
		return fmt.Errorf("保存快照失败: %w", err)
	}

	return nil
}

// Stop 停止采样
func (s *SingleThreadSampler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	s.cancel()
	log.Println("[单线程采样器] 停止采样")

	// 输出最终统计信息
	s.mu.RLock()
	defer s.mu.RUnlock()
	log.Printf("[单线程采样器] 最终统计:\n")
	log.Printf("  ├─ 总处理商品数: %d\n", s.stats.TotalProcessed)
	log.Printf("  ├─ 成功采样: %d\n", s.stats.SuccessRequests)
	log.Printf("  ├─ 失败采样: %d\n", s.stats.FailedRequests)
	log.Printf("  ├─ 有效快照: %d\n", s.stats.ValidSnapshots)
	log.Printf("  ├─ 平均响应时间: %.2fms\n", s.stats.AvgResponseTime)
	log.Printf("  └─ 最后运行: %v\n", s.stats.LastRun)
}

// GetStats 获取统计信息
func (s *SingleThreadSampler) GetStats() SingleThreadStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}
