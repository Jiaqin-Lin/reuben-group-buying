package dynamic

// 所有动态配置定义（中央注册表）。
//
// 命名约定:
//   - 变量名: PascalCase，自描述
//   - Key:    lowercase_with_underscores，前缀按模块分组
//     trial.*   — 试算相关
//     lock.*    — 锁单相关
//     order.*   — 订单相关
//     notify.*  — 回调通知相关
//     timeout.* — 超时扫描相关
//     feature.* — 功能开关（主要用于降级）
//
// 添加新配置: 在下面新增一个 var 定义，然后在 app.go 的 Register 调用中加入即可。

// --- 试算 ---

// TrialCacheTTL 试算结果缓存 TTL（秒）。默认 0 禁用——本地内存缓存已足够快。
var TrialCacheTTL = NewDef("trial.cache_ttl", 0, "试算结果缓存TTL（秒），0=禁用")

// --- 锁单 ---

// LockResultTTL 锁单结果缓存 TTL（秒）。用于幂等：同 outTradeNo 重复请求返回缓存结果。
var LockResultTTL = NewDef("lock.result_ttl", 600, "锁单结果缓存TTL（秒）")

// OrderLockTTL 订单分布式锁 TTL（秒）。防同一 outTradeNo 并发请求。
var OrderLockTTL = NewDef("order.lock_ttl", 3, "订单分布式锁TTL（秒）")

// --- 回调通知 ---

// NotifyMaxRetry 回调通知最大重试次数。超出后任务标记为 Fail(3)。
var NotifyMaxRetry = NewDef("notify.max_retry", 5, "回调通知最大重试次数")

// NotifyWorkerCount 回调通知并发 worker 数。控制同时发送的最大 HTTP 请求数。
var NotifyWorkerCount = NewDef("notify.worker_count", 10, "回调通知并发worker数")

// --- 超时扫描 ---

// TimeoutScanBatch 超时扫描每批处理数量。游标分页，每批处理完后继续下一批。
var TimeoutScanBatch = NewDef("timeout.scan_batch", 100, "超时扫描每批数量")

// --- 功能开关 ---

// FeatureSkipCrowd 降级开关：跳过人群标签检查。紧急降级时设为 true。
var FeatureSkipCrowd = NewDef("feature.skip_crowd", false, "降级：跳过人群标签检查")

// FeatureSkipPayment 降级开关：跳过支付（默认开启，对齐 Java 版无支付）。
var FeatureSkipPayment = NewDef("feature.skip_payment", true, "降级：跳过支付（测试用）")
