package services

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"csqaq-sampler/internal/models"
	youpin "csqaq-sampler/internal/services/youpin"

	"gorm.io/gorm"
)

// OpenAPISampler 流水线采样器（后台异步更新出售价格缓存）
type OpenAPISamplerV3 struct {
	db          *gorm.DB
	ypClient    *youpin.OpenAPIClient
	tokenClient *youpin.OpenAPIClient
	numWorkers  int
	proxyConfig *ProxyConfig
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.RWMutex
	running     bool
	stats       OpenAPISamplerStats

	// 出售价格缓存（后台持续更新）
	onSaleCacheMu sync.RWMutex
	onSaleCache   map[int64]*OnSalePriceData // key: yyyp_template_id, value: 在售价格数据

	// 后台更新标志
	backgroundRunning bool
}

// OnSalePriceData 在售价格 (按 yyyp_template_id 存储)
type OnSalePriceData struct {
	TemplateID   int64
	MinSellPrice float64
	SellCount    int
	ProxyIP      string
}

// PurchasePriceData 求购价格
type PurchasePriceData struct {
	TemplateID  int64
	MaxBuyPrice float64
	ProxyIP     string
}

// OpenAPISamplerStats 统计信息
type OpenAPISamplerStats struct {
	TotalProcessed  int64
	SuccessRequests int64
	FailedRequests  int64
	ValidSnapshots  int64
	LastRun         time.Time
	TotalDuration   time.Duration
	AvgResponseTime float64
}

// NewOpenAPISamplerV3 创建流水线采样器（支持后台异步更新缓存）
func NewOpenAPISamplerV3(
	db *gorm.DB,
	ypClient *youpin.OpenAPIClient,
	tokenClient *youpin.OpenAPIClient,
	numWorkers int,
	proxyConfig *ProxyConfig,
) (*OpenAPISamplerV3, error) {
	if numWorkers <= 0 {
		numWorkers = 10
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &OpenAPISamplerV3{
		db:          db,
		ypClient:    ypClient,
		tokenClient: tokenClient,
		numWorkers:  numWorkers,
		proxyConfig: proxyConfig,
		ctx:         ctx,
		cancel:      cancel,
		onSaleCache: make(map[int64]*OnSalePriceData),
	}, nil
}

// Start 开始采样
func (s *OpenAPISamplerV3) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	log.Printf("[OpenAPI采样器-v3] 启动采样器\n")
	log.Printf("[OpenAPI采样器-v3] ├─ 任务1 (获取售价): %d 个工作线程\n", s.numWorkers)
	log.Printf("[OpenAPI采样器-v3] └─ 任务2 (获取求购): 3 个工作线程\n")

	// 启动主采样线程
	go s.samplingLoop()
}

// samplingLoop 采样主循环
func (s *OpenAPISamplerV3) samplingLoop() {
	time.Sleep(5 * time.Second)
	log.Println("[OpenAPI采样器-v3] 初始延迟完成，开始采样")

	for {
		select {
		case <-s.ctx.Done():
			log.Println("[OpenAPI采样器-v3] 已停止")
			return
		default:
			if err := s.runSamplingCycle(); err != nil {
				log.Printf("[OpenAPI采样器-v3] 错误: %v\n", err)
				time.Sleep(5 * time.Second)
			}
		}
	}
}

// runSamplingCycle 流水线采样周期：单任务、两线程处理
// 每个商品：查售价 → 查求购 → 保存
func (s *OpenAPISamplerV3) runSamplingCycle() error {
	startTime := time.Now()

	var goods []models.CSQAQGood
	if err := s.db.Order("updated_at desc").Find(&goods).Error; err != nil {
		return fmt.Errorf("加载商品失败: %w", err)
	}

	if len(goods) == 0 {
		log.Println("[OpenAPI采样器-v3] 没有商品，跳过")
		return nil
	}

	log.Printf("[OpenAPI采样器-v3] 开始采样周期 (%d 个商品)\n", len(goods))

	// 单任务、两线程处理每个商品
	successCount, failureCount := s.processPipeline(goods)

	duration := time.Since(startTime)
	s.mu.Lock()
	s.stats.TotalProcessed += int64(len(goods))
	s.stats.SuccessRequests += successCount
	s.stats.FailedRequests += failureCount
	s.stats.LastRun = time.Now()
	s.stats.TotalDuration += duration
	s.mu.Unlock()

	log.Printf("[OpenAPI采样器-v3] ===== 周期完成 =====\n")
	log.Printf("[OpenAPI采样器-v3] 总耗时: %v\n", duration)
	log.Printf("[OpenAPI采样器-v3] └─ 成功: %d, 失败: %d\n", successCount, failureCount)

	// 倒计时60秒
	for i := 60; i > 0; i-- {
		if i <= 10 || i%10 == 0 {
			log.Printf("[OpenAPI采样器-v3] %d 秒后开始下一周期...\n", i)
		}
		time.Sleep(1 * time.Second)
	}

	return nil
}

