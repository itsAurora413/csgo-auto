package models

import "time"

// CSQAQGood stores mappings from CSQAQ good_id to names
type CSQAQGood struct {
    ID              uint                  `json:"id" gorm:"primaryKey"`
    GoodID          int64                 `json:"good_id" gorm:"uniqueIndex;not null"`
    MarketHashName  string                `json:"market_hash_name" gorm:"index"`
    Name            string                `json:"name" gorm:"index"`
    CreatedAt       time.Time             `json:"created_at"`
    UpdatedAt       time.Time             `json:"updated_at"`

    // Associations
    Snapshots       []CSQAQGoodSnapshot   `json:"snapshots,omitempty" gorm:"foreignKey:GoodID;references:GoodID"`
}

