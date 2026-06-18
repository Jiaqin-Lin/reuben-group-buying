package model

import "time"

// Payment 支付单
// order_id 是发给支付宝的商家订单号（= orders.order_id）
// trade_no 是支付宝返回的交易流水号（回调时回填）
type Payment struct {
	ID         uint64     `gorm:"primaryKey;autoIncrement" json:"-"`
	OrderID    string     `gorm:"type:varchar(16);uniqueIndex;not null;column:order_id" json:"order_id"`
	OutTradeNo string     `gorm:"type:varchar(64);not null;index;column:out_trade_no" json:"out_trade_no"`
	UserID     string     `gorm:"type:varchar(64);not null;column:user_id" json:"user_id"`
	TeamID     string     `gorm:"type:varchar(16);not null;column:team_id" json:"team_id"`
	Amount     string     `gorm:"type:decimal(10,2);not null" json:"amount"`
	Subject    string     `gorm:"type:varchar(256);not null" json:"subject"`
	TradeNo    *string    `gorm:"type:varchar(64);column:trade_no" json:"trade_no"`
	Status     int8       `gorm:"type:tinyint;not null;default:0;index:idx_status_expire" json:"status"`
	QRCodeURL  *string    `gorm:"type:varchar(512);column:qr_code_url" json:"qr_code_url"`
	PayURL     *string    `gorm:"type:varchar(512);column:pay_url" json:"pay_url"`
	PaidAt     *time.Time `gorm:"type:datetime;column:paid_at" json:"paid_at"`
	ExpireAt   time.Time  `gorm:"type:datetime;not null;index:idx_status_expire;column:expire_at" json:"expire_at"`
	CreatedAt  time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Payment) TableName() string { return "payments" }

// 支付状态常量
const (
	PaymentStatusPending = 0 // 待支付
	PaymentStatusPaid    = 1 // 已支付
	PaymentStatusClosed  = 2 // 已关闭
)
