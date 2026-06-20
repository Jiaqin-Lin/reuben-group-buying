package handler

import (
	"log/slog"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/reuben/group-buying/internal/response"
)

// AdminDashboardHandler 仪表盘。
type AdminDashboardHandler struct {
	db *gorm.DB
}

func NewAdminDashboardHandler(db *gorm.DB) *AdminDashboardHandler {
	return &AdminDashboardHandler{db: db}
}

// GetDashboard GET /api/v1/admin/dashboard
func (h *AdminDashboardHandler) GetDashboard(c *gin.Context) {
	ctx := c.Request.Context()

	var activeActivities, formingTeams, completeTeams, failedTeams, todayOrders int64

	if err := h.db.WithContext(ctx).Table("activities").
		Where("status = 1 AND start_time <= NOW() AND end_time >= NOW()").
		Count(&activeActivities).Error; err != nil {
		slog.Error("dashboard: query failed", "error", err)
	}
	if err := h.db.WithContext(ctx).Table("teams").
		Where("status = 0").Count(&formingTeams).Error; err != nil {
		slog.Error("dashboard: query failed", "error", err)
	}
	if err := h.db.WithContext(ctx).Table("teams").
		Where("status = 1").Count(&completeTeams).Error; err != nil {
		slog.Error("dashboard: query failed", "error", err)
	}
	if err := h.db.WithContext(ctx).Table("teams").
		Where("status = 2").Count(&failedTeams).Error; err != nil {
		slog.Error("dashboard: query failed", "error", err)
	}
	if err := h.db.WithContext(ctx).Table("orders").
		Where("DATE(created_at) = CURDATE()").Count(&todayOrders).Error; err != nil {
		slog.Error("dashboard: query failed", "error", err)
	}

	type recentOrder struct {
		OrderID    string `json:"order_id"`
		OutTradeNo string `json:"out_trade_no"`
		UserID     string `json:"user_id"`
		PayPrice   string `json:"pay_price"`
		Status     int8   `json:"status"`
		CreatedAt  string `json:"created_at"`
	}
	var recent []recentOrder
	if err := h.db.WithContext(ctx).Table("orders").
		Select("order_id, out_trade_no, user_id, pay_price, status, created_at").
		Order("id DESC").Limit(5).Find(&recent).Error; err != nil {
		slog.Error("dashboard: query failed", "error", err)
	}

	response.Success(c, gin.H{
		"active_activities": activeActivities,
		"forming_teams":     formingTeams,
		"complete_teams":    completeTeams,
		"failed_teams":      failedTeams,
		"today_orders":      todayOrders,
		"recent_orders":     recent,
	})
}
