package model

import "time"

// Order 用户订单
// out_trade_no = 外部交易单号（调用方生成），三个用途：
//  1. 幂等：同 out_trade_no 重复请求不重复创建
//  2. 全链路关联：锁单→支付→结算→退单→回调
//  3. Redis 名额占用标识：Lua 脚本用做 permitId
type Order struct {
	ID             uint64     `gorm:"primaryKey;autoIncrement" json:"-"`
	UserID         string     `gorm:"type:varchar(64);not null;index:idx_user_activity;column:user_id" json:"user_id"`
	TeamID         string     `gorm:"type:varchar(16);not null;index;column:team_id" json:"team_id"`
	OrderID        string     `gorm:"type:varchar(16);uniqueIndex;not null;column:order_id" json:"order_id"`
	ActivityID     int64      `gorm:"type:bigint;not null;index:idx_user_activity;column:activity_id" json:"activity_id"`
	GoodsID        string     `gorm:"type:varchar(32);not null;column:goods_id" json:"goods_id"`
	Source         string     `gorm:"type:varchar(16);not null" json:"source"`
	Channel        string     `gorm:"type:varchar(16);not null" json:"channel"`
	OriginalPrice  string     `gorm:"type:decimal(10,2);not null;column:original_price" json:"original_price"`
	DeductionPrice string     `gorm:"type:decimal(10,2);not null;column:deduction_price" json:"deduction_price"`
	PayPrice       string     `gorm:"type:decimal(10,2);not null;column:pay_price" json:"pay_price"`
	Status         int8       `gorm:"type:tinyint;not null;default:0;index:idx_status_created" json:"status"`
	OutTradeNo     string     `gorm:"type:varchar(64);uniqueIndex;not null;column:out_trade_no" json:"out_trade_no"`
	OutTradeTime   *time.Time `gorm:"type:datetime;column:out_trade_time" json:"out_trade_time"`
	CreatedAt      time.Time  `gorm:"autoCreateTime;index:idx_status_created" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Order) TableName() string { return "orders" }

// 订单状态常量
const (
	OrderStatusLocked   = 0 // 锁定（待支付）
	OrderStatusPaid     = 1 // 已支付
	OrderStatusRefunded = 2 // 已退款
)
