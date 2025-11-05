package models

import (
	"time"

	"gorm.io/gorm"
)

// User represents a user in the system
type User struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	SteamID     string         `json:"steam_id" gorm:"unique;not null"`
	Username    string         `json:"username"`
	Avatar      string         `json:"avatar"`
	AccessToken string         `json:"-"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

// Item represents a CSGO item
type Item struct {
	ID         uint           `json:"id" gorm:"primaryKey"`
	Name       string         `json:"name" gorm:"not null"`
	MarketName string         `json:"market_name" gorm:"unique;not null"`
	IconURL    string         `json:"icon_url"`
	Type       string         `json:"type"`
	Weapon     string         `json:"weapon"`
	Exterior   string         `json:"exterior"`
	Rarity     string         `json:"rarity"`
	Collection string         `json:"collection"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}

// Price represents price data for an item on different platforms
type Price struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	ItemID    uint      `json:"item_id" gorm:"not null"`
	Item      Item      `json:"item" gorm:"foreignKey:ItemID"`
	Platform  string    `json:"platform" gorm:"not null"` // steam, buff, youpin
	Price     float64   `json:"price"`
	Volume    int       `json:"volume"`
	Currency  string    `json:"currency" gorm:"default:'USD'"`
	Timestamp time.Time `json:"timestamp"`
	CreatedAt time.Time `json:"created_at"`
}

// Trade represents a trading transaction
type Trade struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	UserID    uint           `json:"user_id" gorm:"not null"`
	User      User           `json:"user" gorm:"foreignKey:UserID"`
	ItemID    uint           `json:"item_id" gorm:"not null"`
	Item      Item           `json:"item" gorm:"foreignKey:ItemID"`
	Platform  string         `json:"platform" gorm:"not null"`
	Type      string         `json:"type" gorm:"not null"` // buy, sell
	Price     float64        `json:"price"`
	Quantity  int            `json:"quantity" gorm:"default:1"`
	Status    string         `json:"status" gorm:"default:'pending'"` // pending, completed, failed, cancelled
	TradeID   string         `json:"trade_id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// Strategy represents a trading strategy
type Strategy struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	UserID      uint           `json:"user_id" gorm:"not null"`
	User        User           `json:"user" gorm:"foreignKey:UserID"`
	Name        string         `json:"name" gorm:"not null"`
	Description string         `json:"description"`
	ItemID      uint           `json:"item_id"`
	Item        Item           `json:"item" gorm:"foreignKey:ItemID"`
	BuyPrice    float64        `json:"buy_price"`
	SellPrice   float64        `json:"sell_price"`
	MaxQuantity int            `json:"max_quantity" gorm:"default:1"`
	IsActive    bool           `json:"is_active" gorm:"default:false"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

// Inventory represents user's inventory items
type Inventory struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"user_id" gorm:"not null"`
	User      User      `json:"user" gorm:"foreignKey:UserID"`
	ItemID    uint      `json:"item_id" gorm:"not null"`
	Item      Item      `json:"item" gorm:"foreignKey:ItemID"`
	Platform  string    `json:"platform" gorm:"not null"`
	AssetID   string    `json:"asset_id"`
	Quantity  int       `json:"quantity" gorm:"default:1"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MarketTrend represents market trend analysis
type MarketTrend struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	ItemID         uint      `json:"item_id" gorm:"not null"`
	Item           Item      `json:"item" gorm:"foreignKey:ItemID"`
	Platform       string    `json:"platform" gorm:"not null"`
	TrendDirection string    `json:"trend_direction"` // up, down, stable
	PriceChange    float64   `json:"price_change"`
	VolumeChange   float64   `json:"volume_change"`
	Confidence     float64   `json:"confidence"`
	AnalysisDate   time.Time `json:"analysis_date"`
	CreatedAt      time.Time `json:"created_at"`
}

// ForecastRecord stores a single forecast output for auditing
type ForecastRecord struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	IndexID     string    `json:"index_id" gorm:"index;not null"`
	Interval    string    `json:"interval" gorm:"index;not null"`
	HorizonDays int       `json:"horizon_days" gorm:"index;not null"`
	Predicted   float64   `json:"predicted"`
	Method      string    `json:"method" gorm:"index"`
	TrainWindow int       `json:"train_window"`
	Slope       float64   `json:"slope"`
	Intercept   float64   `json:"intercept"`
	BarsPerDay  float64   `json:"bars_per_day"`
	LastClose   float64   `json:"last_close"`
	DataPoints  int       `json:"data_points"`
	CreatedAt   time.Time `json:"created_at"`
}

