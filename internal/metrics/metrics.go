// Package metrics Prometheus 指标注册中心。
//
// 提供全局 Counter、Histogram、Gauge，替代原先 app.go 中的 atomic.Int64 全局计数器。
// 所有指标注册到 prometheus.DefaultRegisterer，由 promhttp.Handler 暴露 /metrics 端点。
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// --- Counter: HTTP 请求 ---

// RequestsTotal 请求总数，labels: endpoint（路由路径）、code（HTTP 状态码字符串 "2xx"/"4xx"/"5xx"）。
var RequestsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "groupbuy_requests_total",
		Help: "Total number of HTTP requests.",
	},
	[]string{"endpoint", "code"},
)

// RequestDuration 请求耗时分布，label: endpoint。
// 默认 bucket 适合 API 场景（50ms ~ 5s）。
var RequestDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "groupbuy_request_duration_seconds",
		Help:    "HTTP request latency in seconds.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	},
	[]string{"endpoint"},
)

// --- Counter: 缓存 ---

// CacheOperations 缓存操作计数，label: result（hit/miss）。
var CacheOperations = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "groupbuy_cache_total",
		Help: "Total number of cache operations.",
	},
	[]string{"result"},
)

// --- Counter: Redis ---

// RedisOperations Redis 操作计数，labels: operation（occupy/release/lock/incr/get/set/del）、result（ok/err）。
var RedisOperations = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "groupbuy_redis_total",
		Help: "Total number of Redis operations.",
	},
	[]string{"operation", "result"},
)

// --- Counter: DB ---

// DBOperations 数据库操作计数，labels: operation（select/insert/update/delete）、result（ok/err）。
var DBOperations = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "groupbuy_db_total",
		Help: "Total number of database operations.",
	},
	[]string{"operation", "result"},
)

// --- Counter: 业务错误 ---

// BusinessErrors 业务错误码分布，label: code（errcode 常量如 E0103）。
var BusinessErrors = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "groupbuy_business_errors_total",
		Help: "Total number of business errors by error code.",
	},
	[]string{"code"},
)

func init() {
	// 注册所有指标
	prometheus.MustRegister(
		RequestsTotal,
		RequestDuration,
		CacheOperations,
		RedisOperations,
		DBOperations,
		BusinessErrors,
	)

	// 注意：Go/Process collector 已由 prometheus.DefaultRegisterer 自动注册，无需重复。
}

// --- 便捷函数（供 service / redisx 层调用）---

// IncrCacheHit 缓存命中 +1。
func IncrCacheHit() {
	CacheOperations.WithLabelValues("hit").Inc()
}

// IncrCacheMiss 缓存未命中 +1。
func IncrCacheMiss() {
	CacheOperations.WithLabelValues("miss").Inc()
}

// IncrRedis Redis 操作计数 +1。
// operation: occupy, release, lock, incr, get, set, del
// result: ok, err
func IncrRedis(operation, result string) {
	RedisOperations.WithLabelValues(operation, result).Inc()
}

// IncrDB 数据库操作计数 +1。
// operation: select, insert, update, delete
// result: ok, err
func IncrDB(operation, result string) {
	DBOperations.WithLabelValues(operation, result).Inc()
}

// IncrBusinessError 业务错误计数 +1。
// code: errcode 常量如 "E0103"
func IncrBusinessError(code string) {
	BusinessErrors.WithLabelValues(code).Inc()
}

// RecordRequest 记录一次 HTTP 请求（计数 + 耗时）。
// endpoint: 路由路径（如 /api/v1/trade/lock）
// code: HTTP 状态码分类 "2xx"/"4xx"/"5xx"
// duration: 请求耗时（秒）
func RecordRequest(endpoint, code string, duration float64) {
	RequestsTotal.WithLabelValues(endpoint, code).Inc()
	RequestDuration.WithLabelValues(endpoint).Observe(duration)
}
