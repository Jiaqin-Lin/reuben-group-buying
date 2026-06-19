// Package pay 支付网关对接。
//
// 职责：定义支付网关接口，提供 Mock 实现用于开发测试。
// 真实支付（支付宝/微信）后续实现此接口即可。
package pay

import (
	"context"
	"fmt"

	"github.com/reuben/group-buying/internal/model"
)

// Gateway 支付网关接口。
// 所有支付渠道（支付宝、微信、Mock）实现此接口。
type Gateway interface {
	// CreateOrder 创建支付单，返回支付链接（二维码/收银台 URL）。
	CreateOrder(ctx context.Context, order *model.Order) (*CreateResult, error)

	// QueryOrder 查询支付状态。
	QueryOrder(ctx context.Context, orderID string) (*QueryResult, error)

	// VerifyNotify 验签支付回调。
	VerifyNotify(ctx context.Context, raw []byte) (*Notify, error)
}

// CreateResult 创建支付单结果。
type CreateResult struct {
	PayURL  string `json:"pay_url"`
	TradeNo string `json:"trade_no"` // 支付网关交易号
}

// QueryResult 支付查询结果。
type QueryResult struct {
	TradeNo string `json:"trade_no"`
	Status  string `json:"status"` // WAIT_BUYER_PAY | TRADE_SUCCESS | TRADE_CLOSED
	Amount  string `json:"amount"`
}

// Notify 支付回调通知（验签后）。
type Notify struct {
	TradeNo     string `json:"trade_no"`
	OutTradeNo  string `json:"out_trade_no"` // 对应 orders.order_id
	TotalAmount string `json:"total_amount"`
	Status      string `json:"status"` // TRADE_SUCCESS | TRADE_CLOSED
}

// Mock 支付网关 Mock 实现。
// 所有操作均成功，无真实 HTTP 调用。
type Mock struct{}

// NewMock 创建 Mock 支付网关。
func NewMock() *Mock {
	return &Mock{}
}

// CreateOrder Mock 创建支付单。
// 总是成功，返回一个模拟的支付链接。
func (m *Mock) CreateOrder(_ context.Context, order *model.Order) (*CreateResult, error) {
	return &CreateResult{
		PayURL:  fmt.Sprintf("https://pay.example.com/mock/%s", order.OrderID),
		TradeNo: fmt.Sprintf("MOCK_%s", order.OrderID),
	}, nil
}

// QueryOrder Mock 查询支付状态。
// 总是返回已支付。
func (m *Mock) QueryOrder(_ context.Context, orderID string) (*QueryResult, error) {
	return &QueryResult{
		TradeNo: fmt.Sprintf("MOCK_%s", orderID),
		Status:  "TRADE_SUCCESS",
	}, nil
}

// VerifyNotify Mock 验签。
// 总是通过，返回支付成功通知。
func (m *Mock) VerifyNotify(_ context.Context, raw []byte) (*Notify, error) {
	return &Notify{
		TradeNo:     "MOCK_NOTIFY",
		OutTradeNo:  "",
		TotalAmount: "0.00",
		Status:      "TRADE_SUCCESS",
	}, nil
}