// ForecastBacktest summarizes backtest accuracy for a given run
type ForecastBacktest struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	IndexID     string    `json:"index_id" gorm:"index;not null"`
	Interval    string    `json:"interval" gorm:"index;not null"`
	Method      string    `json:"method" gorm:"index"`
	TrainWindow int       `json:"train_window"`
	HorizonDays int       `json:"horizon_days" gorm:"index;not null"`
	Points      int       `json:"points"`
	MAPE        float64   `json:"mape"`
	MAE         float64   `json:"mae"`
	RMSE        float64   `json:"rmse"`
	CreatedAt   time.Time `json:"created_at"`
}

// YouPinAccount represents YouPin account information
type YouPinAccount struct {
	ID              uint           `json:"id" gorm:"primaryKey"`
	UserID          uint           `json:"user_id" gorm:"not null"`
	User            User           `json:"user" gorm:"foreignKey:UserID"`
	Token           string         `json:"token" gorm:"not null"`
	Nickname        string         `json:"nickname"`
	Phone           string         `json:"phone"`
	Balance         float64        `json:"balance"`
	PurchaseBalance float64        `json:"purchase_balance"`
	IsActive        bool           `json:"is_active" gorm:"default:true"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index"`
}

// YouPinOrder represents YouPin order information
type YouPinOrder struct {
	ID             uint           `json:"id" gorm:"primaryKey"`
	UserID         uint           `json:"user_id" gorm:"not null"`
	User           User           `json:"user" gorm:"foreignKey:UserID"`
	OrderID        string         `json:"order_id" gorm:"unique;not null"`
	OfferID        string         `json:"offer_id"`
	ItemID         uint           `json:"item_id"`
	Item           Item           `json:"item" gorm:"foreignKey:ItemID"`
	ItemName       string         `json:"item_name"`
	Price          float64        `json:"price"`
	Status         string         `json:"status"`
	OrderType      string         `json:"order_type"` // sell, buy, lease
	BuyerNickname  string         `json:"buyer_nickname"`
	SellerNickname string         `json:"seller_nickname"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

// YouPinConfig represents YouPin configuration
type YouPinConfig struct {
	ID                       uint           `json:"id" gorm:"primaryKey"`
	UserID                   uint           `json:"user_id" gorm:"not null"`
	User                     User           `json:"user" gorm:"foreignKey:UserID"`
	AutoSellEnabled          bool           `json:"auto_sell_enabled" gorm:"default:false"`
	AutoBuyEnabled           bool           `json:"auto_buy_enabled" gorm:"default:false"`
	SellItemNames            string         `json:"sell_item_names" gorm:"type:text"` // JSON array stored as string
	BlacklistWords           string         `json:"blacklist_words" gorm:"type:text"` // JSON array stored as string
	MaxSalePrice             float64        `json:"max_sale_price"`
	TakeProfileEnabled       bool           `json:"take_profile_enabled" gorm:"default:false"`
	TakeProfileRatio         float64        `json:"take_profile_ratio"`
	UsePriceAdjustment       bool           `json:"use_price_adjustment" gorm:"default:true"`
	PriceAdjustmentThreshold float64        `json:"price_adjustment_threshold"`
	RunTime                  string         `json:"run_time"`
	Interval                 int            `json:"interval"`
	CreatedAt                time.Time      `json:"created_at"`
	UpdatedAt                time.Time      `json:"updated_at"`
	DeletedAt                gorm.DeletedAt `gorm:"index"`
}

// YouPinItem represents YouPin inventory item
type YouPinItem struct {
	ID            uint           `json:"id" gorm:"primaryKey"`
	UserID        uint           `json:"user_id" gorm:"not null"`
	User          User           `json:"user" gorm:"foreignKey:UserID"`
	SteamAssetID  string         `json:"steam_asset_id" gorm:"not null"`
	TemplateID    string         `json:"template_id"`
	ItemID        uint           `json:"item_id"`
	Item          Item           `json:"item" gorm:"foreignKey:ItemID"`
	CommodityName string         `json:"commodity_name"`
	Price         float64        `json:"price"`
	MarketPrice   float64        `json:"market_price"`
	AssetBuyPrice float64        `json:"asset_buy_price"`
	Tradable      bool           `json:"tradable"`
	AssetStatus   int            `json:"asset_status"`
	IsOnSale      bool           `json:"is_on_sale" gorm:"default:false"`
	IsOnLease     bool           `json:"is_on_lease" gorm:"default:false"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index"`
}

