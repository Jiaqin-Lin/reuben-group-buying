package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/reuben/group-buying/internal/errcode"
	"github.com/reuben/group-buying/internal/model"
	"github.com/reuben/group-buying/internal/response"
)

// AdminActivityHandler 活动与折扣管理。
type AdminActivityHandler struct {
	db *gorm.DB
}

func NewAdminActivityHandler(db *gorm.DB) *AdminActivityHandler {
	return &AdminActivityHandler{db: db}
}

// ===== 活动 CRUD =====

// ListActivities GET /api/v1/admin/activities
func (h *AdminActivityHandler) ListActivities(c *gin.Context) {
	var activities []model.Activity
	if err := h.db.WithContext(c.Request.Context()).Find(&activities).Error; err != nil {
		response.FailHTTP(c, 500, err.Error())
		return
	}
	response.Success(c, activities)
}

// GetActivity GET /api/v1/admin/activities/:id
func (h *AdminActivityHandler) GetActivity(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}
	var a model.Activity
	if err := h.db.WithContext(c.Request.Context()).Where("activity_id = ?", id).First(&a).Error; err != nil {
		response.Fail(c, errcode.CodeOrderNotFound)
		return
	}
	response.Success(c, a)
}

// CreateActivity POST /api/v1/admin/activities
func (h *AdminActivityHandler) CreateActivity(c *gin.Context) {
	var a model.Activity
	if err := c.ShouldBindJSON(&a); err != nil {
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&a).Error; err != nil {
		response.FailHTTP(c, 500, err.Error())
		return
	}
	response.Success(c, a)
}

// UpdateActivity PUT /api/v1/admin/activities/:id
func (h *AdminActivityHandler) UpdateActivity(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}
	var updates map[string]any
	if err := c.ShouldBindJSON(&updates); err != nil {
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}
	// 不允许修改 activity_id
	delete(updates, "activity_id")
	result := h.db.WithContext(c.Request.Context()).Model(&model.Activity{}).
		Where("activity_id = ?", id).Updates(updates)
	if result.Error != nil {
		response.FailHTTP(c, 500, result.Error.Error())
		return
	}
	if result.RowsAffected == 0 {
		response.Fail(c, errcode.CodeUpdateZero)
		return
	}
	response.Success(c, gin.H{"updated": true})
}

// DeleteActivity DELETE /api/v1/admin/activities/:id
func (h *AdminActivityHandler) DeleteActivity(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}
	result := h.db.WithContext(c.Request.Context()).
		Where("activity_id = ?", id).Delete(&model.Activity{})
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

// ===== 折扣 CRUD =====

// ListDiscounts GET /api/v1/admin/discounts
func (h *AdminActivityHandler) ListDiscounts(c *gin.Context) {
	var discounts []model.Discount
	if err := h.db.WithContext(c.Request.Context()).Find(&discounts).Error; err != nil {
		response.FailHTTP(c, 500, err.Error())
		return
	}
	response.Success(c, discounts)
}

// GetDiscount GET /api/v1/admin/discounts/:id
func (h *AdminActivityHandler) GetDiscount(c *gin.Context) {
	id := c.Param("id")
	var d model.Discount
	if err := h.db.WithContext(c.Request.Context()).Where("discount_id = ?", id).First(&d).Error; err != nil {
		response.Fail(c, errcode.CodeOrderNotFound)
		return
	}
	response.Success(c, d)
}

// CreateDiscount POST /api/v1/admin/discounts
func (h *AdminActivityHandler) CreateDiscount(c *gin.Context) {
	var d model.Discount
	if err := c.ShouldBindJSON(&d); err != nil {
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&d).Error; err != nil {
		response.FailHTTP(c, 500, err.Error())
		return
	}
	response.Success(c, d)
}

// UpdateDiscount PUT /api/v1/admin/discounts/:id
func (h *AdminActivityHandler) UpdateDiscount(c *gin.Context) {
	id := c.Param("id")
	var updates map[string]any
	if err := c.ShouldBindJSON(&updates); err != nil {
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}
	delete(updates, "discount_id")
	result := h.db.WithContext(c.Request.Context()).Model(&model.Discount{}).
		Where("discount_id = ?", id).Updates(updates)
	if result.Error != nil {
		response.FailHTTP(c, 500, result.Error.Error())
		return
	}
	if result.RowsAffected == 0 {
		response.Fail(c, errcode.CodeUpdateZero)
		return
	}
	response.Success(c, gin.H{"updated": true})
}

// DeleteDiscount DELETE /api/v1/admin/discounts/:id
func (h *AdminActivityHandler) DeleteDiscount(c *gin.Context) {
	id := c.Param("id")
	result := h.db.WithContext(c.Request.Context()).
		Where("discount_id = ?", id).Delete(&model.Discount{})
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
