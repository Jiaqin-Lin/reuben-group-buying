// Package ratelimit 限流中间件，基于 Redis 令牌桶算法。
package ratelimit

import "github.com/gin-gonic/gin"

// Middleware 返回限流中间件。
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: check rate limit via Redis token bucket
		c.Next()
	}
}
