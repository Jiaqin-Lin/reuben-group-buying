// Package handler HTTP 处理层。
package handler

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/reuben/group-buying/internal/errcode"
	"github.com/reuben/group-buying/internal/model"
	"github.com/reuben/group-buying/internal/pay"
	"github.com/reuben/group-buying/internal/repository"
	"github.com/reuben/group-buying/internal/response"
	"github.com/reuben/group-buying/internal/service"
)

// PayHandler 支付回调处理器。
//
// 职责：接收支付宝异步通知 → 验签 → 去重 → 记录日志 → 更新支付状态 → 触发结算。
type PayHandler struct {
	payGateway    pay.Gateway
	paymentRepo   repository.PaymentRepository
	orderRepo     repository.OrderRepository
	settlementSvc *service.SettlementService
}

// NewPayHandler 构造函数。
func NewPayHandler(
	payGateway pay.Gateway,
	paymentRepo repository.PaymentRepository,
	orderRepo repository.OrderRepository,
	settlementSvc *service.SettlementService,
) *PayHandler {
	return &PayHandler{
		payGateway:    payGateway,
		paymentRepo:   paymentRepo,
		orderRepo:     orderRepo,
		settlementSvc: settlementSvc,
	}
}

// Notify 支付宝异步通知回调。
//
//	POST /api/v1/pay/notify
//
// 支付宝发送 application/x-www-form-urlencoded POST。
// 返回纯文本 "success" 或 "fail"（非 JSON！）。
//
// 流程：
//  1. 读取 raw body
//  2. 验签
//  3. notify_id 去重
//  4. 记录 payment_log
//  5. 更新 payment 状态
//  6. 触发结算
func (h *PayHandler) Notify(c *gin.Context) {
	ctx := c.Request.Context()

	// 1. 读取 raw body
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		slog.ErrorContext(ctx, "pay notify: read body failed", "error", err)
		c.String(http.StatusBadRequest, "fail")
		return
	}

	// 2. 验签 + 解析通知
	noti, err := h.payGateway.VerifyNotify(ctx, rawBody)
	if err != nil {
		slog.WarnContext(ctx, "pay notify: verify failed", "error", err)
		h.recordPaymentLog(ctx, "", "", string(rawBody), model.PayLogStatusFailed, nil)
		c.String(http.StatusBadRequest, "fail")
		return
	}

	// 3. notify_id 去重
	if existing, lookupErr := h.paymentRepo.FindPaymentLogByNotifyID(ctx, noti.NotifyID); lookupErr == nil && existing != nil {
		slog.DebugContext(ctx, "pay notify: duplicate notify_id, already processed",
			"notify_id", noti.NotifyID, "existing_status", existing.Status)
		c.String(http.StatusOK, "success")
		return
	}

	// 4. 记录 payment_log
	tradeStatus := noti.Status
	h.recordPaymentLog(ctx, noti.OutTradeNo, noti.NotifyID, string(rawBody), model.PayLogStatusPassed, &tradeStatus)

	// 5. 只有 TRADE_SUCCESS 才触发结算
	if noti.Status != "TRADE_SUCCESS" {
		slog.InfoContext(ctx, "pay notify: trade not success, skip settlement",
			"notify_id", noti.NotifyID, "status", noti.Status)
		c.String(http.StatusOK, "success")
		return
	}

	// 6. 查 payment（notify.OutTradeNo = 支付宝的 out_trade_no = 我们的 order_id）
	payment, err := h.paymentRepo.FindPaymentByOrderID(ctx, noti.OutTradeNo)
	if err != nil {
		slog.ErrorContext(ctx, "pay notify: payment not found",
			"alipay_out_trade_no", noti.OutTradeNo, "error", err)
		c.String(http.StatusOK, "fail")
		return
	}

	// 7. 更新 payment 为已支付
	if err := h.paymentRepo.UpdatePaymentPaid(ctx, payment.OrderID, noti.TradeNo); err != nil {
		slog.WarnContext(ctx, "pay notify: update payment paid failed",
			"order_id", payment.OrderID, "error", err)
		// 可能已经 paid 了（并发），继续走结算（结算也有幂等）
	}

	// 8. 查 order 获取结算所需字段
	order, err := h.orderRepo.FindOrderByOrderID(ctx, payment.OrderID)
	if err != nil {
		slog.ErrorContext(ctx, "pay notify: order not found",
			"order_id", payment.OrderID, "error", err)
		c.String(http.StatusOK, "fail")
		return
	}

	// 9. 解析支付时间
	paidTime := parseGmtPayment(noti.GmtPayment)

	// 10. 触发结算
	//     注意：Settlement 用 OutTradeNo（调用方的外部单号）查订单，不是 order_id
	_, err = h.settlementSvc.Settle(ctx, service.SettlementRequest{
		UserID:       order.UserID,
		OutTradeNo:   payment.OutTradeNo,
		OutTradeTime: paidTime,
		Source:       order.Source,
		Channel:      order.Channel,
	})
	if err != nil {
		slog.ErrorContext(ctx, "pay notify: settlement failed",
			"order_id", payment.OrderID, "out_trade_no", payment.OutTradeNo, "error", err)
		c.String(http.StatusOK, "fail")
		return
	}

	slog.InfoContext(ctx, "pay notify: settlement done",
		"order_id", payment.OrderID, "out_trade_no", payment.OutTradeNo, "trade_no", noti.TradeNo)

	c.String(http.StatusOK, "success")
}