// processPipeline 流水线处理：两线程逐个处理商品
func (s *OpenAPISamplerV3) processPipeline(goods []models.CSQAQGood) (int64, int64) {
	log.Println("[OpenAPI采样器-v3] [流水线] 开始处理 (5线程)")

	successCount := int64(0)
	failureCount := int64(0)
	processedCount := int64(0)
	noTemplateIDCount := int64(0)
	sellPriceFailCount := int64(0)
	buyPriceFailCount := int64(0)
	buyPriceZeroCount := int64(0)
	dbSaveFailCount := int64(0)

	taskChan := make(chan models.CSQAQGood, 2)
	var wg sync.WaitGroup

	// 两个处理线程（每个线程独立的速率限制）
	for w := 0; w < 3; w++ {
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
					log.Printf("[OpenAPI采样器-v3] [流水线] [Worker-%d] 商品ID %d 失败: 没有 TemplateID\n", wid, good.GoodID)
					continue
				}

				tid := int(*good.YYYPTemplateID)

				// 步骤1：查询售价和在售数量（使用批量API，但只传一个商品）
				timeout := 10 * time.Second
				if s.proxyConfig != nil && s.proxyConfig.Timeout > 0 {
					timeout = s.proxyConfig.Timeout
				}
				ctx, cancel := context.WithTimeout(s.ctx, timeout)
				onSaleResp, err := s.ypClient.BatchGetOnSaleCommodityInfo(ctx, []youpin.BatchPriceQueryItem{
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
					log.Printf("[OpenAPI采样器-v3] [流水线] [Worker-%d] 商品ID %d 失败: 获取售价失败 (err=%v, code=%d)\n", wid, good.GoodID, err, code)
					continue
				}

				sellPrice, _ := strconv.ParseFloat(onSaleResp.Data[0].SaleCommodityResponse.MinSellPrice, 64)
				sellCount := onSaleResp.Data[0].SaleCommodityResponse.SellNum

				// 步骤2：查询求购价
				ctx, cancel = context.WithTimeout(s.ctx, timeout)
				po, err := s.tokenClient.GetTemplatePurchaseOrderList(ctx, &youpin.GetTemplatePurchaseOrderListRequest{
					TemplateId:       tid,
					PageIndex:        1,
					PageSize:         40,
					ShowMaxPriceFlag: false,
				})
				cancel()

				maxPrice := 0.0
				buyPriceStatus := "成功" // 求购价获取状态
				if err == nil && po != nil {
					for _, item := range po.Data {
						if item.PurchasePrice > maxPrice {
							maxPrice = item.PurchasePrice
						}
					}
					if maxPrice == 0 {
						buyPriceStatus = "查询成功但无求购"
						atomic.AddInt64(&buyPriceZeroCount, 1)
						log.Printf("[OpenAPI采样器-v3] [流水线] [Worker-%d] 商品ID %d 求购价查询: %s (TemplateID: %d)\n", wid, good.GoodID, buyPriceStatus, tid)
					} else {
						atomic.AddInt64(&successCount, 1)
						log.Printf("[OpenAPI采样器-v3] [流水线] [Worker-%d] 商品ID %d 求购价查询成功: %.2f (TemplateID: %d, 求购数: %d)\n", wid, good.GoodID, maxPrice, tid, len(po.Data))
					}
				} else {
					buyPriceStatus = "请求失败"
					atomic.AddInt64(&buyPriceFailCount, 1)
					log.Printf("[OpenAPI采样器-v3] [流水线] [Worker-%d] 商品ID %d 求购价查询失败: %v (TemplateID: %d)\n", wid, good.GoodID, err, tid)
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
					log.Printf("[OpenAPI采样器-v3] [流水线] [Worker-%d] 商品ID %d 保存失败: %v (求购价状态: %s)\n", wid, good.GoodID, err, buyPriceStatus)
				} else {
					atomic.AddInt64(&successCount, 1)
					priceWarning := ""
					if sellPrice > 0 && maxPrice > sellPrice*1.5 {
						priceWarning = fmt.Sprintf(" [警告: 求购价异常高]")
					}
					log.Printf("[OpenAPI采样器-v3] [流水线] 商品ID %d 保存成功 (售价: %.2f, 求购价: %.2f [%s], TemplateID: %d)%s\n",
						good.GoodID, sellPrice, maxPrice, buyPriceStatus, tid, priceWarning)
				}

				atomic.AddInt64(&processedCount, 1)

				// 输出进度
				processed := atomic.LoadInt64(&processedCount)
				if processed%50 == 0 || processed == int64(len(goods)) {
					percentage := float64(processed) / float64(len(goods)) * 100
					log.Printf("[OpenAPI采样器-v3] [流水线] [进度 %d/%d %.1f%%] 成功: %d, 失败: %d (无TemplateID: %d, 售价失败: %d, 求购失败: %d, 求购为0: %d, 保存失败: %d)\n",
						processed, len(goods), percentage, atomic.LoadInt64(&successCount), atomic.LoadInt64(&failureCount),
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
				break
			case taskChan <- good:
			}
		}
		close(taskChan)
	}()

	wg.Wait()
	log.Printf("[OpenAPI采样器-v3] [流水线] ========== 完成 ==========\n")

	return atomic.LoadInt64(&successCount), atomic.LoadInt64(&failureCount)
}

