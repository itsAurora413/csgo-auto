package services

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"csqaq-sampler/internal/models"
	youpin "csqaq-sampler/internal/services/youpin"

	"gorm.io/gorm"
)

// DualAccountSampler 双账号采样器
// 将商品列表分成两部分，分别由A、B账号处理
// A账号处理商品0-N/2，B账号处理商品N/2-N
type DualAccountSampler struct {
	db           *gorm.DB
	clientA *youpin.OpenAPIClient
	clientB *youpin.OpenAPIClient
	numWorkers   int
	proxyConfig  *ProxyConfig
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.RWMutex
	running      bool
	stats        DualAccountSamplerStats
}

// DualAccountSamplerStats 统计信息
type DualAccountSamplerStats struct {
	TotalProcessed  int64
	SuccessRequests int64
	FailedRequests  int64
	ValidSnapshots  int64
	LastRun         time.Time
	TotalDuration   time.Duration
	AccountASuccess int64
	AccountAFailure int64
	AccountBSuccess int64
	AccountBFailure int64
}

// NewDualAccountSampler 创建双账号采样器
func NewDualAccountSampler(
	db *gorm.DB,
	clientA *youpin.OpenAPIClient,
	clientB *youpin.OpenAPIClient,
	numWorkers int,
	proxyConfig *ProxyConfig,
) (*DualAccountSampler, error) {
	if numWorkers <= 0 {
		numWorkers = 6 // 每账号6个线程（售价3+求购3）
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &DualAccountSampler{
		db:           db,
		clientA: clientA,
		clientB: clientB,
		numWorkers:   numWorkers,
		proxyConfig:  proxyConfig,
		ctx:          ctx,
		cancel:       cancel,
	}, nil
}

// Start 开始采样
func (s *DualAccountSampler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	log.Printf("[双账号采样器] 启动采样器\n")
	log.Printf("[双账号采样器] ├─ 账号A (ID: 1645231): %d 个工作线程 (售价3个 + 求购3个)\n", s.numWorkers)
	log.Printf("[双账号采样器] └─ 账号B (ID: 12919014): %d 个工作线程 (售价3个 + 求购3个)\n", s.numWorkers)

	// 启动主采样线程
	go s.samplingLoop()
}

// samplingLoop 采样主循环
func (s *DualAccountSampler) samplingLoop() {
	time.Sleep(5 * time.Second)
	log.Println("[双账号采样器] 初始延迟完成，开始采样")

	for {
		select {
		case <-s.ctx.Done():
			log.Println("[双账号采样器] 已停止")
			return
		default:
			if err := s.runSamplingCycle(); err != nil {
				log.Printf("[双账号采样器] 错误: %v\n", err)
				time.Sleep(5 * time.Second)
			}
		}
	}
}

// runSamplingCycle 采样周期：分别处理两部分商品
func (s *DualAccountSampler) runSamplingCycle() error {
	startTime := time.Now()

	var goods []models.CSQAQGood
	if err := s.db.Order("updated_at desc").Find(&goods).Error; err != nil {
		return fmt.Errorf("加载商品失败: %w", err)
	}

	if len(goods) == 0 {
		log.Println("[双账号采样器] 没有商品，跳过")
		return nil
	}

	log.Printf("[双账号采样器] 开始采样周期 (%d 个商品)\n", len(goods))

	// 分割商品列表
	mid := len(goods) / 2
	goodsA := goods[:mid]
	goodsB := goods[mid:]

	log.Printf("[双账号采样器] ├─ 账号A 处理商品 %d 个 (ID: %d-%d)\n", len(goodsA), goodsA[0].GoodID, goodsA[len(goodsA)-1].GoodID)
	log.Printf("[双账号采样器] └─ 账号B 处理商品 %d 个 (ID: %d-%d)\n", len(goodsB), goodsB[0].GoodID, goodsB[len(goodsB)-1].GoodID)

	// 并行处理两部分
	var wg sync.WaitGroup
	wg.Add(2)

	var successA, failureA int64
	var successB, failureB int64

	go func() {
		defer wg.Done()
		s, f := s.processPipelineAccountA(goodsA)
		successA = s
		failureA = f
	}()

	go func() {
		defer wg.Done()
		s, f := s.processPipelineAccountB(goodsB)
		successB = s
		failureB = f
	}()

	wg.Wait()

	duration := time.Since(startTime)
	s.mu.Lock()
	s.stats.TotalProcessed += int64(len(goods))
	s.stats.SuccessRequests += successA + successB
	s.stats.FailedRequests += failureA + failureB
	s.stats.LastRun = time.Now()
	s.stats.TotalDuration += duration
	s.stats.AccountASuccess += successA
	s.stats.AccountAFailure += failureA
	s.stats.AccountBSuccess += successB
	s.stats.AccountBFailure += failureB
	s.mu.Unlock()

	log.Printf("[双账号采样器] ===== 周期完成 =====\n")
	log.Printf("[双账号采样器] 总耗时: %v\n", duration)
	log.Printf("[双账号采样器] 账号A: 成功 %d, 失败 %d\n", successA, failureA)
	log.Printf("[双账号采样器] 账号B: 成功 %d, 失败 %d\n", successB, failureB)
	log.Printf("[双账号采样器] 总计: 成功 %d, 失败 %d\n", successA+successB, failureA+failureB)

	// 倒计时60秒
	for i := 60; i > 0; i-- {
		if i <= 10 || i%10 == 0 {
			log.Printf("[双账号采样器] %d 秒后开始下一周期...\n", i)
		}
		time.Sleep(1 * time.Second)
	}

	return nil
}

// processPipelineAccountA 账号A处理流水线 (3个售价线程 + 3个求购线程)
func (s *DualAccountSampler) processPipelineAccountA(goods []models.CSQAQGood) (int64, int64) {
	return s.processPipelineWithAccounts(goods, "A", s.clientA)
}

// processPipelineAccountB 账号B处理流水线 (3个售价线程 + 3个求购线程)
func (s *DualAccountSampler) processPipelineAccountB(goods []models.CSQAQGood) (int64, int64) {
	return s.processPipelineWithAccounts(goods, "B", s.clientB)
}

// processPipelineWithAccounts 通用流水线处理
func (s *DualAccountSampler) processPipelineWithAccounts(
	goods []models.CSQAQGood,
	accountName string,
	client *youpin.OpenAPIClient,
) (int64, int64) {
	if len(goods) == 0 {
		return 0, 0
	}

	log.Printf("[双账号采样器] [账号%s] 开始处理 (%d 个商品)\n", accountName, len(goods))

	successCount := int64(0)
	failureCount := int64(0)
	processedCount := int64(0)
	noTemplateIDCount := int64(0)
	sellPriceFailCount := int64(0)
	buyPriceFailCount := int64(0)
	buyPriceZeroCount := int64(0)
	dbSaveFailCount := int64(0)

	taskChan := make(chan models.CSQAQGood, s.numWorkers)
	var wg sync.WaitGroup

	// 使用 numWorkers 个线程（每账号独立）
	for w := 0; w < s.numWorkers; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()

			// 每个线程独立的速率限制：250ms/请求
			threadRateLimiter := time.NewTicker(250 * time.Millisecond)
			defer threadRateLimiter.Stop()

			for good := range taskChan {
				// 等待当前线程的速率限制
				<-threadRateLimiter.C

				// 检查 good 是否有 yyyp_template_id
				if good.YYYPTemplateID == nil {
					atomic.AddInt64(&noTemplateIDCount, 1)
					atomic.AddInt64(&failureCount, 1)
					atomic.AddInt64(&processedCount, 1)
					log.Printf("[双账号采样器] [账号%s] [Worker-%d] 商品ID %d 失败: 没有 TemplateID\n", accountName, wid, good.GoodID)
					continue
				}

				tid := int(*good.YYYPTemplateID)

				// 步骤1：查询售价
				timeout := 10 * time.Second
				if s.proxyConfig != nil && s.proxyConfig.Timeout > 0 {
					timeout = s.proxyConfig.Timeout
				}
				ctx, cancel := context.WithTimeout(s.ctx, timeout)
				onSaleResp, err := client.BatchGetOnSaleCommodityInfo(ctx, []youpin.BatchPriceQueryItem{
					{TemplateID: &tid},
				})
				cancel()

				if err != nil || onSaleResp == nil || onSaleResp.Code != 0 || len(onSaleResp.Data) == 0 {
					atomic.AddInt64(&sellPriceFailCount, 1)
					atomic.AddInt64(&failureCount, 1)
					atomic.AddInt64(&processedCount, 1)
					code := 0
					if onSaleResp != nil {
						code = onSaleResp.Code
					}
					log.Printf("[双账号采样器] [账号%s] [Worker-%d] 商品ID %d 失败: 获取售价失败 (err=%v, code=%d)\n", accountName, wid, good.GoodID, err, code)
					continue
				}

				sellPrice := 0.0
				if onSaleResp.Data[0].SaleCommodityResponse.MinSellPrice != "" {
					fmt.Sscanf(onSaleResp.Data[0].SaleCommodityResponse.MinSellPrice, "%f", &sellPrice)
				}
				sellCount := onSaleResp.Data[0].SaleCommodityResponse.SellNum

				// 步骤2：查询求购价
				ctx, cancel = context.WithTimeout(s.ctx, timeout)
				po, err := client.GetTemplatePurchaseOrderList(ctx, &youpin.GetTemplatePurchaseOrderListRequest{
					TemplateId:       tid,
					PageIndex:        1,
					PageSize:         40,
					ShowMaxPriceFlag: false,
				})
				cancel()

				maxPrice := 0.0
				buyPriceStatus := "成功"
				if err == nil && po != nil {
					for _, item := range po.Data {
						if item.PurchasePrice > maxPrice {
							maxPrice = item.PurchasePrice
						}
					}
					if maxPrice == 0 {
						buyPriceStatus = "查询成功但无求购"
						atomic.AddInt64(&buyPriceZeroCount, 1)
						log.Printf("[双账号采样器] [账号%s] [Worker-%d] 商品ID %d 求购价查询: %s (TemplateID: %d)\n", accountName, wid, good.GoodID, buyPriceStatus, tid)
					} else {
						atomic.AddInt64(&successCount, 1)
						log.Printf("[双账号采样器] [账号%s] [Worker-%d] 商品ID %d 求购价查询成功: %.2f (TemplateID: %d, 求购数: %d)\n", accountName, wid, good.GoodID, maxPrice, tid, len(po.Data))
					}
				} else {
					buyPriceStatus = "请求失败"
					atomic.AddInt64(&buyPriceFailCount, 1)
					log.Printf("[双账号采样器] [账号%s] [Worker-%d] 商品ID %d 求购价查询失败: %v (TemplateID: %d)\n", accountName, wid, good.GoodID, err, tid)
				}

				// 步骤3：保存到数据库
				snap := models.CSQAQGoodSnapshot{
					GoodID:         good.GoodID,
					YYYPTemplateID: good.YYYPTemplateID,
					CreatedAt:      time.Now(),
				}

				if sellPrice > 0 {
					snap.YYYPSellPrice = &sellPrice
					snap.YYYPSellCount = &sellCount
				}

				if maxPrice > 0 {
					snap.YYYPBuyPrice = &maxPrice
				}

				if err := s.db.Create(&snap).Error; err != nil {
					atomic.AddInt64(&dbSaveFailCount, 1)
					atomic.AddInt64(&failureCount, 1)
					log.Printf("[双账号采样器] [账号%s] [Worker-%d] 商品ID %d 保存失败: %v (求购价状态: %s)\n", accountName, wid, good.GoodID, err, buyPriceStatus)
				} else {
					atomic.AddInt64(&successCount, 1)
					priceWarning := ""
					if sellPrice > 0 && maxPrice > sellPrice*1.5 {
						priceWarning = " [警告: 求购价异常高]"
					}
					log.Printf("[双账号采样器] [账号%s] 商品ID %d 保存成功 (售价: %.2f, 求购价: %.2f [%s], TemplateID: %d)%s\n",
						accountName, good.GoodID, sellPrice, maxPrice, buyPriceStatus, tid, priceWarning)
				}

				atomic.AddInt64(&processedCount, 1)

				// 输出进度
				processed := atomic.LoadInt64(&processedCount)
				if processed%50 == 0 || processed == int64(len(goods)) {
					percentage := float64(processed) / float64(len(goods)) * 100
					log.Printf("[双账号采样器] [账号%s] [进度 %d/%d %.1f%%] 成功: %d, 失败: %d (无TemplateID: %d, 售价失败: %d, 求购失败: %d, 求购为0: %d, 保存失败: %d)\n",
						accountName, processed, len(goods), percentage, atomic.LoadInt64(&successCount), atomic.LoadInt64(&failureCount),
						atomic.LoadInt64(&noTemplateIDCount), atomic.LoadInt64(&sellPriceFailCount),
						atomic.LoadInt64(&buyPriceFailCount), atomic.LoadInt64(&buyPriceZeroCount), atomic.LoadInt64(&dbSaveFailCount))
				}
			}
		}(w)
	}

	// 分发任务
	go func() {
		for _, good := range goods {
			select {
			case <-s.ctx.Done():
				return
			case taskChan <- good:
			}
		}
		close(taskChan)
	}()

	wg.Wait()
	log.Printf("[双账号采样器] [账号%s] ========== 完成 ==========\n", accountName)

	return atomic.LoadInt64(&successCount), atomic.LoadInt64(&failureCount)
}

// Stop 停止采样
func (s *DualAccountSampler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()
	s.cancel()
}

// GetStats 获取统计
func (s *DualAccountSampler) GetStats() DualAccountSamplerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}
