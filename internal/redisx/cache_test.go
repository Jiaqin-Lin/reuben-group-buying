package redisx

import (
	"sync"
	"testing"
	"time"
)

// ============================================================
// TakeLimitCheckAndIncr 测试
// ============================================================

func TestTakeLimit_ColdStart(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	// 模拟冷启动：DB 中已有 2 次购买
	result, err := TakeLimitCheckAndIncr(ctx, testRDB, 100123, "USER001", 2, 5, 10*time.Minute)
	if err != nil {
		t.Fatalf("TakeLimitCheckAndIncr cold start: %v", err)
	}
	if !result.Allowed {
		t.Error("expected allowed on cold start (2 < 5)")
	}
	if result.NewCount != 3 {
		t.Errorf("expected new count 3, got %d", result.NewCount)
	}

	// 确认 key 的值为 3
	count, _ := GetTakeCount(ctx, testRDB, 100123, "USER001")
	if count != 3 {
		t.Errorf("expected Redis count 3, got %d", count)
	}
}

func TestTakeLimit_Increment(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	// 初始化计数为 0
	InitTakeCount(ctx, testRDB, 100123, "USER001", 0, 10*time.Minute)

	// 连续递增 3 次
	for i := int64(1); i <= 3; i++ {
		result, err := TakeLimitCheckAndIncr(ctx, testRDB, 100123, "USER001", 0, 5, 10*time.Minute)
		if err != nil {
			t.Fatalf("incr %d: %v", i, err)
		}
		if !result.Allowed {
			t.Errorf("incr %d: expected allowed", i)
		}
		if result.NewCount != i {
			t.Errorf("incr %d: expected count %d, got %d", i, i, result.NewCount)
		}
	}

	count, _ := GetTakeCount(ctx, testRDB, 100123, "USER001")
	if count != 3 {
		t.Errorf("expected final count 3, got %d", count)
	}
}

func TestTakeLimit_Exceeded(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	limit := 2

	// 初始化计数为 limit-1
	InitTakeCount(ctx, testRDB, 100123, "USER001", int64(limit-1), 10*time.Minute)

	// 第 1 次请求 → 允许，计数 = limit
	result1, _ := TakeLimitCheckAndIncr(ctx, testRDB, 100123, "USER001", 0, limit, 10*time.Minute)
	if !result1.Allowed {
		t.Error("first request should be allowed (1 < 2)")
	}

	// 第 2 次请求 → 拒绝，已达上限
	result2, _ := TakeLimitCheckAndIncr(ctx, testRDB, 100123, "USER001", 0, limit, 10*time.Minute)
	if result2.Allowed {
		t.Error("second request should be rejected (count == 2 == limit)")
	}
}

func TestTakeLimit_AlreadyAtLimit(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	// 初始化计数 = limit
	InitTakeCount(ctx, testRDB, 100123, "USER001", 3, 10*time.Minute)

	result, _ := TakeLimitCheckAndIncr(ctx, testRDB, 100123, "USER001", 0, 3, 10*time.Minute)
	if result.Allowed {
		t.Error("should be rejected when count == limit")
	}
}

func TestTakeLimit_ColdStartConcurrent(t *testing.T) {
	flushTestDB(t)
	// 模拟两个 goroutine 同时冷启动
	// 都传 dbCount=0，limit=5
	// 结果应该是一致的（不会少算）

	var wg sync.WaitGroup
	results := make([]*TakeLimitResult, 2)

	for i := range 2 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctx, cancel := newTestContext()
			defer cancel()
			result, err := TakeLimitCheckAndIncr(ctx, testRDB, 100123, "USER_CONC", 0, 5, 10*time.Minute)
			if err != nil {
				t.Errorf("goroutine %d: %v", idx, err)
				return
			}
			results[idx] = result
		}(i)
	}
	wg.Wait()

	// 两个都应该允许（0 < 5, 1 < 5）
	if results[0] == nil || results[1] == nil || !results[0].Allowed || !results[1].Allowed {
		t.Error("both concurrent requests should be allowed")
	}

	// 最终计数应该是 2
	ctx, cancel := newTestContext()
	defer cancel()
	count, _ := GetTakeCount(ctx, testRDB, 100123, "USER_CONC")
	if count != 2 {
		t.Errorf("expected final count 2, got %d", count)
	}
}

