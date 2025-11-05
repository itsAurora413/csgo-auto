package main

import (
	"bytes"
	"context"
	"csgo-trader/internal/models"
	"csgo-trader/internal/services/youpin"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// YouPinSellClient 悠悠有品出售API客户端
type YouPinSellClient struct {
	openAPIClient *youpin.OpenAPIClient
	steamID       string
	appKey        string
	privateKey    string
}

// OffShelfCommodityRequest 下架请求
type OffShelfCommodityRequest struct {
	AppKey      string `json:"appKey"`
	Timestamp   string `json:"timestamp"`
	Sign        string `json:"sign"`
	CommodityIDs []int64 `json:"commodityIds"`
}

// ChangePriceRequest 改价请求
type ChangePriceRequest struct {
	AppKey      string                      `json:"appKey"`
	Timestamp   string                      `json:"timestamp"`
	Sign        string                      `json:"sign"`
	SteamID     string                      `json:"steamId"`
	RequestList []ChangePriceRequestItem    `json:"requestList"`
}

type ChangePriceRequestItem struct {
	CommodityID int64   `json:"commodityId"`
	Price       float64 `json:"price"`
}

// 注：APIResponse已在youpin_complete_flow.go中定义

// NewYouPinSellClient 创建出售客户端
func NewYouPinSellClient(openAPIClient *youpin.OpenAPIClient, steamID, appKey, privateKey string) *YouPinSellClient {
	return &YouPinSellClient{
		openAPIClient: openAPIClient,
		steamID:       steamID,
		appKey:        appKey,
		privateKey:    privateKey,
	}
}

// OffShelfCommodity 下架商品
func (c *YouPinSellClient) OffShelfCommodity(ctx context.Context, commodityIDs []int64) error {
	if len(commodityIDs) == 0 {
		return fmt.Errorf("商品ID列表为空")
	}

	// 生成时间戳和签名
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	// 构建请求
	req := OffShelfCommodityRequest{
		AppKey:      c.appKey,
		Timestamp:   timestamp,
		CommodityIDs: commodityIDs,
	}

	// 生成签名（这里需要使用与采样器相同的签名逻辑）
	// TODO: 从youpin服务中获取签名逻辑
	// req.Sign = c.generateSign(req, c.privateKey)

	// 调用API
	url := "https://gw-openapi.youpin898.com/open/v1/api/offShelfCommodity"

	jsonData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %v", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("创建HTTP请求失败: %v", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("下架请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %v", err)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("API错误: %s (code: %d)", apiResp.Msg, apiResp.Code)
	}

	log.Printf("[下架成功] 下架 %d 个商品", len(commodityIDs))
	return nil
}

// ChangeCommodityPrice 改价
func (c *YouPinSellClient) ChangeCommodityPrice(ctx context.Context, changes []ChangePriceRequestItem) error {
	if len(changes) == 0 {
		return fmt.Errorf("改价列表为空")
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")

	req := ChangePriceRequest{
		AppKey:      c.appKey,
		Timestamp:   timestamp,
		SteamID:     c.steamID,
		RequestList: changes,
	}

	// 生成签名
	// TODO: 从youpin服务中获取签名逻辑
	// req.Sign = c.generateSign(req, c.privateKey)

	url := "https://gw-openapi.youpin898.com/open/v1/api/commodityChangePrice"

	jsonData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %v", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("创建HTTP请求失败: %v", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("改价请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %v", err)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("API错误: %s (code: %d)", apiResp.Msg, apiResp.Code)
	}

	log.Printf("[改价成功] 已改价 %d 个商品", len(changes))
	return nil
}

// ExecuteSell 执行销售（改价或下架）
func (m *StopLossGainManager) updateCommodityPriceWithAPI(ctx context.Context, pos models.HoldingPosition, newPrice float64, quantity int) error {
	// 创建出售客户端
	// TODO: 从数据库获取真实的steamID、appKey、privateKey
	sellClient := NewYouPinSellClient(m.ypClient, "", "", "")

	if newPrice == 0 {
		// 下架
		commodityIDs := []int64{pos.CommodityID}
		if err := sellClient.OffShelfCommodity(ctx, commodityIDs); err != nil {
			log.Printf("[下架失败] %s: %v", pos.GoodName, err)
			return err
		}
		log.Printf("[下架成功] %s (数量:%d)", pos.GoodName, quantity)
	} else {
		// 改价
		changes := []ChangePriceRequestItem{
			{
				CommodityID: pos.CommodityID,
				Price:       newPrice,
			},
		}
		if err := sellClient.ChangeCommodityPrice(ctx, changes); err != nil {
			log.Printf("[改价失败] %s: %v", pos.GoodName, err)
			return err
		}
		log.Printf("[改价成功] %s 新价:¥%.2f", pos.GoodName, newPrice)
	}

	return nil
}
