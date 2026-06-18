// Package logging 请求日志中间件，记录 method、path、status、latency、traceID。
package logging

import "github.com/gin-gonic/gin"

// Middleware 返回请求日志中间件。
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: log request start, then log after c.Next() with status + latency
		c.Next()
	}
}
