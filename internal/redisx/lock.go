package redisx

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/bsm/redislock"
	"github.com/reuben/group-buying/internal/metrics"
)

// Lock wraps a redislock.Lock with optional watch-dog auto-extend.
//
// Acquired via AcquireLock (fixed TTL) or AcquireLockWithExtend (auto-extend).
// Release must be called to safely release the lock (Lua script checks token ownership).
//
// Uses bsm/redislock under the hood for:
//   - Token-based safe release (Lua script: GET + DEL if match)
//   - Optional watch-dog auto-extend (Redisson-style)
//   - No dependency on SETNX directly
type Lock struct {
	inner  *redislock.Lock
	key    string
	cancel context.CancelFunc // stops auto-extend goroutine
}

// Key returns the Redis key of this lock.
func (l *Lock) Key() string { return l.key }

// Release safely releases the lock.
//
// If auto-extend is active, the watch-dog goroutine is stopped first.
// The actual release uses a Lua script that checks the token before deleting,
// preventing accidental release of locks held by other processes.
func (l *Lock) Release(ctx context.Context) error {
	if l.cancel != nil {
		l.cancel()
	}
	if err := l.inner.Release(ctx); err != nil {
		return fmt.Errorf("release lock %s: %w", l.key, err)
	}
	return nil
}

// AcquireLock acquires a distributed lock with fixed TTL and no retry.
//
// Returns (lock, true, nil) on success.
// Returns (nil, false, nil) if the lock is held by another process.
// Returns (nil, false, error) on Redis connection errors.
//
// Use for short-lived business locks where TTL comfortably exceeds critical section duration.
func AcquireLock(ctx context.Context, rdb *goredis.Client, key string, ttl time.Duration) (*Lock, bool, error) {
	client := redislock.New(rdb)
	inner, err := client.Obtain(ctx, key, ttl, &redislock.Options{
		RetryStrategy: redislock.NoRetry(),
	})
	if err == redislock.ErrNotObtained {
		return nil, false, nil
	}
	if err != nil {
		metrics.IncrRedis("lock", "err")
		return nil, false, fmt.Errorf("acquire lock %s: %w", key, err)
	}
	metrics.IncrRedis("lock", "ok")
	return &Lock{inner: inner, key: key}, true, nil
}

// AcquireLockWithExtend acquires a distributed lock with watch-dog auto-extend.
//
// A background goroutine refreshes the lock TTL every ttl/3 until Release is called.
// This is the Redisson-style pattern: suitable for long-running critical sections
// (e.g., cron scanner locks) where the duration is unpredictable.
//
// The watch-dog stops automatically when:
//   - Release is called (normal path)
//   - The refresh fails (lock expired or Redis unreachable)
func AcquireLockWithExtend(ctx context.Context, rdb *goredis.Client, key string, ttl time.Duration) (*Lock, bool, error) {
	client := redislock.New(rdb)
	inner, err := client.Obtain(ctx, key, ttl, &redislock.Options{
		RetryStrategy: redislock.NoRetry(),
	})
	if err == redislock.ErrNotObtained {
		return nil, false, nil
	}
	if err != nil {
		metrics.IncrRedis("lock", "err")
		return nil, false, fmt.Errorf("acquire lock with extend %s: %w", key, err)
	}
	metrics.IncrRedis("lock", "ok")

	// Watch-dog: periodically refresh the lock TTL
	extCtx, cancel := context.WithCancel(context.Background())
	l := &Lock{inner: inner, key: key, cancel: cancel}

	interval := ttl / 3
	if interval < time.Second {
		interval = time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-extCtx.Done():
				return
			case <-ticker.C:
				if err := inner.Refresh(extCtx, ttl, nil); err != nil {
					// Refresh failed: lock may have expired or Redis is down.
					// Stop the watch-dog; the critical section should detect
					// this via context cancellation or explicit checks.
					return
				}
			}
		}
	}()

	return l, true, nil
}
