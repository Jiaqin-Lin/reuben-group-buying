// Package redisx 封装 Redis 业务操作（Lua 脚本执行、分布式锁、缓存读写）。
//
// 所有对 Redis 的业务操作都通过本包完成，不直接在 service/repository 层
// 拼接 Redis 命令。Key 命名统一用 bgm: 前缀 + 冒号分隔。
//
// Key 约定:
//
//	bgm:stock:{activityId}:{teamId}:holders   SET    团名额占用（成员=outTradeNo）
//	bgm:stock:{activityId}:{teamId}:full      String  团满标哨兵
//	bgm:take:{activityId}:{userId}            int64   用户限购计数
//	bgm:lock:order:{userId}:{outTradeNo}      String  分布式锁
//	bgm:lock:result:{userId}:{outTradeNo}     JSON    锁单结果缓存（幂等）
//	bgm:tag:{tagId}:members                   SET     人群标签成员
package redisx

import "fmt"

const prefix = "bgm"

// --- 名额占用 ---

// StockHoldersKey 团名额占用 SET 的 key。
// SET 成员为 outTradeNo，SCARD 得到当前已占名额数。
func StockHoldersKey(activityID int64, teamID string) string {
	return fmt.Sprintf("%s:stock:%d:%s:holders", prefix, activityID, teamID)
}

// StockFullKey 团满标哨兵的 key。
// 存在即表示团已满，用于快速拒绝而非每次都 SCARD。
func StockFullKey(activityID int64, teamID string) string {
	return fmt.Sprintf("%s:stock:%d:%s:full", prefix, activityID, teamID)
}

// --- 用户限购 ---

// TakeLimitKey 用户在某活动下的参与次数 key。
// 支付成功时 +1，TTL 对齐活动结束时间。
func TakeLimitKey(activityID int64, userID string) string {
	return fmt.Sprintf("%s:take:%d:%s", prefix, activityID, userID)
}

// --- 分布式锁 ---

// LockOrderKey 订单分布式锁的 key。
// 锁粒度 (userId, outTradeNo)，仅防同一外部单号并发。
func LockOrderKey(userID, outTradeNo string) string {
	return fmt.Sprintf("%s:lock:order:%s:%s", prefix, userID, outTradeNo)
}

// --- 锁单结果缓存 ---

// LockResultKey 锁单结果缓存的 key。
// TTL 10 分钟，用于幂等：同 outTradeNo 重复请求直接返回缓存结果。
func LockResultKey(userID, outTradeNo string) string {
	return fmt.Sprintf("%s:lock:result:%s:%s", prefix, userID, outTradeNo)
}

// --- 人群标签 ---

// CrowdMembersKey 人群标签成员 SET 的 key。
// SET 成员为 userId，SISMEMBER 检查用户是否在标签中。
func CrowdMembersKey(tagID string) string {
	return fmt.Sprintf("%s:tag:%s:members", prefix, tagID)
}

// --- 定时任务锁 ---

// TimeoutScanLockKey 超时扫描定时任务的分布式锁 key。
// 多实例部署时，同一时刻只有一个实例执行超时扫描。
func TimeoutScanLockKey() string {
	return fmt.Sprintf("%s:lock:timeout:scanner", prefix)
}

// --- 动态配置通知 ---

// ConfigChannel 动态配置变更通知的 Redis Pub/Sub 频道名。
// Manager.Set() 写 MySQL 后 Publish 到此频道，所有实例收到后重读 MySQL。
func ConfigChannel() string {
	return fmt.Sprintf("%s:config:updates", prefix)
}
