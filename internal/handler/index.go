// Package handler HTTP 处理层。
// 职责：参数绑定 + 调用 service + 返回统一 JSON 响应。不含业务逻辑。
package handler

import (
	"errors"
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/reuben/group-buying/internal/errcode"
	"github.com/reuben/group-buying/internal/model"
	"github.com/reuben/group-buying/internal/repository"
	"github.com/reuben/group-buying/internal/response"
	"github.com/reuben/group-buying/internal/service"
)

// IndexHandler 首页/试算相关接口。
type IndexHandler struct {
	trialService *service.TrialService
	orderRepo    repository.OrderRepository
}

// NewIndexHandler 构造函数。
func NewIndexHandler(
	trialService *service.TrialService,
	orderRepo repository.OrderRepository,
) *IndexHandler {
	return &IndexHandler{trialService: trialService, orderRepo: orderRepo}
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

// ListTeams 用户端团列表 — GET /api/v1/teams?activity_id=X&page=1&page_size=20。
//
// 只返回 forming 状态的团，按创建时间倒序。
// 响应：{ "items": [...], "total": N, "page": 1, "page_size": 20 }
func (h *IndexHandler) ListTeams(c *gin.Context) {
	activityID, err := strconv.ParseInt(c.Query("activity_id"), 10, 64)
	if err != nil || activityID <= 0 {
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize
	teams, total, err := h.orderRepo.FindTeamsByActivityID(c.Request.Context(), activityID, offset, pageSize)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "list teams failed", "error", err, "activity_id", activityID)
		response.Fail(c, errcode.CodeUnknownErr)
		return
	}

	if teams == nil {
		teams = []model.Team{}
	}

	response.Success(c, gin.H{
		"items":     teams,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GetTeam 用户端团详情 — GET /api/v1/teams/:team_id。
//
// 返回 team 信息 + 团内订单列表（成员）。
// 响应：{ "team": {...}, "members": [{user_id, order_id, status, created_at}] }
func (h *IndexHandler) GetTeam(c *gin.Context) {
	teamID := c.Param("team_id")
	if teamID == "" {
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}

	team, err := h.orderRepo.FindTeamByID(c.Request.Context(), teamID)
	if err != nil {
		response.FailWithMsg(c, errcode.CodeOrderNotFound, "团不存在")
		return
	}

	orders, err := h.orderRepo.FindOrdersByTeamID(c.Request.Context(), teamID)
	if err != nil {
		slog.WarnContext(c.Request.Context(), "get team: find orders failed", "team_id", teamID, "error", err)
		orders = []model.Order{}
	}

	response.Success(c, gin.H{
		"team":    team,
		"members": orders,
	})
}