// recordPaymentLog 记录支付回调日志（最佳努力，失败仅打日志）。
func (h *PayHandler) recordPaymentLog(ctx context.Context, orderID, notifyID, raw string, status int8, tradeStatus *string) {
	pl := &model.PaymentLog{
		OrderID:     orderID,
		NotifyID:    notifyID,
		NotifyRaw:   raw,
		Status:      status,
		TradeStatus: tradeStatus,
	}
	if err := h.paymentRepo.CreatePaymentLog(ctx, pl); err != nil {
		slog.WarnContext(ctx, "pay notify: create payment log failed",
			"notify_id", notifyID, "error", err)
	}
}

// GetPayment 查询支付单 — GET /api/v1/payments/:out_trade_no。
//
// 前端"继续支付"或支付弹窗轮询时调用。
// 两种自动完成路径：
//  1. Mock 支付：trade_no 以 "MOCK_" 开头，首次查询即自动完成
//  2. 真实支付：主动向支付宝查询，已支付则更新本地状态并触发结算
//     这是支付宝异步通知未送达时的兜底机制
//
// 响应：{ "payment": {...} }
func (h *PayHandler) GetPayment(c *gin.Context) {
	ctx := c.Request.Context()
	outTradeNo := c.Param("out_trade_no")
	if outTradeNo == "" {
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}

	payment, err := h.paymentRepo.FindPaymentByOutTradeNo(ctx, outTradeNo)
	if err != nil {
		response.FailWithMsg(c, errcode.CodeOrderNotFound, "payment not found")
		return
	}

	if payment.Status == model.PaymentStatusPending {
		isMock := payment.TradeNo != nil && *payment.TradeNo != "" &&
			len(*payment.TradeNo) >= 5 && (*payment.TradeNo)[:5] == "MOCK_"

		if isMock {
			// Mock 支付自动完成：首次查询即标记为已支付并触发结算
			h.autoSettle(ctx, payment)
			payment, _ = h.paymentRepo.FindPaymentByOutTradeNo(ctx, outTradeNo)
		} else if h.payGateway != nil {
			// 真实支付：主动向支付宝查询支付状态（兜底异步通知未送达）
			h.syncPaymentFromGateway(ctx, payment)
			payment, _ = h.paymentRepo.FindPaymentByOutTradeNo(ctx, outTradeNo)
		}
	}

	response.Success(c, gin.H{"payment": payment})
}

// autoSettle Mock 支付自动完成：更新 payment → 触发结算。
// 失败仅打日志，不影响 HTTP 响应。
func (h *PayHandler) autoSettle(ctx context.Context, payment *model.Payment) {
	if payment.TradeNo == nil {
		return
	}
	if err := h.paymentRepo.UpdatePaymentPaid(ctx, payment.OrderID, *payment.TradeNo); err != nil {
		slog.WarnContext(ctx, "pay: mock auto-settle update payment failed",
			"order_id", payment.OrderID, "error", err)
		return
	}
	order, err := h.orderRepo.FindOrderByOrderID(ctx, payment.OrderID)
	if err != nil {
		slog.WarnContext(ctx, "pay: mock auto-settle find order failed",
			"order_id", payment.OrderID, "error", err)
		return
	}
	_, err = h.settlementSvc.Settle(ctx, service.SettlementRequest{
		UserID:       order.UserID,
		OutTradeNo:   payment.OutTradeNo,
		OutTradeTime: time.Now(),
		Source:       order.Source,
		Channel:      order.Channel,
	})
	if err != nil {
		slog.ErrorContext(ctx, "pay: mock auto-settle failed",
			"order_id", payment.OrderID, "error", err)
	}
}

