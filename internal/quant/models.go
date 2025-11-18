package quant

import "time"

// GoodSnapshot 商品价格快照
type GoodSnapshot struct {
	GoodID        int64
	YYYPSellPrice float64
	YYYPBuyPrice  float64
	YYYPSellCount int
	YYYPBuyCount  int
	CreatedAt     time.Time
}

// GoodTimeSeries 商品时间序列数据
type GoodTimeSeries struct {
	GoodID    int64
	GoodName  string
	Snapshots []GoodSnapshot
}

// TradeSignal 交易信号
type TradeSignal struct {
	GoodID              int64
	GoodName            string
	CurrentBuyPrice     float64
	CurrentSellPrice    float64
	PredictedPrice7d    float64
	PredictedProfitRate float64
	ConfidenceScore     float64
	SignalStrength      float64
	TrendScore          float64
	SpreadScore         float64
	LiquidityScore      float64
	Volatility          float64
	RecommendedQuantity int
	MaxInvestment       float64
	Reason              string
	StrategyVersion     string
	SignalTime          time.Time
	ExpiresAt           time.Time
}

// TradeRecord 交易记录
type TradeRecord struct {
	ID                   int
	GoodID               int64
	GoodName             string
	BuyPrice             float64
	BuyTime              time.Time
	SellPrice            *float64
	SellTime             *time.Time
	ActualProfitRate     *float64
	PredictedProfitRate  float64
	StrategyVersion      string
	Status               string // holding, sold, cancelled
	CreatedAt            time.Time
}

// StrategyConfig 策略配置
type StrategyConfig struct {
	// 回测参数
	TrainDays int // 训练期天数，默认30
	TestDays  int // 测试期天数，默认7

	// 信号阈值
	TrendThreshold     float64 // 趋势得分阈值，默认0.6
	SpreadThreshold    float64 // 价差得分阈值，默认0.5
	LiquidityThreshold float64 // 流动性阈值，默认0.7

	// 收益要求
	MinProfitRate float64 // 最小预期收益率，默认3%

	// 风险控制
	MaxVolatility   float64 // 最大波动率，默认15%
	MaxDrawdown     float64 // 最大回撤，默认5%
	MaxPositionSize float64 // 单笔最大金额，默认1000

	// 仓位控制
	MaxItemsPerGood           int     // 单个饰品最多持有件数，默认3
	DefaultQuantity           int     // 默认购买数量，默认1
	HighConfidenceThreshold   float64 // 高置信度阈值，默认0.95
	HighConfidenceMinProfit   float64 // 高置信度最小收益率，默认5%
	MaxMultipleQuantity       int     // 高置信度最多购买数量，默认3

	// 冷却期和费率
	CooldownDays int     // 冷却期天数，固定7
	FeeRate      float64 // 手续费率，固定1%（0.01）
}

// DefaultStrategyConfig 默认配置
func DefaultStrategyConfig() *StrategyConfig {
	return &StrategyConfig{
		TrainDays:                 30,
		TestDays:                  7,
		TrendThreshold:            0.6,
		SpreadThreshold:           0.5,
		LiquidityThreshold:        0.7,
		MinProfitRate:             3.0,
		MaxVolatility:             15.0,
		MaxDrawdown:               5.0,
		MaxPositionSize:           1000.0,
		MaxItemsPerGood:           3,
		DefaultQuantity:           1,
		HighConfidenceThreshold:   0.95,
		HighConfidenceMinProfit:   5.0,
		MaxMultipleQuantity:       3,
		CooldownDays:              7,
		FeeRate:                   0.01,
	}
}

// BacktestResult 回测结果
type BacktestResult struct {
	TotalTrades      int
	WinningTrades    int
	LosingTrades     int
	WinRate          float64
	TotalReturn      float64
	TotalReturnRate  float64
	SharpeRatio      float64
	MaxDrawdown      float64
	AvgHoldingDays   float64
	TradeDetails     []BacktestTrade
	ProfitsByGood    map[int64]float64
}

// BacktestTrade 回测交易记录
type BacktestTrade struct {
	GoodID         int64
	GoodName       string
	BuyTime        time.Time
	BuyPrice       float64
	SellTime       time.Time
	SellPrice      float64
	ProfitRate     float64
	NetProfit      float64
	HoldingDays    int
	SignalStrength float64
}

// StrategyVersion 策略版本
type StrategyVersion struct {
	ID                   int
	Version              string
	Config               *StrategyConfig
	MLModelPath          string
	Status               string // training, validating, active, archived
	BacktestMetrics      *BacktestResult
	ValidationStartTime  *time.Time
	CreatedAt            time.Time
	ActivatedAt          *time.Time
}

// LearningLog 学习日志
type LearningLog struct {
	ID                   int
	LearningTime         time.Time
	DataRange            string
	BestParams           map[string]interface{}
	BacktestSharpeRatio  float64
	BacktestReturnRate   float64
	BacktestWinRate      float64
	Status               string // success, failed
	ErrorMessage         string
	CreatedAt            time.Time
}

// Features 特征数据
type Features struct {
	// 价格特征
	CurrentPrice   float64
	MA24h          float64
	MA72h          float64
	MA168h         float64
	EMA24h         float64
	Volatility     float64
	PriceChange1d  float64
	PriceChange7d  float64

	// 趋势特征
	TrendSlope     float64
	TrendStrength  float64

	// 价差特征
	Spread         float64
	SpreadRatio    float64
	AvgSpread      float64
	SpreadDeviation float64

	// 流动性特征
	SellCount      int
	BuyCount       int
	PriceChangFreq float64
}

