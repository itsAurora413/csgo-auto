package youpin

import (
    "bytes"
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "log"
    "math"
    "math/rand"
    "net/http"
    "os"
    "os/exec"
    "strings"
    "time"
)

// Service 悠悠有品服务
type Service struct {
    client          *Client
    config          *YouPinConfig
    salePriceCache  map[string]*PriceCacheItem
    buyPriceCache   map[string]float64
    saleInventory   []YouPinOrder
    acceptSteamOfferFn func(context.Context, string) error
}

// PriceCacheItem 价格缓存项
type PriceCacheItem struct {
	CommodityName string    `json:"commodity_name"`
	SalePrice     float64   `json:"sale_price"`
	CacheTime     time.Time `json:"cache_time"`
}

// NewService 创建新的悠悠有品服务
func NewService(token string, config *YouPinConfig) (*Service, error) {
	client, err := NewClient(token)
	if err != nil {
		return nil, fmt.Errorf("创建悠悠有品客户端失败: %w", err)
	}

    return &Service{
        client:         client,
        config:         config,
        salePriceCache: make(map[string]*PriceCacheItem),
        buyPriceCache:  make(map[string]float64),
    }, nil
}

// SetSteamOfferAccepter 注入Steam报价接受实现
func (s *Service) SetSteamOfferAccepter(fn func(context.Context, string) error) {
    s.acceptSteamOfferFn = fn
}

// AutoSell 自动出售功能
func (s *Service) AutoSell(ctx context.Context) error {
	log.Println("悠悠有品出售自动上架插件已启动")

	// 发送设备信息
	if err := s.client.SendDeviceInfo(ctx); err != nil {
		log.Printf("发送设备信息失败: %v", err)
	}

	// 随机等待避免频繁访问
	s.operateSleep(5, 15)

	log.Println("正在获取悠悠有品库存...")
	inventory, err := s.client.GetInventory(ctx, true)
	if err != nil {
		return fmt.Errorf("获取库存失败: %w", err)
	}

	var saleItemList []YouPinSaleItem

	for _, item := range inventory {
		// 检查物品状态
		if !item.Tradable || item.AssetStatus != 0 {
			continue
		}

		// 检查物品名称过滤
		if !s.shouldSellItem(item.CommodityName) {
			continue
		}

		// 检查黑名单
		if s.isInBlacklist(item.CommodityName) {
			log.Printf("物品 %s 命中黑名单，将不会上架", item.CommodityName)
			continue
		}

		// 获取市场价格
		salePrice, err := s.getMarketSalePrice(ctx, item.TemplateID, item.CommodityName)
		if err != nil {
			log.Printf("获取 %s 的市场价格失败: %v，暂时跳过", item.CommodityName, err)
			continue
		}

		// 应用止盈率
		if s.config.TakeProfileEnabled {
			buyPrice := s.buyPriceCache[item.TemplateID]
			if buyPrice > 0 {
				takeProfilePrice := s.getTakeProfilePrice(buyPrice)
				salePrice = math.Max(salePrice, takeProfilePrice)
				log.Printf("按%.2f止盈率设置价格，最终出售价格%.2f", s.config.TakeProfileRatio, salePrice)
			}
		}

		if salePrice == 0 {
			continue
		}

		// 价格调整
		if s.config.UsePriceAdjustment && salePrice > s.config.PriceAdjustmentThreshold {
			salePrice = math.Max(s.config.PriceAdjustmentThreshold, salePrice-0.01)
			salePrice = math.Round(salePrice*100) / 100
		}

		// 检查最高价格限制
		if s.config.MaxSalePrice > 0 && salePrice > s.config.MaxSalePrice {
			log.Printf("物品 %s 的价格超过了设定的最高价格，将不会上架", item.CommodityName)
			continue
		}

		log.Printf("即将上架：%s 价格：%.2f", item.CommodityName, salePrice)

		saleItem := YouPinSaleItem{
			AssetID:   item.SteamAssetID,
			IsCanLease: false,
			IsCanSold:  true,
			Price:     salePrice,
			Remark:    "",
		}

		saleItemList = append(saleItemList, saleItem)
	}

	log.Printf("上架%d件物品中...", len(saleItemList))

	if len(saleItemList) > 0 {
		s.operateSleep(5, 15)
		err = s.client.SellItems(ctx, saleItemList)
		if err != nil {
			return fmt.Errorf("上架物品失败: %w", err)
		}
		log.Println("上架完成")
	}

	return nil
}

