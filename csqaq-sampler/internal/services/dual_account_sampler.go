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

// GoodWithSellPrice 带售价信息的商品
type GoodWithSellPrice struct {
	Good      models.CSQAQGood
	SellPrice float64
	SellCount int
	Timestamp time.Time
}

// AccountQueue 账号队列
type AccountQueue struct {
	queue       chan GoodWithSellPrice
	mu          sync.RWMutex
	queueEmpty  chan struct{} // 队列为空信号
	lastProduce time.Time     // 上次生产时间
	size        int64         // 当前队列大小
}

// NewAccountQueue 创建账号队列
func NewAccountQueue(capacity int) *AccountQueue {
	return &AccountQueue{
		queue:      make(chan GoodWithSellPrice, capacity),
		queueEmpty: make(chan struct{}, 1),
		size:       0,
	}
}

// Push 向队列添加商品
func (q *AccountQueue) Push(item GoodWithSellPrice) {
	q.queue <- item
	atomic.AddInt64(&q.size, 1)
}

// Pop 从队列取出商品
func (q *AccountQueue) Pop() (GoodWithSellPrice, bool) {
	select {
	case item := <-q.queue:
		atomic.AddInt64(&q.size, -1)
		return item, true
	default:
		return GoodWithSellPrice{}, false
	}
}

// Size 获取队列大小
func (q *AccountQueue) Size() int64 {
	return atomic.LoadInt64(&q.size)
}

