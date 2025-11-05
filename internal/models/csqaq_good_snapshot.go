package models

import "time"

// CSQAQGoodSnapshot stores periodic snapshots for a CSQAQ good
// to build historical series and K-line aggregations.
type CSQAQGoodSnapshot struct {
	ID     uint  `json:"id" gorm:"primaryKey"`
	GoodID int64 `json:"good_id" gorm:"index;not null"`
	// YouPin template id (for direct mapping to YYYP commodity)
	YYYPTemplateID *int64 `json:"yyyp_template_id" gorm:"index"`
	// Prefer YYYP prices as baseline; also store BUFF for comparison
	YYYPSellPrice *float64 `json:"yyyp_sell_price"`
	YYYPBuyPrice  *float64 `json:"yyyp_buy_price"`
	YYYPSellCount *int     `json:"yyyp_sell_count"` // 悠悠有品在售数量
	YYYPBuyCount  *int     `json:"yyyp_buy_count"`  // 悠悠有品求购数量
	BuffSellPrice *float64 `json:"buff_sell_price"`
	BuffBuyPrice  *float64 `json:"buff_buy_price"`
	RankNum       *int     `json:"rank_num" gorm:"index"` // 热度排名（来自CSQAQ API）
	// Source timestamp
	CreatedAt time.Time `json:"created_at" gorm:"index"`
}
