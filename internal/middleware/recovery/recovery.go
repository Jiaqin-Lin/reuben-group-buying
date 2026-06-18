// Package recovery panic 恢复中间件，捕获 panic 并返回 500。
package recovery

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

// Middleware 返回 panic 恢复中间件。
func Middleware(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				stack := string(debug.Stack())
				traceID, _ := c.Get("trace_id")

				log.Error("panic recovered",
					"panic", fmt.Sprintf("%v", r),
					"trace_id", traceID,
					"path", c.Request.URL.Path,
					"method", c.Request.Method,
					"stack", stack,
				)

				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"code": "0001",
					"info": "服务器内部错误",
				})
			}
		}()

		c.Next()
	}
}
