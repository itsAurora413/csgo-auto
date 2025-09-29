package models

import "time"

// SteamCredentials stores Steam authentication parameters similar to Steamauto
type SteamCredentials struct {
    ID             uint      `json:"id" gorm:"primaryKey"`
    UserID         uint      `json:"user_id" gorm:"not null"`
    SharedSecret   string    `json:"shared_secret" gorm:"type:text"`
    IdentitySecret string    `json:"identity_secret" gorm:"type:text"`
    SteamUsername  string    `json:"steam_username"`
    SteamPassword  string    `json:"steam_password" gorm:"type:text"`
    APIKey         string    `json:"api_key" gorm:"type:text"`
    CreatedAt      time.Time `json:"created_at"`
    UpdatedAt      time.Time `json:"updated_at"`
}
