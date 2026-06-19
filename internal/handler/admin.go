package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/reuben/group-buying/internal/config/dynamic"
	"github.com/reuben/group-buying/internal/errcode"
	"github.com/reuben/group-buying/internal/response"
)

// AdminHandler 管理接口。
type AdminHandler struct {
	dynMgr *dynamic.Manager
}

// NewAdminHandler 构造函数。
func NewAdminHandler(dynMgr *dynamic.Manager) *AdminHandler {
	return &AdminHandler{dynMgr: dynMgr}
}

// ListConfigs GET /api/v1/admin/configs
// 返回所有动态配置项的当前值。
func (h *AdminHandler) ListConfigs(c *gin.Context) {
	all := h.dynMgr.GetAll()
	response.Success(c, all)
}

// updateConfigReq 更新配置请求体。
type updateConfigReq struct {
	Value     any    `json:"value" binding:"required"`
	UpdatedBy string `json:"updated_by"`
}

// UpdateConfig PUT /api/v1/admin/configs/:key
// 更新指定配置项，即时生效并同步所有实例。
//
// 请求体: {"value": 30, "updated_by": "admin"}
func (h *AdminHandler) UpdateConfig(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}

	var req updateConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.CodeInvalidParam)
		return
	}

	if req.UpdatedBy == "" {
		req.UpdatedBy = "api"
	}

	if err := h.dynMgr.Set(c.Request.Context(), key, req.Value, req.UpdatedBy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": errcode.CodeUnknownErr,
			"info": err.Error(),
		})
		return
	}

	response.Success(c, gin.H{"key": key, "updated": true})
}
