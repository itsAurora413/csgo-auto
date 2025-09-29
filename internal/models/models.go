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
	ID          uint           `json:"id" gorm:"primaryKey"`
	Name        string         `json:"name" gorm:"not null"`
	MarketName  string         `json:"market_name" gorm:"unique;not null"`
	IconURL     string         `json:"icon_url"`
	Type        string         `json:"type"`
	Weapon      string         `json:"weapon"`
	Exterior    string         `json:"exterior"`
	Rarity      string         `json:"rarity"`
	Collection  string         `json:"collection"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

// Price represents price data for an item on different platforms
type Price struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	ItemID     uint      `json:"item_id" gorm:"not null"`
	Item       Item      `json:"item" gorm:"foreignKey:ItemID"`
	Platform   string    `json:"platform" gorm:"not null"` // steam, buff, youpin
	Price      float64   `json:"price"`
	Volume     int       `json:"volume"`
	Currency   string    `json:"currency" gorm:"default:'USD'"`
	Timestamp  time.Time `json:"timestamp"`
	CreatedAt  time.Time `json:"created_at"`
}

// Trade represents a trading transaction
type Trade struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	UserID      uint           `json:"user_id" gorm:"not null"`
	User        User           `json:"user" gorm:"foreignKey:UserID"`
	ItemID      uint           `json:"item_id" gorm:"not null"`
	Item        Item           `json:"item" gorm:"foreignKey:ItemID"`
	Platform    string         `json:"platform" gorm:"not null"`
	Type        string         `json:"type" gorm:"not null"` // buy, sell
	Price       float64        `json:"price"`
	Quantity    int            `json:"quantity" gorm:"default:1"`
	Status      string         `json:"status" gorm:"default:'pending'"` // pending, completed, failed, cancelled
	TradeID     string         `json:"trade_id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
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
	ID              uint      `json:"id" gorm:"primaryKey"`
	ItemID          uint      `json:"item_id" gorm:"not null"`
	Item            Item      `json:"item" gorm:"foreignKey:ItemID"`
	Platform        string    `json:"platform" gorm:"not null"`
	TrendDirection  string    `json:"trend_direction"` // up, down, stable
	PriceChange     float64   `json:"price_change"`
	VolumeChange    float64   `json:"volume_change"`
	Confidence      float64   `json:"confidence"`
	AnalysisDate    time.Time `json:"analysis_date"`
	CreatedAt       time.Time `json:"created_at"`
}

// ForecastRecord stores a single forecast output for auditing
type ForecastRecord struct {
    ID           uint      `json:"id" gorm:"primaryKey"`
    IndexID      string    `json:"index_id" gorm:"index;not null"`
    Interval     string    `json:"interval" gorm:"index;not null"`
    HorizonDays  int       `json:"horizon_days" gorm:"index;not null"`
    Predicted    float64   `json:"predicted"`
    Method       string    `json:"method" gorm:"index"`
    TrainWindow  int       `json:"train_window"`
    Slope        float64   `json:"slope"`
    Intercept    float64   `json:"intercept"`
    BarsPerDay   float64   `json:"bars_per_day"`
    LastClose    float64   `json:"last_close"`
    DataPoints   int       `json:"data_points"`
    CreatedAt    time.Time `json:"created_at"`
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
	ID          uint           `json:"id" gorm:"primaryKey"`
	UserID      uint           `json:"user_id" gorm:"not null"`
	User        User           `json:"user" gorm:"foreignKey:UserID"`
	Token       string         `json:"token" gorm:"not null"`
	Nickname    string         `json:"nickname"`
	Phone       string         `json:"phone"`
    Balance     float64        `json:"balance"`
    PurchaseBalance float64     `json:"purchase_balance"`
	IsActive    bool           `json:"is_active" gorm:"default:true"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
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
	ID           uint           `json:"id" gorm:"primaryKey"`
	UserID       uint           `json:"user_id" gorm:"not null"`
	User         User           `json:"user" gorm:"foreignKey:UserID"`
	OrderID      string         `json:"order_id" gorm:"unique;not null"`
	TemplateID   string         `json:"template_id"`
	ItemID       uint           `json:"item_id"`
	Item         Item           `json:"item" gorm:"foreignKey:ItemID"`
	ItemName     string         `json:"item_name"`
	Price        float64        `json:"price"`
	Quantity     int            `json:"quantity"`
	Status       string         `json:"status"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}
