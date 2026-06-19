package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/reuben/group-buying/internal/redisx"
)

// CacheRepository 缓存操作接口。
//
// 所有缓存操作集中定义在这里，实现对 redisx 包的薄封装。
// repository 层的其他文件通过此接口访问缓存，不直接依赖 Redis。
//
// Redis Key 约定（详见 CLAUDE.md）：
//
//	团名额占用     bgm:stock:{activityId}:{teamId}:holders   SET
//	团满标         bgm:stock:{activityId}:{teamId}:full      String
//	用户限购计数   bgm:take:{activityId}:{userId}             int64
//	锁单结果缓存   bgm:lock:result:{userId}:{outTradeNo}      JSON (10min TTL)
//	分布式锁       bgm:lock:order:{userId}:{outTradeNo}       lock (3s TTL)
//	人群标签成员   bgm:tag:{tagId}:members                    SET
type CacheRepository interface {
	// --- 名额占用（Lua 脚本，原子操作）---

	// TryOccupyStock 尝试占用团名额。
	// permitID = out_trade_no，防同一外部单号重复占位。
	// 返回 true 表示占用成功（含幂等），false 表示已满。
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

	// SetCrowdMembers 设置人群标签成员（批量 SADD）。
	SetCrowdMembers(ctx context.Context, tagID string, userIDs []string) error

	// CheckCrowdMember 检查用户是否在人群标签中（SISMEMBER）。
	CheckCrowdMember(ctx context.Context, tagID, userID string) (bool, error)
}

// redisCacheRepo 基于 go-redis 的 CacheRepository 实现。
// 对 redisx 包的薄封装：主要负责 key 构建和结果映射，
// 真正的 Redis 原子操作（Lua 脚本）放在 redisx 包。
type redisCacheRepo struct {
	rdb *goredis.Client
}

// NewRedisCacheRepo 创建 Redis 缓存仓库。
func NewRedisCacheRepo(rdb *goredis.Client) CacheRepository {
	return &redisCacheRepo{rdb: rdb}
}

// --- 名额占用 ---

func (r *redisCacheRepo) TryOccupyStock(ctx context.Context, activityID int64, teamID string, permitID string, maxSlots int, ttl time.Duration) (bool, error) {
	result, err := redisx.TryOccupyStock(ctx, r.rdb, activityID, teamID, permitID, maxSlots, ttl)
	if err != nil {
		return false, fmt.Errorf("cache try occupy stock: %w", err)
	}
	// OccupyOK (1) 和 OccupyIdempotent (2) 都视为成功
	// OccupyFull (-1) 才返回 false
	return result != redisx.OccupyFull, nil
}

func (r *redisCacheRepo) ReleaseStock(ctx context.Context, activityID int64, teamID string, permitID string) error {
	err := redisx.ReleaseStock(ctx, r.rdb, activityID, teamID, permitID)
	if err != nil {
		return fmt.Errorf("cache release stock: %w", err)
	}
	return nil
}

func (r *redisCacheRepo) CheckStock(ctx context.Context, activityID int64, teamID string, permitID string) (bool, error) {
	ok, err := redisx.CheckStock(ctx, r.rdb, activityID, teamID, permitID)
	if err != nil {
		return false, fmt.Errorf("cache check stock: %w", err)
	}
	return ok, nil
}

func (r *redisCacheRepo) MarkTeamFull(ctx context.Context, activityID int64, teamID string, ttl time.Duration) error {
	err := redisx.MarkTeamFull(ctx, r.rdb, activityID, teamID, ttl)
	if err != nil {
		return fmt.Errorf("cache mark team full: %w", err)
	}
	return nil
}

func (r *redisCacheRepo) IsTeamFull(ctx context.Context, activityID int64, teamID string) (bool, error) {
	ok, err := redisx.IsTeamFull(ctx, r.rdb, activityID, teamID)
	if err != nil {
		return false, fmt.Errorf("cache is team full: %w", err)
	}
	return ok, nil
}

// --- 用户限购计数 ---

func (r *redisCacheRepo) IncrTakeCount(ctx context.Context, activityID int64, userID string) (int64, error) {
	count, err := redisx.IncrTakeCount(ctx, r.rdb, activityID, userID)
	if err != nil {
		return 0, fmt.Errorf("cache incr take count: %w", err)
	}
	return count, nil
}

func (r *redisCacheRepo) GetTakeCount(ctx context.Context, activityID int64, userID string) (int64, error) {
	count, err := redisx.GetTakeCount(ctx, r.rdb, activityID, userID)
	if err != nil {
		return 0, fmt.Errorf("cache get take count: %w", err)
	}
	return count, nil
}

// --- 锁单结果缓存 ---

func (r *redisCacheRepo) CacheLockResult(ctx context.Context, userID, outTradeNo string, result json.RawMessage, ttl time.Duration) error {
	key := redisx.LockResultKey(userID, outTradeNo)
	// redisx.CacheSet 内部会 json.Marshal，所以传 string(result) 会被双重编码。
	// 直接用 rdb.Set 存储原始 JSON 字节。
	ttlSec := max(ttl.Seconds(), 1)
	err := r.rdb.Set(ctx, key, string(result), time.Duration(ttlSec)*time.Second).Err()
	if err != nil {
		return fmt.Errorf("cache lock result: %w", err)
	}
	return nil
}

func (r *redisCacheRepo) GetLockResult(ctx context.Context, userID, outTradeNo string) (json.RawMessage, error) {
	key := redisx.LockResultKey(userID, outTradeNo)
	data, err := r.rdb.Get(ctx, key).Bytes()
	if err == goredis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cache get lock result: %w", err)
	}
	return json.RawMessage(data), nil
}

// --- 分布式锁 ---

func (r *redisCacheRepo) AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	acquired, err := redisx.AcquireLockSimple(ctx, r.rdb, key, ttl)
	if err != nil {
		return false, fmt.Errorf("cache acquire lock: %w", err)
	}
	return acquired, nil
}

func (r *redisCacheRepo) ReleaseLock(ctx context.Context, key string) error {
	err := redisx.ReleaseLockSimple(ctx, r.rdb, key)
	if err != nil {
		return fmt.Errorf("cache release lock: %w", err)
	}
	return nil
}

// --- 人群标签缓存 ---

func (r *redisCacheRepo) SetCrowdMembers(ctx context.Context, tagID string, userIDs []string) error {
	err := redisx.AddCrowdMembers(ctx, r.rdb, tagID, userIDs)
	if err != nil {
		return fmt.Errorf("cache set crowd members: %w", err)
	}
	return nil
}

func (r *redisCacheRepo) CheckCrowdMember(ctx context.Context, tagID, userID string) (bool, error) {
	ok, err := redisx.CheckCrowdMember(ctx, r.rdb, tagID, userID)
	if err != nil {
		return false, fmt.Errorf("cache check crowd member: %w", err)
	}
	return ok, nil
}
