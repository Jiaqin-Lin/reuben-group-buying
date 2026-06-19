package model

import "time"

// Discount 折扣规则
type Discount struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement" json:"-"`
	DiscountID   string    `gorm:"type:varchar(16);uniqueIndex;not null;column:discount_id" json:"discount_id"`
	Name         string    `gorm:"type:varchar(64);not null" json:"name"`
	Description  string    `gorm:"type:varchar(256);not null;default:''" json:"description"`
	PlanType     string    `gorm:"type:varchar(4);not null;default:ZJ;column:plan_type" json:"plan_type"`     // ZJ=直减 MJ=满减 ZK=折扣 N=N元购
	Expression   string    `gorm:"type:varchar(32);not null;column:expression" json:"expression"`             // 计算表达式
	DiscountType int8      `gorm:"type:tinyint;not null;default:0;column:discount_type" json:"discount_type"` // 0=基础 1=人群标签
	TagID        *string   `gorm:"type:varchar(32);column:tag_id" json:"tag_id"`                              // 人群标签ID（nullable）
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Discount) TableName() string { return "discounts" }

// 折扣计划类型常量
const (
	PlanZJ = "ZJ" // 直减
	PlanMJ = "MJ" // 满减
	PlanZK = "ZK" // 折扣
	PlanN  = "N"  // N元购
)

// 折扣类型常量
const (
	DiscountTypeBase = 0 // 基础折扣
	DiscountTypeTag  = 1 // 人群标签折扣
)