// stage1GetOnSalePrices 第一阶段：Batch获取在售价格（200个/批，1s最多5次）
func (s *OpenAPISamplerV3) stage1GetOnSalePrices(goods []models.CSQAQGood) (map[int64]*OnSalePriceData, error) {
	log.Println("[OpenAPI采样器-v3] [第一阶段] 开始获取在售价格 (200/批,1s5次)")

	result := make(map[int64]*OnSalePriceData)
	successCount := int64(0)
	failureCount := int64(0)
	processedBatches := int64(0)

	// 分批处理
	batchSize := 200
	totalBatches := (len(goods) + batchSize - 1) / batchSize
	taskChan := make(chan int, s.numWorkers*2)
	var wg sync.WaitGroup
	mu := sync.Mutex{}

	// 批次速率限制器 (1s最多5次 = 200ms/批)
	batchRateLimiter := time.NewTicker(200 * time.Millisecond)
	defer batchRateLimiter.Stop()

	// Worker
	for w := 0; w < s.numWorkers; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			for batchIdx := range taskChan {
				// 等待批次速率限制器
				<-batchRateLimiter.C

				start := batchIdx * batchSize
				end := start + batchSize
				if end > len(goods) {
					end = len(goods)
				}

				batch := goods[start:end]
				reqList := make([]youpin.BatchPriceQueryItem, 0)
				goodsMap := make(map[int]int64)
				goodsDetailMap := make(map[int64]models.CSQAQGood) // 用于快速查找good对象

				// 构建请求 - 使用 csqaq_goods 表中的 yyyp_template_id
				for _, g := range batch {
					if g.YYYPTemplateID != nil && *g.YYYPTemplateID > 0 {
						tid := int(*g.YYYPTemplateID)
						reqList = append(reqList, youpin.BatchPriceQueryItem{TemplateID: &tid})
						goodsMap[len(reqList)-1] = g.GoodID
						goodsDetailMap[g.GoodID] = g
					}
				}

				if len(reqList) == 0 {
					atomic.AddInt64(&failureCount, int64(len(batch)))
					atomic.AddInt64(&processedBatches, 1)

					// 输出进度
					currentBatch := atomic.LoadInt64(&processedBatches)
					currentSuccess := atomic.LoadInt64(&successCount)
					currentFailure := atomic.LoadInt64(&failureCount)
					percentage := float64(currentBatch) / float64(totalBatches) * 100
					lastGoodID := batch[len(batch)-1].GoodID
					log.Printf("[OpenAPI采样器-v3] [第一阶段] [进度 %d/%d %.1f%%] 最后商品ID: %d, 成功: %d, 失败: %d (代理IP: 无)\n",
						currentBatch, totalBatches, percentage, lastGoodID, currentSuccess, currentFailure)
					continue
				}

				// 调用API (OpenAPI不需要代理)
				ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
				resp, err := s.ypClient.BatchGetOnSaleCommodityInfo(ctx, reqList)
				cancel()

				if err == nil && resp != nil {
					mu.Lock()
					for i, data := range resp.Data {
						if goodID, ok := goodsMap[i]; ok {
							if g, ok := goodsDetailMap[goodID]; ok && g.YYYPTemplateID != nil {
								price, _ := strconv.ParseFloat(data.SaleCommodityResponse.MinSellPrice, 64)
								result[*g.YYYPTemplateID] = &OnSalePriceData{
									TemplateID:   *g.YYYPTemplateID,
									MinSellPrice: price,
									SellCount:    data.SaleCommodityResponse.SellNum,
									ProxyIP:      "",
								}
								atomic.AddInt64(&successCount, 1)
							}
						}
					}
					mu.Unlock()
				} else {
					atomic.AddInt64(&failureCount, int64(len(reqList)))
					log.Printf("[OpenAPI采样器-v3] [第一阶段] [Worker-%d] 批次 %d 失败: %v\n", wid, batchIdx+1, err)
				}

				atomic.AddInt64(&processedBatches, 1)

				// 输出进度
				currentBatch := atomic.LoadInt64(&processedBatches)
				currentSuccess := atomic.LoadInt64(&successCount)
				currentFailure := atomic.LoadInt64(&failureCount)
				percentage := float64(currentBatch) / float64(totalBatches) * 100
				lastGoodID := batch[len(batch)-1].GoodID
				if currentBatch%5 == 0 || currentBatch == int64(totalBatches) {
					log.Printf("[OpenAPI采样器-v3] [第一阶段] [进度 %d/%d %.1f%%] 最后商品ID: %d, 成功: %d, 失败: %d\n",
						currentBatch, totalBatches, percentage, lastGoodID, currentSuccess, currentFailure)
				}
			}
		}(w)
	}

	// 分发任务
	go func() {
		for i := 0; i < totalBatches; i++ {
			select {
			case <-s.ctx.Done():
				break
			case taskChan <- i:
			}
		}
		close(taskChan)
	}()

	wg.Wait()
	log.Printf("[OpenAPI采样器-v3] [第一阶段] ========== 完成 ==========\n")
	log.Printf("[OpenAPI采样器-v3] [第一阶段] 总批次: %d, 成功: %d, 失败: %d\n", totalBatches, atomic.LoadInt64(&successCount), atomic.LoadInt64(&failureCount))
	return result, nil
}

