package model

import "time"

// DynamicConfig 动态配置。
// MySQL 存储，Redis Pub/Sub 通知跨实例同步，本地 atomic.Value 热读。
type DynamicConfig struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"-"`
	ConfigKey   string    `gorm:"type:varchar(128);uniqueIndex;not null;column:config_key" json:"config_key"`
	ConfigValue string    `gorm:"type:text;not null;column:config_value" json:"config_value"` // JSON 值
	Version     uint      `gorm:"type:int unsigned;not null;default:1" json:"version"`
	UpdatedBy   string    `gorm:"type:varchar(64);not null;default:'';column:updated_by" json:"updated_by"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (DynamicConfig) TableName() string { return "dynamic_configs" }
