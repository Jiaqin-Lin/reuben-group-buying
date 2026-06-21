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
	// ttl 控制 key 过期时间，对齐活动结束时间以防内存泄漏。
	// 返回增加后的值。
	IncrTakeCount(ctx context.Context, activityID int64, userID string, ttl time.Duration) (int64, error)

	// GetTakeCount 获取用户当前参与次数。
	GetTakeCount(ctx context.Context, activityID int64, userID string) (int64, error)

	// TakeLimitCheckAndIncr 原子检查用户限购次数 + 递增。
	// 支付成功时调用，Lua 脚本原子完成：key 不存在→用 dbCount 初始化→检查>=limit→未达上限则 INCR。
	// 返回 (newCount, true) 表示允许参与，(0, false) 表示已达上限。
	TakeLimitCheckAndIncr(ctx context.Context, activityID int64, userID string, dbCount int64, limit int, ttl time.Duration) (newCount int64, allowed bool, err error)

	// --- 活跃订单计数（锁定+已支付，锁单时软检查限购用）---

	// GetActiveCount 获取用户活跃订单数。key 不存在返回 0。
	GetActiveCount(ctx context.Context, activityID int64, userID string) (int64, error)

	// IncrActiveCount 递增用户活跃订单数（锁单成功时 +1）。
	IncrActiveCount(ctx context.Context, activityID int64, userID string, ttl time.Duration) (int64, error)

	// DecrActiveCount 递减用户活跃订单数（退单/超时时 -1）。
	DecrActiveCount(ctx context.Context, activityID int64, userID string) error

	// InitActiveCount 初始化用户活跃订单数（从 DB 加载后回种，SETNX 语义）。
	InitActiveCount(ctx context.Context, activityID int64, userID string, count int64, ttl time.Duration) error

	// --- 锁单结果缓存 ---

	// CacheLockResult 缓存锁单结果（10min TTL）。
	// 用于幂等：同 out_trade_no 重复请求直接返回缓存结果。
	CacheLockResult(ctx context.Context, userID, outTradeNo string, result json.RawMessage, ttl time.Duration) error

	// GetLockResult 获取缓存的锁单结果。
	// 返回 nil 表示未命中缓存。
	GetLockResult(ctx context.Context, userID, outTradeNo string) (json.RawMessage, error)

	// --- 分布式锁 ---

	// AcquireLock 获取分布式锁（固定 TTL，无自动续期）。
	// 返回 (lock, true, nil) 表示获取成功。
	// 返回 (nil, false, nil) 表示锁被其它进程持有。
	// 调用方必须 defer lock.Release(ctx) 释放锁。
	// 适用于短临界区（业务锁单，TTL 远大于临界区耗时）。
	AcquireLock(ctx context.Context, key string, ttl time.Duration) (*redisx.Lock, bool, error)

	// AcquireLockWithExtend 获取分布式锁（watch-dog 自动续期）。
	// 后台 goroutine 每 ttl/3 续期一次，直到 Release 调用。
	// 适用于长临界区（cron 扫描器，执行时间不可预测）。
	AcquireLockWithExtend(ctx context.Context, key string, ttl time.Duration) (*redisx.Lock, bool, error)

	// --- 人群标签缓存 ---

	// SetCrowdMembers 设置人群标签成员（批量 SADD）。
	SetCrowdMembers(ctx context.Context, tagID string, userIDs []string) error

	// CheckCrowdMember 检查用户是否在人群标签中（SISMEMBER）。
	CheckCrowdMember(ctx context.Context, tagID, userID string) (bool, error)

	// --- 试算结果缓存 ---

	// CacheTrialResult 缓存试算结果（短 TTL，防重试穿透）。
	CacheTrialResult(ctx context.Context, userID, source, channel, goodsID string, result any, ttl time.Duration) error

	// GetTrialResult 获取缓存的试算结果。
	// hit=false 表示缓存未命中。
	GetTrialResult(ctx context.Context, userID, source, channel, goodsID string, target any) (hit bool, err error)
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

func (r *redisCacheRepo) IncrTakeCount(ctx context.Context, activityID int64, userID string, ttl time.Duration) (int64, error) {
	count, err := redisx.IncrTakeCount(ctx, r.rdb, activityID, userID, ttl)
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

func (r *redisCacheRepo) TakeLimitCheckAndIncr(ctx context.Context, activityID int64, userID string, dbCount int64, limit int, ttl time.Duration) (int64, bool, error) {
	result, err := redisx.TakeLimitCheckAndIncr(ctx, r.rdb, activityID, userID, dbCount, limit, ttl)
	if err != nil {
		return 0, false, fmt.Errorf("cache take limit check and incr: %w", err)
	}
	if !result.Allowed {
		return 0, false, nil
	}
	return result.NewCount, true, nil
}

// --- 活跃订单计数（锁定+已支付）---

func (r *redisCacheRepo) GetActiveCount(ctx context.Context, activityID int64, userID string) (int64, error) {
	count, err := redisx.GetActiveCount(ctx, r.rdb, activityID, userID)
	if err != nil {
		return 0, fmt.Errorf("cache get active count: %w", err)
	}
	return count, nil
}

func (r *redisCacheRepo) IncrActiveCount(ctx context.Context, activityID int64, userID string, ttl time.Duration) (int64, error) {
	count, err := redisx.IncrActiveCount(ctx, r.rdb, activityID, userID, ttl)
	if err != nil {
		return 0, fmt.Errorf("cache incr active count: %w", err)
	}
	return count, nil
}

func (r *redisCacheRepo) DecrActiveCount(ctx context.Context, activityID int64, userID string) error {
	err := redisx.DecrActiveCount(ctx, r.rdb, activityID, userID)
	if err != nil {
		return fmt.Errorf("cache decr active count: %w", err)
	}
	return nil
}

func (r *redisCacheRepo) InitActiveCount(ctx context.Context, activityID int64, userID string, count int64, ttl time.Duration) error {
	err := redisx.InitActiveCount(ctx, r.rdb, activityID, userID, count, ttl)
	if err != nil {
		return fmt.Errorf("cache init active count: %w", err)
	}
	return nil
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

func (r *redisCacheRepo) AcquireLock(ctx context.Context, key string, ttl time.Duration) (*redisx.Lock, bool, error) {
	lock, acquired, err := redisx.AcquireLock(ctx, r.rdb, key, ttl)
	if err != nil {
		return nil, false, fmt.Errorf("cache acquire lock: %w", err)
	}
	return lock, acquired, nil
}

func (r *redisCacheRepo) AcquireLockWithExtend(ctx context.Context, key string, ttl time.Duration) (*redisx.Lock, bool, error) {
	lock, acquired, err := redisx.AcquireLockWithExtend(ctx, r.rdb, key, ttl)
	if err != nil {
		return nil, false, fmt.Errorf("cache acquire lock with extend: %w", err)
	}
	return lock, acquired, nil
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

// --- 试算结果缓存 ---

func (r *redisCacheRepo) CacheTrialResult(ctx context.Context, userID, source, channel, goodsID string, result any, ttl time.Duration) error {
	err := redisx.CacheTrialResult(ctx, r.rdb, userID, source, channel, goodsID, result, ttl)
	if err != nil {
		return fmt.Errorf("cache trial result: %w", err)
	}
	return nil
}

func (r *redisCacheRepo) GetTrialResult(ctx context.Context, userID, source, channel, goodsID string, target any) (bool, error) {
	hit, err := redisx.GetTrialResult(ctx, r.rdb, userID, source, channel, goodsID, target)
	if err != nil {
		return false, fmt.Errorf("get trial result: %w", err)
	}
	return hit, nil
}