// stage2GetPurchasePrices 第二阶段：查询求购价格（使用代理）
func (s *OpenAPISamplerV3) stage2GetPurchasePrices(goods []models.CSQAQGood) (map[int64]*PurchasePriceData, error) {
	log.Println("[OpenAPI采样器-v3] [第二阶段] 开始获取求购价格")

	// 日志：显示代理配置
	if s.proxyConfig != nil && s.proxyConfig.Enabled {
		log.Printf("[OpenAPI采样器-v3] [第二阶段] 使用代理: %s (超时: %v)\n", s.proxyConfig.URL, s.proxyConfig.Timeout)
	}

	result := make(map[int64]*PurchasePriceData)
	successCount := int64(0)
	failureCount := int64(0)
	processedCount := int64(0)
	lastGoodID := int64(0)
	noTemplateIDCount := int64(0)
	buyPriceZeroCount := int64(0)
	buyPriceFailCount := int64(0)
	taskChan := make(chan models.CSQAQGood, s.numWorkers*2)
	var wg sync.WaitGroup
	mu := sync.Mutex{}
	rateLimiter := time.NewTicker(500 * time.Millisecond) // 每500ms最多发一个请求，避免API速率限制
	defer rateLimiter.Stop()

	// 为了避免API速率限制（84104错误），使用较少的Worker和请求间隔
	// Token认证接口对同一账户的并发和频率有严格限制
	numWorkers := 3
	if s.numWorkers < 3 {
		numWorkers = s.numWorkers
	}

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()

			for good := range taskChan {
				// 等待速率限制器
				<-rateLimiter.C

				// 检查 good 是否有 yyyp_template_id
				if good.YYYPTemplateID == nil {
					atomic.AddInt64(&noTemplateIDCount, 1)
					atomic.AddInt64(&failureCount, 1)
					atomic.AddInt64(&processedCount, 1)
					atomic.StoreInt64(&lastGoodID, good.GoodID)
					log.Printf("[OpenAPI采样器-v3] [第二阶段] [Worker-%d] 商品ID %d 失败: 没有 TemplateID\n", wid, good.GoodID)
					continue
				}

				tid := int(*good.YYYPTemplateID)

				// 使用配置的超时时间
				timeout := 10 * time.Second
				if s.proxyConfig != nil && s.proxyConfig.Timeout > 0 {
					timeout = s.proxyConfig.Timeout
				}
				ctx, cancel := context.WithTimeout(s.ctx, timeout)
				po, err := s.tokenClient.GetTemplatePurchaseOrderList(ctx, &youpin.GetTemplatePurchaseOrderListRequest{
					TemplateId:       tid,
					PageIndex:        1,
					PageSize:         40,
					ShowMaxPriceFlag: false,
				})
				cancel()

				if err == nil && po != nil {
					maxPrice := 0.0
					for _, item := range po.Data {
						if item.PurchasePrice > maxPrice {
							maxPrice = item.PurchasePrice
						}
					}
					if maxPrice == 0 {
						atomic.AddInt64(&buyPriceZeroCount, 1)
						log.Printf("[OpenAPI采样器-v3] [第二阶段] [Worker-%d] 商品ID %d 求购价为0: 查询成功但无求购\n", wid, good.GoodID)
					} else {
						log.Printf("[OpenAPI采样器-v3] [第二阶段] [Worker-%d] 商品ID %d 求购价查询成功: %.2f (TemplateID: %d, 求购数: %d)\n", wid, good.GoodID, maxPrice, tid, len(po.Data))
					}
					mu.Lock()
					result[*good.YYYPTemplateID] = &PurchasePriceData{
						TemplateID:  *good.YYYPTemplateID,
						MaxBuyPrice: maxPrice,
						ProxyIP:     s.proxyConfig.URL,
					}
					mu.Unlock()
					atomic.AddInt64(&successCount, 1)
				} else {
					atomic.AddInt64(&buyPriceFailCount, 1)
					atomic.AddInt64(&failureCount, 1)
					log.Printf("[OpenAPI采样器-v3] [第二阶段] [Worker-%d] 商品ID %d 求购价查询失败: %v (TemplateID: %d, 代理: %s)\n", wid, good.GoodID, err, tid, s.proxyConfig.URL)
				}

				atomic.AddInt64(&processedCount, 1)
				atomic.StoreInt64(&lastGoodID, good.GoodID)

				// 输出进度（每50个商品或处理完成时）
				processed := atomic.LoadInt64(&processedCount)
				if processed%50 == 0 || processed == int64(len(goods)) {
					percentage := float64(processed) / float64(len(goods)) * 100
					log.Printf("[OpenAPI采样器-v3] [第二阶段] [进度 %d/%d %.1f%%] 最后商品ID: %d, 成功: %d, 失败: %d (无TemplateID: %d, 求购失败: %d, 求购为0: %d, 代理: %s)\n",
						processed, len(goods), percentage, atomic.LoadInt64(&lastGoodID), atomic.LoadInt64(&successCount), atomic.LoadInt64(&failureCount),
						atomic.LoadInt64(&noTemplateIDCount), atomic.LoadInt64(&buyPriceFailCount), atomic.LoadInt64(&buyPriceZeroCount), s.proxyConfig.URL)
				}
			}
		}(w)
	}

	go func() {
		for _, good := range goods {
			select {
			case <-s.ctx.Done():
				break
			case taskChan <- good:
			}
		}
		close(taskChan)
	}()

	wg.Wait()
	log.Printf("[OpenAPI采样器-v3] [第二阶段] ========== 完成 ==========\n")
	log.Printf("[OpenAPI采样器-v3] [第二阶段] 总数: %d, 成功: %d, 失败: %d\n", len(goods), atomic.LoadInt64(&successCount), atomic.LoadInt64(&failureCount))
	return result, nil
}

