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

	lock, acquired, err := AcquireLock(ctx, testRDB, "test:lock:1", 5*time.Second)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	if !acquired {
		t.Error("expected lock acquired")
	}
	if lock == nil {
		t.Error("expected non-nil lock")
	}

	// Safe release
	if err := lock.Release(ctx); err != nil {
		t.Errorf("Release: %v", err)
	}
}

func TestAcquireLock_AlreadyHeld(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	// 第一次获取成功
	lock1, acquired1, _ := AcquireLock(ctx, testRDB, "test:lock:2", 5*time.Second)
	if !acquired1 {
		t.Fatal("first acquire should succeed")
	}
	defer lock1.Release(ctx)

	// 第二次获取失败（锁被持有）
	_, acquired2, err := AcquireLock(ctx, testRDB, "test:lock:2", 5*time.Second)
	if err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	if acquired2 {
		t.Error("second acquire should fail (lock held)")
	}
}

func TestAcquireLock_ReleaseAndReacquire(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	lock1, acquired, _ := AcquireLock(ctx, testRDB, "test:lock:3", 5*time.Second)
	if !acquired {
		t.Fatal("first acquire should succeed")
	}

	// 释放
	if err := lock1.Release(ctx); err != nil {
		t.Fatalf("Release: %v", err)
	}

	// 现在可以重新获取
	lock2, acquired2, _ := AcquireLock(ctx, testRDB, "test:lock:3", 5*time.Second)
	if !acquired2 {
		t.Error("lock should be acquirable after release")
	}
	if lock2 != nil {
		lock2.Release(ctx)
	}
}

func TestAcquireLock_AfterExpiry(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	lock, acquired, _ := AcquireLock(ctx, testRDB, "test:lock:4", 1*time.Second)
	if !acquired {
		t.Fatal("acquire should succeed")
	}
	defer lock.Release(ctx)

	// 等待锁过期
	time.Sleep(1200 * time.Millisecond)

	// 新请求可以获取锁（旧锁已过期）
	lock2, acquired2, _ := AcquireLock(ctx, testRDB, "test:lock:4", 5*time.Second)
	if !acquired2 {
		t.Error("lock should be acquirable after expiry")
	}
	if lock2 != nil {
		lock2.Release(ctx)
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
			lock, acquired, err := AcquireLock(ctx, testRDB, "test:lock:concurrent", 5*time.Second)
			if err != nil {
				t.Errorf("goroutine: %v", err)
				return
			}
			if acquired {
				mu.Lock()
				successCount++
				mu.Unlock()
				lock.Release(ctx)
			}
		}()
	}
	wg.Wait()

	if successCount != 1 {
		t.Errorf("expected exactly 1 goroutine to acquire lock, got %d", successCount)
	}
}

func TestAcquireLockWithExtend(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	// Acquire with auto-extend (short TTL for testing)
	lock, acquired, err := AcquireLockWithExtend(ctx, testRDB, "test:lock:extend", 2*time.Second)
	if err != nil {
		t.Fatalf("AcquireLockWithExtend: %v", err)
	}
	if !acquired {
		t.Fatal("expected lock acquired")
	}

	// Wait longer than the original TTL — watch-dog should have extended it
	time.Sleep(3 * time.Second)

	// Lock should still be held (watch-dog extended it)
	_, acquired2, _ := AcquireLock(ctx, testRDB, "test:lock:extend", 1*time.Second)
	if acquired2 {
		t.Error("lock should still be held after watch-dog extend")
	}

	// Release should stop watch-dog and release
	if err := lock.Release(ctx); err != nil {
		t.Fatalf("Release: %v", err)
	}

	// Now should be acquirable
	lock2, acquired3, _ := AcquireLock(ctx, testRDB, "test:lock:extend", 5*time.Second)
	if !acquired3 {
		t.Error("lock should be acquirable after release")
	}
	if lock2 != nil {
		lock2.Release(ctx)
	}
}
