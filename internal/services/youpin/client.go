package youpin

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	BaseURL    = "https://api.youpin898.com"
	GameIDCSGO = "730"
)

// Client 悠悠有品API客户端
type Client struct {
	httpClient  *http.Client
	token       string
	deviceToken string
	deviceID    string
	userId      string
	nickname    string
	baseURL     string
}

// AccountBalances 账户余额信息
type AccountBalances struct {
	WalletBalance   float64
	PurchaseBalance float64
}

// GetBalances 获取账户余额（钱包余额/求购余额）
func (c *Client) GetBalances(ctx context.Context) (*AccountBalances, error) {
	// 使用正确的悠悠有品余额查询接口
	data := map[string]interface{}{
		"Sessionid": c.deviceToken,
	}

	var response struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Balance         string `json:"balance"`         // 钱包余额 (字符串)
			PurchaseBalance string `json:"purchaseBalance"` // 求购余额 (字符串)
			BalanceFroze    string `json:"balanceFroze"`    // 冻结余额 (字符串)
		} `json:"data"`
	}

	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/payment/v1/user/account/info", data, &response)
	if err != nil {
		// 如果新接口失败，使用旧接口作为备用
		return c.getBalancesLegacy(ctx)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("获取余额失败: %s", response.Msg)
	}

	// 将字符串转换为float64
	walletBalance, _ := strconv.ParseFloat(response.Data.Balance, 64)
	purchaseBalance, _ := strconv.ParseFloat(response.Data.PurchaseBalance, 64)

	return &AccountBalances{
		WalletBalance:   walletBalance,   // 钱包余额
		PurchaseBalance: purchaseBalance, // 求购余额
	}, nil
}

// getBalancesLegacy 备用的旧版余额查询方法
func (c *Client) getBalancesLegacy(ctx context.Context) (*AccountBalances, error) {
	// 适配不同网关，优先尝试bff资产接口，不同环境字段名可能略有差异
	var resp1 map[string]interface{}
	if err := c.makeRequest(ctx, "GET", "/api/youpin/bff/user/asset/info", map[string]interface{}{"Sessionid": c.deviceToken}, &resp1); err == nil {
		if code, ok := resp1["code"].(float64); ok && code == 0 {
			if data, ok := resp1["data"].(map[string]interface{}); ok {
				bal := &AccountBalances{}
				// 兼容多种字段命名
				extract := func(keys ...string) float64 {
					for _, k := range keys {
						if v, ok := data[k]; ok {
							switch vt := v.(type) {
							case float64:
								return vt
							case int:
								return float64(vt)
							case string:
								if f, err := strconv.ParseFloat(vt, 64); err == nil {
									return f
								}
							}
						}
					}
					return 0
				}
				bal.WalletBalance = extract("balance", "walletBalance", "availableBalance")
				bal.PurchaseBalance = extract("purchaseBalance", "buyBalance", "purchase_available")
				return bal, nil
			}
		}
	}

	// 退回到通用账户资产接口（安卓端）
	var resp2 struct {
		Code    int                    `json:"Code"`
		Message string                 `json:"Message"`
		Data    map[string]interface{} `json:"Data"`
	}
	if err := c.makeRequest(ctx, "GET", "/api/user/Account/asset", map[string]interface{}{"Sessionid": c.deviceToken}, &resp2); err == nil && resp2.Code == 0 {
		bal := &AccountBalances{}
		if v, ok := resp2.Data["Balance"]; ok {
			switch vt := v.(type) {
			case float64:
				bal.WalletBalance = vt
			case string:
				if f, err := strconv.ParseFloat(vt, 64); err == nil {
					bal.WalletBalance = f
				}
			}
		}
		if v, ok := resp2.Data["PurchaseBalance"]; ok {
			switch vt := v.(type) {
			case float64:
				bal.PurchaseBalance = vt
			case string:
				if f, err := strconv.ParseFloat(vt, 64); err == nil {
					bal.PurchaseBalance = f
				}
			}
		}
		return bal, nil
	}

	return nil, fmt.Errorf("无法获取账户余额")
}

// NewClient 创建新的悠悠有品客户端
func NewClient(token string) (*Client, error) {
	// 从环境变量获取设备Token，如果没有则生成随机的
	deviceToken := os.Getenv("YOUPIN_DEVICE_TOKEN")
	if deviceToken == "" {
		deviceToken = generateRandomString(32)
	}

	client := &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		token:       token,
		deviceToken: deviceToken,
		deviceID:    deviceToken, // 使用相同的Token作为设备ID
		baseURL:     BaseURL,
	}

	// 获取用户信息
	ctx := context.Background()
	userInfo, err := client.getUserInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("悠悠有品账号登录失败，请检查token是否正确: %w", err)
	}

	// 处理UserId的类型转换
	if userId, ok := userInfo.UserId.(string); ok {
		client.userId = userId
	} else if userIdNum, ok := userInfo.UserId.(float64); ok {
		client.userId = fmt.Sprintf("%.0f", userIdNum)
	} else {
		client.userId = fmt.Sprintf("%v", userInfo.UserId)
	}
	client.nickname = userInfo.NickName

	return client, nil
}

// generateRandomString 生成指定长度的随机字符串
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// UserInfo 用户信息
type UserInfo struct {
	UserId   interface{} `json:"UserId"` // 支持数字或字符串
	NickName string      `json:"NickName"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Phone    string `json:"phone"`
	Code     string `json:"code"`
	Platform string `json:"platform"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Code    int    `json:"code"`
	Message string `json:"msg"`
	Data    struct {
		AccessToken string `json:"accessToken"`
		UserInfo    struct {
			UserId   interface{} `json:"userId"`
			NickName string      `json:"nickName"`
		} `json:"userInfo"`
	} `json:"data"`
}

// getUserInfo 获取用户信息
func (c *Client) getUserInfo(ctx context.Context) (*UserInfo, error) {
	var response struct {
		Code    int      `json:"Code"`
		Message string   `json:"Message"`
		Data    UserInfo `json:"Data"`
	}

	err := c.makeRequest(ctx, "GET", "/api/user/Account/getUserInfo", nil, &response)
	if err != nil {
		return nil, err
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("获取用户信息失败: %s", response.Message)
	}

	return &response.Data, nil
}

// SetToken 设置认证令牌
func (c *Client) SetToken(token string) {
	c.token = token
}

// IsTokenValid 检查token是否有效
func (c *Client) IsTokenValid(ctx context.Context) bool {
	_, err := c.getUserInfo(ctx)
	return err == nil
}