// ============================================================
// IncrTakeCount / GetTakeCount 测试
// ============================================================

func TestIncrTakeCount(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	count1, err := IncrTakeCount(ctx, testRDB, 100123, "USER001")
	if err != nil {
		t.Fatalf("IncrTakeCount: %v", err)
	}
	if count1 != 1 {
		t.Errorf("expected count 1 on first incr, got %d", count1)
	}

	count2, _ := IncrTakeCount(ctx, testRDB, 100123, "USER001")
	if count2 != 2 {
		t.Errorf("expected count 2 on second incr, got %d", count2)
	}
}

func TestGetTakeCount_Empty(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	count, err := GetTakeCount(ctx, testRDB, 100123, "USER_NOT_EXIST")
	if err != nil {
		t.Fatalf("GetTakeCount empty: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 for non-existent key, got %d", count)
	}
}

func TestInitTakeCount(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	// 初始化
	InitTakeCount(ctx, testRDB, 100123, "USER001", 5, 10*time.Minute)

	// 再次初始化 → 应该不被覆盖（SETNX）
	InitTakeCount(ctx, testRDB, 100123, "USER001", 999, 10*time.Minute)

	count, _ := GetTakeCount(ctx, testRDB, 100123, "USER001")
	if count != 5 {
		t.Errorf("expected count 5 (SETNX should reject overwrite), got %d", count)
	}
}

// ============================================================
// CacheSet / CacheGet 测试
// ============================================================

func TestCacheSetGet(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	type testData struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	// Set
	original := testData{Name: "hello", Count: 42}
	err := CacheSet(ctx, testRDB, "test:cache:1", original, 10*time.Minute)
	if err != nil {
		t.Fatalf("CacheSet: %v", err)
	}

	// Get
	var got testData
	hit, err := CacheGet(ctx, testRDB, "test:cache:1", &got)
	if err != nil {
		t.Fatalf("CacheGet: %v", err)
	}
	if !hit {
		t.Error("expected cache hit")
	}
	if got.Name != "hello" || got.Count != 42 {
		t.Errorf("unmarshaled data mismatch: %+v", got)
	}
}

func TestCacheGet_Miss(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	var v string
	hit, err := CacheGet(ctx, testRDB, "test:cache:not_exist", &v)
	if err != nil {
		t.Fatalf("CacheGet miss: %v", err)
	}
	if hit {
		t.Error("expected cache miss")
	}
}

func TestCacheGetRaw(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	CacheSet(ctx, testRDB, "test:raw:1", "hello world", 10*time.Minute)

	data, hit, err := CacheGetRaw(ctx, testRDB, "test:raw:1")
	if err != nil {
		t.Fatalf("CacheGetRaw: %v", err)
	}
	if !hit {
		t.Error("expected cache hit")
	}
	// JSON 序列化会加引号
	if string(data) != `"hello world"` {
		t.Logf("raw data: %s", string(data))
	}
}

func TestCacheDel(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	CacheSet(ctx, testRDB, "test:del:1", "value", 10*time.Minute)
	CacheDel(ctx, testRDB, "test:del:1")

	var v string
	hit, _ := CacheGet(ctx, testRDB, "test:del:1", &v)
	if hit {
		t.Error("expected cache miss after del")
	}
}

// ============================================================
// 人群标签测试
// ============================================================

func TestCrowdMembers(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	// 添加成员
	err := AddCrowdMembers(ctx, testRDB, "TAG001", []string{"USER1", "USER2", "USER3"})
	if err != nil {
		t.Fatalf("AddCrowdMembers: %v", err)
	}

	// 检查成员
	member, _ := CheckCrowdMember(ctx, testRDB, "TAG001", "USER1")
	if !member {
		t.Error("USER1 should be member")
	}

	nonMember, _ := CheckCrowdMember(ctx, testRDB, "TAG001", "USER999")
	if nonMember {
		t.Error("USER999 should not be member")
	}
}

func TestAddCrowdMembers_Empty(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	// 空列表不报错
	err := AddCrowdMembers(ctx, testRDB, "TAG001", nil)
	if err != nil {
		t.Fatalf("AddCrowdMembers empty: %v", err)
	}

	err = AddCrowdMembers(ctx, testRDB, "TAG001", []string{})
	if err != nil {
		t.Fatalf("AddCrowdMembers empty slice: %v", err)
	}
}
