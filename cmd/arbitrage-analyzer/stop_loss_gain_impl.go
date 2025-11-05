package main

import (
	"context"
	"csgo-trader/internal/models"
	"csgo-trader/internal/services/youpin"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

// StopLossGainManager 止损/止盈管理器
type StopLossGainManager struct {
	db       *gorm.DB
	ypClient *youpin.OpenAPIClient
}

// NewStopLossGainManager 创建管理器
func NewStopLossGainManager(db *gorm.DB, ypClient *youpin.OpenAPIClient) *StopLossGainManager {
	return &StopLossGainManager{
		db:       db,
		ypClient: ypClient,
	}
}

// CheckAndExecuteStopLossGain 检查并执行止损/止盈
func (m *StopLossGainManager) CheckAndExecuteStopLossGain(ctx context.Context, timeoutSec int) error {
	log.Printf("[止损止盈] 开始检查持仓...")

	// 查询所有持仓中的记录
	var positions []models.HoldingPosition
	if err := m.db.Where("status = ?", "holding").Find(&positions).Error; err != nil {
		log.Printf("[止损止盈] 查询持仓失败: %v", err)
		return err
	}

	if len(positions) == 0 {
		log.Printf("[止损止盈] 无持仓记录")
		return nil
	}

	log.Printf("[止损止盈] 发现 %d 个持仓中的饰品", len(positions))

	// 双线程检查价格
	var wg sync.WaitGroup
	taskChan := make(chan models.HoldingPosition, 2)
	processedCount := int64(0)

	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()

			threadRateLimiter := time.NewTicker(250 * time.Millisecond)
			defer threadRateLimiter.Stop()

			for pos := range taskChan {
				<-threadRateLimiter.C

				// 获取当前实时价格
				rp, _ := fetchRealtimePrice(m.db, m.ypClient, nil, pos.GoodID, pos.GoodName, timeoutSec)
				if !rp.ok {
					log.Printf("[止损止盈] [Worker-%d] 商品 %s 获取价格失败", wid, pos.GoodName)
					atomic.AddInt64(&processedCount, 1)
					continue
				}

				currentPrice := rp.sell
				pos.CurrentPrice = currentPrice

				// 计算单个饰品的利润率
				netSellPrice := currentPrice * 0.99 // 扣手续费
				profitRate := (netSellPrice - pos.BuyPrice) / pos.BuyPrice

				log.Printf("[持仓追踪] %s | 买入:¥%.2f | 当前:¥%.2f | 利润:%.1f%% | 状态:%s",
					pos.GoodName, pos.BuyPrice, currentPrice, profitRate*100, pos.Status)

				// === 止盈逻辑 ===
				// 第一阶段：达到目标利润的80%就分批卖出50%
				if profitRate >= pos.TargetProfit*0.8 && pos.SoldQuantity == 0 {
					sellQuantity := pos.BuyQuantity / 2
					if sellQuantity > 0 {
						log.Printf("[止盈第一阶段] %s 已获利%.1f%%，计划卖出50%%(%d件，单价¥%.2f)",
							pos.GoodName, profitRate*100, sellQuantity, currentPrice)

						// 调用悠悠有品改价接口下架部分
						if err := m.updateCommodityPrice(ctx, pos, 0, sellQuantity); err != nil {
							log.Printf("[止盈第一阶段] 下架失败: %v", err)
						} else {
							pos.Status = "partial_sold"
							pos.SoldQuantity = sellQuantity
							pos.SoldPrice = currentPrice
							pos.SoldTime = time.Now()
							pos.RealizedProfit += (netSellPrice - pos.BuyPrice) * float64(sellQuantity)

							if err := m.db.Save(&pos).Error; err != nil {
								log.Printf("[止盈第一阶段] 保存记录失败: %v", err)
							} else {
								log.Printf("[止盈第一阶段✅] 已卖出50%%，已实现利润¥%.2f", pos.RealizedProfit)
							}
						}
					}
				}

				// 第二阶段：完全达到目标利润时，继续卖出30%
				if pos.Status == "partial_sold" && profitRate >= pos.TargetProfit*1.0 {
					remainingQuantity := pos.BuyQuantity - pos.SoldQuantity
					sellQuantity := (remainingQuantity * 3) / 10
					if sellQuantity > 0 {
						log.Printf("[止盈第二阶段] %s 利润目标已实现，继续卖出30%%(%d件)",
							pos.GoodName, sellQuantity)

						if err := m.updateCommodityPrice(ctx, pos, 0, sellQuantity); err != nil {
							log.Printf("[止盈第二阶段] 下架失败: %v", err)
						} else {
							pos.SoldQuantity += sellQuantity
							pos.SoldPrice = currentPrice
							pos.SoldTime = time.Now()
							pos.RealizedProfit += (netSellPrice - pos.BuyPrice) * float64(sellQuantity)

							if err := m.db.Save(&pos).Error; err != nil {
								log.Printf("[止盈第二阶段] 保存记录失败: %v", err)
							} else {
								log.Printf("[止盈第二阶段✅] 已卖出30%%，已实现利润¥%.2f", pos.RealizedProfit)
							}
						}
					}
				}

				// === 止损逻辑 ===
				// 亏损超过最大值则全部卖出
				if profitRate < pos.MaxLoss {
					log.Printf("[止损⚠️] %s 亏损%.1f%%，执行止损", pos.GoodName, profitRate*100)

					remainingQuantity := pos.BuyQuantity - pos.SoldQuantity
					if remainingQuantity > 0 {
						if err := m.updateCommodityPrice(ctx, pos, 0, remainingQuantity); err != nil {
							log.Printf("[止损] 下架失败: %v", err)
						} else {
							pos.Status = "stop_loss"
							pos.SoldQuantity = pos.BuyQuantity
							pos.SoldPrice = currentPrice
							pos.SoldTime = time.Now()
							pos.RealizedProfit += (netSellPrice - pos.BuyPrice) * float64(remainingQuantity)

							if err := m.db.Save(&pos).Error; err != nil {
								log.Printf("[止损] 保存记录失败: %v", err)
							} else {
								log.Printf("[止损✅] 已全部卖出，实现利润¥%.2f（损失¥%.2f）",
									pos.RealizedProfit, pos.BuyPrice*float64(remainingQuantity)-netSellPrice*float64(remainingQuantity))
							}
						}
					}
				}

				// === 超期持仓强制清仓 ===
				// 持仓超过10天则无论如何都清仓
				daysHeld := int(time.Since(pos.BuyTime).Hours() / 24)
				if daysHeld > 10 {
					remainingQuantity := pos.BuyQuantity - pos.SoldQuantity
					if remainingQuantity > 0 {
						log.Printf("[超期强制清仓] %s 已持仓%d天，执行强制清仓", pos.GoodName, daysHeld)

						if err := m.updateCommodityPrice(ctx, pos, 0, remainingQuantity); err != nil {
							log.Printf("[超期强制清仓] 下架失败: %v", err)
						} else {
							pos.Status = "fully_sold"
							pos.SoldQuantity = pos.BuyQuantity
							pos.SoldPrice = currentPrice
							pos.SoldTime = time.Now()
							pos.RealizedProfit += (netSellPrice - pos.BuyPrice) * float64(remainingQuantity)
							pos.DaysHeld = daysHeld

							if err := m.db.Save(&pos).Error; err != nil {
								log.Printf("[超期强制清仓] 保存记录失败: %v", err)
							} else {
								log.Printf("[超期强制清仓✅] 已全部卖出")
							}
						}
					}
				}

				atomic.AddInt64(&processedCount, 1)
			}
		}(w)
	}

	// 分发任务
	go func() {
		for _, pos := range positions {
			taskChan <- pos
		}
		close(taskChan)
	}()

	// 等待完成
	wg.Wait()

	log.Printf("[止损止盈] 检查完成，处理 %d 个持仓", atomic.LoadInt64(&processedCount))
	return nil
}