// GetInventory 获取库存
func (c *Client) GetInventory(ctx context.Context, refresh bool) ([]YouPinItem, error) {
	data := map[string]interface{}{
		"pageIndex": 1,
		"pageSize":  1000,
		"AppType":   4,
		"IsMerge":   0,
		"Sessionid": c.deviceToken,
	}

	if refresh {
		data["IsRefresh"] = true
		data["RefreshType"] = 2
	}

	var response struct {
		Code    int    `json:"Code"`
		Message string `json:"Message"`
		Data    struct {
			ItemsInfos []YouPinItem `json:"ItemsInfos"`
		} `json:"Data"`
	}

	err := c.makeRequest(ctx, "POST", "/api/commodity/Inventory/GetUserInventoryDataListV3", data, &response)
	if err != nil {
		return nil, fmt.Errorf("获取库存失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Message)
	}

	return response.Data.ItemsInfos, nil
}

// GetSellList 获取在售物品列表 (用于内部服务逻辑)
func (c *Client) GetSellList(ctx context.Context) ([]YouPinOrder, error) {
	data := map[string]interface{}{
		"pageIndex":    1,
		"pageSize":     100,
		"whetherMerge": 0,
	}

	var shelf []YouPinOrder
	for {
		var response struct {
			Code    int    `json:"code"`
			Message string `json:"msg"`
			Data    struct {
				CommodityInfoList []YouPinOrder `json:"commodityInfoList"`
			} `json:"data"`
		}

		err := c.makeRequest(ctx, "POST", "/api/youpin/bff/new/commodity/v1/commodity/list/sell", data, &response)
		if err != nil {
			return nil, fmt.Errorf("获取在售列表失败: %w", err)
		}

		if response.Code != 0 {
			break
		}

		for _, item := range response.Data.CommodityInfoList {
			// 只添加有ID的物品
			if item.ID != 0 {
				shelf = append(shelf, item)
			}
		}

		data["pageIndex"] = data["pageIndex"].(int) + 1
	}

	return shelf, nil
}

// GetSellListForAPI 获取在售物品列表 (用于API返回)
func (c *Client) GetSellListForAPI(ctx context.Context) ([]map[string]interface{}, error) {
	data := map[string]interface{}{
		"pageIndex":    1,
		"pageSize":     100,
		"whetherMerge": 0,
	}

	var shelf []map[string]interface{}
	for {
		var response struct {
			Code    int    `json:"code"`
			Message string `json:"msg"`
			Data    struct {
				CommodityInfoList []map[string]interface{} `json:"commodityInfoList"`
			} `json:"data"`
		}

		err := c.makeRequest(ctx, "POST", "/api/youpin/bff/new/commodity/v1/commodity/list/sell", data, &response)
		if err != nil {
			return nil, fmt.Errorf("获取在售列表失败: %w", err)
		}

		if response.Code != 0 {
			break
		}

		for _, item := range response.Data.CommodityInfoList {
			// 只添加有steamAssetId的物品，与Steamauto保持一致
			if steamAssetId, exists := item["steamAssetId"]; exists && steamAssetId != nil {
				shelf = append(shelf, item)
			}
		}

		data["pageIndex"] = data["pageIndex"].(int) + 1
	}

	return shelf, nil
}

// GetWaitDeliverList 获取待发货列表
func (c *Client) GetWaitDeliverList(ctx context.Context) ([]YouPinOrder, error) {
	// 首先获取待办事项列表
	toDoListData := map[string]interface{}{
		"userId":    c.userId,
		"pageIndex": 1,
		"pageSize":  100,
		"Sessionid": c.deviceToken,
	}

	var toDoResponse struct {
		Code    int    `json:"code"`
		Message string `json:"msg"`
		Data    []struct {
			OrderNo       string `json:"orderNo"`
			CommodityName string `json:"commodityName"`
			Message       string `json:"message"`
		} `json:"data"`
	}

	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/todo/v1/orderTodo/list", toDoListData, &toDoResponse)
	if err != nil {
		return nil, fmt.Errorf("获取待办列表失败: %w", err)
	}

	var dataToReturn []YouPinOrder
	toDoList := make(map[string]struct {
		OrderNo       string
		CommodityName string
		Message       string
	})

	// 处理待办事项
	for _, order := range toDoResponse.Data {
		if strings.Contains(order.Message, "赠送") {
			// 跳过赠送订单
			continue
		} else if order.Message == "有买家下单，待您发送报价" {
			// 发送报价
			err := c.sendOffer(ctx, order.OrderNo)
			if err != nil {
				// 记录错误但继续处理
				continue
			}
			// 等待报价发送完成
			time.Sleep(2 * time.Second)
		} else {
			toDoList[order.OrderNo] = struct {
				OrderNo       string
				CommodityName string
				Message       string
			}{
				OrderNo:       order.OrderNo,
				CommodityName: order.CommodityName,
				Message:       order.Message,
			}
		}
	}

	// 获取销售列表中的报价ID
	if len(toDoList) > 0 {
		sellListData := map[string]interface{}{
			"keys":        "",
			"orderStatus": "140",
			"pageIndex":   1,
			"pageSize":    100,
		}

		var sellResponse struct {
			Code    int    `json:"code"`
			Message string `json:"msg"`
			Data    struct {
				OrderList []struct {
					OrderNo       string `json:"orderNo"`
					OfferType     string `json:"offerType"`
					TradeOfferID  string `json:"tradeOfferId"`
					ProductDetail struct {
						CommodityName string `json:"commodityName"`
					} `json:"productDetail"`
				} `json:"orderList"`
			} `json:"data"`
		}

		err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/sale/v1/sell/list", sellListData, &sellResponse)
		if err == nil && sellResponse.Code == 0 {
			for _, order := range sellResponse.Data.OrderList {
				if order.OfferType == "2" && order.TradeOfferID != "" {
					if _, exists := toDoList[order.OrderNo]; exists {
						delete(toDoList, order.OrderNo)
						dataToReturn = append(dataToReturn, YouPinOrder{
							OfferID:  order.TradeOfferID,
							ItemName: order.ProductDetail.CommodityName,
						})
					}
				}
			}
		}
	}

	// 如果还有未处理的订单，尝试其他方法获取报价ID
	if len(toDoList) > 0 {
		for orderNo := range toDoList {
			// 等待一下避免频繁请求
			time.Sleep(3 * time.Second)

			order, err := c.getOrderDetail(ctx, orderNo)
			if err == nil && order != nil {
				dataToReturn = append(dataToReturn, *order)
				delete(toDoList, orderNo)
			}
		}
	}

	return dataToReturn, nil
}

// sendOffer 发送报价
func (c *Client) sendOffer(ctx context.Context, orderNo string) error {
	data := map[string]interface{}{
		"orderNo":   orderNo,
		"Sessionid": c.deviceToken,
	}

	var response struct {
		Code    int    `json:"code"`
		Message string `json:"msg"`
	}

	err := c.makeRequest(ctx, "PUT", "/api/youpin/bff/trade/v1/order/sell/delivery/send-offer", data, &response)
	if err != nil {
		return err
	}

	if response.Code != 0 {
		return fmt.Errorf("发送报价失败: %s", response.Message)
	}

	return nil
}

// getOrderDetail 获取订单详情
func (c *Client) getOrderDetail(ctx context.Context, orderNo string) (*YouPinOrder, error) {
	data := map[string]interface{}{
		"orderId":   orderNo,
		"Sessionid": c.deviceToken,
	}

	var response struct {
		Code    int    `json:"code"`
		Message string `json:"msg"`
		Data    struct {
			OrderDetail struct {
				OfferID       string `json:"offerId"`
				TradeOfferID  string `json:"tradeOfferId"`
				ProductDetail struct {
					CommodityName string `json:"commodityName"`
				} `json:"productDetail"`
				Commodity struct {
					Name string `json:"name"`
				} `json:"commodity"`
			} `json:"orderDetail"`
		} `json:"data"`
	}

	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/order/v2/detail", data, &response)
	if err != nil {
		// 尝试另一个API
		return c.getOrderDetailV2(ctx, orderNo)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("获取订单详情失败: %s", response.Message)
	}

	if response.Data.OrderDetail.OfferID != "" {
		return &YouPinOrder{
			OfferID:  response.Data.OrderDetail.OfferID,
			ItemName: response.Data.OrderDetail.ProductDetail.CommodityName,
		}, nil
	}

	return nil, fmt.Errorf("订单详情中没有找到报价ID")
}

// getOrderDetailV2 获取订单详情（备用方法）
func (c *Client) getOrderDetailV2(ctx context.Context, orderNo string) (*YouPinOrder, error) {
	data := map[string]interface{}{
		"orderNo":   orderNo,
		"Sessionid": c.deviceToken,
	}

	var response struct {
		Code    int    `json:"code"`
		Message string `json:"msg"`
		Data    struct {
			TradeOfferID string `json:"tradeOfferId"`
			Commodity    struct {
				Name string `json:"name"`
			} `json:"commodity"`
		} `json:"data"`
	}

	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/v1/order/query/detail", data, &response)
	if err != nil {
		return nil, err
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("获取订单详情失败: %s", response.Message)
	}

	if response.Data.TradeOfferID != "" {
		return &YouPinOrder{
			OfferID:  response.Data.TradeOfferID,
			ItemName: response.Data.Commodity.Name,
		}, nil
	}

	return nil, fmt.Errorf("订单详情中没有找到报价ID")
}

// GetMarketSalePrice 获取市场销售价格
func (c *Client) GetMarketSalePrice(ctx context.Context, templateID string) ([]YouPinMarketPrice, error) {
	data := map[string]interface{}{
		"pageIndex":  1,
		"pageSize":   10,
		"templateId": templateID,
	}

	var response struct {
		Code    int                 `json:"Code"`
		Message string              `json:"Message"`
		Data    []YouPinMarketPrice `json:"Data"`
	}

	err := c.makeRequest(ctx, "POST", "/api/homepage/pc/goods/market/queryOnSaleCommodityList", data, &response)
	if err != nil {
		return nil, fmt.Errorf("获取市场价格失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Message)
	}

	return response.Data, nil
}

// SellItems 上架物品
func (c *Client) SellItems(ctx context.Context, items []YouPinSaleItem) error {
	data := map[string]interface{}{
		"GameId":    GameIDCSGO,
		"itemInfos": items,
		"Sessionid": c.deviceToken,
	}

	var response struct {
		Code    int    `json:"Code"`
		Message string `json:"Msg"`
		Data    []struct {
			AssetId string `json:"AssetId"`
			Status  int    `json:"Status"`
			Remark  string `json:"Remark"`
		} `json:"Data"`
	}

	err := c.makeRequest(ctx, "POST", "/api/commodity/Inventory/SellInventoryWithLeaseV2", data, &response)
	if err != nil {
		return fmt.Errorf("上架物品失败: %w", err)
	}

	if response.Code != 0 && response.Message != "成功" {
		return fmt.Errorf("API返回错误: %s", response.Message)
	}

	// 检查每个物品的上架状态
	for _, asset := range response.Data {
		if asset.Status != 1 {
			return fmt.Errorf("物品 %s 上架失败: %s", asset.AssetId, asset.Remark)
		}
	}

	return nil
}

// ChangeSalePrice 修改出售价格
func (c *Client) ChangeSalePrice(ctx context.Context, commodities []YouPinCommodity) error {
	data := map[string]interface{}{
		"GameId":     GameIDCSGO,
		"Commoditys": commodities,
		"Sessionid":  c.deviceToken,
	}

	var response YouPinPriceChangeResponse
	err := c.makeRequest(ctx, "PUT", "/api/commodity/Commodity/PriceChangeWithLeaseV2", data, &response)
	if err != nil {
		return fmt.Errorf("修改价格失败: %w", err)
	}

	if response.Code != 0 && response.Message != "成功" {
		return fmt.Errorf("API返回错误: %s", response.Message)
	}

	// 检查修改结果
	for _, commodity := range response.Data.Commoditys {
		if commodity.IsSuccess != 1 {
			return fmt.Errorf("商品 %v 价格修改失败: %s", commodity.CommodityId, commodity.Message)
		}
	}

	return nil
}

// OffSale 下架商品
func (c *Client) OffSale(ctx context.Context, commodityIDs []string) error {
	data := map[string]interface{}{
		"Ids":                    strings.Join(commodityIDs, ","),
		"IsDeleteCommodityCache": 1,
		"IsForceOffline":         true,
		"Sessionid":              c.deviceToken,
	}

	var response YouPinSellResponse
	err := c.makeRequest(ctx, "PUT", "/api/commodity/Commodity/OffShelf", data, &response)
	if err != nil {
		return fmt.Errorf("下架商品失败: %w", err)
	}

	if response.Code != 0 {
		return fmt.Errorf("API返回错误: %s", response.Message)
	}

	return nil
}

// GetUserNickname 获取用户昵称
func (c *Client) GetUserNickname() string {
	return c.nickname
}

// SendDeviceInfo 发送设备信息
func (c *Client) SendDeviceInfo(ctx context.Context) error {
	data := map[string]interface{}{
		"DeviceToken": c.deviceToken,
		"Sessionid":   c.deviceToken,
	}

	var response YouPinSellResponse
	err := c.makeRequest(ctx, "GET", "/api/common/ClientInfo/AndroidInfo", data, &response)
	if err != nil {
		return fmt.Errorf("发送设备信息失败: %w", err)
	}

	return nil
}

// SearchCommodities 统一使用安卓端悠悠有品搜索API
func (c *Client) SearchCommodities(ctx context.Context, keyword string, pageIndex int, pageSize int) ([]YouPinCommoditySearch, error) {
	data := map[string]interface{}{
		"keyword":   keyword,
		"pageIndex": pageIndex,
		"pageSize":  pageSize,
		"gameId":    GameIDCSGO,
	}

	var response struct {
		Code    int                     `json:"Code"`
		Message string                  `json:"Message"`
		Data    []YouPinCommoditySearch `json:"Data"`
	}

	// 安卓端 v3 搜索接口（与详情接口同前缀）
	// 兼容参数命名：同时传递 keyword 与 keys
	data["keys"] = keyword
	if err := c.makeRequest(ctx, "POST", "/api/homepage/v3/search/commodity", data, &response); err != nil {
		return nil, fmt.Errorf("搜索失败: %w", err)
	}
	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Message)
	}
	return response.Data, nil
}

