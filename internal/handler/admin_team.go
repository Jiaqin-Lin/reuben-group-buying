package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/reuben/group-buying/internal/errcode"
	"github.com/reuben/group-buying/internal/model"
	"github.com/reuben/group-buying/internal/response"
)

// AdminTeamHandler 队伍监控（只读）。
type AdminTeamHandler struct {
	db *gorm.DB
}

func NewAdminTeamHandler(db *gorm.DB) *AdminTeamHandler {
	return &AdminTeamHandler{db: db}
}

// ListTeams GET /api/v1/admin/teams?status=0&page=1&page_size=20
func (h *AdminTeamHandler) ListTeams(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	db := h.db.WithContext(c.Request.Context()).Model(&model.Team{})
	if s := c.Query("status"); s != "" {
		status, _ := strconv.Atoi(s)
		db = db.Where("status = ?", status)
	}
	if aid := c.Query("activity_id"); aid != "" {
		id, _ := strconv.ParseInt(aid, 10, 64)
		db = db.Where("activity_id = ?", id)
	}

	var total int64
	db.Count(&total)

	var teams []model.Team
	offset := (page - 1) * pageSize
	db.Order("id DESC").Offset(offset).Limit(pageSize).Find(&teams)

	response.Success(c, gin.H{
		"items":     teams,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GetTeam GET /api/v1/admin/teams/:team_id
func (h *AdminTeamHandler) GetTeam(c *gin.Context) {
	teamID := c.Param("team_id")
	var team model.Team
	if err := h.db.WithContext(c.Request.Context()).Where("team_id = ?", teamID).First(&team).Error; err != nil {
		response.Fail(c, errcode.CodeOrderNotFound)
		return
	}
	response.Success(c, team)
}

// GetTeamOrders GET /api/v1/admin/teams/:team_id/orders
func (h *AdminTeamHandler) GetTeamOrders(c *gin.Context) {
	teamID := c.Param("team_id")
	var orders []model.Order
	h.db.WithContext(c.Request.Context()).
		Where("team_id = ?", teamID).
		Order("id ASC").
		Find(&orders)
	response.Success(c, orders)
}
