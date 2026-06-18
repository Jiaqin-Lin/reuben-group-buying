package redisx

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

//go:embed lua/occupy_stock.lua
var occupyStockScript string

//go:embed lua/release_stock.lua
var releaseStockScript string

var (
	occupyScript  = goredis.NewScript(occupyStockScript)
	releaseScript = goredis.NewScript(releaseStockScript)
)

// OccupyStockResult 名额占用结果枚举。
type OccupyStockResult int

const (
	// OccupyOK 占用成功（首次加入，团未满）。
	OccupyOK OccupyStockResult = 1
	// OccupyIdempotent 已占用（幂等，permitId 已在 SET 中）。
	OccupyIdempotent OccupyStockResult = 2
	// OccupyFull 团已满（无法加入）。
	OccupyFull OccupyStockResult = -1
)

// TryOccupyStock 尝试占用团名额（原子操作）。
//
// 内部用 Lua 脚本原子完成：
//  1. 检查 full 哨兵 key（快速拒绝）
//  2. 检查 permitId 幂等（SISMEMBER）
//  3. 检查容量（SCARD < targetCount）
//  4. 加入 holders SET（SADD）
//  5. 首次加入时设置 TTL
//  6. 满员时标记 full key
//
// 参数:
//   - activityID, teamID: 定位团
//   - outTradeNo: 外部交易单号，作为 SET 成员标识（防重）
//   - targetCount: 团目标人数上限
//   - ttl: 团有效期（从第一个成员加入开始计时）
//
// 返回值:
//   - OccupyOK: 占用成功
//   - OccupyIdempotent: 已占用（幂等）
//   - OccupyFull: 团已满
func TryOccupyStock(ctx context.Context, rdb *goredis.Client, activityID int64, teamID string, outTradeNo string, targetCount int, ttl time.Duration) (OccupyStockResult, error) {
	holdersKey := StockHoldersKey(activityID, teamID)
	fullKey := StockFullKey(activityID, teamID)
	ttlSec := max(int64(ttl.Seconds()), 1) // 最少 1 秒，防止传 0 导致 key 永不过期

	result, err := occupyScript.Run(ctx, rdb, []string{holdersKey, fullKey}, outTradeNo, targetCount, ttlSec).Int()
	if err != nil {
		return 0, fmt.Errorf("occupy stock: %w", err)
	}

	return OccupyStockResult(result), nil
}

// ReleaseStock 释放团名额（原子操作）。
//
// 退单/超时取消时调用。操作是幂等的：
//   - SREM 移除 permitId（不在 SET 中也无妨）
//   - DEL 删除 full key（允许新人重新加入）
//
// 注意：本操作不关心 holders SET 是否为空。空的 holders SET
// 会在 TTL 到期后自动清理。不主动删 key，防止删 key 后
// 和新 occupy 的 TTL 设置产生竞态。
func ReleaseStock(ctx context.Context, rdb *goredis.Client, activityID int64, teamID string, outTradeNo string) error {
	holdersKey := StockHoldersKey(activityID, teamID)
	fullKey := StockFullKey(activityID, teamID)

	_, err := releaseScript.Run(ctx, rdb, []string{holdersKey, fullKey}, outTradeNo).Result()
	if err != nil {
		return fmt.Errorf("release stock: %w", err)
	}
	return nil
}

// CheckStock 检查 permitId 是否已占用名额。
// 直接调用 SISMEMBER，不需要 Lua 脚本。
func CheckStock(ctx context.Context, rdb *goredis.Client, activityID int64, teamID string, outTradeNo string) (bool, error) {
	holdersKey := StockHoldersKey(activityID, teamID)
	result, err := rdb.SIsMember(ctx, holdersKey, outTradeNo).Result()
	if err != nil {
		return false, fmt.Errorf("check stock: %w", err)
	}
	return result, nil
}

// MarkTeamFull 主动标记团已满。
// 用于外部已知团满时（如从 DB 查出 complete_count >= target_count）同步到 Redis。
func MarkTeamFull(ctx context.Context, rdb *goredis.Client, activityID int64, teamID string, ttl time.Duration) error {
	fullKey := StockFullKey(activityID, teamID)
	ttlSec := max(ttl.Seconds(), 1)
	err := rdb.Set(ctx, fullKey, "1", time.Duration(ttlSec)*time.Second).Err()
	if err != nil {
		return fmt.Errorf("mark team full: %w", err)
	}
	return nil
}

// IsTeamFull 检查团是否已满。
func IsTeamFull(ctx context.Context, rdb *goredis.Client, activityID int64, teamID string) (bool, error) {
	fullKey := StockFullKey(activityID, teamID)
	exists, err := rdb.Exists(ctx, fullKey).Result()
	if err != nil {
		return false, fmt.Errorf("check team full: %w", err)
	}
	return exists == 1, nil
}

// StockHoldersCount 返回当前已占用名额数（SCARD）。
// 仅用于监控/调试，不参与业务逻辑判断。
func StockHoldersCount(ctx context.Context, rdb *goredis.Client, activityID int64, teamID string) (int64, error) {
	holdersKey := StockHoldersKey(activityID, teamID)
	count, err := rdb.SCard(ctx, holdersKey).Result()
	if err != nil {
		return 0, fmt.Errorf("stock holders count: %w", err)
	}
	return count, nil
}
