// Package tracing 注入 traceID 到请求上下文，并在响应头中返回。
// 如果请求头已有 X-Trace-Id 则沿用，否则生成新的 UUID。
package tracing

import "github.com/gin-gonic/gin"

// Middleware 返回 traceID 注入中间件。
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: extract or generate traceID, set in context + response header
		c.Next()
	}
}
