package handler

import (
	"log/slog"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/reuben/group-buying/internal/errcode"
	"github.com/reuben/group-buying/internal/model"
	"github.com/reuben/group-buying/internal/response"
)

// AdminActivityProductHandler 活动-商品映射管理。
type AdminActivityProductHandler struct {
	db *gorm.DB
}

func NewAdminActivityProductHandler(db *gorm.DB) *AdminActivityProductHandler {
	return &AdminActivityProductHandler{db: db}
}

// ListActivityProducts GET /api/v1/admin/activity-products
func (h *AdminActivityProductHandler) ListActivityProducts(c *gin.Context) {
	var mappings []model.ActivityProduct
	if err := h.db.WithContext(c.Request.Context()).Find(&mappings).Error; err != nil {
		slog.Error("admin: internal error", "error", err)
		response.FailHTTP(c, 500, "internal server error")
		return
	}
	response.Success(c, mappings)
}

// CreateActivityProduct POST /api/v1/admin/activity-products
func (h *AdminActivityProductHandler) CreateActivityProduct(c *gin.Context) {
	var m model.ActivityProduct
	if err := c.ShouldBindJSON(&m); err != nil {
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}
	if m.Source == "" || m.Channel == "" || m.GoodsID == "" {
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&m).Error; err != nil {
		slog.Error("admin: internal error", "error", err)
		response.FailHTTP(c, 500, "internal server error")
		return
	}
	response.Success(c, m)
}

// DeleteActivityProduct DELETE /api/v1/admin/activity-products
// 通过 query params: source, channel, goods_id
func (h *AdminActivityProductHandler) DeleteActivityProduct(c *gin.Context) {
	source := c.Query("source")
	channel := c.Query("channel")
	goodsID := c.Query("goods_id")
	if source == "" || channel == "" || goodsID == "" {
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}
	result := h.db.WithContext(c.Request.Context()).
		Where("source = ? AND channel = ? AND goods_id = ?", source, channel, goodsID).
		Delete(&model.ActivityProduct{})
	if result.Error != nil {
		response.FailHTTP(c, 500, result.Error.Error())
		return
	}
	if result.RowsAffected == 0 {
		response.Fail(c, errcode.CodeUpdateZero)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}
