package redisx

import (
	"sync"
	"testing"
	"time"
)

func TestAcquireLock_Success(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	token, acquired, err := AcquireLock(ctx, testRDB, "test:lock:1", 5*time.Second)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	if !acquired {
		t.Error("expected lock acquired")
	}
	if token == "" {
		t.Error("expected non-empty token")
	}
}

func TestAcquireLock_AlreadyHeld(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	// 第一次获取成功
	_, acquired1, _ := AcquireLock(ctx, testRDB, "test:lock:2", 5*time.Second)
	if !acquired1 {
		t.Fatal("first acquire should succeed")
	}

	// 第二次获取失败（锁被持有）
	_, acquired2, err := AcquireLock(ctx, testRDB, "test:lock:2", 5*time.Second)
	if err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	if acquired2 {
		t.Error("second acquire should fail (lock held)")
	}
}

func TestReleaseLock_WrongToken(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	token, acquired, _ := AcquireLock(ctx, testRDB, "test:lock:3", 5*time.Second)
	if !acquired {
		t.Fatal("acquire should succeed")
	}

	// 用错误 token 释放 → 不应该删除 key
	err := ReleaseLock(ctx, testRDB, "test:lock:3", "wrong-token")
	if err != nil {
		t.Fatalf("ReleaseLock wrong token: %v", err)
	}

	// 锁应该还在
	_, acquired2, _ := AcquireLock(ctx, testRDB, "test:lock:3", 5*time.Second)
	if acquired2 {
		t.Error("lock should still be held after wrong-token release")
	}

	// 用正确 token 释放
	ReleaseLock(ctx, testRDB, "test:lock:3", token)

	// 现在可以重新获取
	_, acquired3, _ := AcquireLock(ctx, testRDB, "test:lock:3", 5*time.Second)
	if !acquired3 {
		t.Error("lock should be acquirable after correct-token release")
	}
}

func TestReleaseLock_AfterExpiry(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	token, acquired, _ := AcquireLock(ctx, testRDB, "test:lock:4", 1*time.Second)
	if !acquired {
		t.Fatal("acquire should succeed")
	}

	// 等待锁过期
	time.Sleep(1200 * time.Millisecond)

	// 释放已过期的锁 → 不报错（key 不存在，Lua 返回 0）
	err := ReleaseLock(ctx, testRDB, "test:lock:4", token)
	if err != nil {
		t.Fatalf("ReleaseLock after expiry: %v", err)
	}

	// 新请求可以获取锁
	_, acquired2, _ := AcquireLock(ctx, testRDB, "test:lock:4", 5*time.Second)
	if !acquired2 {
		t.Error("lock should be acquirable after expiry")
	}
}

func TestAcquireLock_Concurrency(t *testing.T) {
	flushTestDB(t)
	numGoroutines := 10

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := newTestContext()
			defer cancel()
			_, acquired, err := AcquireLock(ctx, testRDB, "test:lock:concurrent", 5*time.Second)
			if err != nil {
				t.Errorf("goroutine: %v", err)
				return
			}
			if acquired {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if successCount != 1 {
		t.Errorf("expected exactly 1 goroutine to acquire lock, got %d", successCount)
	}
}

func TestAcquireLockSimple(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	acquired, err := AcquireLockSimple(ctx, testRDB, "test:lock:simple", 5*time.Second)
	if err != nil {
		t.Fatalf("AcquireLockSimple: %v", err)
	}
	if !acquired {
		t.Error("expected lock acquired")
	}

	// ReleaseLockSimple 直接删除
	ReleaseLockSimple(ctx, testRDB, "test:lock:simple")

	// 确认已释放
	acquired2, _ := AcquireLockSimple(ctx, testRDB, "test:lock:simple", 5*time.Second)
	if !acquired2 {
		t.Error("expected lock re-acquirable after simple release")
	}
}
