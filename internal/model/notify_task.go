package model

import "time"

// NotifyTask 回调通知任务
type NotifyTask struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement" json:"-"`
	ActivityID   int64     `gorm:"type:bigint;not null;column:activity_id" json:"activity_id"`
	TeamID       string    `gorm:"type:varchar(16);not null;column:team_id" json:"team_id"`
	Category     *string   `gorm:"type:varchar(64);column:category" json:"category"`
	NotifyType   string    `gorm:"type:varchar(8);not null;default:HTTP;column:notify_type" json:"notify_type"`
	NotifyTarget *string   `gorm:"type:varchar(512);column:notify_target" json:"notify_target"`
	RetryCount   int       `gorm:"type:int;not null;default:0;column:retry_count" json:"retry_count"`
	Status       int8      `gorm:"type:tinyint;not null;default:0;index:idx_status_created" json:"status"`
	Payload      string    `gorm:"type:text;not null" json:"payload"`
	UUID         string    `gorm:"type:varchar(128);uniqueIndex;not null;column:uuid" json:"uuid"`
	CreatedAt    time.Time `gorm:"autoCreateTime;index:idx_status_created" json:"created_at"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (NotifyTask) TableName() string { return "notify_tasks" }

// 通知任务状态常量
const (
	NotifyStatusInit    = 0 // 待发送
	NotifyStatusSuccess = 1 // 成功
	NotifyStatusRetry   = 2 // 重试中
	NotifyStatusFail    = 3 // 失败（达上限）
)

// 通知分类常量
const (
	NotifyCategorySettlement     = "trade_settlement"       // 成团结算
	NotifyCategoryUnpaidRefund   = "trade_unpaid_refund"    // 未支付退单
	NotifyCategoryPaidRefund     = "trade_paid_refund"      // 已支付未成团退单
	NotifyCategoryPaidTeamRefund = "trade_paid_team_refund" // 已成团退单
)
