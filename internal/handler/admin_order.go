package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/reuben/group-buying/internal/errcode"
	"github.com/reuben/group-buying/internal/model"
	"github.com/reuben/group-buying/internal/response"
)

// AdminOrderHandler 订单监控（只读）。
type AdminOrderHandler struct {
	db *gorm.DB
}

func NewAdminOrderHandler(db *gorm.DB) *AdminOrderHandler {
	return &AdminOrderHandler{db: db}
}

// ListOrders GET /api/v1/admin/orders?status=0&user_id=X&page=1&page_size=20
func (h *AdminOrderHandler) ListOrders(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	db := h.db.WithContext(c.Request.Context()).Model(&model.Order{})
	if s := c.Query("status"); s != "" {
		status, _ := strconv.Atoi(s)
		db = db.Where("status = ?", status)
	}
	if aid := c.Query("activity_id"); aid != "" {
		id, _ := strconv.ParseInt(aid, 10, 64)
		db = db.Where("activity_id = ?", id)
	}
	if uid := c.Query("user_id"); uid != "" {
		db = db.Where("user_id = ?", uid)
	}

	var total int64
	db.Count(&total)

	var orders []model.Order
	offset := (page - 1) * pageSize
	db.Order("id DESC").Offset(offset).Limit(pageSize).Find(&orders)

	response.Success(c, gin.H{
		"items":     orders,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GetOrder GET /api/v1/admin/orders/:order_id
func (h *AdminOrderHandler) GetOrder(c *gin.Context) {
	orderID := c.Param("order_id")
	var order model.Order
	if err := h.db.WithContext(c.Request.Context()).Where("order_id = ?", orderID).First(&order).Error; err != nil {
		response.Fail(c, errcode.CodeOrderNotFound)
		return
	}
	response.Success(c, order)
}

// ===== 用户端订单接口 =====

// ListOrdersByUser GET /api/v1/orders?user_id=X
func (h *AdminOrderHandler) ListOrdersByUser(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}
	var orders []model.Order
	h.db.WithContext(c.Request.Context()).
		Where("user_id = ?", userID).
		Order("id DESC").Limit(100).
		Find(&orders)
	response.Success(c, orders)
}

// GetOrderByOutTradeNo GET /api/v1/orders/:out_trade_no
func (h *AdminOrderHandler) GetOrderByOutTradeNo(c *gin.Context) {
	outTradeNo := c.Param("out_trade_no")
	var order model.Order
	if err := h.db.WithContext(c.Request.Context()).
		Where("out_trade_no = ?", outTradeNo).First(&order).Error; err != nil {
		response.Fail(c, errcode.CodeOrderNotFound)
		return
	}
	response.Success(c, order)
}
