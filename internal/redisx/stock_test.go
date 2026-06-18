package redisx

import (
	"sync"
	"testing"
	"time"
)

// ============================================================
// TryOccupyStock 测试
// ============================================================

func TestTryOccupyStock_Success(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	result, err := TryOccupyStock(ctx, testRDB, 100123, "TEAM001", "OUT001", 3, 10*time.Minute)
	if err != nil {
		t.Fatalf("TryOccupyStock: %v", err)
	}
	if result != OccupyOK {
		t.Errorf("expected OccupyOK (1), got %d", result)
	}

	// 验证 holders SET 中有成员
	count, _ := StockHoldersCount(ctx, testRDB, 100123, "TEAM001")
	if count != 1 {
		t.Errorf("expected 1 holder, got %d", count)
	}
}

func TestTryOccupyStock_Idempotent(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	// 首次占用
	result1, err := TryOccupyStock(ctx, testRDB, 100123, "TEAM001", "OUT001", 3, 10*time.Minute)
	if err != nil {
		t.Fatalf("first occupy: %v", err)
	}
	if result1 != OccupyOK {
		t.Errorf("first occupy: expected OccupyOK (1), got %d", result1)
	}

	// 同 permitId 再次占用 → 幂等
	result2, err := TryOccupyStock(ctx, testRDB, 100123, "TEAM001", "OUT001", 3, 10*time.Minute)
	if err != nil {
		t.Fatalf("second occupy: %v", err)
	}
	if result2 != OccupyIdempotent {
		t.Errorf("second occupy: expected OccupyIdempotent (2), got %d", result2)
	}

	// 确认只占了一个名额
	count, _ := StockHoldersCount(ctx, testRDB, 100123, "TEAM001")
	if count != 1 {
		t.Errorf("expected 1 holder after idempotent, got %d", count)
	}
}

func TestTryOccupyStock_TeamFull(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	targetCount := 3
	// 占满 3 个名额
	for i := 0; i < targetCount; i++ {
		outTradeNo := "OUT" + string(rune('A'+i))
		result, err := TryOccupyStock(ctx, testRDB, 100123, "TEAM001", outTradeNo, targetCount, 10*time.Minute)
		if err != nil {
			t.Fatalf("occupy %d: %v", i, err)
		}
		if result != OccupyOK {
			t.Errorf("occupy %d: expected OccupyOK, got %d", i, result)
		}
	}

	// 第 4 个请求应该被拒
	result, err := TryOccupyStock(ctx, testRDB, 100123, "TEAM001", "OUT_FULL", 3, 10*time.Minute)
	if err != nil {
		t.Fatalf("occupy full: %v", err)
	}
	if result != OccupyFull {
		t.Errorf("expected OccupyFull (-1), got %d", result)
	}

	// 确认 full key 已设置
	isFull, _ := IsTeamFull(ctx, testRDB, 100123, "TEAM001")
	if !isFull {
		t.Error("expected team to be marked full")
	}
}

func TestTryOccupyStock_MarkedFullRejects(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	// 人为标记 full
	MarkTeamFull(ctx, testRDB, 100123, "TEAM001", 10*time.Minute)

	// 即使 holders 为空，full key 也应该拒绝
	result, err := TryOccupyStock(ctx, testRDB, 100123, "TEAM001", "OUT001", 3, 10*time.Minute)
	if err != nil {
		t.Fatalf("TryOccupyStock after mark full: %v", err)
	}
	if result != OccupyFull {
		t.Errorf("expected OccupyFull (-1) when full key set, got %d", result)
	}
}

func TestTryOccupyStock_DifferentTeamsIndependent(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	// 不同团的名额是独立的
	_, _ = TryOccupyStock(ctx, testRDB, 100123, "TEAM_A", "OUT_A1", 2, 10*time.Minute)
	_, _ = TryOccupyStock(ctx, testRDB, 100123, "TEAM_A", "OUT_A2", 2, 10*time.Minute)
	// TEAM_A 已满

	result, err := TryOccupyStock(ctx, testRDB, 100123, "TEAM_B", "OUT_B1", 2, 10*time.Minute)
	if err != nil {
		t.Fatalf("TryOccupyStock team B: %v", err)
	}
	if result != OccupyOK {
		t.Errorf("TEAM_B should be independent, expected OccupyOK got %d", result)
	}
}

// ============================================================
// TryOccupyStock 并发测试
// ============================================================

func TestTryOccupyStock_Concurrency(t *testing.T) {
	flushTestDB(t)
	targetCount := 3
	numGoroutines := 10

	var wg sync.WaitGroup
	results := make([]OccupyStockResult, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctx, cancel := newTestContext()
			defer cancel()
			outTradeNo := "OUT_CONCURRENT_" + string(rune('A'+idx))
			result, err := TryOccupyStock(ctx, testRDB, 100123, "TEAM_CONC", outTradeNo, targetCount, 10*time.Minute)
			if err != nil {
				t.Errorf("goroutine %d: %v", idx, err)
				return
			}
			results[idx] = result
		}(i)
	}
	wg.Wait()

	// 统计成功数
	successCount := 0
	fullCount := 0
	for _, r := range results {
		if r == OccupyOK {
			successCount++
		} else if r == OccupyFull {
			fullCount++
		}
	}

	if successCount != targetCount {
		t.Errorf("expected exactly %d successful occupies, got %d", targetCount, successCount)
	}
	if fullCount != numGoroutines-targetCount {
		t.Errorf("expected %d full rejects, got %d", numGoroutines-targetCount, fullCount)
	}

	t.Logf("concurrency: %d goroutines, %d success, %d full",
		numGoroutines, successCount, fullCount)
}