// AutoChangePrice 自动修改价格
func (s *Service) AutoChangePrice(ctx context.Context) error {
	log.Println("悠悠有品出售自动修改价格已启动")

	// 发送设备信息
	if err := s.client.SendDeviceInfo(ctx); err != nil {
		log.Printf("发送设备信息失败: %v", err)
	}

	s.operateSleep(5, 15)

	log.Println("正在获取悠悠有品出售已上架物品...")
	if err := s.getSaleInventory(ctx); err != nil {
		return fmt.Errorf("获取已上架物品失败: %w", err)
	}

	var newSaleItemList []YouPinCommodity

	for _, item := range s.saleInventory {
		// 检查物品名称过滤
		if !s.shouldSellItem(item.ItemName) {
			continue
		}

		// 检查黑名单
		if s.isInBlacklist(item.ItemName) {
			log.Printf("改价跳过：%s 命中黑名单", item.ItemName)
			continue
		}

		// 获取市场价格
		salePrice, err := s.getMarketSalePrice(ctx, "", item.ItemName) // 这里需要模板ID
		if err != nil {
			log.Printf("获取 %s 的市场价格失败: %v，暂时跳过", item.ItemName, err)
			continue
		}

		// 应用止盈率
		if s.config.TakeProfileEnabled {
			buyPrice := s.buyPriceCache[""] // 需要通过其他方式获取购买价格
			if buyPrice > 0 {
				takeProfilePrice := s.getTakeProfilePrice(buyPrice)
				salePrice = math.Max(salePrice, takeProfilePrice)
			}
		}

		if salePrice == 0 {
			continue
		}

		// 价格调整
		if s.config.UsePriceAdjustment && salePrice > s.config.PriceAdjustmentThreshold {
			salePrice = math.Max(s.config.PriceAdjustmentThreshold, salePrice-0.01)
			salePrice = math.Round(salePrice*100) / 100
		}

		commodity := YouPinCommodity{
			CommodityID: fmt.Sprintf("%d", item.ID),
			IsCanLease:  false,
			IsCanSold:   true,
			Price:       salePrice,
			Remark:      "",
		}

		newSaleItemList = append(newSaleItemList, commodity)
	}

	log.Printf("%d件物品可以更新出售价格", len(newSaleItemList))

	if len(newSaleItemList) > 0 {
		s.operateSleep(5, 15)
		err := s.client.ChangeSalePrice(ctx, newSaleItemList)
		if err != nil {
			return fmt.Errorf("修改价格失败: %w", err)
		}
		log.Println("价格修改完成")
	}

	return nil
}

