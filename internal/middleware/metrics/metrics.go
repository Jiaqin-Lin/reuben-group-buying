// Package metrics HTTP 请求指标中间件。
//
// 记录每个请求的 endpoint、HTTP 状态码和耗时，写入 Prometheus Counter/Histogram。
package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/reuben/group-buying/internal/metrics"
)

// Middleware 返回一个记录请求指标的 gin 中间件。
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start).Seconds()
		endpoint := c.FullPath()
		if endpoint == "" {
			endpoint = "unknown"
		}

		// 状态码分类
		status := c.Writer.Status()
		var code string
		switch {
		case status >= 500:
			code = "5xx"
		case status >= 400:
			code = "4xx"
		case status >= 200:
			code = "2xx"
		default:
			code = strconv.Itoa(status)
		}

		metrics.RecordRequest(endpoint, code, duration)
	}
}
