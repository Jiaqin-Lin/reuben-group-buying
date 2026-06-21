package pay

import (
	"context"

	"github.com/reuben/group-buying/internal/config/dynamic"
	"github.com/reuben/group-buying/internal/model"
)

// DynamicGateway 运行时动态切换 Mock / 真实支付网关。
//
// 与启动时静态选择不同，DynamicGateway 每次调用时检查 FeatureUseMockPayment
// 动态配置，可在不重启的情况下切换支付网关。
//
// 使用方式：
//   - 正常：feature.use_mock_payment=false → 走真实支付宝
//   - 测试：feature.use_mock_payment=true  → 走 Mock
type DynamicGateway struct {
	mock    Gateway
	alipay  Gateway // nil 表示未配置真实网关
	useMock *dynamic.Def[bool]
}

// NewDynamicGateway 创建动态支付网关。
// mock 总是可用；alipay 为 nil 则强制走 mock（忽略 useMock 配置）。
func NewDynamicGateway(mock, alipay Gateway, useMock *dynamic.Def[bool]) *DynamicGateway {
	return &DynamicGateway{
		mock:    mock,
		alipay:  alipay,
		useMock: useMock,
	}
}

// real 当前生效的网关。
func (d *DynamicGateway) real() Gateway {
	if d.alipay == nil {
		return d.mock
	}
	if d.useMock.Get() {
		return d.mock
	}
	return d.alipay
}

// CreateOrder 创建支付单。
func (d *DynamicGateway) CreateOrder(ctx context.Context, order *model.Order) (*CreateResult, error) {
	return d.real().CreateOrder(ctx, order)
}

// QueryOrder 查询支付状态。
func (d *DynamicGateway) QueryOrder(ctx context.Context, orderID string) (*QueryResult, error) {
	return d.real().QueryOrder(ctx, orderID)
}

// VerifyNotify 验签支付回调。
func (d *DynamicGateway) VerifyNotify(ctx context.Context, raw []byte) (*Notify, error) {
	return d.real().VerifyNotify(ctx, raw)
}

// Refund 退款。
func (d *DynamicGateway) Refund(ctx context.Context, orderID string, refundAmount string) (*RefundResult, error) {
	return d.real().Refund(ctx, orderID, refundAmount)
}
