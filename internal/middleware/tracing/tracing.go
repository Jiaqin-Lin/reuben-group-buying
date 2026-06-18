// Package tracing 注入 traceID 到请求上下文，并在响应头中返回。
// 如果请求头已有 X-Trace-Id 则沿用，否则生成新的 UUID v4。
package tracing

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// TraceIDKey context 中 traceID 的 key 类型。
type TraceIDKey struct{}

// Middleware 返回 traceID 注入中间件。
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("X-Trace-Id")
		if traceID == "" {
			traceID = uuid.New().String()
		}

		// 注入到 context
		ctx := context.WithValue(c.Request.Context(), TraceIDKey{}, traceID)
		c.Request = c.Request.WithContext(ctx)

		// 响应头返回
		c.Header("X-Trace-Id", traceID)

		// 存入 gin.Context，方便 handler 直接取
		c.Set("trace_id", traceID)

		c.Next()
	}
}

// GetTraceID 从 context 获取 traceID。
func GetTraceID(ctx context.Context) string {
	if v, ok := ctx.Value(TraceIDKey{}).(string); ok {
		return v
	}
	return ""
}
