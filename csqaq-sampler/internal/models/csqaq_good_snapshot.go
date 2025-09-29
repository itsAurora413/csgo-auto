package models

import "time"

// CSQAQGoodSnapshot stores periodic snapshots for a CSQAQ good
// to build historical series and K-line aggregations.
type CSQAQGoodSnapshot struct {
    ID             uint      `json:"id" gorm:"primaryKey"`
    GoodID         int64     `json:"good_id" gorm:"index;not null"`
    // Prefer YYYP prices as baseline; also store BUFF for comparison
    YYYPSellPrice  *float64  `json:"yyyp_sell_price"`
    YYYPBuyPrice   *float64  `json:"yyyp_buy_price"`
    BuffSellPrice  *float64  `json:"buff_sell_price"`
    BuffBuyPrice   *float64  `json:"buff_buy_price"`
    // Source timestamp
    CreatedAt      time.Time `json:"created_at" gorm:"index"`
}

