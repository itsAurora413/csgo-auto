package models

import "time"

// YouPinAccount represents a YouPin (悠悠有品) user account with API token
type YouPinAccount struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	UserID           uint      `json:"user_id" gorm:"not null"`
	Token            string    `json:"token" gorm:"not null"`
	Nickname         string    `json:"nickname"`
	Phone            string    `json:"phone"`
	Balance          float64   `json:"balance"`
	PurchaseBalance  float64   `json:"purchase_balance"`
	IsActive         bool      `json:"is_active" gorm:"default:true"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// TableName specifies the table name for YouPinAccount
func (YouPinAccount) TableName() string {
	return "youpin_accounts"
}
