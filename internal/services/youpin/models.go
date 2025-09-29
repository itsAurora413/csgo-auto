package youpin

import (
	"fmt"
	"time"
)

// YouPinAccount 悠悠有品账户信息
type YouPinAccount struct {
	Token       string    `json:"token"`
	UserID      string    `json:"user_id"`
	Nickname    string    `json:"nickname"`
	Avatar      string    `json:"avatar"`
	Balance     float64   `json:"balance"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// YouPinItem 悠悠有品物品信息
type YouPinItem struct {
	SteamAssetID        string  `json:"SteamAssetId"`
	ShotName           string  `json:"ShotName"`
	ActionLink         string  `json:"ActionLink"`
	MarketHashName     string  `json:"MarketHashName"`
	HaveNameTag        int     `json:"HaveNameTag"`
	Stickers           []interface{} `json:"Stickers"`
	Pendants           []interface{} `json:"Pendants"`
	StickerType        int     `json:"StickerType"`
	HasSticker         int     `json:"HasSticker"`
	Analysis2dStatus   int     `json:"Analysis2dStatus"`
	IsCanAnalysis      bool    `json:"IsCanAnalysis"`
	AssetRemark        string  `json:"AssetRemark"`
	IsMerge            int     `json:"IsMerge"`
	AssetStatus        int     `json:"AssetStatus"`
	AssetTagColor      string  `json:"AssetTagColor"`
	Tradable           bool    `json:"Tradable"`
	ClassId            string  `json:"ClassId"`
	InstanceId         string  `json:"InstanceId"`

	// 模板信息
	TemplateInfo       struct {
		CommodityName string  `json:"CommodityName"`
		IconUrl       string  `json:"IconUrl"`
		IconUrlLarge  string  `json:"IconUrlLarge"`
		MarkPrice     float64 `json:"MarkPrice"`
		Id            int     `json:"Id"`
	} `json:"TemplateInfo"`

	// 账户信息 - 用于前端显示
	AccountID       uint   `json:"account_id,omitempty"`
	AccountNickname string `json:"account_nickname,omitempty"`

	// 兼容字段 - 保持向后兼容性
	Name               string  `json:"-"` // 使用ShotName的值
	CommodityName      string  `json:"-"` // 使用TemplateInfo.CommodityName的值
	TemplateID         string  `json:"-"` // 使用TemplateInfo.Id的值
	Price              float64 `json:"-"` // 使用TemplateInfo.MarkPrice的值
	ImageURL           string  `json:"-"` // 使用TemplateInfo.IconUrl的值
}

// GetCommodityName 获取商品名称
func (item *YouPinItem) GetCommodityName() string {
	if item.TemplateInfo.CommodityName != "" {
		return item.TemplateInfo.CommodityName
	}
	return item.ShotName
}

// GetTemplateID 获取模板ID
func (item *YouPinItem) GetTemplateID() string {
	return fmt.Sprintf("%d", item.TemplateInfo.Id)
}

// IsTradable 检查是否可交易
func (item *YouPinItem) IsTradable() bool {
	return item.Tradable && item.AssetStatus == 0
}

// GetName 获取物品名称
func (item *YouPinItem) GetName() string {
	return item.ShotName
}

// GetPrice 获取市场价格
func (item *YouPinItem) GetPrice() float64 {
	return item.TemplateInfo.MarkPrice
}

// GetImageURL 获取图片URL
func (item *YouPinItem) GetImageURL() string {
	return item.TemplateInfo.IconUrl
}

// YouPinSaleItem 出售物品信息
type YouPinSaleItem struct {
	AssetID     string  `json:"AssetId"`
	IsCanLease  bool    `json:"IsCanLease"`
	IsCanSold   bool    `json:"IsCanSold"`
	Price       float64 `json:"Price"`
	Remark      string  `json:"Remark"`
}

// YouPinCommodity 商品信息
type YouPinCommodity struct {
	CommodityID string  `json:"CommodityId"`
	IsCanLease  bool    `json:"IsCanLease"`
	IsCanSold   bool    `json:"IsCanSold"`
	Price       float64 `json:"Price"`
	Remark      string  `json:"Remark"`
}

// YouPinOrder 订单信息
type YouPinOrder struct {
	ID           int64     `json:"id"`
	OrderID      string    `json:"order_id"`
	OfferID      string    `json:"offer_id"`
	ItemName     string    `json:"item_name"`
	Price        float64   `json:"price"`
	Status       string    `json:"status"`
	BuyerNickname string   `json:"buyer_nickname"`
	SellerNickname string  `json:"seller_nickname"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// YouPinLeaseItem 租赁物品信息
type YouPinLeaseItem struct {
	AssetID              string  `json:"AssetId"`
	IsCanLease           bool    `json:"IsCanLease"`
	IsCanSold            bool    `json:"IsCanSold"`
	LeaseMaxDays         int     `json:"LeaseMaxDays"`
	LeaseUnitPrice       float64 `json:"LeaseUnitPrice"`
	LongLeaseUnitPrice   *float64 `json:"LongLeaseUnitPrice,omitempty"`
	LeaseDeposit         string  `json:"LeaseDeposit"`
	CompensationType     int     `json:"CompensationType"`
}

// YouPinMarketPrice 市场价格信息
type YouPinMarketPrice struct {
	Price         float64 `json:"price"`
	CommodityName string  `json:"commodityName"`
	Abrade        string  `json:"abrade"`
	Quality       string  `json:"quality"`
}

// YouPinInventoryResponse 库存响应
type YouPinInventoryResponse struct {
	Code    int             `json:"Code"`
	Message string          `json:"Message"`
	Data    []YouPinItem    `json:"Data"`
}

// YouPinSellResponse 出售响应
type YouPinSellResponse struct {
	Code    int    `json:"Code"`
	Message string `json:"Message"`
	Data    interface{} `json:"Data"`
}

// YouPinMarketPriceResponse 市场价格响应
type YouPinMarketPriceResponse struct {
	Code    int                 `json:"Code"`
	Message string              `json:"Message"`
	Data    []YouPinMarketPrice `json:"Data"`
}

// YouPinOrderResponse 订单响应
type YouPinOrderResponse struct {
	Code    int           `json:"Code"`
	Message string        `json:"Message"`
	Data    []YouPinOrder `json:"Data"`
}

// YouPinPriceChangeResponse 改价响应
type YouPinPriceChangeResponse struct {
    Code    int    `json:"Code"`
    Message string `json:"Message"`
    Data    struct {
        SuccessCount int `json:"SuccessCount"`
        FailCount    int `json:"FailCount"`
        Commoditys   []struct {
            // 部分环境返回数字类型，兼容 string/number
            CommodityId interface{} `json:"CommodityId"`
            IsSuccess   int    `json:"IsSuccess"`
            Message     string `json:"Message"`
        } `json:"Commoditys"`
    } `json:"Data"`
}

// YouPinBuyOrder 求购订单
type YouPinBuyOrder struct {
	ID           string    `json:"id"`
	TemplateID   string    `json:"template_id"`
	ItemName     string    `json:"item_name"`
	Price        float64   `json:"price"`
	Quantity     int       `json:"quantity"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// YouPinConfig 悠悠有品配置
type YouPinConfig struct {
	Token                    string   `json:"token"`
	AutoSellEnabled          bool     `json:"auto_sell_enabled"`
	AutoBuyEnabled           bool     `json:"auto_buy_enabled"`
	SellItemNames            []string `json:"sell_item_names"`
	BlacklistWords           []string `json:"blacklist_words"`
	MaxSalePrice             float64  `json:"max_sale_price"`
	TakeProfileEnabled       bool     `json:"take_profile_enabled"`
	TakeProfileRatio         float64  `json:"take_profile_ratio"`
	UsePriceAdjustment       bool     `json:"use_price_adjustment"`
	PriceAdjustmentThreshold float64  `json:"price_adjustment_threshold"`
	RunTime                  string   `json:"run_time"`
	Interval                 int      `json:"interval"`
}

// YouPinCommoditySearch 搜索商品结果
type YouPinCommoditySearch struct {
	ID                  int     `json:"id"`
	GameID              int     `json:"gameId"`
	GameName            string  `json:"gameName"`
	GameIcon            string  `json:"gameIcon"`
	CommodityName       string  `json:"commodityName"`
	CommodityHashName   string  `json:"commodityHashName"`
	IconUrl             string  `json:"iconUrl"`
	IconUrlLarge        string  `json:"iconUrlLarge"`
	OnSaleCount         int     `json:"onSaleCount"`
	OnLeaseCount        int     `json:"onLeaseCount"`
	LeaseUnitPrice      string  `json:"leaseUnitPrice"`
	LongLeaseUnitPrice  string  `json:"longLeaseUnitPrice"`
	LeaseDeposit        string  `json:"leaseDeposit"`
	Price               string  `json:"price"`
	SteamPrice          string  `json:"steamPrice"`
	SteamUsdPrice       string  `json:"steamUsdPrice"`
	TypeName            string  `json:"typeName"`
	Exterior            string  `json:"exterior"`
	ExteriorColor       string  `json:"exteriorColor"`
	Rarity              string  `json:"rarity"`
	RarityColor         string  `json:"rarityColor"`
	Quality             string  `json:"quality"`
	QualityColor        string  `json:"qualityColor"`
	SortID              int     `json:"sortId"`
	HaveLease           int     `json:"haveLease"`
	StickersIsSort      bool    `json:"stickersIsSort"`
	SubsidyPurchase     int     `json:"subsidyPurchase"`
	Stickers            interface{} `json:"stickers"`
	Label               interface{} `json:"label"`
	Rent                string  `json:"rent"`
	MinLeaseDeposit     interface{} `json:"minLeaseDeposit"`
	ListType            interface{} `json:"listType"`
	TemplatePurchaseCountText interface{} `json:"templatePurchaseCountText"`
	TemplateTags        interface{} `json:"templateTags"`
}

// YouPinCommodityDetail 商品详情
type YouPinCommodityDetail struct {
	TemplateID       string                    `json:"TemplateId"`
	TemplateHashName string                    `json:"TemplateHashName"`
	CommodityName    string                    `json:"CommodityName"`
	IconUrl          string                    `json:"IconUrl"`
	Description      string                    `json:"Description"`
	Category         string                    `json:"Category"`
	Rarity           string                    `json:"Rarity"`
	Quality          string                    `json:"Quality"`
	GameId           string                    `json:"GameId"`
	WearLevels       []YouPinWearLevel         `json:"WearLevels"`
	MarketSummary    YouPinMarketSummary       `json:"MarketSummary"`
}

// YouPinWearLevel 磨损等级
type YouPinWearLevel struct {
	WearName    string  `json:"WearName"`    // 磨损名称，如"崭新出厂"
	WearCode    string  `json:"WearCode"`    // 磨损代码，如"FN"
	MinAbrade   float64 `json:"MinAbrade"`   // 最小磨损值
	MaxAbrade   float64 `json:"MaxAbrade"`   // 最大磨损值
	MarketCount int     `json:"MarketCount"` // 市场数量
	MinPrice    float64 `json:"MinPrice"`    // 最低价格
}

// YouPinMarketSummary 市场概况
type YouPinMarketSummary struct {
	TotalMarketCount   int     `json:"TotalMarketCount"`   // 总市场数量
	TotalPurchaseCount int     `json:"TotalPurchaseCount"` // 总求购数量
	LowestPrice        float64 `json:"LowestPrice"`        // 最低价格
	HighestPurchase    float64 `json:"HighestPurchase"`    // 最高求购价
}

// YouPinMarketItem 市场物品
type YouPinMarketItem struct {
	CommodityId      string  `json:"CommodityId"`
	Price            float64 `json:"Price"`
	Abrade           float64 `json:"Abrade"`
	WearName         string  `json:"WearName"`
	StickerInfo      string  `json:"StickerInfo"`
	SellerNickname   string  `json:"SellerNickname"`
	SellTime         string  `json:"SellTime"`
	CanBuy           bool    `json:"CanBuy"`
	IsLeasing        bool    `json:"IsLeasing"`
	LeasePrice       float64 `json:"LeasePrice"`
	LeaseDays        int     `json:"LeaseDays"`
}

// YouPinPurchaseOrder 求购订单
type YouPinPurchaseOrder struct {
	OrderId          string  `json:"OrderId"`
	TemplateId       string  `json:"TemplateId"`
	PurchasePrice    float64 `json:"PurchasePrice"`
	PurchaseNum      int     `json:"PurchaseNum"`
	SupplyQuantity   int     `json:"SupplyQuantity"`
	MinAbrade        float64 `json:"MinAbrade"`
	MaxAbrade        float64 `json:"MaxAbrade"`
	WearName         string  `json:"WearName"`
	BuyerNickname    string  `json:"BuyerNickname"`
	CreateTime       string  `json:"CreateTime"`
	ExpireTime       string  `json:"ExpireTime"`
	CanSell          bool    `json:"CanSell"`
	IsMyOrder        bool    `json:"IsMyOrder"`
}

// YouPinSearchRequest 搜索请求
type YouPinSearchRequest struct {
	Keyword   string `json:"keyword" binding:"required"`
	PageIndex int    `json:"page_index"`
	PageSize  int    `json:"page_size"`
}

// YouPinMarketRequest 市场查询请求
type YouPinMarketRequest struct {
	TemplateId string  `json:"template_id" binding:"required"`
	PageIndex  int     `json:"page_index"`
	PageSize   int     `json:"page_size"`
	MinAbrade  float64 `json:"min_abrade"`
	MaxAbrade  float64 `json:"max_abrade"`
}

// YouPinPurchaseRequest 求购请求
type YouPinPurchaseRequest struct {
	TemplateId       string  `json:"template_id" binding:"required"`
	TemplateHashName string  `json:"template_hash_name" binding:"required"`
	CommodityName    string  `json:"commodity_name" binding:"required"`
	PurchasePrice    float64 `json:"purchase_price" binding:"required,min=0"`
	PurchaseNum      int     `json:"purchase_num" binding:"required,min=1"`
	MinAbrade        float64 `json:"min_abrade"`
	MaxAbrade        float64 `json:"max_abrade"`
}

// YouPinBuyRequest 购买请求
type YouPinBuyRequest struct {
	CommodityId string  `json:"commodity_id" binding:"required"`
	Price       float64 `json:"price" binding:"required,min=0"`
}

// YouPinBuyWithBalanceRequest 使用余额购买请求
type YouPinBuyWithBalanceRequest struct {
	CommodityId   string  `json:"commodity_id" binding:"required"`   // 接收字符串，使用时转换为int64
	Price         float64 `json:"price" binding:"required,min=0"`
	PaymentMethod string  `json:"payment_method" binding:"required"` // 支付方式，固定为"balance"
}

// 基于HAR文件分析的YouPin多步骤购买流程结构

// YouPinOrderPreCheckRequest 订单预检查请求
type YouPinOrderPreCheckRequest struct {
	CommodityId int64  `json:"commodityId" binding:"required"`
	Sessionid   string `json:"Sessionid" binding:"required"`
}

// YouPinOrderPreCheckResponse 订单预检查响应
type YouPinOrderPreCheckResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		CanBuy       bool   `json:"canBuy"`
		Message      string `json:"message"`
		CommodityId  string `json:"commodityId"`
		Price        string `json:"price"`
		SellerUserId string `json:"sellerUserId"`
	} `json:"data"`
}

// YouPinCreateOrderRequest 创建订单请求
type YouPinCreateOrderRequest struct {
	CommodityId      string `json:"commodityId" binding:"required"`
	BuyerUserId      string `json:"buyerUserId" binding:"required"`
	Price            string `json:"price" binding:"required"`
	PaymentMethod    string `json:"paymentMethod" binding:"required"`
	BusinessType     string `json:"businessType"`
	GameId           string `json:"gameId"`
}

// YouPinCreateOrderResponse 创建订单响应
type YouPinCreateOrderResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		OrderNo       string `json:"orderNo"`
		CommodityId   string `json:"commodityId"`
		Price         string `json:"price"`
		Status        string `json:"status"`
		BusinessType  string `json:"businessType"`
		CreateTime    string `json:"createTime"`
	} `json:"data"`
}

// YouPinPaymentConfirmRequest 支付确认请求
type YouPinPaymentConfirmRequest struct {
	OrderNo       string `json:"orderNo" binding:"required"`
	PaymentMethod string `json:"paymentMethod" binding:"required"`
	UserId        string `json:"userId" binding:"required"`
}

// YouPinPaymentConfirmResponse 支付确认响应
type YouPinPaymentConfirmResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		OrderNo       string `json:"orderNo"`
		PaymentStatus string `json:"paymentStatus"`
		TradeStatus   string `json:"tradeStatus"`
		Message       string `json:"message"`
	} `json:"data"`
}

// YouPinOrderStatusRequest 订单状态查询请求
type YouPinOrderStatusRequest struct {
	OrderNo string `json:"orderNo" binding:"required"`
	UserId  string `json:"userId" binding:"required"`
}

// YouPinOrderStatusResponse 订单状态响应
type YouPinOrderStatusResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		OrderNo       string `json:"orderNo"`
		Status        string `json:"status"`
		TradeStatus   string `json:"tradeStatus"`
		CommodityName string `json:"commodityName"`
		Price         string `json:"price"`
		CreateTime    string `json:"createTime"`
		UpdateTime    string `json:"updateTime"`
		OfferStatus   string `json:"offerStatus"`
	} `json:"data"`
}

// YouPinMultiStepBuyRequest 多步骤购买请求（前端接口）
type YouPinMultiStepBuyRequest struct {
	CommodityId   string  `json:"commodity_id" binding:"required"`   // 接收字符串，使用时转换为int64
	Price         float64 `json:"price" binding:"required,min=0"`
	PaymentMethod string  `json:"payment_method" binding:"required"` // 支付方式，固定为"balance"
}

// YouPinMultiStepBuyResponse 多步骤购买响应
type YouPinMultiStepBuyResponse struct {
	Success       bool   `json:"success"`
	Message       string `json:"message"`
	OrderNo       string `json:"order_no,omitempty"`
	Status        string `json:"status,omitempty"`
	Step          string `json:"step,omitempty"` // precheck, create_order, payment, status_check
	Error         string `json:"error,omitempty"`
}

// SearchResponse 搜索商品响应 - 根据抓包信息添加
type SearchResponse struct {
	Code      int    `json:"code"`
	Msg       string `json:"msg"`
	Timestamp int64  `json:"timestamp"`
	Data      struct {
		RequestFlag    string          `json:"requestFlag"`
		RankInfoList   []RankInfo      `json:"rankInfoList"`
	} `json:"data"`
}

// RankInfo 排行信息
type RankInfo struct {
	ID                string `json:"id"`
	TemplateGroupName string `json:"templateGroupName"`
	TemplateId        int    `json:"templateId"`
	TemplateIconUrl   string `json:"templateIconUrl"`
	ExteriorName      string `json:"exteriorName"`
	ExteriorColor     string `json:"exteriorColor"`
	Ranking           string `json:"ranking"`
	Abrade            string `json:"abrade"`
	PaintSeed         string `json:"paintSeed"`
	SellFlag          bool   `json:"sellFlag"`
	SellPriceDesc     string `json:"sellPriceDesc"`
	JumpType          string `json:"jumpType"`
	JumpId            string `json:"jumpId"`
}

// CommodityListResponse 商品列表响应
type CommodityListResponse struct {
	Code       int    `json:"Code"`
	Msg        string `json:"Msg"`
	TotalCount int    `json:"TotalCount"`
	Data       struct {
		CommodityList []CommodityInfo `json:"commodityList"`
		TotalCount    int             `json:"totalCount"`
	} `json:"Data"`
}

// CommodityInfo 商品信息
type CommodityInfo struct {
	CommodityId      string  `json:"commodityId"`
	CommodityNo      string  `json:"commodityNo"`     // 来自YouPin API的commodityNo，用于购买
	TemplateId       int     `json:"templateId"`
	CommodityName    string  `json:"commodityName"`
	IconUrl          string  `json:"iconUrl"`
	Price            string  `json:"price"`           // 价格作为字符串
	Abrade           string  `json:"abrade"`          // 磨损值作为字符串
	ExteriorName     string  `json:"exteriorName"`
	ExteriorColor    string  `json:"exteriorColor"`
	SellerNickname   string  `json:"sellerNickname"`
	SellTime         string  `json:"sellTime"`
	CanSold          int     `json:"canSold"`         // 1表示可以购买，0表示不可购买
}

// IsCanBuy 检查是否可以购买
func (c *CommodityInfo) IsCanBuy() bool {
	return c.CanSold == 1
}

// FilterConfigResponse 筛选配置响应
type FilterConfigResponse struct {
	Code      int    `json:"code"`
	Msg       string `json:"msg"`
	Timestamp int64  `json:"timestamp"`
	Data      struct {
		FilterConfig []FilterItem `json:"filterConfig"`
	} `json:"data"`
}

// FilterItem 筛选项
type FilterItem struct {
	Type int    `json:"type"`
	Name string `json:"name"`
	Code string `json:"code"`
}

// LeaseOrderListResponse 租赁订单列表响应 - 根据正确的API路径
type LeaseOrderListResponse struct {
	Code      int    `json:"code"`
	Msg       string `json:"msg"`
	Timestamp int64  `json:"timestamp"`
	Data      struct {
		StatisticsDataDesc string        `json:"statisticsDataDesc"`
		OrderDataList      []OrderData   `json:"orderDataList"`
		SortConfigList     []SortConfig  `json:"sortConfigList"`
		TotalCount         int           `json:"totalCount"`
		CsInspectionVersion int          `json:"csInspectionVersion"`
	} `json:"data"`
}

// OrderData 订单数据
type OrderData struct {
	OrderNo       string  `json:"orderNo"`
	CommodityName string  `json:"commodityName"`
	Price         float64 `json:"price"`
	Status        string  `json:"status"`
	CreateTime    string  `json:"createTime"`
}

// SortConfig 排序配置
type SortConfig struct {
	SortType int    `json:"sortType"`
	SortDesc string `json:"sortDesc"`
}

// SearchNewListResponse 搜索商品新接口响应 - 基于实际API响应
type SearchNewListResponse struct {
	Code int    `json:"Code"`
	Msg  string `json:"Msg"`
	Data struct {
		CommodityTemplateList []CommodityTemplate `json:"commodityTemplateList"`
		TotalCount           int                 `json:"totalCount"`
		PageIndex            int                 `json:"pageIndex"`
		PageSize             int                 `json:"pageSize"`
	} `json:"Data"`
}

// CommodityTemplate 商品模板信息 - API解析用
type CommodityTemplate struct {
	ID                    int     `json:"Id"`
	GameID                int     `json:"GameId"`
	GameName              string  `json:"GameName"`
	CommodityName         string  `json:"CommodityName"`
	CommodityHashName     string  `json:"CommodityHashName"`
	IconUrl               string  `json:"IconUrl"`
	IconUrlLarge          string  `json:"IconUrlLarge"`
	OnSaleCount           int     `json:"OnSaleCount"`
	OnLeaseCount          int     `json:"OnLeaseCount"`
	LeaseUnitPrice        string  `json:"LeaseUnitPrice"`
	LeaseDeposit          string  `json:"LeaseDeposit"`
	Price                 string  `json:"Price"`
	SteamPrice            string  `json:"SteamPrice"`
	TypeName              string  `json:"TypeName"`
	Exterior              string  `json:"Exterior"`
	ExteriorColor         string  `json:"ExteriorColor"`
	Rarity                string  `json:"Rarity"`
	RarityColor           string  `json:"RarityColor"`
	Quality               string  `json:"Quality"`
	QualityColor          string  `json:"QualityColor"`
	HaveLease             int     `json:"HaveLease"`
	Rent                  string  `json:"Rent"`
}

// ToFrontendFormat 转换为前端期望的格式
func (c *CommodityTemplate) ToFrontendFormat() map[string]interface{} {
	return map[string]interface{}{
		"id":               c.ID,
		"gameId":           c.GameID,
		"gameName":         c.GameName,
		"commodityName":    c.CommodityName,
		"commodityHashName": c.CommodityHashName,
		"iconUrl":          c.IconUrl,
		"iconUrlLarge":     c.IconUrlLarge,
		"onSaleCount":      c.OnSaleCount,
		"onLeaseCount":     c.OnLeaseCount,
		"leaseUnitPrice":   c.LeaseUnitPrice,
		"leaseDeposit":     c.LeaseDeposit,
		"price":            c.Price,
		"steamPrice":       c.SteamPrice,
		"typeName":         c.TypeName,
		"exterior":         c.Exterior,
		"exteriorColor":    c.ExteriorColor,
		"rarity":           c.Rarity,
		"rarityColor":      c.RarityColor,
		"quality":          c.Quality,
		"qualityColor":     c.QualityColor,
		"haveLease":        c.HaveLease,
		"rent":             c.Rent,
	}
}

// GetOfferStatusRequest 获取报价状态请求（基于HAR分析）
type GetOfferStatusRequest struct {
	OrderNo string `json:"orderNo" binding:"required"`
	UserId  string `json:"userId" binding:"required"`
}

// GetOfferStatusResponse 获取报价状态响应
type GetOfferStatusResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		OrderNo     string `json:"orderNo"`
		OfferStatus string `json:"offerStatus"`
		TradeOfferId string `json:"tradeOfferId"`
		Message     string `json:"message"`
	} `json:"data"`
}

// SearchCommodity 搜索商品信息
type SearchCommodity struct {
	ID                    int     `json:"id"`
	GameID                int     `json:"gameId"`
	GameName              string  `json:"gameName"`
	GameIcon              string  `json:"gameIcon"`
	CommodityName         string  `json:"commodityName"`
	CommodityHashName     string  `json:"commodityHashName"`
	IconUrl               string  `json:"iconUrl"`
	IconUrlLarge          string  `json:"iconUrlLarge"`
	OnSaleCount           int     `json:"onSaleCount"`
	OnLeaseCount          int     `json:"onLeaseCount"`
	LeaseUnitPrice        string  `json:"leaseUnitPrice"`
	LongLeaseUnitPrice    string  `json:"longLeaseUnitPrice"`
	LeaseDeposit          string  `json:"leaseDeposit"`
	Price                 string  `json:"price"`
	SteamPrice            string  `json:"steamPrice"`
	SteamUsdPrice         string  `json:"steamUsdPrice"`
	TypeName              string  `json:"typeName"`
	Exterior              string  `json:"exterior"`
	ExteriorColor         string  `json:"exteriorColor"`
	Rarity                string  `json:"rarity"`
	RarityColor           string  `json:"rarityColor"`
	Quality               string  `json:"quality"`
	QualityColor          string  `json:"qualityColor"`
	SortID                int     `json:"sortId"`
	HaveLease             int     `json:"haveLease"`
	StickersIsSort        bool    `json:"stickersIsSort"`
	SubsidyPurchase       int     `json:"subsidyPurchase"`
	Stickers              interface{} `json:"stickers"`
	Label                 interface{} `json:"label"`
	Rent                  string  `json:"rent"`
	MinLeaseDeposit       interface{} `json:"minLeaseDeposit"`
	ListType              interface{} `json:"listType"`
	TemplatePurchaseCountText interface{} `json:"templatePurchaseCountText"`
	TemplateTags          interface{} `json:"templateTags"`
}
