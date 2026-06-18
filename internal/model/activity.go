package model

import "time"

// Activity 拼团活动
type Activity struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement" json:"-"`
	ActivityID   int64     `gorm:"type:bigint;uniqueIndex;not null;column:activity_id" json:"activity_id"`
	Name         string    `gorm:"type:varchar(128);not null" json:"name"`
	DiscountID   string    `gorm:"type:varchar(16);not null;column:discount_id" json:"discount_id"`
	GroupType    int8      `gorm:"type:tinyint;not null;default:0;column:group_type" json:"group_type"`
	TargetCount  int       `gorm:"type:int;not null;default:1;column:target_count" json:"target_count"`
	TakeLimit    int       `gorm:"type:int;not null;default:1;column:take_limit" json:"take_limit"`
	ValidMinutes int       `gorm:"type:int;not null;default:15;column:valid_minutes" json:"valid_minutes"`
	Status       int8      `gorm:"type:tinyint;not null;default:0" json:"status"`
	StartTime    time.Time `gorm:"type:datetime;not null;column:start_time" json:"start_time"`
	EndTime      time.Time `gorm:"type:datetime;not null;column:end_time" json:"end_time"`
	TagID        *string   `gorm:"type:varchar(32);column:tag_id" json:"tag_id"`
	TagScope     *string   `gorm:"type:varchar(4);column:tag_scope" json:"tag_scope"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Activity) TableName() string { return "activities" }

// 活动状态常量
const (
	ActivityStatusCreated   = 0 // 创建
	ActivityStatusActive    = 1 // 生效
	ActivityStatusExpired   = 2 // 过期
	ActivityStatusAbandoned = 3 // 废弃
)

// 成团类型常量
const (
	GroupTypeAuto   = 0 // 自动成团
	GroupTypeTarget = 1 // 目标拼团
)