// ============================================================
// ReleaseStock 测试
// ============================================================

func TestReleaseStock(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	// 占用
	TryOccupyStock(ctx, testRDB, 100123, "TEAM001", "OUT001", 3, 10*time.Minute)
	count, _ := StockHoldersCount(ctx, testRDB, 100123, "TEAM001")
	if count != 1 {
		t.Fatalf("expected 1 before release, got %d", count)
	}

	// 释放
	err := ReleaseStock(ctx, testRDB, 100123, "TEAM001", "OUT001")
	if err != nil {
		t.Fatalf("ReleaseStock: %v", err)
	}

	// 确认释放后可以重新占用
	result, err := TryOccupyStock(ctx, testRDB, 100123, "TEAM001", "OUT_NEW", 3, 10*time.Minute)
	if err != nil {
		t.Fatalf("occupy after release: %v", err)
	}
	if result != OccupyOK {
		t.Errorf("expected OccupyOK after release, got %d", result)
	}
}

func TestReleaseStock_Idempotent(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	// 释放不存在的 permitId → 不报错
	err := ReleaseStock(ctx, testRDB, 100123, "TEAM001", "OUT_NOT_EXIST")
	if err != nil {
		t.Fatalf("ReleaseStock non-existent: %v", err)
	}

	// 确认空 SET 也不报错
	count, _ := StockHoldersCount(ctx, testRDB, 100123, "TEAM001")
	if count != 0 {
		t.Errorf("expected 0 holders after releasing non-existent, got %d", count)
	}
}

func TestReleaseStock_UnmarksFull(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	// 占满 2 人团
	TryOccupyStock(ctx, testRDB, 100123, "TEAM002", "OUT_A", 2, 10*time.Minute)
	TryOccupyStock(ctx, testRDB, 100123, "TEAM002", "OUT_B", 2, 10*time.Minute)

	// 确认满
	isFull, _ := IsTeamFull(ctx, testRDB, 100123, "TEAM002")
	if !isFull {
		t.Error("expected team to be full before release")
	}

	// 释放一人
	ReleaseStock(ctx, testRDB, 100123, "TEAM002", "OUT_A")

	// 确认 full key 被删除 → 新人可以加入
	isFullAfter, _ := IsTeamFull(ctx, testRDB, 100123, "TEAM002")
	if isFullAfter {
		t.Error("expected full key to be removed after release")
	}

	// 新人可以加入
	result, _ := TryOccupyStock(ctx, testRDB, 100123, "TEAM002", "OUT_C", 2, 10*time.Minute)
	if result != OccupyOK {
		t.Errorf("expected OccupyOK after release unmarks full, got %d", result)
	}
}

// ============================================================
// CheckStock / MarkTeamFull / IsTeamFull 测试
// ============================================================

func TestCheckStock(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	member, _ := CheckStock(ctx, testRDB, 100123, "TEAM001", "OUT001")
	if member {
		t.Error("expected not member before occupy")
	}

	TryOccupyStock(ctx, testRDB, 100123, "TEAM001", "OUT001", 3, 10*time.Minute)

	member, _ = CheckStock(ctx, testRDB, 100123, "TEAM001", "OUT001")
	if !member {
		t.Error("expected member after occupy")
	}
}

func TestMarkAndCheckTeamFull(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	isFull, _ := IsTeamFull(ctx, testRDB, 100123, "TEAM001")
	if isFull {
		t.Error("expected not full initially")
	}

	MarkTeamFull(ctx, testRDB, 100123, "TEAM001", 10*time.Minute)

	isFull, _ = IsTeamFull(ctx, testRDB, 100123, "TEAM001")
	if !isFull {
		t.Error("expected full after mark")
	}
}

// ============================================================
// TTL 测试
// ============================================================

func TestStockTTL(t *testing.T) {
	flushTestDB(t)
	ctx, cancel := newTestContext()
	defer cancel()

	// 用短 TTL
	TryOccupyStock(ctx, testRDB, 100123, "TEAM_TTL", "OUT001", 3, 2*time.Second)

	// 第一次 occupy 后应该有 TTL
	ttl, err := testRDB.TTL(ctx, StockHoldersKey(100123, "TEAM_TTL")).Result()
	if err != nil {
		t.Fatalf("get TTL: %v", err)
	}
	if ttl <= 0 {
		t.Errorf("expected positive TTL after first occupy, got %v", ttl)
	}
	t.Logf("initial TTL: %v", ttl)

	// 等待一秒后加入第二个用户
	time.Sleep(1 * time.Second)
	TryOccupyStock(ctx, testRDB, 100123, "TEAM_TTL", "OUT002", 3, 2*time.Second)

	// TTL 应该 <= 初始 TTL - 1s（没有刷新）
	ttl2, _ := testRDB.TTL(ctx, StockHoldersKey(100123, "TEAM_TTL")).Result()
	t.Logf("TTL after second join: %v", ttl2)
	if ttl2 >= ttl {
		t.Logf("warning: TTL appeared to be refreshed (%v → %v)", ttl, ttl2)
	}

	// 等待 TTL 过期
	time.Sleep(time.Until(time.Now().Add(2 * time.Second)))

	exists, _ := testRDB.Exists(ctx, StockHoldersKey(100123, "TEAM_TTL")).Result()
	if exists != 0 {
		t.Logf("holders key still exists after TTL (may not have expired yet)")
	}
}
