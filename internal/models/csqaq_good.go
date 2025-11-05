package models

import "time"

// CSQAQGood stores mappings from CSQAQ good_id to names
type CSQAQGood struct {
    ID              uint                  `json:"id" gorm:"primaryKey"`
    GoodID          int64                 `json:"good_id" gorm:"uniqueIndex;not null"`
    MarketHashName  string                `json:"market_hash_name" gorm:"index"`
    Name            string                `json:"name" gorm:"index"`
    YYYPTemplateID  *int64                `json:"yyyp_template_id" gorm:"index"` // 悠悠有品模板ID

    // 价格涨跌数据（来自上游CSQAQ API: https://api.csqaq.com/api/v1/info/good）
    SellPriceRate1d   float64             `json:"sell_price_rate_1" gorm:"default:0"`   // 1天售价涨跌率
    SellPriceRate7d   float64             `json:"sell_price_rate_7" gorm:"default:0"`   // 7天售价涨跌率
    SellPriceRate30d  float64             `json:"sell_price_rate_30" gorm:"default:0"`  // 30天售价涨跌率
    SellPriceRate180d float64             `json:"sell_price_rate_180" gorm:"default:0"` // 180天售价涨跌率

    SellPrice1d   float64                 `json:"sell_price_1" gorm:"default:0"`   // 1天售价涨跌量
    SellPrice7d   float64                 `json:"sell_price_7" gorm:"default:0"`   // 7天售价涨跌量
    SellPrice30d  float64                 `json:"sell_price_30" gorm:"default:0"`  // 30天售价涨跌量
    SellPrice180d float64                 `json:"sell_price_180" gorm:"default:0"` // 180天售价涨跌量

    LastUpdateTime  time.Time             `json:"last_update_time"`                // 上次更新涨跌数据的时间

    CreatedAt       time.Time             `json:"created_at"`
    UpdatedAt       time.Time             `json:"updated_at"`

    // Associations
    Snapshots       []CSQAQGoodSnapshot   `json:"snapshots,omitempty" gorm:"foreignKey:GoodID;references:GoodID"`
}

