package handler

import (
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

	_ = h.db.WithContext(ctx).Table("activities").
		Where("status = 1 AND start_time <= NOW() AND end_time >= NOW()").
		Count(&activeActivities).Error
	_ = h.db.WithContext(ctx).Table("teams").
		Where("status = 0").Count(&formingTeams).Error
	_ = h.db.WithContext(ctx).Table("teams").
		Where("status = 1").Count(&completeTeams).Error
	_ = h.db.WithContext(ctx).Table("teams").
		Where("status = 2").Count(&failedTeams).Error
	_ = h.db.WithContext(ctx).Table("orders").
		Where("DATE(created_at) = CURDATE()").Count(&todayOrders).Error

	type recentOrder struct {
		OrderID    string `json:"order_id"`
		OutTradeNo string `json:"out_trade_no"`
		UserID     string `json:"user_id"`
		PayPrice   string `json:"pay_price"`
		Status     int8   `json:"status"`
		CreatedAt  string `json:"created_at"`
	}
	var recent []recentOrder
	_ = h.db.WithContext(ctx).Table("orders").
		Select("order_id, out_trade_no, user_id, pay_price, status, created_at").
		Order("id DESC").Limit(5).Find(&recent).Error

	response.Success(c, gin.H{
		"active_activities": activeActivities,
		"forming_teams":     formingTeams,
		"complete_teams":    completeTeams,
		"failed_teams":      failedTeams,
		"today_orders":      todayOrders,
		"recent_orders":     recent,
	})
}