// stage3SaveData 第三阶段：保存数据到数据库
func (s *OpenAPISamplerV3) stage3SaveData(
	goods []models.CSQAQGood,
	onSaleMap map[int64]*OnSalePriceData,
	purchaseMap map[int64]*PurchasePriceData,
) (int64, int64, error) {
	log.Println("[OpenAPI采样器-v3] [第三阶段] 开始保存数据")

	successCount := int64(0)
	failureCount := int64(0)

	for idx, good := range goods {
		// 检查 good 是否有 yyyp_template_id
		if good.YYYPTemplateID == nil {
			atomic.AddInt64(&failureCount, 1)

			// 输出进度（每100个或处理完成时）
			if (idx+1)%100 == 0 || idx+1 == len(goods) {
				percentage := float64(idx+1) / float64(len(goods)) * 100
				log.Printf("[OpenAPI采样器-v3] [第三阶段] [进度 %d/%d %.1f%%] 最后商品ID: %d, 成功: %d, 失败: %d\n",
					idx+1, len(goods), percentage, good.GoodID, atomic.LoadInt64(&successCount), atomic.LoadInt64(&failureCount))
			}
			continue
		}

		// 从售价 map 查找（key 是 template_id）
		onSale, ok := onSaleMap[*good.YYYPTemplateID]
		if !ok {
			atomic.AddInt64(&failureCount, 1)

			// 输出进度（每100个或处理完成时）
			if (idx+1)%100 == 0 || idx+1 == len(goods) {
				percentage := float64(idx+1) / float64(len(goods)) * 100
				log.Printf("[OpenAPI采样器-v3] [第三阶段] [进度 %d/%d %.1f%%] 最后商品ID: %d, 成功: %d, 失败: %d\n",
					idx+1, len(goods), percentage, good.GoodID, atomic.LoadInt64(&successCount), atomic.LoadInt64(&failureCount))
			}
			continue
		}

		snap := models.CSQAQGoodSnapshot{
			GoodID:         good.GoodID,
			YYYPTemplateID: good.YYYPTemplateID,
			CreatedAt:      time.Now(),
		}

		if onSale.MinSellPrice > 0 {
			snap.YYYPSellPrice = &onSale.MinSellPrice
			snap.YYYPSellCount = &onSale.SellCount
		}

		// 从求购 map 查找（key 是 template_id）
		// 写入求购价：若查不到最大求购价，避免写入NULL，回退写入0.0
		if purchase, ok := purchaseMap[*good.YYYPTemplateID]; ok && purchase.MaxBuyPrice > 0 {
			snap.YYYPBuyPrice = &purchase.MaxBuyPrice
		} else {
			zero := 0.0
			snap.YYYPBuyPrice = &zero
		}

		if err := s.db.Create(&snap).Error; err != nil {
			atomic.AddInt64(&failureCount, 1)
			log.Printf("[OpenAPI采样器-v3] [第三阶段] 商品ID %d 保存失败: %v\n", good.GoodID, err)
		} else {
			atomic.AddInt64(&successCount, 1)
			priceWarning := ""
			if onSale.MinSellPrice > 0 && snap.YYYPBuyPrice != nil && *snap.YYYPBuyPrice > onSale.MinSellPrice*1.5 {
				priceWarning = fmt.Sprintf(" [警告: 求购价异常高]")
			}
			log.Printf("[OpenAPI采样器-v3] [第三阶段] 商品ID %d 保存成功 (售价: %.2f, 求购价: %.2f, TemplateID: %d)%s\n",
				good.GoodID, onSale.MinSellPrice,
				func() float64 {
					if snap.YYYPBuyPrice != nil {
						return *snap.YYYPBuyPrice
					}
					return 0
				}(),
				*good.YYYPTemplateID, priceWarning)
		}

		// 输出进度（每100个或处理完成时）
		if (idx+1)%100 == 0 || idx+1 == len(goods) {
			percentage := float64(idx+1) / float64(len(goods)) * 100
			log.Printf("[OpenAPI采样器-v3] [第三阶段] [进度 %d/%d %.1f%%] 最后商品ID: %d, 成功: %d, 失败: %d\n",
				idx+1, len(goods), percentage, good.GoodID, atomic.LoadInt64(&successCount), atomic.LoadInt64(&failureCount))
		}
	}

	log.Printf("[OpenAPI采样器-v3] [第三阶段] ========== 完成 ==========\n")
	log.Printf("[OpenAPI采样器-v3] [第三阶段] 总数: %d, 成功: %d, 失败: %d\n", len(goods), atomic.LoadInt64(&successCount), atomic.LoadInt64(&failureCount))
	return atomic.LoadInt64(&successCount), atomic.LoadInt64(&failureCount), nil
}

