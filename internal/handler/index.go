// Package handler HTTP 处理层。
// 职责：参数绑定 + 调用 service + 返回统一 JSON 响应。不含业务逻辑。
package handler

import (
	"errors"
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/reuben/group-buying/internal/errcode"
	"github.com/reuben/group-buying/internal/response"
	"github.com/reuben/group-buying/internal/service"
)

// IndexHandler 首页/试算相关接口。
type IndexHandler struct {
	trialService *service.TrialService
}

// NewIndexHandler 构造函数。
func NewIndexHandler(trialService *service.TrialService) *IndexHandler {
	return &IndexHandler{trialService: trialService}
}

// Trial 试算接口 — POST /api/v1/trial。
//
// 请求体：{ "user_id": "U1", "goods_id": "GOODS001", "source": "APP", "channel": "WECHAT" }
// 响应：{ "code": "0000", "info": "成功", "data": { ... } }
func (h *IndexHandler) Trial(c *gin.Context) {
	var req service.TrialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.WarnContext(c.Request.Context(), "trial: bind json failed", "error", err)
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}

	result, err := h.trialService.Trial(c.Request.Context(), req)
	if err != nil {
		// 从 TrialError 提取业务错误码
		var trialErr *service.TrialError
		if errors.As(err, &trialErr) {
			response.FailWithMsg(c, trialErr.ErrorCode(), err.Error())
			return
		}
		slog.ErrorContext(c.Request.Context(), "trial: unexpected error", "error", err)
		response.Fail(c, errcode.CodeUnknownErr)
		return
	}

	response.Success(c, result)
}