// YouPinBuyOrder represents YouPin buy order (求购)
type YouPinBuyOrder struct {
	ID         uint           `json:"id" gorm:"primaryKey"`
	UserID     uint           `json:"user_id" gorm:"not null"`
	User       User           `json:"user" gorm:"foreignKey:UserID"`
	OrderID    string         `json:"order_id" gorm:"unique;not null"`
	TemplateID string         `json:"template_id"`
	ItemID     uint           `json:"item_id"`
	Item       Item           `json:"item" gorm:"foreignKey:ItemID"`
	ItemName   string         `json:"item_name"`
	Price      float64        `json:"price"`
	Quantity   int            `json:"quantity"`
	Status     string         `json:"status"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}

// YouPinPriceSnapshot 悠品价格快照 - 用于记录商品的历史价格
type YouPinPriceSnapshot struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	TemplateID      int       `json:"template_id" gorm:"not null;index"`
	CommodityName   string    `json:"commodity_name"`
	HighestBuyPrice float64   `json:"highest_buy_price"` // 最高求购价
	LowestSellPrice float64   `json:"lowest_sell_price"` // 最低在售价
	BuyOrderCount   int       `json:"buy_order_count"`   // 求购订单数
	SellOrderCount  int       `json:"sell_order_count"`  // 在售订单数
	SnapshotTime    time.Time `json:"snapshot_time" gorm:"index"`
	CreatedAt       time.Time `json:"created_at"`
}

// ArbitrageOpportunity 套利机会分析结果
type ArbitrageOpportunity struct {
	ID                  uint      `json:"id" gorm:"primaryKey"`
	GoodID              int64     `json:"good_id" gorm:"index;not null"`
	GoodName            string    `json:"good_name"`
	CurrentBuyPrice     float64   `json:"current_buy_price"`        // 当前悠悠有品求购价
	CurrentSellPrice    float64   `json:"current_sell_price"`       // 当前悠悠有品售价
	ProfitRate          float64   `json:"profit_rate" gorm:"index"` // 预期利润率
	EstimatedProfit     float64   `json:"estimated_profit"`         // 预期利润金额
	AvgBuyPrice7d       float64   `json:"avg_buy_price_7d"`         // 7天平均求购价
	AvgSellPrice7d      float64   `json:"avg_sell_price_7d"`        // 7天平均售价
	PriceTrend          string    `json:"price_trend"`              // 价格趋势: up/down/stable
	DaysOfData          int       `json:"days_of_data"`             // 拥有多少天的历史数据
	RiskLevel           string    `json:"risk_level" gorm:"index"`  // 风险等级: low/medium/high
	BuyOrderCount       int       `json:"buy_order_count"`          // 求购订单数量
	SellOrderCount      int       `json:"sell_order_count"`         // 在售订单数量
	RankNum             *int      `json:"rank_num"`                 // 热度排名（来自 /info/good 接口）
	RecommendedBuyPrice float64   `json:"recommended_buy_price"`    // 推荐求购价格（略高于当前最高求购）
	RecommendedQuantity int       `json:"recommended_quantity"`
	Score               float64   `json:"score" gorm:"index"`         // 综合评分（0-100分，量化评估模型）
	AnalysisTime        time.Time `json:"analysis_time" gorm:"index"` // 分析时间
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// ArbitrageOpportunityHistory 套利机会历史归档
type ArbitrageOpportunityHistory struct {
	ID                  uint      `json:"id" gorm:"primaryKey"`
	GoodID              int64     `json:"good_id" gorm:"index;not null"`
	GoodName            string    `json:"good_name"`
	CurrentBuyPrice     float64   `json:"current_buy_price"`
	CurrentSellPrice    float64   `json:"current_sell_price"`
	ProfitRate          float64   `json:"profit_rate" gorm:"index"`
	EstimatedProfit     float64   `json:"estimated_profit"`
	AvgBuyPrice7d       float64   `json:"avg_buy_price_7d"`
	AvgSellPrice7d      float64   `json:"avg_sell_price_7d"`
	PriceTrend          string    `json:"price_trend"`
	DaysOfData          int       `json:"days_of_data"`
	RiskLevel           string    `json:"risk_level" gorm:"index"`
	BuyOrderCount       int       `json:"buy_order_count"`
	SellOrderCount      int       `json:"sell_order_count"`
	RankNum             *int      `json:"rank_num"` // 热度排名（来自 /info/good 接口）
	RecommendedBuyPrice float64   `json:"recommended_buy_price"`
	RecommendedQuantity int       `json:"recommended_quantity"`
	Score               float64   `json:"score" gorm:"index"`
	AnalysisTime        time.Time `json:"analysis_time" gorm:"index"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (ArbitrageOpportunityHistory) TableName() string { return "arbitrage_opportunities_history" }

// PurchasePlan 求购计划/清单
type PurchasePlan struct {
	ID         uint               `json:"id" gorm:"primaryKey"`
	Budget     float64            `json:"budget"`                          // 总预算
	TotalItems int                `json:"total_items"`                     // 总件数
	TotalCost  float64            `json:"total_cost"`                      // 实际花费
	Status     string             `json:"status" gorm:"default:'pending'"` // pending/completed/cancelled
	Items      []PurchasePlanItem `json:"items" gorm:"foreignKey:PlanID"`
	CreatedAt  time.Time          `json:"created_at"`
	UpdatedAt  time.Time          `json:"updated_at"`
}

// PurchasePlanItem 求购计划中的饰品明细
type PurchasePlanItem struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	PlanID         uint      `json:"plan_id" gorm:"index;not null"` // 关联的计划ID
	GoodID         int64     `json:"good_id"`                       // CSQAQ商品ID (仅用于记录)
	YYYPTemplateID *int64    `json:"yyyp_template_id" gorm:"index"` // 悠悠有品模板ID (用于实际求购)
	GoodName       string    `json:"good_name"`                     // 商品名称
	BuyPrice       float64   `json:"buy_price"`                     // 求购价格
	Quantity       int       `json:"quantity"`                      // 求购数量
	Subtotal       float64   `json:"subtotal"`                      // 小计
	ProfitRate     float64   `json:"profit_rate"`                   // 预期利润率
	RiskLevel      string    `json:"risk_level"`                    // 风险等级
	CreatedAt      time.Time `json:"created_at"`
}

