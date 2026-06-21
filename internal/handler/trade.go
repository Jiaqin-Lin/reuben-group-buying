package handler

import (
	"errors"
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/reuben/group-buying/internal/errcode"
	"github.com/reuben/group-buying/internal/metrics"
	"github.com/reuben/group-buying/internal/response"
	"github.com/reuben/group-buying/internal/service"
)

// TradeHandler 交易相关接口（锁单/结算/退单）。
type TradeHandler struct {
	lockSvc       *service.LockService
	settlementSvc *service.SettlementService
	refundSvc     *service.RefundService
}

// NewTradeHandler 构造函数。
func NewTradeHandler(
	lockSvc *service.LockService,
	settlementSvc *service.SettlementService,
	refundSvc *service.RefundService,
) *TradeHandler {
	return &TradeHandler{
		lockSvc:       lockSvc,
		settlementSvc: settlementSvc,
		refundSvc:     refundSvc,
	}
}

// LockOrder 锁单接口 — POST /api/v1/trade/lock。
//
// 请求体：{ "user_id", "activity_id", "goods_id", "source", "channel", "out_trade_no", "team_id"?, "notify_url"? }
// 响应：{ "code": "0000", "info": "成功", "data": { "order_id", "out_trade_no", ... } }
func (h *TradeHandler) LockOrder(c *gin.Context) {
	var req service.LockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.WarnContext(c.Request.Context(), "lock: bind json failed", "error", err)
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}

	result, err := h.lockSvc.Lock(c.Request.Context(), req)
	if err != nil {
		// 提取业务错误码（支持 LockError 和 TrialError）
		var lockErr *service.LockError
		var trialErr *service.TrialError
		switch {
		case errors.As(err, &lockErr):
			metrics.IncrBusinessError(lockErr.ErrorCode())
			response.FailWithMsg(c, lockErr.ErrorCode(), err.Error())
		case errors.As(err, &trialErr):
			metrics.IncrBusinessError(trialErr.ErrorCode())
			response.FailWithMsg(c, trialErr.ErrorCode(), err.Error())
		default:
			slog.ErrorContext(c.Request.Context(), "lock: unexpected error", "error", err)
			response.Fail(c, errcode.CodeUnknownErr)
		}
		return
	}

	response.Success(c, result)
}

// Settlement 结算接口 — POST /api/v1/trade/settlement。
//
// 请求体：{ "user_id", "out_trade_no", "out_trade_time", "source", "channel" }
// 响应：{ "code": "0000", "info": "成功", "data": { "order_id", "team_id", "is_complete", ... } }
func (h *TradeHandler) Settlement(c *gin.Context) {
	var req service.SettlementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.WarnContext(c.Request.Context(), "settlement: bind json failed", "error", err)
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}

	result, err := h.settlementSvc.Settle(c.Request.Context(), req)
	if err != nil {
		var settleErr *service.SettlementError
		if errors.As(err, &settleErr) {
			metrics.IncrBusinessError(settleErr.ErrorCode())
			response.FailWithMsg(c, settleErr.ErrorCode(), err.Error())
		} else {
			slog.ErrorContext(c.Request.Context(), "settlement: unexpected error", "error", err)
			response.Fail(c, errcode.CodeUnknownErr)
		}
		return
	}

	response.Success(c, result)
}

// Refund 退单接口 — POST /api/v1/trade/refund。
//
// 请求体：{ "user_id", "out_trade_no" }
// 响应：{ "code": "0000", "info": "成功", "data": { "order_id", "out_trade_no", "team_id", "refund_type", ... } }
func (h *TradeHandler) Refund(c *gin.Context) {
	var req service.RefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.WarnContext(c.Request.Context(), "refund: bind json failed", "error", err)
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}

	result, err := h.refundSvc.Refund(c.Request.Context(), req)
	if err != nil {
		var refundErr *service.RefundError
		if errors.As(err, &refundErr) {
			metrics.IncrBusinessError(refundErr.ErrorCode())
			response.FailWithMsg(c, refundErr.ErrorCode(), err.Error())
		} else {
			slog.ErrorContext(c.Request.Context(), "refund: unexpected error", "error", err)
			response.Fail(c, errcode.CodeUnknownErr)
		}
		return
	}

	response.Success(c, result)
}