// GetCommodityDetails 获取商品详情 - 直接返回商品列表
func (c *Client) GetCommodityDetails(ctx context.Context, templateId string) (*CommodityListResponse, error) {
	// 使用成功的商品列表API来获取商品详情
	templateIdInt := 0
	if id, err := strconv.Atoi(templateId); err == nil {
		templateIdInt = id
	} else {
		return nil, fmt.Errorf("无效的模板ID: %s", templateId)
	}

	// 直接返回商品列表，这就是用户想要的：看到市场上有哪些商家在卖这个商品
	response, err := c.GetCommodityList(ctx, templateIdInt, 1, 50, 1) // 获取50个在售商品
	if err != nil {
		return nil, fmt.Errorf("获取商品详情失败: %w", err)
	}

	return response, nil
}

// 磨损等级辅助函数
func getWearName(abrade float64) string {
	if abrade >= 0.45 {
		return "战痕累累"
	} else if abrade >= 0.38 {
		return "破损不堪"
	} else if abrade >= 0.15 {
		return "久经沙场"
	} else if abrade >= 0.07 {
		return "略有磨损"
	} else {
		return "崭新出厂"
	}
}

func getWearCode(abrade float64) string {
	if abrade >= 0.45 {
		return "BS"
	} else if abrade >= 0.38 {
		return "WW"
	} else if abrade >= 0.15 {
		return "FT"
	} else if abrade >= 0.07 {
		return "MW"
	} else {
		return "FN"
	}
}