// AutoAcceptOffer 自动接受报价
func (s *Service) AutoAcceptOffer(ctx context.Context) error {
    log.Println("悠悠有品自动发货监听已启动（包含Steam报价自动接受）")

    processed := make(map[string]struct{})
    pollInterval := 30 * time.Second

    for {
        select {
        case <-ctx.Done():
            return nil
        default:
        }

        waitDeliverList, err := s.client.GetWaitDeliverList(ctx)
        if err != nil {
            log.Printf("获取待发货列表失败: %v", err)
            time.Sleep(pollInterval)
            continue
        }

        if len(waitDeliverList) > 0 {
            log.Printf("待发货订单数: %d", len(waitDeliverList))
        }

        for i, item := range waitDeliverList {
            if item.OfferID == "" {
                log.Printf("订单[%s]无TradeOfferID（可能需要手动处理），跳过。", item.ItemName)
                continue
            }

            if _, seen := processed[item.OfferID]; seen {
                continue
            }

            log.Printf("准备自动接受Steam报价，商品: %s，OfferID: %s", item.ItemName, item.OfferID)

            // 优先使用注入的实现，否则使用本地HTTP/命令行方式
            var err error
            if s.acceptSteamOfferFn != nil {
                err = s.acceptSteamOfferFn(ctx, item.OfferID)
            } else {
                err = acceptSteamOffer(ctx, item.OfferID)
            }
            if err != nil {
                log.Printf("接受Steam报价失败(offerId=%s): %v", item.OfferID, err)
                continue
            }

            processed[item.OfferID] = struct{}{}
            log.Printf("已接受Steam报价(offerId=%s)，等待状态同步...", item.OfferID)
            time.Sleep(2 * time.Second)

            if i != len(waitDeliverList)-1 {
                log.Println("间隔等待以避免频繁访问Steam/YouPin...")
                time.Sleep(5 * time.Second)
            }
        }

        time.Sleep(pollInterval)
    }
}

// acceptSteamOfferViaCommand 通过外部命令接受Steam报价
// 使用环境变量 STEAM_ACCEPT_CMD 配置命令模板，例如：
//   STEAM_ACCEPT_CMD="/usr/local/bin/steam-accept-offer --offer {offer_id}"
// 命令返回码为0视为成功，其它视为失败
func acceptSteamOfferViaCommand(ctx context.Context, offerID string) error {
    cmdTpl := os.Getenv("STEAM_ACCEPT_CMD")
    if strings.TrimSpace(cmdTpl) == "" {
        return errors.New("未配置STEAM_ACCEPT_CMD，无法自动接受Steam报价")
    }
    // 简单模板替换
    cmdLine := strings.ReplaceAll(cmdTpl, "{offer_id}", offerID)

    // 以shell执行，便于支持管道/重定向与参数
    c := exec.CommandContext(ctx, "/bin/sh", "-c", cmdLine)
    c.Env = os.Environ()
    out, err := c.CombinedOutput()
    if err != nil {
        return fmt.Errorf("执行命令失败: %v, 输出: %s", err, string(out))
    }
    log.Printf("Steam报价接受命令执行成功: %s", strings.TrimSpace(string(out)))
    return nil
}

// acceptSteamOffer 支持两种方式：
// 1) 通过本地HTTP服务（环境变量 STEAM_ACCEPT_URL）
// 2) 通过本地命令行（环境变量 STEAM_ACCEPT_CMD）
// 若两者均未配置，则返回错误。
func acceptSteamOffer(ctx context.Context, offerID string) error {
    if url := strings.TrimSpace(os.Getenv("STEAM_ACCEPT_URL")); url != "" {
        payload := map[string]string{"offer_id": offerID}
        b, _ := json.Marshal(payload)
        req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
        if err != nil {
            return fmt.Errorf("创建HTTP请求失败: %w", err)
        }
        req.Header.Set("Content-Type", "application/json")
        resp, err := http.DefaultClient.Do(req)
        if err != nil {
            return fmt.Errorf("调用STEAM_ACCEPT_URL失败: %w", err)
        }
        defer resp.Body.Close()
        if resp.StatusCode < 200 || resp.StatusCode >= 300 {
            return fmt.Errorf("STEAM_ACCEPT_URL返回错误状态: %s", resp.Status)
        }
        return nil
    }

    // 回退到命令行方式
    return acceptSteamOfferViaCommand(ctx, offerID)
}