// backgroundUpdateOnSaleCache 后台异步更新出售价格缓存（持续运行）
func (s *OpenAPISamplerV3) backgroundUpdateOnSaleCache() {
	log.Println("[OpenAPI采样器-v3] [后台缓存] 启动后台出售价格缓存更新")

	var lastGoods []models.CSQAQGood
	updateTicker := time.NewTicker(5 * time.Second) // 每5秒更新一次缓存
	defer updateTicker.Stop()

	batchSize := 200
	var wg sync.WaitGroup

	// 第一次立即更新缓存（预热） - 必须等待完成
	log.Println("[OpenAPI采样器-v3] [后台缓存] 执行初始缓存预热...")
	var goods []models.CSQAQGood
	if err := s.db.Order("updated_at desc").Find(&goods).Error; err == nil {
		lastGoods = goods
		s.performCacheUpdate(goods, batchSize, &wg)
		log.Println("[OpenAPI采样器-v3] [后台缓存] 初始缓存预热完成")
	}

	for {
		select {
		case <-s.ctx.Done():
			log.Println("[OpenAPI采样器-v3] [后台缓存] 停止后台缓存更新")
			return
		case <-updateTicker.C:
			// 定期更新全部商品的出售价格缓存
			var goods []models.CSQAQGood
			if err := s.db.Order("updated_at desc").Find(&goods).Error; err != nil {
				log.Printf("[OpenAPI采样器-v3] [后台缓存] 加载商品失败: %v\n", err)
				continue
			}

			if len(goods) == 0 {
				continue
			}

			// 检查商品列表是否有变化
			if len(goods) != len(lastGoods) || (len(goods) > 0 && len(lastGoods) > 0 && goods[0].GoodID != lastGoods[0].GoodID) {
				log.Printf("[OpenAPI采样器-v3] [后台缓存] 商品列表更新，当前: %d 个商品\n", len(goods))
				lastGoods = goods
			}

			// 执行缓存更新
			s.performCacheUpdate(goods, batchSize, &wg)
		}
	}
}