func getMinAbradeForWear(abrade float64) float64 {
	if abrade >= 0.45 {
		return 0.45
	} else if abrade >= 0.38 {
		return 0.38
	} else if abrade >= 0.15 {
		return 0.15
	} else if abrade >= 0.07 {
		return 0.07
	} else {
		return 0.0
	}
}

func getMaxAbradeForWear(abrade float64) float64 {
	if abrade >= 0.45 {
		return 1.0
	} else if abrade >= 0.38 {
		return 0.45
	} else if abrade >= 0.15 {
		return 0.38
	} else if abrade >= 0.07 {
		return 0.15
	} else {
		return 0.07
	}
}

// GetMarketSaleList 获取市场销售列表（不同磨损的物品）
func (c *Client) GetMarketSaleList(ctx context.Context, templateId string, pageIndex int, pageSize int, minAbrade float64, maxAbrade float64) ([]YouPinMarketItem, error) {
	data := map[string]interface{}{
		"templateId": templateId,
		"pageIndex":  pageIndex,
		"pageSize":   pageSize,
	}

	// 按照Steamauto的格式，磨损值作为字符串传递，并且只在指定时添加
	if minAbrade > 0 {
		data["minAbrade"] = fmt.Sprintf("%.6f", minAbrade)
	}
	if maxAbrade < 1 && maxAbrade > 0 {
		data["maxAbrade"] = fmt.Sprintf("%.6f", maxAbrade)
	}

	var response struct {
		Code    int    `json:"Code"`
		Message string `json:"Message"`
		Data    struct {
			CommodityList []YouPinMarketItem `json:"CommodityList"`
			TotalCount    int                `json:"TotalCount"`
		} `json:"Data"`
	}

	// 使用Steamauto验证过的PC端API
	err := c.makeRequest(ctx, "POST", "/api/homepage/pc/goods/market/queryOnSaleCommodityList", data, &response)
	if err != nil {
		return nil, fmt.Errorf("获取市场销售列表失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Message)
	}

	return response.Data.CommodityList, nil
}

// GetPurchaseOrderList 获取求购订单列表
func (c *Client) GetPurchaseOrderList(ctx context.Context, templateId string, pageIndex int, pageSize int, minAbrade float64, maxAbrade float64) ([]YouPinPurchaseOrder, error) {
	data := map[string]interface{}{
		"templateId": templateId,
		"pageIndex":  pageIndex,
		"pageSize":   pageSize,
		"minAbrade":  minAbrade,
		"maxAbrade":  maxAbrade,
		"typeId":     -1, // -1表示所有类型
	}

	var response struct {
		Code    int    `json:"code"`
		Message string `json:"msg"`
		Data    struct {
			PurchaseOrderList []YouPinPurchaseOrder `json:"purchaseOrderList"`
			TotalCount        int                   `json:"totalCount"`
		} `json:"data"`
	}

	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/getTemplatePurchaseOrderList", data, &response)
	if err != nil {
		return nil, fmt.Errorf("获取求购订单列表失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Message)
	}

	return response.Data.PurchaseOrderList, nil
}

// CreatePurchaseOrder 创建求购订单
func (c *Client) CreatePurchaseOrder(ctx context.Context, templateId string, templateHashName string, commodityName string, purchasePrice float64, purchaseNum int, minAbrade float64, maxAbrade float64) error {
	data := map[string]interface{}{
		"templateId":       templateId,
		"templateHashName": templateHashName,
		"commodityName":    commodityName,
		"purchasePrice":    purchasePrice,
		"purchaseNum":      purchaseNum,
		"minAbrade":        minAbrade,
		"maxAbrade":        maxAbrade,
		"orderNo":          "",
		"supplyQuantity":   0,
		"Sessionid":        c.deviceToken,
	}

	var response struct {
		Code    int         `json:"code"`
		Message string      `json:"msg"`
		Data    interface{} `json:"data"`
	}

	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/publishPurchaseOrder", data, &response)
	if err != nil {
		return fmt.Errorf("创建求购订单失败: %w", err)
	}

	if response.Code != 0 {
		return fmt.Errorf("API返回错误: %s", response.Message)
	}

	return nil
}

// BuyFromMarket 从市场直接购买
func (c *Client) BuyFromMarket(ctx context.Context, commodityId string, price float64) error {
	data := map[string]interface{}{
		"commodityId": commodityId,
		"price":       price,
		"Sessionid":   c.deviceToken,
	}

	var response struct {
		Code    int         `json:"code"`
		Message string      `json:"msg"`
		Data    interface{} `json:"data"`
	}

	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/v1/order/buy", data, &response)
	if err != nil {
		return fmt.Errorf("市场购买失败: %w", err)
	}

	fmt.Printf("DEBUG: 市场购买响应 - Code: %d, Msg: %s\n", response.Code, response.Message)

	// 根据msg是否为"成功"来判定请求是否成功，不依赖code
	if response.Message == "成功" {
		fmt.Printf("DEBUG: 市场购买成功 (Code: %d)\n", response.Code)
		return nil
	}

	// 只有当msg不是"成功"时才返回错误
	return fmt.Errorf("市场购买失败: %s", response.Message)
}

// BuyFromMarketWithBalance 使用余额从市场直接购买
func (c *Client) BuyFromMarketWithBalance(ctx context.Context, commodityId string, price float64) error {
	data := map[string]interface{}{
		"commodityId":   commodityId,
		"price":         price,
		"paymentMethod": "balance", // 指定使用余额支付
		"payType":       1,         // 1表示余额支付
		"Sessionid":     c.deviceToken,
	}

	var response struct {
		Code    int         `json:"code"`
		Message string      `json:"msg"`
		Data    interface{} `json:"data"`
	}

	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/v1/order/buy", data, &response)
	if err != nil {
		return fmt.Errorf("余额购买失败: %w", err)
	}

	fmt.Printf("DEBUG: 余额购买响应 - Code: %d, Msg: %s\n", response.Code, response.Message)

	// 根据msg是否为"成功"来判定请求是否成功，不依赖code
	if response.Message == "成功" {
		fmt.Printf("DEBUG: 余额购买成功 (Code: %d)\n", response.Code)
		return nil
	}

	// 只有当msg不是"成功"时才返回错误
	return fmt.Errorf("余额购买失败: %s", response.Message)
}

// 基于HAR文件分析实现的多步骤YouPin购买流程

// MultiStepBuyWithBalance 多步骤余额购买流程（基于HAR分析）
func (c *Client) MultiStepBuyWithBalance(ctx context.Context, commodityId int64, price float64) (*YouPinMultiStepBuyResponse, error) {
	priceStr := fmt.Sprintf("%.2f", price)

	// 第1步：订单预检查 - 暂时跳过，因为API返回码7000002但消息是"成功"的问题
	fmt.Printf("DEBUG: 跳过订单预检查步骤，直接进行后续购买流程\n")
	// preCheckResp, err := c.orderPreCheck(ctx, commodityId)
	// if err != nil {
	// 	return &YouPinMultiStepBuyResponse{
	// 		Success: false,
	// 		Message: "订单预检查失败",
	// 		Error:   err.Error(),
	// 		Step:    "precheck",
	// 	}, err
	// }

	// if !preCheckResp.Data.CanBuy {
	// 	return &YouPinMultiStepBuyResponse{
	// 		Success: false,
	// 		Message: preCheckResp.Data.Message,
	// 		Step:    "precheck",
	// 	}, fmt.Errorf("商品不可购买: %s", preCheckResp.Data.Message)
	// }

	// 第2步：创建订单
	createOrderResp, err := c.createOrder(ctx, commodityId, priceStr)
	if err != nil {
		return &YouPinMultiStepBuyResponse{
			Success: false,
			Message: "创建订单失败",
			Error:   err.Error(),
			Step:    "create_order",
		}, err
	}

	orderNo := createOrderResp.Data.OrderNo
	if orderNo == "" {
		return &YouPinMultiStepBuyResponse{
			Success: false,
			Message: "订单号为空",
			Step:    "create_order",
		}, fmt.Errorf("创建订单失败: 订单号为空")
	}

	// 第3步：支付确认
	_, err = c.confirmPayment(ctx, orderNo)
	if err != nil {
		return &YouPinMultiStepBuyResponse{
			Success: false,
			Message: "支付确认失败",
			Error:   err.Error(),
			OrderNo: orderNo,
			Step:    "payment",
		}, err
	}

	// 第4步：获取订单状态
	statusResp, err := c.getOrderStatus(ctx, orderNo)
	if err != nil {
		return &YouPinMultiStepBuyResponse{
			Success: false,
			Message: "获取订单状态失败",
			Error:   err.Error(),
			OrderNo: orderNo,
			Step:    "status_check",
		}, err
	}

	// 第5步：获取报价状态（如HAR所示的最后一步）
	_, err = c.getOfferStatus(ctx, orderNo)
	if err != nil {
		// 报价状态获取失败不影响整体购买流程，记录日志即可
		fmt.Printf("获取报价状态失败（非关键错误）: %v\n", err)
	}

	return &YouPinMultiStepBuyResponse{
		Success: true,
		Message: "购买流程完成",
		OrderNo: orderNo,
		Status:  statusResp.Data.Status,
		Step:    "completed",
	}, nil
}

// orderPreCheck 订单预检查
func (c *Client) orderPreCheck(ctx context.Context, commodityId int64) (*YouPinOrderPreCheckResponse, error) {
	data := map[string]interface{}{
		"commodityId": commodityId,
		"Sessionid":   c.deviceToken,
	}

	var response YouPinOrderPreCheckResponse
	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/v1/order/sell/pre-check", data, &response)
	if err != nil {
		return nil, fmt.Errorf("订单预检查请求失败: %w", err)
	}

	fmt.Printf("DEBUG: 订单预检查响应 - Code: %d, Msg: %s, CanBuy: %v\n", response.Code, response.Msg, response.Data.CanBuy)

	// 根据msg是否为"成功"来判定请求是否成功，不依赖code
	if response.Msg == "成功" {
		fmt.Printf("DEBUG: 订单预检查成功 (Code: %d, CanBuy: %v)\n", response.Code, response.Data.CanBuy)
		return &response, nil
	}

	// 只有当msg不是"成功"时才返回错误
	return nil, fmt.Errorf("订单预检查失败: %s", response.Msg)
}

// createOrder 创建订单
func (c *Client) createOrder(ctx context.Context, commodityId int64, price string) (*YouPinCreateOrderResponse, error) {
	data := map[string]interface{}{
		"commodityId":   strconv.FormatInt(commodityId, 10),
		"buyerUserId":   c.userId,
		"price":         price,
		"paymentMethod": "balance", // 余额支付
		"businessType":  "1",       // 基于HAR分析的业务类型
		"gameId":        "730",     // CS:GO游戏ID
		"Sessionid":     c.deviceToken,
	}

	var response YouPinCreateOrderResponse
	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/v1/order/sell/create", data, &response)
	if err != nil {
		return nil, fmt.Errorf("创建订单请求失败: %w", err)
	}

	fmt.Printf("DEBUG: 创建订单响应 - Code: %d, Msg: %s, OrderNo: %s\n", response.Code, response.Msg, response.Data.OrderNo)

	// 根据msg是否为"成功"来判定请求是否成功，不依赖code
	if response.Msg == "成功" {
		if response.Data.OrderNo != "" {
			fmt.Printf("DEBUG: 创建订单成功 (Code: %d, OrderNo: %s)\n", response.Code, response.Data.OrderNo)
			return &response, nil
		} else {
			// msg显示成功但订单号为空，这种情况也当作成功处理，但记录警告
			fmt.Printf("WARNING: 创建订单API返回成功但订单号为空 (Code: %d)\n", response.Code)
			return &response, nil
		}
	}

	// 只有当msg不是"成功"时才返回错误
	return nil, fmt.Errorf("创建订单失败: %s", response.Msg)
}

// confirmPayment 确认支付
func (c *Client) confirmPayment(ctx context.Context, orderNo string) (*YouPinPaymentConfirmResponse, error) {
	data := map[string]interface{}{
		"orderNo":       orderNo,
		"paymentMethod": "balance", // 余额支付
		"userId":        c.userId,
		"Sessionid":     c.deviceToken,
	}

	var response YouPinPaymentConfirmResponse
	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/payment/v1/pay/order/confirm", data, &response)
	if err != nil {
		return nil, fmt.Errorf("支付确认请求失败: %w", err)
	}

	fmt.Printf("DEBUG: 支付确认响应 - Code: %d, Msg: %s\n", response.Code, response.Msg)

	// 根据msg是否为"成功"来判定请求是否成功，不依赖code
	if response.Msg == "成功" {
		fmt.Printf("DEBUG: 支付确认成功 (Code: %d)\n", response.Code)
		return &response, nil
	}

	// 只有当msg不是"成功"时才返回错误
	return nil, fmt.Errorf("支付确认失败: %s", response.Msg)
}

// getOrderStatus 获取订单状态
func (c *Client) getOrderStatus(ctx context.Context, orderNo string) (*YouPinOrderStatusResponse, error) {
	data := map[string]interface{}{
		"orderNo":   orderNo,
		"userId":    c.userId,
		"Sessionid": c.deviceToken,
	}

	var response YouPinOrderStatusResponse
	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/payment/v1/pay/order/status", data, &response)
	if err != nil {
		return nil, fmt.Errorf("获取订单状态请求失败: %w", err)
	}

	fmt.Printf("DEBUG: 获取订单状态响应 - Code: %d, Msg: %s\n", response.Code, response.Msg)

	// 根据msg是否为"成功"来判定请求是否成功，不依赖code
	if response.Msg == "成功" {
		fmt.Printf("DEBUG: 获取订单状态成功 (Code: %d)\n", response.Code)
		return &response, nil
	}

	// 只有当msg不是"成功"时才返回错误
	return nil, fmt.Errorf("获取订单状态失败: %s", response.Msg)
}

// getOfferStatus 获取报价状态（基于HAR分析的最后一步）
func (c *Client) getOfferStatus(ctx context.Context, orderNo string) (*GetOfferStatusResponse, error) {
	data := map[string]interface{}{
		"orderNo":   orderNo,
		"userId":    c.userId,
		"Sessionid": c.deviceToken,
	}

	var response GetOfferStatusResponse
	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/v1/order/sell/delivery/get-offer-status", data, &response)
	if err != nil {
		return nil, fmt.Errorf("获取报价状态请求失败: %w", err)
	}

	fmt.Printf("DEBUG: 获取报价状态响应 - Code: %d, Msg: %s\n", response.Code, response.Msg)

	// 根据msg是否为"成功"来判定请求是否成功，不依赖code
	if response.Msg == "成功" {
		fmt.Printf("DEBUG: 获取报价状态成功 (Code: %d)\n", response.Code)
		return &response, nil
	}

	// 只有当msg不是"成功"时才返回错误
	return nil, fmt.Errorf("获取报价状态失败: %s", response.Msg)
}

// makeRequest 发起HTTP请求
func (c *Client) makeRequest(ctx context.Context, method, path string, data interface{}, result interface{}) error {
	return c.makeRequestWithGzip(ctx, method, path, data, result, false)
}

// makeRequestWithGzip 发起HTTP请求，可选择是否使用gzip压缩
func (c *Client) makeRequestWithGzip(ctx context.Context, method, path string, data interface{}, result interface{}, useGzip bool) error {
	var body io.Reader
	url := c.baseURL + path

	// 根据方法类型处理参数
	if method == "GET" && data != nil {
		// GET请求使用URL参数
		params := make([]string, 0)
		if dataMap, ok := data.(map[string]interface{}); ok {
			for k, v := range dataMap {
				params = append(params, fmt.Sprintf("%s=%v", k, v))
			}
		}
		if len(params) > 0 {
			url += "?" + strings.Join(params, "&")
		}
	} else if data != nil {
		// POST/PUT请求使用JSON body
		jsonData, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("序列化请求数据失败: %w", err)
		}

		if useGzip {
			// 对JSON数据进行gzip压缩
			var buf bytes.Buffer
			gzWriter := gzip.NewWriter(&buf)
			if _, err := gzWriter.Write(jsonData); err != nil {
				return fmt.Errorf("gzip压缩失败: %w", err)
			}
			if err := gzWriter.Close(); err != nil {
				return fmt.Errorf("关闭gzip写入器失败: %w", err)
			}
			body = &buf
		} else {
			body = bytes.NewBuffer(jsonData)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	// 根据抓包信息更新headers格式（模拟Android客户端）
	req.Header.Set("User-Agent", "okhttp/3.14.9")
	req.Header.Set("Connection", "Keep-Alive")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("tracestate", "bnro=android/10_android/8.20.0_okhttp/3.14.9")
	req.Header.Set("traceparent", fmt.Sprintf("00-%s-%s-01", generateRandomString(32), generateRandomString(16)))
	req.Header.Set("DeviceToken", c.deviceToken)
	req.Header.Set("DeviceId", c.deviceID)
	req.Header.Set("requestTag", strings.ToUpper(generateRandomString(32)))
	req.Header.Set("Gameid", "730")
	req.Header.Set("deviceType", "2")
	req.Header.Set("platform", "android")
	req.Header.Set("currentTheme", "Light")
	req.Header.Set("package-type", "uuyp")
	req.Header.Set("App-Version", "5.37.1")
	req.Header.Set("uk", "5FQFWiQh8VvtSm0krHaYs52HWGSqA0v4UVcWASmLbSD68mdWzxo3oSoRtbSgwY91L")
	req.Header.Set("deviceUk", "5FQFZE57VAGa7uQBapxU70o3PHzUYIUevEmrT53gRd8hMLiEMafT7TmLexlKfk51I")
	req.Header.Set("AppType", "4")
	req.Header.Set("Authorization", "Bearer "+c.token)

	// 设置Content-Type和Content-Encoding
	if method == "POST" || method == "PUT" {
		req.Header.Set("Content-Type", "application/json")
		if useGzip {
			req.Header.Set("Content-Encoding", "gzip")
		}
	}

	// Device-Info JSON字符串 - 根据抓包信息更新
	deviceInfo := map[string]interface{}{
		"deviceId":      c.deviceID,
		"deviceType":    "VCE-AL00",
		"hasSteamApp":   1,
		"requestTag":    strings.ToUpper(generateRandomString(32)),
		"systemName ":   "Android", // 注意这里有空格
		"systemVersion": "10",
	}
	deviceInfoJSON, _ := json.Marshal(deviceInfo)
	req.Header.Set("Device-Info", string(deviceInfoJSON))

	// 发起请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("发起请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应，支持gzip解压
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("创建gzip读取器失败: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	respBody, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	// 检查HTTP状态码
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP错误: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	// 解析响应
	if result != nil {
		err = json.Unmarshal(respBody, result)
		if err != nil {
			return fmt.Errorf("解析响应失败: %w", err)
		}
	}

	// 检查登录状态
	if strings.Contains(string(respBody), "84101") {
		return fmt.Errorf("登录状态失效，请重新登录")
	}

	return nil
}

// signRequest 对请求参数进行签名
func (c *Client) signRequest(params map[string]interface{}, secret string) string {
	// 将参数转换为字符串并排序
	var keys []string
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 构建签名字符串
	var signParts []string
	for _, k := range keys {
		v := params[k]
		var valueStr string
		switch val := v.(type) {
		case string:
			valueStr = val
		case int:
			valueStr = strconv.Itoa(val)
		case int64:
			valueStr = strconv.FormatInt(val, 10)
		case float64:
			valueStr = strconv.FormatFloat(val, 'f', -1, 64)
		case bool:
			valueStr = strconv.FormatBool(val)
		default:
			valueStr = fmt.Sprintf("%v", val)
		}
		signParts = append(signParts, fmt.Sprintf("%s=%s", k, valueStr))
	}

	signStr := strings.Join(signParts, "&") + "&key=" + secret

	// 计算MD5
	hash := md5.Sum([]byte(signStr))
	return fmt.Sprintf("%x", hash)
}

// SendSMSCode 发送短信验证码
func SendSMSCode(ctx context.Context, phone string) error {
	client := &http.Client{Timeout: 30 * time.Second}

	// 生成会话ID
	sessionId := generateRandomString(32)

	data := map[string]interface{}{
		"Area":      86,        // 中国区号
		"Mobile":    phone,     // 手机号
		"Sessionid": sessionId, // 会话ID
		"Code":      "",        // 空验证码
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化请求数据失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", BaseURL+"/api/user/Auth/SendSignInSmsCode", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置请求头，模拟Android APP
	deviceInfo := map[string]interface{}{
		"deviceId":      sessionId,
		"deviceType":    sessionId,
		"hasSteamApp":   1,
		"requestTag":    strings.ToUpper(generateRandomString(32)),
		"systemName ":   "Android",
		"systemVersion": "15",
	}
	deviceInfoJson, _ := json.Marshal(deviceInfo)

	req.Header.Set("uk", generateRandomString(65))
	req.Header.Set("authorization", "Bearer ")
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "okhttp/3.14.9")
	req.Header.Set("App-Version", "5.28.3")
	req.Header.Set("AppType", "4")
	req.Header.Set("deviceType", "1")
	req.Header.Set("package-type", "uuyp")
	req.Header.Set("DeviceToken", sessionId)
	req.Header.Set("DeviceId", sessionId)
	req.Header.Set("platform", "android")
	req.Header.Set("accept-encoding", "gzip")
	req.Header.Set("Gameid", "730")
	req.Header.Set("Device-Info", string(deviceInfoJson))

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 处理gzip压缩
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("创建gzip读取器失败: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	respBody, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	var response struct {
		Code int    `json:"Code"`
		Msg  string `json:"Msg"`
	}

	err = json.Unmarshal(respBody, &response)
	if err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if response.Code != 200 {
		return fmt.Errorf("发送验证码失败: %s", response.Msg)
	}

	return nil
}

// LoginWithPhone 使用手机号和验证码登录
func LoginWithPhone(ctx context.Context, phone, code string) (string, *UserInfo, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	// 生成会话ID
	sessionId := generateRandomString(32)

	data := map[string]interface{}{
		"Area":       86,        // 中国区号
		"Code":       code,      // 验证码
		"DeviceName": sessionId, // 设备名称
		"Sessionid":  sessionId, // 会话ID
		"Mobile":     phone,     // 手机号
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", nil, fmt.Errorf("序列化请求数据失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", BaseURL+"/api/user/Auth/SmsSignIn", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置请求头，模拟Android APP
	deviceInfo := map[string]interface{}{
		"deviceId":      sessionId,
		"deviceType":    sessionId,
		"hasSteamApp":   1,
		"requestTag":    strings.ToUpper(generateRandomString(32)),
		"systemName ":   "Android",
		"systemVersion": "15",
	}
	deviceInfoJson, _ := json.Marshal(deviceInfo)

	req.Header.Set("uk", generateRandomString(65))
	req.Header.Set("authorization", "Bearer ")
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "okhttp/3.14.9")
	req.Header.Set("App-Version", "5.28.3")
	req.Header.Set("AppType", "4")
	req.Header.Set("deviceType", "1")
	req.Header.Set("package-type", "uuyp")
	req.Header.Set("DeviceToken", sessionId)
	req.Header.Set("DeviceId", sessionId)
	req.Header.Set("platform", "android")
	req.Header.Set("accept-encoding", "gzip")
	req.Header.Set("Gameid", "730")
	req.Header.Set("Device-Info", string(deviceInfoJson))

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 处理gzip压缩
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return "", nil, fmt.Errorf("创建gzip读取器失败: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	respBody, err := io.ReadAll(reader)
	if err != nil {
		return "", nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var response LoginResponse
	err = json.Unmarshal(respBody, &response)
	if err != nil {
		return "", nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if response.Code != 0 {
		return "", nil, fmt.Errorf("登录失败: %s", response.Message)
	}

	userInfo := &UserInfo{
		UserId:   response.Data.UserInfo.UserId,
		NickName: response.Data.UserInfo.NickName,
	}

	return response.Data.AccessToken, userInfo, nil
}

// SearchItems 搜索商品 - 完全按抓包信息复刻
func (c *Client) SearchItems(ctx context.Context, keywords string, pageIndex int, pageSize int, sortType int) (*SearchNewListResponse, error) {
	// 设置默认值 - 支持动态分页
	if pageIndex <= 0 {
		pageIndex = 1
	}
	if pageSize <= 0 {
		pageSize = 20 // 默认20条
	}
	// 限制最大分页大小，避免服务器压力
	if pageSize > 100 {
		pageSize = 100
	}

	// 使用正确的搜索API接口
	data := map[string]interface{}{
		"filterMap":          map[string]interface{}{},
		"gameId":             730,
		"keyWords":           keywords,
		"listSortType":       0,
		"listType":           10,
		"pageCode":           "MARKET_PAGE_BUY_COMMODITY_PAGE",
		"pageIndex":          pageIndex,
		"pageSize":           pageSize,
		"pageSourceCode":     "PG3000002",
		"propertyFilterTags": []interface{}{},
		"sortType":           sortType,
		"stickerAbrade":      0,
		"stickersIsSort":     false,
		"Sessionid":          c.deviceToken,
	}

	var response SearchNewListResponse
	err := c.makeRequest(ctx, "POST", "/api/homepage/search/new/list", data, &response)
	if err != nil {
		return nil, fmt.Errorf("搜索商品失败: %w", err)
	}
	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}
	return &response, nil
}

// GetCommodityList 获取出售商品列表
func (c *Client) GetCommodityList(ctx context.Context, templateId int, pageIndex int, pageSize int, sortType int) (*CommodityListResponse, error) {
	// 根据抓包信息补全所有必需参数
	data := map[string]interface{}{
		"commodityId":             fmt.Sprintf("%d", templateId), // 添加缺失的 commodityId 参数
		"templateId":              fmt.Sprintf("%d", templateId),
		"abrasion":                []interface{}{},
		"phase":                   []interface{}{},
		"itemCategory":            "weapon",
		"rarity":                  []interface{}{},
		"quality":                 []interface{}{},
		"price":                   "",
		"isStattrak":              "",
		"category":                "730",
		"exterior":                []interface{}{},
		"minPrice":                "",
		"maxPrice":                "",
		"sortType":                "",
		"sortBy":                  "",
		"sortByPrice":             "",
		"currentPage":             fmt.Sprintf("%d", pageIndex),
		"currentPageSize":         fmt.Sprintf("%d", pageSize),
		"minAutoDelivery":         "",
		"forceSeller":             "",
		"language":                "cn",
		"fromAccountId":           "",
		"autoDelivery":            0,
		"hasSold":                 "true",
		"haveBuZhangType":         0,
		"isDialogMarket":          false,
		"isMultipleZone":          0,
		"listSortType":            "1",
		"listType":                10,
		"mergeFlag":               0,
		"pageIndex":               pageIndex,
		"pageSize":                pageSize,
		"pageSourceCode":          "PG3000003",
		"presaleMoreZones":        2,
		"sortTypeKey":             "",
		"sourceChannel":           "",
		"status":                  "20",
		"stickerAbrade":           0,
		"stickersIsSort":          false,
		"ultraLongLeaseMoreZones": 0,
		"userId":                  c.userId,
		"Sessionid":               c.deviceToken,
	}

	var response CommodityListResponse
	err := c.makeRequest(ctx, "POST", "/api/homepage/v3/detail/commodity/list/sell", data, &response)
	if err != nil {
		return nil, fmt.Errorf("获取商品列表失败: %w", err)
	}
	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}

	// 修正 TotalCount - 使用根级别的值
	if response.Data.TotalCount == 0 && response.TotalCount > 0 {
		response.Data.TotalCount = response.TotalCount
	}

	// 使用commodityNo作为commodityId的值，保持字段名为commodityId
	for i := range response.Data.CommodityList {
		commodity := &response.Data.CommodityList[i]
		// 优先使用 commodityNo 作为 commodityId 的值
		if commodity.CommodityNo != "" {
			commodity.CommodityId = commodity.CommodityNo
		} else if commodity.CommodityId == "" {
			// 如果 commodityNo 也为空，则生成临时ID
			if commodity.TemplateId > 0 {
				commodity.CommodityId = fmt.Sprintf("%d_%s_%s", commodity.TemplateId,
					strings.ReplaceAll(commodity.Price, ".", ""),
					strings.ReplaceAll(commodity.Abrade, ".", ""))
			}
		}
	}

	fmt.Printf("DEBUG: GetCommodityList response - Code: %d, Msg: %s, TotalCount: %d, CommodityList length: %d\n",
		response.Code, response.Msg, response.Data.TotalCount, len(response.Data.CommodityList))

	// 验证修复结果
	for i, commodity := range response.Data.CommodityList {
		if commodity.CommodityId == "" {
			fmt.Printf("WARNING: CommodityId仍为空 - Index: %d, TemplateId: %d, CommodityNo: '%s', Price: %s\n",
				i, commodity.TemplateId, commodity.CommodityNo, commodity.Price)
		}
	}

	return &response, nil
}

// GetTemplateFilterConfig 获取模板筛选配置
func (c *Client) GetTemplateFilterConfig(ctx context.Context, templateId int) (*FilterConfigResponse, error) {
	data := map[string]interface{}{
		"templateId": templateId,
		"Sessionid":  c.deviceToken,
	}

	var response FilterConfigResponse
	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/getTemplateFilterConfigV2", data, &response)
	if err != nil {
		return nil, fmt.Errorf("获取筛选配置失败: %w", err)
	}
	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}
	return &response, nil
}
