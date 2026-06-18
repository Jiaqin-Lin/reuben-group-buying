package redisx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// releaseLockScript 安全释放锁的 Lua 脚本。
// 只有锁的持有者（持有相同 token）才能释放，防止误删他人的锁。
const releaseLockScript = `
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
else
  return 0
end
`

var releaseLockLua = goredis.NewScript(releaseLockScript)

// AcquireLock 获取分布式锁。
//
// 实现: SET key token NX EX ttl
//   - NX: 仅当 key 不存在时才设置（互斥）
//   - EX: 设置过期时间（防死锁）
//
// 返回:
//   - token: 随机令牌，释放锁时必须提供，防止误删他人锁
//   - acquired: true 表示获取成功
//
// 设计说明:
//
//	不用 Redisson watch-dog 自动续期。锁粒度是 (userId, outTradeNo)，
//	仅防同一外部单号并发。即使锁提前过期，outTradeNo 唯一索引 +
//	幂等缓存提供兜底保护。固定 TTL 更简单，没有续期失败的风险。
func AcquireLock(ctx context.Context, rdb *goredis.Client, key string, ttl time.Duration) (token string, acquired bool, err error) {
	token, err = generateLockToken()
	if err != nil {
		return "", false, fmt.Errorf("acquire lock: %w", err)
	}

	ttlSec := ttl.Seconds()
	if ttlSec < 1 {
		ttlSec = 1
	}

	ok, err := rdb.SetNX(ctx, key, token, time.Duration(ttlSec)*time.Second).Result()
	if err != nil {
		return "", false, fmt.Errorf("acquire lock: %w", err)
	}

	return token, ok, nil
}

// ReleaseLock 释放分布式锁（安全释放）。
//
// 用 Lua 脚本原子检查 token 再删除:
//
//	if redis.call('GET', KEYS[1]) == ARGV[1] then
//	  return redis.call('DEL', KEYS[1])
//	end
//
// 只有锁的持有者才能释放，防止误删。
func ReleaseLock(ctx context.Context, rdb *goredis.Client, key, token string) error {
	_, err := releaseLockLua.Run(ctx, rdb, []string{key}, token).Result()
	if err != nil {
		return fmt.Errorf("release lock: %w", err)
	}
	return nil
}

// AcquireLockSimple 获取分布式锁（简化版，不返回 token）。
//
// 适用于不需要"持有者校验"的场景（如定时任务互斥）。
// ReleaseLockSimple 调用者自行保证只在持有锁时释放。
func AcquireLockSimple(ctx context.Context, rdb *goredis.Client, key string, ttl time.Duration) (bool, error) {
	_, acquired, err := AcquireLock(ctx, rdb, key, ttl)
	return acquired, err
}

// ReleaseLockSimple 释放分布式锁（简化版，直接 DEL）。
//
// 注意：不校验 token，存在误删他人锁的风险。
// 仅在"释放时机远早于 TTL"的场景（如请求结束即释放）安全使用。
func ReleaseLockSimple(ctx context.Context, rdb *goredis.Client, key string) error {
	err := rdb.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("release lock simple: %w", err)
	}
	return nil
}

// generateLockToken 生成随机令牌（16 字节 hex = 32 字符）。
func generateLockToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