// DualAccountSampler 双账号采样器
// 将商品列表分成两部分，分别由A、B账号处理
// A账号处理商品0-N/2，B账号处理商品N/2-N
// 使用生产者-消费者模式：生产者批量查询售价，消费者查询求购价
type DualAccountSampler struct {
	db          *gorm.DB
	clientA     *youpin.OpenAPIClient
	clientB     *youpin.OpenAPIClient
	numWorkers  int
	proxyConfig *ProxyConfig
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.RWMutex
	running     bool
	stats       DualAccountSamplerStats
	queueA      *AccountQueue // 账号A的商品队列
	queueB      *AccountQueue // 账号B的商品队列
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
		numWorkers = 6 // 每账号6个消费者线程
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &DualAccountSampler{
		db:          db,
		clientA:     clientA,
		clientB:     clientB,
		numWorkers:  numWorkers,
		proxyConfig: proxyConfig,
		ctx:         ctx,
		cancel:      cancel,
		queueA:      NewAccountQueue(500), // 队列容量500
		queueB:      NewAccountQueue(500),
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

	log.Printf("[双账号采样器] 启动采样器 (生产者-消费者模式)\n")
	log.Printf("[双账号采样器] ├─ 账号A (ID: 1645231): 1个生产者线程 + %d个消费者线程\n", s.numWorkers)
	log.Printf("[双账号采样器] └─ 账号B (ID: 12919014): 1个生产者线程 + %d个消费者线程\n", s.numWorkers)

	// 启动主采样线程
	go s.samplingLoop()
}

// samplingLoop 采样主循环
func (s *DualAccountSampler) samplingLoop() {
	time.Sleep(5 * time.Second)
	log.Println("[双账号采样器] 初始延迟完成，开始采样")

	// 获取商品列表并分割
	var goods []models.CSQAQGood
	if err := s.db.Order("updated_at desc").Find(&goods).Error; err != nil {
		log.Printf("[双账号采样器] 加载商品失败: %v\n", err)
		return
	}

	if len(goods) == 0 {
		log.Println("[双账号采样器] 没有商品，退出")
		return
	}

	mid := len(goods) / 2
	goodsA := goods[:mid]
	goodsB := goods[mid:]

	log.Printf("[双账号采样器] 商品分配完成\n")
	log.Printf("[双账号采样器] ├─ 账号A: %d 个商品\n", len(goodsA))
	log.Printf("[双账号采样器] └─ 账号B: %d 个商品\n", len(goodsB))

	var wg sync.WaitGroup
	wg.Add(2)

	// 启动账号A的生产者和消费者
	go func() {
		defer wg.Done()
		s.runAccountPipeline("A", goodsA, s.clientA, s.queueA)
	}()

	// 启动账号B的生产者和消费者
	go func() {
		defer wg.Done()
		s.runAccountPipeline("B", goodsB, s.clientB, s.queueB)
	}()

	wg.Wait()
	log.Println("[双账号采样器] 所有任务完成")
}

// runAccountPipeline 运行单个账号的生产者-消费者流水线
func (s *DualAccountSampler) runAccountPipeline(
	accountName string,
	goods []models.CSQAQGood,
	client *youpin.OpenAPIClient,
	queue *AccountQueue,
) {
	log.Printf("[双账号采样器] [账号%s] 启动流水线 (%d 个商品)\n", accountName, len(goods))

	var wg sync.WaitGroup
	wg.Add(2)

	// 统计信息
	var producedCount int64
	var successCount int64
	var failureCount int64

	// 启动生产者线程
	go func() {
		defer wg.Done()
		produced := s.runProducer(accountName, goods, client, queue)
		atomic.AddInt64(&producedCount, produced)
	}()

	// 启动消费者线程池
	go func() {
		defer wg.Done()
		success, failure := s.runConsumers(accountName, client, queue, len(goods))
		atomic.AddInt64(&successCount, success)
		atomic.AddInt64(&failureCount, failure)
	}()

	wg.Wait()

	// 更新统计
	s.mu.Lock()
	s.stats.TotalProcessed += int64(len(goods))
	s.stats.SuccessRequests += successCount
	s.stats.FailedRequests += failureCount
	if accountName == "A" {
		s.stats.AccountASuccess += successCount
		s.stats.AccountAFailure += failureCount
	} else {
		s.stats.AccountBSuccess += successCount
		s.stats.AccountBFailure += failureCount
	}
	s.mu.Unlock()

	log.Printf("[双账号采样器] [账号%s] 流水线完成 - 生产: %d, 成功: %d, 失败: %d\n",
		accountName, producedCount, successCount, failureCount)
}

// runProducer 生产者线程：批量查询售价并放入队列
// 每批200个商品，每批查询后等待300秒
func (s *DualAccountSampler) runProducer(
	accountName string,
	goods []models.CSQAQGood,
	client *youpin.OpenAPIClient,
	queue *AccountQueue,
) int64 {
	const batchSize = 200
	const batchInterval = 300 * time.Second // 每批之间间隔300秒

	producedTotal := int64(0)
	batchNum := 0
	totalBatches := (len(goods) + batchSize - 1) / batchSize

	log.Printf("[双账号采样器] [账号%s] [生产者] 开始循环生产 (共 %d 批, 每批间隔 %v)\n",
		accountName, totalBatches, batchInterval)

	for {
		select {
		case <-s.ctx.Done():
			log.Printf("[双账号采样器] [账号%s] [生产者] 收到停止信号\n", accountName)
			return producedTotal
		default:
		}

		// 获取当前批次的商品
		startIdx := (batchNum % totalBatches) * batchSize
		endIdx := startIdx + batchSize
		if endIdx > len(goods) {
			endIdx = len(goods)
		}
		batch := goods[startIdx:endIdx]

		currentBatchNum := (batchNum % totalBatches) + 1
		batchNum++

		log.Printf("[双账号采样器] [账号%s] [生产者] ===== 批次 %d/%d (共 %d 个商品) =====\n",
			accountName, currentBatchNum, totalBatches, len(batch))
		log.Printf("[双账号采样器] [账号%s] [生产者] 当前队列大小: %d\n", accountName, queue.Size())

		// 构建批量查询请求
		var queryItems []youpin.BatchPriceQueryItem
		goodsMap := make(map[int]models.CSQAQGood) // templateID -> good

		for _, good := range batch {
			if good.YYYPTemplateID != nil {
				tid := int(*good.YYYPTemplateID)
				queryItems = append(queryItems, youpin.BatchPriceQueryItem{
					TemplateID: &tid,
				})
				goodsMap[tid] = good
			}
		}

		if len(queryItems) == 0 {
			log.Printf("[双账号采样器] [账号%s] [生产者] 批次 %d: 没有有效的TemplateID，跳过\n",
				accountName, currentBatchNum)

			// 即使跳过也要等待300秒
			log.Printf("[双账号采样器] [账号%s] [生产者] 等待 %v 后处理下一批...\n", accountName, batchInterval)
			time.Sleep(batchInterval)
			continue
		}

		// 批量查询售价
		batchStart := time.Now()
		timeout := 30 * time.Second
		if s.proxyConfig != nil && s.proxyConfig.Timeout > 0 {
			timeout = s.proxyConfig.Timeout
		}
		ctx, cancel := context.WithTimeout(s.ctx, timeout)
		onSaleResp, err := client.BatchGetOnSaleCommodityInfo(ctx, queryItems)
		cancel()

		if err != nil || onSaleResp == nil || onSaleResp.Code != 0 {
			code := 0
			if onSaleResp != nil {
				code = onSaleResp.Code
			}
			log.Printf("[双账号采样器] [账号%s] [生产者] 批次 %d: 查询失败 (err=%v, code=%d)\n",
				accountName, currentBatchNum, err, code)

			// 失败也要等待300秒再继续
			log.Printf("[双账号采样器] [账号%s] [生产者] 等待 %v 后处理下一批...\n", accountName, batchInterval)
			time.Sleep(batchInterval)
			continue
		}

		// 将结果放入队列
		batchProduced := 0
		for _, item := range onSaleResp.Data {
			if item.SaleTemplateResponse.TemplateId == 0 {
				continue
			}

			tid := item.SaleTemplateResponse.TemplateId
			good, exists := goodsMap[tid]
			if !exists {
				continue
			}

			sellPrice := 0.0
			if item.SaleCommodityResponse.MinSellPrice != "" {
				fmt.Sscanf(item.SaleCommodityResponse.MinSellPrice, "%f", &sellPrice)
			}

			queue.Push(GoodWithSellPrice{
				Good:      good,
				SellPrice: sellPrice,
				SellCount: item.SaleCommodityResponse.SellNum,
				Timestamp: time.Now(),
			})

			batchProduced++
		}

		producedTotal += int64(batchProduced)
		batchDuration := time.Since(batchStart)

		log.Printf("[双账号采样器] [账号%s] [生产者] 批次 %d: 成功生产 %d 个商品到队列 (耗时 %v)\n",
			accountName, currentBatchNum, batchProduced, batchDuration)
		log.Printf("[双账号采样器] [账号%s] [生产者] 累计生产: %d 个, 当前队列: %d 个\n",
			accountName, producedTotal, queue.Size())

		// 等待300秒后处理下一批
		log.Printf("[双账号采样器] [账号%s] [生产者] 等待 %v 后处理下一批...\n", accountName, batchInterval)

		// 倒计时显示
		for i := 300; i > 0; i -= 10 {
			select {
			case <-s.ctx.Done():
				return producedTotal
			case <-time.After(10 * time.Second):
				if i <= 30 || i%60 == 0 {
					log.Printf("[双账号采样器] [账号%s] [生产者] 还需等待 %d 秒... (队列: %d)\n",
						accountName, i-10, queue.Size())
				}
			}
		}

		queue.mu.Lock()
		queue.lastProduce = time.Now()
		queue.mu.Unlock()
	}
}

// runConsumers 消费者线程池：从队列获取商品并查询求购价
func (s *DualAccountSampler) runConsumers(
	accountName string,
	client *youpin.OpenAPIClient,
	queue *AccountQueue,
	totalGoods int,
) (int64, int64) {
	log.Printf("[双账号采样器] [账号%s] [消费者] 启动 %d 个消费者线程\n", accountName, s.numWorkers)

	var successCount int64
	var failureCount int64
	var processedCount int64
	var buyPriceFailCount int64
	var buyPriceZeroCount int64
	var dbSaveFailCount int64
	var emptyPollCount int64

	var wg sync.WaitGroup

	// 启动 numWorkers 个消费者线程
	for w := 0; w < s.numWorkers; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()

			// 每个线程独立的速率限制：250ms/请求
			rateLimiter := time.NewTicker(250 * time.Millisecond)
			defer rateLimiter.Stop()

			consecutiveEmpty := 0

			for {
				select {
				case <-s.ctx.Done():
					log.Printf("[双账号采样器] [账号%s] [消费者-%d] 收到停止信号\n", accountName, wid)
					return
				default:
				}

				// 等待速率限制
				<-rateLimiter.C

				// 从队列获取商品
				item, ok := queue.Pop()
				if !ok {
					// 队列为空
					consecutiveEmpty++
					atomic.AddInt64(&emptyPollCount, 1)

					// 如果连续10次为空，发送队列为空信号
					if consecutiveEmpty >= 10 {
						select {
						case queue.queueEmpty <- struct{}{}:
						default:
						}
						consecutiveEmpty = 0
					}

					// 消费者不退出，等待新的生产
					// 生产者是无限循环的，消费者也应该一直运行
					time.Sleep(1 * time.Second)
					continue
				}

				consecutiveEmpty = 0
				good := item.Good
				sellPrice := item.SellPrice
				sellCount := item.SellCount

				if good.YYYPTemplateID == nil {
					atomic.AddInt64(&failureCount, 1)
					atomic.AddInt64(&processedCount, 1)
					continue
				}

				tid := int(*good.YYYPTemplateID)

				// 查询求购价
				timeout := 10 * time.Second
				if s.proxyConfig != nil && s.proxyConfig.Timeout > 0 {
					timeout = s.proxyConfig.Timeout
				}
				ctx, cancel := context.WithTimeout(s.ctx, timeout)
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
					for _, poItem := range po.Data {
						if poItem.PurchasePrice > maxPrice {
							maxPrice = poItem.PurchasePrice
						}
					}
					if maxPrice == 0 {
						buyPriceStatus = "查询成功但无求购"
						atomic.AddInt64(&buyPriceZeroCount, 1)
					}
				} else {
					buyPriceStatus = "请求失败"
					atomic.AddInt64(&buyPriceFailCount, 1)
				}

				// 保存到数据库
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
					log.Printf("[双账号采样器] [账号%s] [消费者-%d] 商品ID %d 保存失败: %v (求购价状态: %s)\n",
						accountName, wid, good.GoodID, err, buyPriceStatus)
				} else {
					atomic.AddInt64(&successCount, 1)
					priceWarning := ""
					if sellPrice > 0 && maxPrice > sellPrice*1.5 {
						priceWarning = " [警告: 求购价异常高]"
					}

					// 所有线程都输出日志
					log.Printf("[双账号采样器] [账号%s] [消费者-%d] 商品ID %d 保存成功 (售价: %.2f, 求购: %.2f [%s])%s\n",
						accountName, wid, good.GoodID, sellPrice, maxPrice, buyPriceStatus, priceWarning)
				}

				atomic.AddInt64(&processedCount, 1)

				// 输出进度（只由worker-0负责）
				if wid == 0 {
					processed := atomic.LoadInt64(&processedCount)
					if processed%100 == 0 || processed == int64(totalGoods) {
						percentage := float64(processed) / float64(totalGoods) * 100
						queueSize := queue.Size()
						log.Printf("[双账号采样器] [账号%s] [消费者] [进度 %d/%d %.1f%%] 成功: %d, 失败: %d (求购失败: %d, 求购为0: %d, 保存失败: %d) 队列: %d\n",
							accountName, processed, totalGoods, percentage,
							atomic.LoadInt64(&successCount), atomic.LoadInt64(&failureCount),
							atomic.LoadInt64(&buyPriceFailCount), atomic.LoadInt64(&buyPriceZeroCount),
							atomic.LoadInt64(&dbSaveFailCount), queueSize)
					}
				}
			}
		}(w)
	}

	wg.Wait()
	log.Printf("[双账号采样器] [账号%s] [消费者] 所有消费者线程完成\n", accountName)

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
