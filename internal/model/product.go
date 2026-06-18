package model

import "time"

// Product 商品
type Product struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement" json:"-"`
	GoodsID       string    `gorm:"type:varchar(32);uniqueIndex;not null;column:goods_id" json:"goods_id"`
	GoodsName     string    `gorm:"type:varchar(256);not null;column:goods_name" json:"goods_name"`
	OriginalPrice string    `gorm:"type:decimal(10,2);not null;column:original_price" json:"original_price"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Product) TableName() string { return "products" }