// stage2PipelineGetAndSave 流水线处理Stage 2：获取求购价格 + 实时保存（每5个商品为一批）
func (s *OpenAPISamplerV3) stage2PipelineGetAndSave(goods []models.CSQAQGood) (int64, int64, error) {
	log.Println("[OpenAPI采样器-v3] [流水线] 开始流水线采样（边获取边保存）")

	if s.proxyConfig != nil && s.proxyConfig.Enabled {
		log.Printf("[OpenAPI采样器-v3] [流水线] 使用代理: %s (超时: %v)\n", s.proxyConfig.URL, s.proxyConfig.Timeout)
	}

	successCount := int64(0)
	failureCount := int64(0)
	processedCount := int64(0)
	noTemplateIDCount := int64(0)
	noCacheCount := int64(0)
	buyPriceFailCount := int64(0)
	buyPriceZeroCount := int64(0)
	dbSaveFailCount := int64(0)

	taskChan := make(chan models.CSQAQGood, s.numWorkers*2)
	var wg sync.WaitGroup
	rateLimiter := time.NewTicker(500 * time.Millisecond) // 每500ms最多发一个请求
	defer rateLimiter.Stop()

	// 为了避免API速率限制，使用较少的Worker
	numWorkers := 3
	if s.numWorkers < 3 {
		numWorkers = s.numWorkers
	}

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()

			for good := range taskChan {
				// 等待速率限制器
				<-rateLimiter.C

				// 检查 good 的 yyyp_template_id - 使用 csqaq_goods 表中的值
				if good.YYYPTemplateID == nil {
					atomic.AddInt64(&noTemplateIDCount, 1)
					atomic.AddInt64(&failureCount, 1)
					atomic.AddInt64(&processedCount, 1)
					log.Printf("[OpenAPI采样器-v3] [流水线] [Worker-%d] 商品ID %d 失败: 没有 TemplateID\n", wid, good.GoodID)
					continue
				}

				tid := int(*good.YYYPTemplateID)

				// 从缓存获取出售价格 (使用 template_id 作为 key)
				s.onSaleCacheMu.RLock()
				onSale, ok := s.onSaleCache[*good.YYYPTemplateID]
				s.onSaleCacheMu.RUnlock()

				if !ok {
					atomic.AddInt64(&noCacheCount, 1)
					atomic.AddInt64(&failureCount, 1)
					atomic.AddInt64(&processedCount, 1)
					log.Printf("[OpenAPI采样器-v3] [流水线] [Worker-%d] 商品ID %d 失败: 缓存中无售价数据\n", wid, good.GoodID)
					continue
				}

				// 获取求购价格
				timeout := 10 * time.Second
				if s.proxyConfig != nil && s.proxyConfig.Timeout > 0 {
					timeout = s.proxyConfig.Timeout
				}
				ctx, cancel := context.WithTimeout(s.ctx, timeout)
				po, err := s.tokenClient.GetTemplatePurchaseOrderList(ctx, &youpin.GetTemplatePurchaseOrderListRequest{
					TemplateId:       tid,
					PageIndex:        1,
					PageSize:         40,
					ShowMaxPriceFlag: false,
				})
				cancel()

				buyPriceStatus := "成功"
				if err == nil && po != nil {
					maxPrice := 0.0
					for _, item := range po.Data {
						if item.PurchasePrice > maxPrice {
							maxPrice = item.PurchasePrice
						}
					}

					if maxPrice == 0 {
						buyPriceStatus = "查询成功但无求购"
						atomic.AddInt64(&buyPriceZeroCount, 1)
						log.Printf("[OpenAPI采样器-v3] [流水线] [Worker-%d] 商品ID %d 求购价查询: %s (TemplateID: %d)\n", wid, good.GoodID, buyPriceStatus, tid)
					} else {
						log.Printf("[OpenAPI采样器-v3] [流水线] [Worker-%d] 商品ID %d 求购价查询成功: %.2f (TemplateID: %d, 求购数: %d)\n", wid, good.GoodID, maxPrice, tid, len(po.Data))
					}

					// 构建快照并立即保存（不等待）
					snapShot := models.CSQAQGoodSnapshot{
						GoodID:         good.GoodID,
						YYYPTemplateID: good.YYYPTemplateID,
						CreatedAt:      time.Now(),
					}

					if onSale.MinSellPrice > 0 {
						snapShot.YYYPSellPrice = &onSale.MinSellPrice
						snapShot.YYYPSellCount = &onSale.SellCount
					}

					// 写入求购价：若没有有效最大价，回退写入0.0，避免为NULL
					if maxPrice > 0 {
						snapShot.YYYPBuyPrice = &maxPrice
					} else {
						zero := 0.0
						snapShot.YYYPBuyPrice = &zero
					}

					// 立即保存到数据库
					if err := s.db.Create(&snapShot).Error; err != nil {
						atomic.AddInt64(&dbSaveFailCount, 1)
						atomic.AddInt64(&failureCount, 1)
						log.Printf("[OpenAPI采样器-v3] [流水线] 商品ID %d 保存失败: %v (求购价状态: %s)\n", good.GoodID, err, buyPriceStatus)
					} else {
						atomic.AddInt64(&successCount, 1)
						priceWarning := ""
						if maxPrice > onSale.MinSellPrice && maxPrice > onSale.MinSellPrice*1.5 {
							priceWarning = fmt.Sprintf(" [警告: 求购价异常高]")
						}
						log.Printf("[OpenAPI采样器-v3] [流水线] 商品ID %d 实时保存成功 (售价: %.2f, 求购价: %.2f [%s], TemplateID: %d)%s\n", good.GoodID, onSale.MinSellPrice, maxPrice, buyPriceStatus, tid, priceWarning)
					}
				} else {
					buyPriceStatus = "请求失败"
					atomic.AddInt64(&buyPriceFailCount, 1)
					atomic.AddInt64(&failureCount, 1)
					log.Printf("[OpenAPI采样器-v3] [流水线] [Worker-%d] 商品ID %d 求购价查询失败: %v (TemplateID: %d)\n", wid, good.GoodID, err, tid)
				}

				atomic.AddInt64(&processedCount, 1)

				// 输出进度（每50个商品）
				processed := atomic.LoadInt64(&processedCount)
				if processed%50 == 0 || processed == int64(len(goods)) {
					percentage := float64(processed) / float64(len(goods)) * 100
					log.Printf("[OpenAPI采样器-v3] [流水线] [进度 %d/%d %.1f%%] 成功: %d, 失败: %d (无TemplateID: %d, 缓存无售价: %d, 求购失败: %d, 求购为0: %d, 保存失败: %d)\n",
						processed, len(goods), percentage, atomic.LoadInt64(&successCount), atomic.LoadInt64(&failureCount),
						atomic.LoadInt64(&noTemplateIDCount), atomic.LoadInt64(&noCacheCount), atomic.LoadInt64(&buyPriceFailCount), atomic.LoadInt64(&buyPriceZeroCount), atomic.LoadInt64(&dbSaveFailCount))
				}
			}
		}(w)
	}

	go func() {
		for _, good := range goods {
			select {
			case <-s.ctx.Done():
				break
			case taskChan <- good:
			}
		}
		close(taskChan)
	}()

	wg.Wait()
	log.Printf("[OpenAPI采样器-v3] [流水线] ========== 完成 ==========\n")
	log.Printf("[OpenAPI采样器-v3] [流水线] 总数: %d, 成功: %d, 失败: %d\n", len(goods), atomic.LoadInt64(&successCount), atomic.LoadInt64(&failureCount))
	return atomic.LoadInt64(&successCount), atomic.LoadInt64(&failureCount), nil
}

