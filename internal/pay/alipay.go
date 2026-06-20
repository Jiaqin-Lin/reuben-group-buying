// Package pay 支付网关对接。
package pay

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/smartwalle/alipay/v3"

	"github.com/reuben/group-buying/internal/model"
)

// AlipayConfig 支付宝网关配置。
type AlipayConfig struct {
	AppID           string
	PrivateKey      string
	AlipayPublicKey string
	NotifyURL       string
	ReturnURL       string
	Sandbox         bool
	SignType        string
}

// AlipayGateway 支付宝支付网关。
// 实现 Gateway 接口，通过 smartwalle/alipay/v3 SDK 对接支付宝沙箱/正式环境。
type AlipayGateway struct {
	client *alipay.Client
	cfg    AlipayConfig
}

// NewAlipay 创建支付宝支付网关。
//
// 密钥支持三种格式：
//   - PEM 内容（以 "-----BEGIN" 开头）
//   - 裸 base64（支付宝密钥工具导出格式）→ 自动补 PEM 头
//   - 文件路径（从磁盘读取）
//
// 沙箱模式下自动使用沙箱网关。
func NewAlipay(cfg AlipayConfig) (*AlipayGateway, error) {
	if cfg.AppID == "" {
		return nil, fmt.Errorf("alipay config: app_id is required")
	}

	privateKey, err := resolveKey(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("alipay private key: %w", err)
	}

	alipayPublicKey, err := resolveKey(cfg.AlipayPublicKey)
	if err != nil {
		return nil, fmt.Errorf("alipay public key: %w", err)
	}

	// 创建客户端：第三个参数 isProduction=false → 沙箱
	// 设置 8s 超时，避免支付宝 API 慢响应卡死锁单流程（锁 TTL 只有 3s，但至少不会无限等）
	httpClient := &http.Client{Timeout: 8 * time.Second}
	client, err := alipay.New(cfg.AppID, privateKey, !cfg.Sandbox, alipay.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("alipay new client: %w", err)
	}

	if err := client.LoadAliPayPublicKey(alipayPublicKey); err != nil {
		return nil, fmt.Errorf("alipay load public key: %w", err)
	}

	return &AlipayGateway{client: client, cfg: cfg}, nil
}

// CreateOrder 创建支付单（当面付预创建 → 返回二维码链接）。
//
// 映射：
//   - order.OrderID → 支付宝 out_trade_no（商家订单号）
//   - order.PayPrice → 支付宝 total_amount
func (g *AlipayGateway) CreateOrder(ctx context.Context, order *model.Order) (*CreateResult, error) {
	param := alipay.TradePreCreate{
		Trade: alipay.Trade{
			Subject:     "拼团订单",
			OutTradeNo:  order.OrderID,
			TotalAmount: order.PayPrice,
			ProductCode: "FACE_TO_FACE_PAYMENT",
		},
	}
	if g.cfg.NotifyURL != "" {
		param.NotifyURL = g.cfg.NotifyURL
	}

	rsp, err := g.client.TradePreCreate(ctx, param)
	if err != nil {
		slog.ErrorContext(ctx, "alipay precreate failed", "order_id", order.OrderID, "error", err)
		return nil, fmt.Errorf("alipay create order %s: %w", order.OrderID, err)
	}
	if rsp.Code != "10000" {
		slog.ErrorContext(ctx, "alipay precreate error",
			"order_id", order.OrderID, "code", rsp.Code, "msg", rsp.Msg, "sub_msg", rsp.SubMsg)
		return nil, fmt.Errorf("alipay create order %s: code=%s msg=%s", order.OrderID, rsp.Code, rsp.Msg)
	}

	slog.DebugContext(ctx, "alipay precreate ok", "order_id", order.OrderID, "out_trade_no", rsp.OutTradeNo)
	return &CreateResult{
		PayURL:  rsp.QRCode,
		TradeNo: "",
	}, nil
}

// QueryOrder 查询支付状态。
func (g *AlipayGateway) QueryOrder(ctx context.Context, orderID string) (*QueryResult, error) {
	rsp, err := g.client.TradeQuery(ctx, alipay.TradeQuery{OutTradeNo: orderID})
	if err != nil {
		return nil, fmt.Errorf("alipay query order %s: %w", orderID, err)
	}
	if rsp.Code != "10000" {
		return nil, fmt.Errorf("alipay query order %s: code=%s msg=%s", orderID, rsp.Code, rsp.Msg)
	}

	return &QueryResult{
		TradeNo: rsp.TradeNo,
		Status:  string(rsp.TradeStatus),
		Amount:  rsp.TotalAmount,
	}, nil
}

