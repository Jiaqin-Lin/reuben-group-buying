package model

import "time"

// PaymentLog 支付回调日志（调试/审计用）
type PaymentLog struct {
	ID          uint64     `gorm:"primaryKey;autoIncrement" json:"-"`
	OrderID     string     `gorm:"type:varchar(16);not null;index;column:order_id" json:"order_id"`
	NotifyID    string     `gorm:"type:varchar(128);uniqueIndex;not null;column:notify_id" json:"notify_id"`
	NotifyRaw   string     `gorm:"type:text;not null;column:notify_raw" json:"notify_raw"`
	Status      int8       `gorm:"type:tinyint;not null;default:0" json:"status"`
	TradeStatus *string    `gorm:"type:varchar(32);column:trade_status" json:"trade_status"`
	VerifiedAt  *time.Time `gorm:"type:datetime;column:verified_at" json:"verified_at"`
	CreatedAt   time.Time  `gorm:"autoCreateTime" json:"created_at"`
}

func (PaymentLog) TableName() string { return "payment_logs" }

// 回调验证状态常量
const (
	PayLogStatusUnverified = 0 // 未验证
	PayLogStatusPassed     = 1 // 验签通过
	PayLogStatusFailed     = 2 // 验签失败
)
