// Package recovery panic 恢复中间件，捕获 panic 并返回 500。
package recovery

import "github.com/gin-gonic/gin"

// Middleware 返回 panic 恢复中间件。
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: recover from panic, log stack trace, return 500
		c.Next()
	}
}
