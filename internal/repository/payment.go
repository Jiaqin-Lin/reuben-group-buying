package repository

import (
	"context"
	"fmt"

	"github.com/reuben/group-buying/internal/model"
	"gorm.io/gorm"
)

// PaymentRepository 支付单和支付回调日志的数据访问接口。
//
// 支付流程（Go 版新增，Java 版无此表）：
//  1. 锁单成功后创建 payments 记录（status=0 待支付）
//  2. 调用支付宝下单，回填 qr_code_url / pay_url
//  3. 支付宝异步回调 → 验签 → 记录 payment_logs → 更新 payments.status=1 + trade_no
//  4. 结算完成后触发后续流程
type PaymentRepository interface {
	// --- 支付单 ---

	// CreatePayment 创建支付单。
	// 锁单成功后立即创建，order_id 作为支付宝的商家订单号。
	CreatePayment(ctx context.Context, payment *model.Payment) error

	// FindPaymentByOrderID 按内部订单号查支付单。
	FindPaymentByOrderID(ctx context.Context, orderID string) (*model.Payment, error)

	// FindPaymentByOutTradeNo 按外部交易单号查支付单（冗余索引，方便外部系统查询）。
	FindPaymentByOutTradeNo(ctx context.Context, outTradeNo string) (*model.Payment, error)

	// UpdatePaymentPaid 支付成功回调更新。
	// 更新 status=1, trade_no（支付宝交易号）, paid_at。
	UpdatePaymentPaid(ctx context.Context, orderID string, tradeNo string) error

	// UpdatePaymentClosed 关闭支付单（超时未支付或主动取消）。
	UpdatePaymentClosed(ctx context.Context, orderID string) error

	// --- 支付日志 ---

	// CreatePaymentLog 记录支付宝回调原始日志。
	// notify_id UK 去重：支付宝可能重复推送同一条回调。
	CreatePaymentLog(ctx context.Context, log *model.PaymentLog) error

	// FindPaymentLogByNotifyID 按支付宝 notify_id 查日志。
	// 用于去重判断：已处理过的回调直接返回成功。
	FindPaymentLogByNotifyID(ctx context.Context, notifyID string) (*model.PaymentLog, error)
}

// paymentRepo GORM 实现。
type paymentRepo struct {
	db *gorm.DB
}

// NewPaymentRepo 构造函数。
func NewPaymentRepo(db *gorm.DB) PaymentRepository {
	return &paymentRepo{db: db}
}

// --- 支付单 ---

func (r *paymentRepo) CreatePayment(ctx context.Context, payment *model.Payment) error {
	err := r.db.WithContext(ctx).Create(payment).Error
	if err != nil {
		return fmt.Errorf("create payment %s: %w", payment.OrderID, err)
	}
	return nil
}

func (r *paymentRepo) FindPaymentByOrderID(ctx context.Context, orderID string) (*model.Payment, error) {
	var p model.Payment
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).First(&p).Error
	if err != nil {
		return nil, fmt.Errorf("find payment by order %s: %w", orderID, err)
	}
	return &p, nil
}

func (r *paymentRepo) FindPaymentByOutTradeNo(ctx context.Context, outTradeNo string) (*model.Payment, error) {
	var p model.Payment
	err := r.db.WithContext(ctx).Where("out_trade_no = ?", outTradeNo).First(&p).Error
	if err != nil {
		return nil, fmt.Errorf("find payment by out_trade_no %s: %w", outTradeNo, err)
	}
	return &p, nil
}

// UpdatePaymentPaid 支付成功回调更新。
// trade_no 是支付宝返回的交易流水号。
func (r *paymentRepo) UpdatePaymentPaid(ctx context.Context, orderID string, tradeNo string) error {
	result := r.db.WithContext(ctx).
		Model(&model.Payment{}).
		Where("order_id = ? AND status = ?", orderID, model.PaymentStatusPending).
		Updates(map[string]any{
			"status":   model.PaymentStatusPaid,
			"trade_no": tradeNo,
			"paid_at":  gorm.Expr("NOW()"),
		})
	if result.Error != nil {
		return fmt.Errorf("update payment paid %s: %w", orderID, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("update payment paid %s: %w", orderID, gorm.ErrRecordNotFound)
	}
	return nil
}

// UpdatePaymentClosed 关闭支付单。
func (r *paymentRepo) UpdatePaymentClosed(ctx context.Context, orderID string) error {
	result := r.db.WithContext(ctx).
		Model(&model.Payment{}).
		Where("order_id = ? AND status = ?", orderID, model.PaymentStatusPending).
		Update("status", model.PaymentStatusClosed)
	if result.Error != nil {
		return fmt.Errorf("close payment %s: %w", orderID, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("close payment %s: %w", orderID, gorm.ErrRecordNotFound)
	}
	return nil
}

// --- 支付日志 ---

func (r *paymentRepo) CreatePaymentLog(ctx context.Context, pl *model.PaymentLog) error {
	err := r.db.WithContext(ctx).Create(pl).Error
	if err != nil {
		return fmt.Errorf("create payment log %s: %w", pl.NotifyID, err)
	}
	return nil
}

func (r *paymentRepo) FindPaymentLogByNotifyID(ctx context.Context, notifyID string) (*model.PaymentLog, error) {
	var pl model.PaymentLog
	err := r.db.WithContext(ctx).Where("notify_id = ?", notifyID).First(&pl).Error
	if err != nil {
		return nil, fmt.Errorf("find payment log by notify_id %s: %w", notifyID, err)
	}
	return &pl, nil
}