// Stop 停止采样
func (s *OpenAPISamplerV3) Stop() {
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
func (s *OpenAPISamplerV3) GetStats() OpenAPISamplerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// ClearCache 清空缓存，避免使用过时数据（启动时调用）
func (s *OpenAPISamplerV3) ClearCache() {
	s.onSaleCacheMu.Lock()
	defer s.onSaleCacheMu.Unlock()

	oldSize := len(s.onSaleCache)
	s.onSaleCache = make(map[int64]*OnSalePriceData)

	if oldSize > 0 {
		log.Printf("[OpenAPI采样器-v3] [缓存] 清空旧缓存，释放 %d 条记录\n", oldSize)
	}
}

// GetCacheSize 获取缓存大小
func (s *OpenAPISamplerV3) GetCacheSize() int {
	s.onSaleCacheMu.RLock()
	defer s.onSaleCacheMu.RUnlock()
	return len(s.onSaleCache)
}

// performCacheUpdate 执行缓存更新的内部方法
func (s *OpenAPISamplerV3) performCacheUpdate(goods []models.CSQAQGood, batchSize int, wg *sync.WaitGroup) {
	totalBatches := (len(goods) + batchSize - 1) / batchSize
	taskChan := make(chan int, s.numWorkers)

	for w := 0; w < s.numWorkers; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			for batchIdx := range taskChan {
				start := batchIdx * batchSize
				end := start + batchSize
				if end > len(goods) {
					end = len(goods)
				}

				batch := goods[start:end]
				reqList := make([]youpin.BatchPriceQueryItem, 0)
				goodsMap := make(map[int]int64)
				goodsDetailMap := make(map[int64]models.CSQAQGood)

				// 构建请求 - 使用 csqaq_goods 表中的 yyyp_template_id
				for _, g := range batch {
					if g.YYYPTemplateID != nil && *g.YYYPTemplateID > 0 {
						tid := int(*g.YYYPTemplateID)
						reqList = append(reqList, youpin.BatchPriceQueryItem{TemplateID: &tid})
						goodsMap[len(reqList)-1] = g.GoodID
						goodsDetailMap[g.GoodID] = g
					}
				}

				if len(reqList) == 0 {
					continue
				}

				// 调用API获取在售价格
				ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
				resp, err := s.ypClient.BatchGetOnSaleCommodityInfo(ctx, reqList)
				cancel()

				if err == nil && resp != nil {
					s.onSaleCacheMu.Lock()
					for i, data := range resp.Data {
						if goodID, ok := goodsMap[i]; ok {
							if g, ok := goodsDetailMap[goodID]; ok && g.YYYPTemplateID != nil {
								price, _ := strconv.ParseFloat(data.SaleCommodityResponse.MinSellPrice, 64)
								s.onSaleCache[*g.YYYPTemplateID] = &OnSalePriceData{
									TemplateID:   *g.YYYPTemplateID,
									MinSellPrice: price,
									SellCount:    data.SaleCommodityResponse.SellNum,
									ProxyIP:      "",
								}
							}
						}
					}
					s.onSaleCacheMu.Unlock()
				}

				time.Sleep(200 * time.Millisecond) // 1s最多5次
			}
		}(w)
	}

	// 分发任务
	go func() {
		for i := 0; i < totalBatches; i++ {
			select {
			case <-s.ctx.Done():
				break
			case taskChan <- i:
			}
		}
		close(taskChan)
	}()

	wg.Wait()

	// 输出缓存统计
	s.onSaleCacheMu.RLock()
	cacheSize := len(s.onSaleCache)
	s.onSaleCacheMu.RUnlock()
	log.Printf("[OpenAPI采样器-v3] [后台缓存] 缓存更新完成，缓存大小: %d/%d\n", cacheSize, len(goods))
}
