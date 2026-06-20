package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/reuben/group-buying/internal/errcode"
	"github.com/reuben/group-buying/internal/model"
	"github.com/reuben/group-buying/internal/response"
)

// AdminCrowdHandler 人群标签管理。
type AdminCrowdHandler struct {
	db *gorm.DB
}

func NewAdminCrowdHandler(db *gorm.DB) *AdminCrowdHandler {
	return &AdminCrowdHandler{db: db}
}

// ===== 标签 CRUD =====

// ListTags GET /api/v1/admin/crowd-tags
func (h *AdminCrowdHandler) ListTags(c *gin.Context) {
	var tags []model.CrowdTag
	if err := h.db.WithContext(c.Request.Context()).Find(&tags).Error; err != nil {
		response.FailHTTP(c, 500, err.Error())
		return
	}
	response.Success(c, tags)
}

// GetTag GET /api/v1/admin/crowd-tags/:tag_id
func (h *AdminCrowdHandler) GetTag(c *gin.Context) {
	tagID := c.Param("tag_id")
	var tag model.CrowdTag
	if err := h.db.WithContext(c.Request.Context()).Where("tag_id = ?", tagID).First(&tag).Error; err != nil {
		response.Fail(c, errcode.CodeOrderNotFound)
		return
	}
	response.Success(c, tag)
}

// CreateTag POST /api/v1/admin/crowd-tags
func (h *AdminCrowdHandler) CreateTag(c *gin.Context) {
	var tag model.CrowdTag
	if err := c.ShouldBindJSON(&tag); err != nil {
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&tag).Error; err != nil {
		response.FailHTTP(c, 500, err.Error())
		return
	}
	response.Success(c, tag)
}

// UpdateTag PUT /api/v1/admin/crowd-tags/:tag_id
func (h *AdminCrowdHandler) UpdateTag(c *gin.Context) {
	tagID := c.Param("tag_id")
	var updates map[string]any
	if err := c.ShouldBindJSON(&updates); err != nil {
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}
	delete(updates, "tag_id")
	result := h.db.WithContext(c.Request.Context()).Model(&model.CrowdTag{}).
		Where("tag_id = ?", tagID).Updates(updates)
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

// DeleteTag DELETE /api/v1/admin/crowd-tags/:tag_id
func (h *AdminCrowdHandler) DeleteTag(c *gin.Context) {
	tagID := c.Param("tag_id")
	result := h.db.WithContext(c.Request.Context()).
		Where("tag_id = ?", tagID).Delete(&model.CrowdTag{})
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

// ===== 成员管理 =====

// ListMembers GET /api/v1/admin/crowd-tags/:tag_id/members?page=1&page_size=50
func (h *AdminCrowdHandler) ListMembers(c *gin.Context) {
	tagID := c.Param("tag_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 500 {
		pageSize = 50
	}

	var total int64
	h.db.WithContext(c.Request.Context()).Model(&model.CrowdTagDetail{}).
		Where("tag_id = ?", tagID).Count(&total)

	var members []model.CrowdTagDetail
	offset := (page - 1) * pageSize
	h.db.WithContext(c.Request.Context()).
		Where("tag_id = ?", tagID).
		Order("id ASC").Offset(offset).Limit(pageSize).
		Find(&members)

	response.Success(c, gin.H{
		"items":     members,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// AddMember POST /api/v1/admin/crowd-tags/:tag_id/members
func (h *AdminCrowdHandler) AddMember(c *gin.Context) {
	tagID := c.Param("tag_id")
	var req struct {
		UserID string `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}

	member := model.CrowdTagDetail{
		TagID:  tagID,
		UserID: req.UserID,
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&member).Error; err != nil {
		response.FailHTTP(c, 500, err.Error())
		return
	}
	response.Success(c, member)
}

// RemoveMember DELETE /api/v1/admin/crowd-tags/:tag_id/members/:user_id
func (h *AdminCrowdHandler) RemoveMember(c *gin.Context) {
	tagID := c.Param("tag_id")
	userID := c.Param("user_id")
	result := h.db.WithContext(c.Request.Context()).
		Where("tag_id = ? AND user_id = ?", tagID, userID).
		Delete(&model.CrowdTagDetail{})
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
