package repository

import (
	"context"
	"encoding/json"
	"time"
)

// CacheRepository 缓存操作接口。
//
// 所有缓存操作集中定义在这里，实现放 redisx 包（Phase 3）。
// repository 层的其他文件通过此接口访问缓存，不直接依赖 Redis。
//
// Redis Key 约定（详见 CLAUDE.md）：
//
//	团名额占用     bgm:stock:{activityId}:{teamId}:holders   SET
//	团满标         bgm:stock:{activityId}:{teamId}:full      String
//	用户限购计数   bgm:take:{activityId}:{userId}             int64
//	锁单结果缓存   bgm:lock:result:{userId}:{outTradeNo}      JSON (10min TTL)
//	分布式锁       bgm:lock:order:{userId}:{outTradeNo}       lock (3s TTL)
//	人群标签成员   bgm:tag:{tagId}:members                    BitSet
type CacheRepository interface {
	// --- 名额占用（Lua 脚本，原子操作）---

	// TryOccupyStock 尝试占用团名额。
	// permitID = out_trade_no，防同一外部单号重复占位。
	// 返回 true 表示占用成功，false 表示已满或已占用。
	TryOccupyStock(ctx context.Context, activityID int64, teamID string, permitID string, maxSlots int, ttl time.Duration) (bool, error)

	// ReleaseStock 释放名额（SREM out_trade_no）。
	// 退单时调用，不关心是否成功（可能本来就不在集合中）。
	ReleaseStock(ctx context.Context, activityID int64, teamID string, permitID string) error

	// CheckStock 检查名额是否已被占用（SISMEMBER）。
	CheckStock(ctx context.Context, activityID int64, teamID string, permitID string) (bool, error)

	// MarkTeamFull 标记团已满。
	MarkTeamFull(ctx context.Context, activityID int64, teamID string, ttl time.Duration) error

	// IsTeamFull 检查团是否已满。
	IsTeamFull(ctx context.Context, activityID int64, teamID string) (bool, error)

	// --- 用户限购计数 ---

	// IncrTakeCount 原子增加用户参与次数（支付成功时 +1）。
	// 返回增加后的值。
	IncrTakeCount(ctx context.Context, activityID int64, userID string) (int64, error)

	// GetTakeCount 获取用户当前参与次数。
	GetTakeCount(ctx context.Context, activityID int64, userID string) (int64, error)

	// --- 锁单结果缓存 ---

	// CacheLockResult 缓存锁单结果（10min TTL）。
	// 用于幂等：同 out_trade_no 重复请求直接返回缓存结果。
	CacheLockResult(ctx context.Context, userID, outTradeNo string, result json.RawMessage, ttl time.Duration) error

	// GetLockResult 获取缓存的锁单结果。
	// 返回 nil 表示未命中缓存。
	GetLockResult(ctx context.Context, userID, outTradeNo string) (json.RawMessage, error)

	// --- 分布式锁 ---

	// AcquireLock 获取分布式锁。
	// 返回 true 表示获取成功。
	AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error)

	// ReleaseLock 释放分布式锁。
	ReleaseLock(ctx context.Context, key string) error

	// --- 人群标签缓存 ---

	// SetCrowdMembers 设置人群标签成员 BitSet。
	SetCrowdMembers(ctx context.Context, tagID string, userIDs []string) error

	// CheckCrowdMember 检查用户是否在人群标签中（缓存层）。
	CheckCrowdMember(ctx context.Context, tagID, userID string) (bool, error)
}

// stubCacheRepo 空实现，Phase 3 替换为真实 Redis 实现。
type stubCacheRepo struct{}

// NewStubCacheRepo Phase 2 临时使用，Phase 3 废弃。
func NewStubCacheRepo() CacheRepository {
	return &stubCacheRepo{}
}

func (s *stubCacheRepo) TryOccupyStock(ctx context.Context, activityID int64, teamID string, permitID string, maxSlots int, ttl time.Duration) (bool, error) {
	return true, nil
}
func (s *stubCacheRepo) ReleaseStock(ctx context.Context, activityID int64, teamID string, permitID string) error {
	return nil
}
func (s *stubCacheRepo) CheckStock(ctx context.Context, activityID int64, teamID string, permitID string) (bool, error) {
	return false, nil
}
func (s *stubCacheRepo) MarkTeamFull(ctx context.Context, activityID int64, teamID string, ttl time.Duration) error {
	return nil
}
func (s *stubCacheRepo) IsTeamFull(ctx context.Context, activityID int64, teamID string) (bool, error) {
	return false, nil
}
func (s *stubCacheRepo) IncrTakeCount(ctx context.Context, activityID int64, userID string) (int64, error) {
	return 0, nil
}
func (s *stubCacheRepo) GetTakeCount(ctx context.Context, activityID int64, userID string) (int64, error) {
	return 0, nil
}
func (s *stubCacheRepo) CacheLockResult(ctx context.Context, userID, outTradeNo string, result json.RawMessage, ttl time.Duration) error {
	return nil
}
func (s *stubCacheRepo) GetLockResult(ctx context.Context, userID, outTradeNo string) (json.RawMessage, error) {
	return nil, nil
}
func (s *stubCacheRepo) AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return true, nil
}
func (s *stubCacheRepo) ReleaseLock(ctx context.Context, key string) error {
	return nil
}
func (s *stubCacheRepo) SetCrowdMembers(ctx context.Context, tagID string, userIDs []string) error {
	return nil
}
func (s *stubCacheRepo) CheckCrowdMember(ctx context.Context, tagID, userID string) (bool, error) {
	return false, nil
}