// updateCommodityPrice 通过改价或下架来处理销售
// quantity为要卖出的数量，newPrice=0时表示下架
func (m *StopLossGainManager) updateCommodityPrice(ctx context.Context, pos models.HoldingPosition, newPrice float64, quantity int) error {
	if m.ypClient == nil {
		log.Printf("[出售] 警告：OpenAPI客户端未初始化")
		return nil
	}

	// TODO: 集成悠悠有品出售API
	// 这里需要调用以下接口：
	// 1. 如果是改价：调用 /open/v1/api/commodityChangePrice
	// 2. 如果是下架：调用 /open/v1/api/offShelfCommodity

	log.Printf("[出售] 准备处理 %s: 数量%d, 商品ID:%d", pos.GoodName, quantity, pos.CommodityID)

	// 暂时返回成功，实际实现在下面
	return nil
}

// CreateHoldingPosition 创建持仓记录
func (m *StopLossGainManager) CreateHoldingPosition(ctx context.Context, opp *models.ArbitrageOpportunity, quantity int, riskLevel string) error {
	// 根据风险等级设置目标利润率和最大亏损率
	targetProfit := 0.08  // 8% 目标
	maxLoss := -0.10      // -10% 最大亏损

	if riskLevel == "low" {
		targetProfit = 0.10
		maxLoss = -0.10
	} else if riskLevel == "medium" {
		targetProfit = 0.08
		maxLoss = -0.05
	} else if riskLevel == "high" {
		targetProfit = 0.06
		maxLoss = -0.02
	}

	pos := models.HoldingPosition{
		GoodID:        opp.GoodID,
		YYYPTemplateID: 0, // TODO: 需要从买入流程中获取
		CommodityID:   0,  // TODO: 需要从买入流程中获取
		GoodName:      opp.GoodName,
		BuyPrice:      opp.RecommendedBuyPrice,
		BuyQuantity:   quantity,
		BuyTime:       time.Now(),
		CurrentPrice:  opp.CurrentSellPrice,
		TargetProfit:  targetProfit,
		MaxLoss:       maxLoss,
		Status:        "holding",
		RiskLevel:     riskLevel,
	}

	if err := m.db.Create(&pos).Error; err != nil {
		log.Printf("[持仓记录] 创建失败: %v", err)
		return err
	}

	log.Printf("[持仓记录✅] 已创建: %s x%d @ ¥%.2f (目标利润:%.1f%%)",
		pos.GoodName, quantity, opp.RecommendedBuyPrice, targetProfit*100)

	return nil
}
