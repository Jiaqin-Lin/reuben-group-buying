package handler

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/reuben/group-buying/internal/errcode"
	"github.com/reuben/group-buying/internal/model"
	"github.com/reuben/group-buying/internal/response"
)

// AdminProductHandler 商品管理。
type AdminProductHandler struct {
	db *gorm.DB
}

func NewAdminProductHandler(db *gorm.DB) *AdminProductHandler {
	return &AdminProductHandler{db: db}
}

// ListProducts GET /api/v1/admin/products
func (h *AdminProductHandler) ListProducts(c *gin.Context) {
	var products []model.Product
	if err := h.db.WithContext(c.Request.Context()).Find(&products).Error; err != nil {
		response.FailHTTP(c, 500, err.Error())
		return
	}
	response.Success(c, products)
}

// CreateProduct POST /api/v1/admin/products
func (h *AdminProductHandler) CreateProduct(c *gin.Context) {
	var p model.Product
	if err := c.ShouldBindJSON(&p); err != nil {
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&p).Error; err != nil {
		response.FailHTTP(c, 500, err.Error())
		return
	}
	response.Success(c, p)
}

// UpdateProduct PUT /api/v1/admin/products/:goods_id
func (h *AdminProductHandler) UpdateProduct(c *gin.Context) {
	goodsID := c.Param("goods_id")
	var updates map[string]any
	if err := c.ShouldBindJSON(&updates); err != nil {
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}
	delete(updates, "goods_id")
	result := h.db.WithContext(c.Request.Context()).Model(&model.Product{}).
		Where("goods_id = ?", goodsID).Updates(updates)
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

// DeleteProduct DELETE /api/v1/admin/products/:goods_id
func (h *AdminProductHandler) DeleteProduct(c *gin.Context) {
	goodsID := c.Param("goods_id")
	result := h.db.WithContext(c.Request.Context()).
		Where("goods_id = ?", goodsID).Delete(&model.Product{})
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
