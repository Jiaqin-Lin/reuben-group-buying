package handler

import (
	"errors"
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/reuben/group-buying/internal/errcode"
	"github.com/reuben/group-buying/internal/response"
	"github.com/reuben/group-buying/internal/service"
)

// TradeHandler 交易相关接口（锁单/结算/退单）。
type TradeHandler struct {
	lockSvc *service.LockService
}

// NewTradeHandler 构造函数。
func NewTradeHandler(lockSvc *service.LockService) *TradeHandler {
	return &TradeHandler{lockSvc: lockSvc}
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
			response.FailWithMsg(c, lockErr.ErrorCode(), err.Error())
		case errors.As(err, &trialErr):
			response.FailWithMsg(c, trialErr.ErrorCode(), err.Error())
		default:
			slog.ErrorContext(c.Request.Context(), "lock: unexpected error", "error", err)
			response.Fail(c, errcode.CodeUnknownErr)
		}
		return
	}

	response.Success(c, result)
}