// VerifyNotify 验签支付宝异步通知。
//
// 流程：
//  1. 解析 application/x-www-form-urlencoded body
//  2. 调用 SDK DecodeNotification（内部 VerifySign + 提取字段）
//  3. 映射到 Notify struct
func (g *AlipayGateway) VerifyNotify(ctx context.Context, raw []byte) (*Notify, error) {
	values, err := url.ParseQuery(string(raw))
	if err != nil {
		return nil, fmt.Errorf("alipay notify: parse form: %w", err)
	}

	noti, err := g.client.DecodeNotification(ctx, values)
	if err != nil {
		slog.WarnContext(ctx, "alipay notify verify failed", "error", err)
		return nil, fmt.Errorf("alipay notify: verify: %w", err)
	}

	slog.DebugContext(ctx, "alipay notify verified",
		"notify_id", noti.NotifyId,
		"out_trade_no", noti.OutTradeNo,
		"trade_no", noti.TradeNo,
		"trade_status", noti.TradeStatus,
	)

	return &Notify{
		TradeNo:     noti.TradeNo,
		OutTradeNo:  noti.OutTradeNo,
		TotalAmount: noti.TotalAmount,
		Status:      string(noti.TradeStatus),
		NotifyID:    noti.NotifyId,
		GmtPayment:  noti.GmtPayment,
	}, nil
}

// Refund 调用支付宝退款。
//
// orderID 对应支付宝的 out_trade_no（即我们的内部订单号）。
// outRequestNo 用 orderID 生成，支付宝用它做退款幂等。
func (g *AlipayGateway) Refund(ctx context.Context, orderID string, refundAmount string) (*RefundResult, error) {
	param := alipay.TradeRefund{
		OutTradeNo:   orderID,
		RefundAmount: refundAmount,
		RefundReason: "拼团退单",
		OutRequestNo: "refund_" + orderID,
	}

	rsp, err := g.client.TradeRefund(ctx, param)
	if err != nil {
		slog.ErrorContext(ctx, "alipay refund failed", "order_id", orderID, "error", err)
		return nil, fmt.Errorf("alipay refund %s: %w", orderID, err)
	}
	if rsp.Code != "10000" {
		slog.ErrorContext(ctx, "alipay refund error",
			"order_id", orderID, "code", rsp.Code, "msg", rsp.Msg, "sub_msg", rsp.SubMsg)
		return nil, fmt.Errorf("alipay refund %s: code=%s msg=%s", orderID, rsp.Code, rsp.Msg)
	}

	slog.InfoContext(ctx, "alipay refund ok", "order_id", orderID, "refund_fee", rsp.RefundFee)
	return &RefundResult{
		RefundTradeNo: rsp.TradeNo,
		Amount:        rsp.RefundFee,
	}, nil
}

// resolveKey 解析密钥。
// 支持三种格式：
//  1. PEM 内容（以 -----BEGIN 开头）→ 直接返回
//  2. 裸 base64（支付宝密钥工具导出格式）→ 自动补 PEM 头
//  3. 文件路径 → 读取内容
func resolveKey(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("empty key")
	}
	if strings.HasPrefix(s, "-----BEGIN") {
		return s, nil
	}
	// 裸 base64：只含 base64 字符和换行（文件名一定有空格等分隔符）
	if looksLikeBase64(s) {
		return wrapPEM(s), nil
	}
	data, err := os.ReadFile(s)
	if err != nil {
		return "", fmt.Errorf("read key file %s: %w", s, err)
	}
	return string(data), nil
}

// looksLikeBase64 判断字符串是否像裸 base64 编码的密钥。
func looksLikeBase64(s string) bool {
	for _, c := range s {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
			c == '+' || c == '/' || c == '=' || c == '\n' || c == '\r' {
			continue
		}
		return false
	}
	return len(s) > 64 // 最短的 RSA 密钥 base64 也远超 64 字符
}

// wrapPEM 将裸 base64 密钥包裹为 PEM 格式。
// 自动识别私钥和公钥（公钥 base64 以 MIIBIj 开头）。
func wrapPEM(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.TrimSpace(s)
	// 去掉可能已有的头尾标记（防止重复包裹）
	for _, tag := range []string{
		"-----BEGIN RSA PRIVATE KEY-----", "-----BEGIN PRIVATE KEY-----",
		"-----BEGIN PUBLIC KEY-----", "-----BEGIN RSA PUBLIC KEY-----",
		"-----END RSA PRIVATE KEY-----", "-----END PRIVATE KEY-----",
		"-----END PUBLIC KEY-----", "-----END RSA PUBLIC KEY-----",
	} {
		s = strings.ReplaceAll(s, tag, "")
	}
	s = strings.TrimSpace(s)

	if strings.HasPrefix(s, "MIIBIj") {
		return "-----BEGIN PUBLIC KEY-----\n" + s + "\n-----END PUBLIC KEY-----"
	}
	return "-----BEGIN RSA PRIVATE KEY-----\n" + s + "\n-----END RSA PRIVATE KEY-----"
}

// Ensure AlipayGateway implements Gateway.
var _ Gateway = (*AlipayGateway)(nil)
