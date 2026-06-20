// Package admin 管理端鉴权中间件。
//
// 通过 X-Admin-Token header 校验管理员身份。
// token 从配置文件的 admin.token 读取，不配置时拒绝所有管理端请求。
package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/reuben/group-buying/internal/errcode"
	"github.com/reuben/group-buying/internal/response"
)

// Middleware 返回管理端鉴权中间件。
//
// 检查 X-Admin-Token header，与 token 对比。
// 匹配通过；不匹配或缺失返回 403 + JSON 响应。
//
// 使用方式：
//
//	adminGroup := router.Group("/api/v1/admin")
//	adminGroup.Use(admin.Middleware(cfg.AdminToken))
func Middleware(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if token == "" {
			response.FailWithMsg(c, errcode.CodeAuthFailed, "admin token not configured")
			c.Abort()
			return
		}

		if c.GetHeader("X-Admin-Token") != token {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code": errcode.CodeAuthFailed,
				"info": "forbidden: invalid admin token",
			})
			return
		}

		c.Next()
	}
}