// HoldingPosition 持仓记录（用于止损/止盈追踪）
type HoldingPosition struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	GoodID         int64     `json:"good_id" gorm:"index;not null"`          // CSQAQ商品ID
	YYYPTemplateID int64     `json:"yyyp_template_id" gorm:"index;not null"` // 悠悠有品模板ID
	CommodityID    int64     `json:"commodity_id" gorm:"index"`              // 悠悠有品在售商品ID（用于改价/下架）
	GoodName       string    `json:"good_name"`                              // 商品名称
	BuyPrice       float64   `json:"buy_price"`                              // 单件买入价
	BuyQuantity    int       `json:"buy_quantity"`                           // 买入数量
	BuyTime        time.Time `json:"buy_time" gorm:"index"`                  // 买入时间
	CurrentPrice   float64   `json:"current_price"`                          // 当前价格
	TargetProfit   float64   `json:"target_profit"`                          // 目标利润率（如0.08表示8%）
	MaxLoss        float64   `json:"max_loss"`                               // 最大亏损率（如-0.10表示-10%）
	Status         string    `json:"status" gorm:"index"`                    // holding/partial_sold/fully_sold/stop_loss
	SoldPrice      float64   `json:"sold_price"`                             // 卖出价
	SoldTime       time.Time `json:"sold_time"`                              // 卖出时间
	SoldQuantity   int       `json:"sold_quantity"`                          // 已卖出数量
	RealizedProfit float64   `json:"realized_profit"`                        // 已实现利润（元）
	DaysHeld       int       `json:"days_held"`                              // 持仓天数
	RiskLevel      string    `json:"risk_level"`                             // 风险等级：low/medium/high
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
