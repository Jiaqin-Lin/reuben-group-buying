package model

import "time"

// Team 拼团队伍
type Team struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement" json:"-"`
	TeamID         string    `gorm:"type:varchar(16);uniqueIndex;not null;column:team_id" json:"team_id"`
	ActivityID     int64     `gorm:"type:bigint;not null;index:idx_activity_status;column:activity_id" json:"activity_id"`
	Source         string    `gorm:"type:varchar(16);not null" json:"source"`
	Channel        string    `gorm:"type:varchar(16);not null" json:"channel"`
	OriginalPrice  string    `gorm:"type:decimal(10,2);not null;column:original_price" json:"original_price"`
	DeductionPrice string    `gorm:"type:decimal(10,2);not null;column:deduction_price" json:"deduction_price"`
	PayPrice       string    `gorm:"type:decimal(10,2);not null;column:pay_price" json:"pay_price"`
	TargetCount    int       `gorm:"type:int;not null;column:target_count" json:"target_count"`
	CompleteCount  int       `gorm:"type:int;not null;default:0;column:complete_count" json:"complete_count"`
	LockCount      int       `gorm:"type:int;not null;default:0;column:lock_count" json:"lock_count"`
	Status         int8      `gorm:"type:tinyint;not null;default:0;index:idx_activity_status;index:idx_valid_end_status" json:"status"`
	ValidStart     time.Time `gorm:"type:datetime;not null;column:valid_start" json:"valid_start"`
	ValidEnd       time.Time `gorm:"type:datetime;not null;index:idx_valid_end_status;column:valid_end" json:"valid_end"`
	NotifyType     string    `gorm:"type:varchar(8);not null;default:HTTP;column:notify_type" json:"notify_type"`
	NotifyURL      *string   `gorm:"type:varchar(512);column:notify_url" json:"notify_url"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Team) TableName() string { return "teams" }

// 团队状态常量
const (
	TeamStatusForming          = 0 // 拼团中
	TeamStatusComplete         = 1 // 已成团
	TeamStatusFailed           = 2 // 拼团失败
	TeamStatusCompleteRefunded = 3 // 已成团含退款
)

// 通知类型常量
const (
	NotifyTypeHTTP = "HTTP"
	NotifyTypeMQ   = "MQ"
)