// getMarketSalePrice 获取市场销售价格
func (s *Service) getMarketSalePrice(ctx context.Context, templateID, itemName string) (float64, error) {
	// 检查缓存
	if cached, exists := s.salePriceCache[templateID]; exists {
		if time.Since(cached.CacheTime) <= 5*time.Minute {
			log.Printf("%s 使用缓存结果，出售价格：%.2f", cached.CommodityName, cached.SalePrice)
			return cached.SalePrice, nil
		}
	}

	if templateID == "" {
		return 0, fmt.Errorf("模板ID为空")
	}

	prices, err := s.client.GetMarketSalePrice(ctx, templateID)
	if err != nil {
		return 0, err
	}

	if len(prices) == 0 {
		log.Printf("市场上没有指定筛选条件的物品")
		return 0, nil
	}

	var salePrice float64
	var commodityName string

	if len(prices) > 0 {
		commodityName = prices[0].CommodityName

		var priceList []float64
		count := int(math.Min(10, float64(len(prices))))

		for i := 0; i < count; i++ {
			if prices[i].Price > 0 {
				priceList = append(priceList, prices[i].Price)
			}
		}

		if len(priceList) == 1 {
			salePrice = priceList[0]
		} else if len(priceList) > 1 {
			// 排序价格列表
			minPrice := math.Min(priceList[0], priceList[1])
			if priceList[1] < minPrice*1.05 {
				salePrice = minPrice
			} else {
				salePrice = priceList[1]
			}
		}

		log.Printf("物品名称：%s，出售价格：%.2f", commodityName, salePrice)
	}

	salePrice = math.Round(salePrice*100) / 100

	// 缓存结果
	if salePrice != 0 {
		s.salePriceCache[templateID] = &PriceCacheItem{
			CommodityName: commodityName,
			SalePrice:     salePrice,
			CacheTime:     time.Now(),
		}
	}

	return salePrice, nil
}

// getSaleInventory 获取已上架物品
func (s *Service) getSaleInventory(ctx context.Context) error {
	inventory, err := s.client.GetSellList(ctx)
	if err != nil {
		return err
	}

	s.saleInventory = inventory
	log.Printf("已上架物品数量 %d", len(inventory))
	return nil
}

// shouldSellItem 检查是否应该出售物品
func (s *Service) shouldSellItem(itemName string) bool {
	if len(s.config.SellItemNames) == 0 {
		return true
	}

	for _, name := range s.config.SellItemNames {
		if name != "" && strings.Contains(itemName, name) {
			return true
		}
	}
	return false
}

// isInBlacklist 检查是否在黑名单中
func (s *Service) isInBlacklist(itemName string) bool {
	for _, word := range s.config.BlacklistWords {
		if word != "" && strings.Contains(itemName, word) {
			return true
		}
	}
	return false
}

// getTakeProfilePrice 计算止盈价格
func (s *Service) getTakeProfilePrice(buyPrice float64) float64 {
	return buyPrice * (1 + s.config.TakeProfileRatio)
}

// operateSleep 操作间隔等待
func (s *Service) operateSleep(min, max int) {
	if min == 0 && max == 0 {
		min, max = 5, 15
	}
	sleepTime := rand.Intn(max-min+1) + min
	log.Printf("为了避免频繁访问接口，操作间隔 %d 秒", sleepTime)
	time.Sleep(time.Duration(sleepTime) * time.Second)
}

// StartAutoSell 启动自动出售任务
func (s *Service) StartAutoSell(ctx context.Context) error {
	log.Printf("以下物品会出售：%v", s.config.SellItemNames)

	// 执行自动出售
	if err := s.AutoSell(ctx); err != nil {
		return err
	}

	// 如果配置了定时任务，这里可以添加定时器
	if s.config.RunTime != "" {
		log.Printf("[自动出售] 等待到 %s 开始执行", s.config.RunTime)
	}

	if s.config.Interval > 0 {
		log.Printf("[自动修改价格] 每隔 %d 分钟执行一次", s.config.Interval)
	}

	return nil
}

// GetUserInfo 获取用户信息
func (s *Service) GetUserInfo() string {
	return s.client.GetUserNickname()
}