// syncPaymentFromGateway 向支付网关查询支付状态，已支付则自动完成。
//
// 这是支付宝异步通知未送达时的兜底机制——前端每次轮询 GetPayment
// 都会触发主动查询，用户付了钱就能及时完成结算。
// 网关查询失败静默跳过（网关不可用时不影响前端展示当前状态）。
func (h *PayHandler) syncPaymentFromGateway(ctx context.Context, payment *model.Payment) {
	result, err := h.payGateway.QueryOrder(ctx, payment.OrderID)
	if err != nil {
		slog.DebugContext(ctx, "pay: gateway query failed (not critical)",
			"order_id", payment.OrderID, "error", err)
		return
	}
	if result.Status != "TRADE_SUCCESS" {
		return
	}

	// 更新 payment
	tradeNo := result.TradeNo
	if tradeNo == "" {
		tradeNo = "GATEWAY_" + payment.OrderID
	}
	if err := h.paymentRepo.UpdatePaymentPaid(ctx, payment.OrderID, tradeNo); err != nil {
		slog.WarnContext(ctx, "pay: sync update payment failed",
			"order_id", payment.OrderID, "error", err)
		// 可能已经 paid 了（并发），继续走结算
	}

	// 触发结算
	order, err := h.orderRepo.FindOrderByOrderID(ctx, payment.OrderID)
	if err != nil {
		slog.WarnContext(ctx, "pay: sync find order failed",
			"order_id", payment.OrderID, "error", err)
		return
	}
	_, err = h.settlementSvc.Settle(ctx, service.SettlementRequest{
		UserID:       order.UserID,
		OutTradeNo:   payment.OutTradeNo,
		OutTradeTime: time.Now(),
		Source:       order.Source,
		Channel:      order.Channel,
	})
	if err != nil {
		slog.ErrorContext(ctx, "pay: sync settlement failed",
			"order_id", payment.OrderID, "error", err)
	} else {
		slog.InfoContext(ctx, "pay: synced from gateway",
			"order_id", payment.OrderID, "trade_no", tradeNo)
	}
}

// ShowQR 展示支付二维码页面。
//
//	GET /api/v1/pay/qr?url=https://qr.alipay.com/...
//
// 返回一个简单的 HTML 页面，用 qrcode.js 将支付宝二维码链接渲染为可扫描的二维码。
// 手机上扫描，或在沙箱支付宝 APP 中识别二维码完成支付。
func (h *PayHandler) ShowQR(c *gin.Context) {
	payURL := c.Query("url")
	if payURL == "" {
		c.String(http.StatusBadRequest, "缺少 url 参数")
		return
	}

	// 支付宝的 qr_code 是支付宝域名的短链接，直接用
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, qrPageHTML, payURL, payURL, payURL)
}

// qrPageHTML 二维码展示页面。
const qrPageHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>扫码支付</title>
<script src="https://cdn.jsdelivr.net/npm/qrcodejs@1.0.0/qrcode.min.js"></script>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{display:flex;flex-direction:column;align-items:center;justify-content:center;min-height:100vh;
font-family:-apple-system,BlinkMacSystemFont,sans-serif;background:#f5f5f5;padding:20px}
.card{background:#fff;border-radius:16px;padding:40px;box-shadow:0 2px 20px rgba(0,0,0,.08);text-align:center;max-width:380px;width:100%}
h2{font-size:20px;color:#333;margin-bottom:8px}
.amount{font-size:14px;color:#999;margin-bottom:24px}
.qr-wrap{display:inline-block;padding:16px;border:1px solid #eee;border-radius:12px}
.info{font-size:12px;color:#999;margin-top:20px;line-height:1.6}
</style></head><body>
<div class="card">
<h2>拼团订单支付</h2>
<div class="amount">请使用支付宝扫码支付</div>
<div class="qr-wrap"><div id="qrcode"></div></div>
<div class="info">用<strong>沙箱版支付宝</strong>扫码<br>买家账号和密码见沙箱管理页面<br>支付链接: <a href="%s" target="_blank">%s</a></div>
</div>
<script>new QRCode(document.getElementById("qrcode"),{text:"%s",width:260,height:260})</script>
</body></html>`

// parseGmtPayment 解析支付宝支付时间。
// 格式：2006-01-02 15:04:05，解析失败返回当前时间。
func parseGmtPayment(s string) time.Time {
	if s == "" {
		return time.Now()
	}
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		return time.Now()
	}
	return t
}
