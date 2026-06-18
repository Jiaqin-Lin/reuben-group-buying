package model

import "time"

// ActivityProduct 商品-活动映射（试算入口，source+channel 唯一查询维度）
type ActivityProduct struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"-"`
	Source     string    `gorm:"type:varchar(16);uniqueIndex:uk_source_channel_goods;not null" json:"source"`
	Channel    string    `gorm:"type:varchar(16);uniqueIndex:uk_source_channel_goods;not null" json:"channel"`
	GoodsID    string    `gorm:"type:varchar(32);uniqueIndex:uk_source_channel_goods;not null;column:goods_id" json:"goods_id"`
	ActivityID int64     `gorm:"type:bigint;not null;index;column:activity_id" json:"activity_id"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (ActivityProduct) TableName() string { return "activity_products" }
