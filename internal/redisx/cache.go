package redisx

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

//go:embed lua/take_limit_incr.lua
var takeLimitScript string

var takeLimitLua = goredis.NewScript(takeLimitScript)

// TakeLimitResult 限购检查结果。
type TakeLimitResult struct {
	NewCount int64 // 递增后的新计数（仅 Allowed=true 时有效）
	Allowed  bool  // 是否允许参与
}

// TakeLimitCheckAndIncr 原子检查用户限购次数 + 递增。
//
// 内部用 Lua 脚本原子完成：
//  1. Key 不存在 → 用 dbCount 初始化（冷启动）
//  2. 获取当前计数
//  3. 检查是否 >= limit
//  4. 未达上限则 INCR
//
// 参数:
//   - dbCount: DB 中已有的支付次数（仅 key 不存在时用于初始化）
//   - limit: 参与上限（activities.take_limit）
//   - ttl: key 过期时间（对齐活动 endTime）
//
// 软限制说明:
//
//	两个不同 outTradeNo 的请求可能同时看到 count=4, limit=5，
//	都 INCR 到 6。这是营销约束不是库存约束，可接受。
func TakeLimitCheckAndIncr(ctx context.Context, rdb *goredis.Client, activityID int64, userID string, dbCount int64, limit int, ttl time.Duration) (*TakeLimitResult, error) {
	key := TakeLimitKey(activityID, userID)
	ttlSec := max(int64(ttl.Seconds()), 1)

	result, err := takeLimitLua.Run(ctx, rdb, []string{key}, dbCount, limit, ttlSec).Int64()
	if err != nil {
		return nil, fmt.Errorf("take limit check and incr: %w", err)
	}

	if result == 0 {
		return &TakeLimitResult{Allowed: false}, nil
	}
	return &TakeLimitResult{NewCount: result, Allowed: true}, nil
}

// IncrTakeCount 递增用户参与计数（不检查上限）。
//
// 用于结算时：已通过 take_limit 检查后，直接 INCR。
// 返回递增后的值。
func IncrTakeCount(ctx context.Context, rdb *goredis.Client, activityID int64, userID string, ttl time.Duration) (int64, error) {
	key := TakeLimitKey(activityID, userID)
	count, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("incr take count: %w", err)
	}
	// INCR 在 key 不存在时新建（TTL=-1），需手动设置过期时间
	if ttl > 0 {
		ttlSec := max(int64(ttl.Seconds()), 1)
		if expErr := rdb.Expire(ctx, key, time.Duration(ttlSec)*time.Second).Err(); expErr != nil {
			// Expire 失败仅打日志，不影响主流程（key 无 TTL 最坏是内存泄漏，DB 会兜底 take_limit）
			_ = expErr
		}
	}
	return count, nil
}

// GetTakeCount 获取用户当前参与计数。
func GetTakeCount(ctx context.Context, rdb *goredis.Client, activityID int64, userID string) (int64, error) {
	key := TakeLimitKey(activityID, userID)
	count, err := rdb.Get(ctx, key).Int64()
	if err == goredis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get take count: %w", err)
	}
	return count, nil
}

// InitTakeCount 初始化用户参与计数（从 DB 加载后回种）。
//
// SETNX 语义：仅在 key 不存在时设置。并发安全。
func InitTakeCount(ctx context.Context, rdb *goredis.Client, activityID int64, userID string, count int64, ttl time.Duration) error {
	key := TakeLimitKey(activityID, userID)
	ttlSec := max(ttl.Seconds(), 1)
	err := rdb.SetNX(ctx, key, count, time.Duration(ttlSec)*time.Second).Err()
	if err != nil {
		return fmt.Errorf("init take count: %w", err)
	}
	return nil
}

// --- 通用缓存操作 ---

// CacheSet 设置缓存（JSON 序列化）。
func CacheSet(ctx context.Context, rdb *goredis.Client, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache set marshal: %w", err)
	}
	ttlSec := max(ttl.Seconds(), 1)
	err = rdb.Set(ctx, key, data, time.Duration(ttlSec)*time.Second).Err()
	if err != nil {
		return fmt.Errorf("cache set: %w", err)
	}
	return nil
}

// CacheGet 获取缓存（JSON 反序列化）。
// 返回 hit=false 表示缓存未命中（key 不存在或已过期）。
func CacheGet(ctx context.Context, rdb *goredis.Client, key string, target any) (hit bool, err error) {
	data, err := rdb.Get(ctx, key).Bytes()
	if err == goredis.Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("cache get: %w", err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return false, fmt.Errorf("cache get unmarshal: %w", err)
	}
	return true, nil
}

// CacheGetRaw 获取缓存的原始字节。
// 用于锁单结果缓存——service 层自行处理 JSON。
func CacheGetRaw(ctx context.Context, rdb *goredis.Client, key string) ([]byte, bool, error) {
	data, err := rdb.Get(ctx, key).Bytes()
	if err == goredis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("cache get raw: %w", err)
	}
	return data, true, nil
}

// CacheDel 删除缓存。
func CacheDel(ctx context.Context, rdb *goredis.Client, key string) error {
	err := rdb.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("cache del: %w", err)
	}
	return nil
}

// --- 试算结果缓存 ---

// CacheTrialResult 缓存试算结果（短 TTL，防重试穿透）。
func CacheTrialResult(ctx context.Context, rdb *goredis.Client, userID, source, channel, goodsID string, result any, ttl time.Duration) error {
	key := TrialResultKey(userID, source, channel, goodsID)
	return CacheSet(ctx, rdb, key, result, ttl)
}

// GetTrialResult 获取缓存的试算结果。
// 返回 hit=false 表示缓存未命中。
func GetTrialResult(ctx context.Context, rdb *goredis.Client, userID, source, channel, goodsID string, target any) (hit bool, err error) {
	key := TrialResultKey(userID, source, channel, goodsID)
	return CacheGet(ctx, rdb, key, target)
}

// --- 人群标签操作 ---

// AddCrowdMembers 添加人群标签成员（批量 SADD）。
func AddCrowdMembers(ctx context.Context, rdb *goredis.Client, tagID string, userIDs []string) error {
	if len(userIDs) == 0 {
		return nil
	}
	key := CrowdMembersKey(tagID)
	members := make([]any, len(userIDs))
	for i, uid := range userIDs {
		members[i] = uid
	}
	err := rdb.SAdd(ctx, key, members...).Err()
	if err != nil {
		return fmt.Errorf("add crowd members: %w", err)
	}
	return nil
}

// CheckCrowdMember 检查用户是否在人群标签中。
func CheckCrowdMember(ctx context.Context, rdb *goredis.Client, tagID, userID string) (bool, error) {
	key := CrowdMembersKey(tagID)
	result, err := rdb.SIsMember(ctx, key, userID).Result()
	if err != nil {
		return false, fmt.Errorf("check crowd member: %w", err)
	}
	return result, nil
}
