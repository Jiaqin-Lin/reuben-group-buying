package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/reuben/group-buying/internal/model"
	"github.com/reuben/group-buying/internal/redisx"
	"github.com/reuben/group-buying/internal/repository"
)

// seedLockTestData 插入锁单测试所需的种子数据（在 TestMain 中调用）。
func seedLockTestData(db *gorm.DB) {
	// 清空锁单相关表
	db.Exec("SET FOREIGN_KEY_CHECKS = 0")
	for _, t := range []string{"orders", "teams", "payments", "payment_logs"} {
		db.Exec("DELETE FROM " + t)
	}
	db.Exec("SET FOREIGN_KEY_CHECKS = 1")

	// 活动（锁单专用）
	db.Exec(`INSERT INTO activities (activity_id, name, discount_id, group_type, target_count, take_limit, valid_minutes, status, start_time, end_time, tag_id, tag_scope) VALUES
		(300001, '锁单-正常活动',    'D_ZJ', 0, 3, 5, 30, 1, '2025-01-01', '2029-12-31', NULL, NULL),
		(300002, '锁单-限购1次',    'D_ZJ', 0, 3, 1, 30, 1, '2025-01-01', '2029-12-31', NULL, NULL),
		(300003, '锁单-2人团',      'D_ZJ', 0, 2, 5, 30, 1, '2025-01-01', '2029-12-31', NULL, NULL)
	`)

	// 商品
	db.Exec(`INSERT INTO products (goods_id, goods_name, original_price) VALUES
		('G_LOCK1', '锁单测试商品1', 100.00),
		('G_LOCK2', '锁单测试商品2', 100.00),
		('G_LOCK3', '锁单测试商品3', 100.00)
	`)

	// 活动-商品映射（每个 goods_id 只映射一个 activity，避免歧义）
	db.Exec(`INSERT INTO activity_products (source, channel, goods_id, activity_id) VALUES
		('APP', 'WECHAT', 'G_LOCK1', 300001),
		('APP', 'WECHAT', 'G_LOCK2', 300002),
		('APP', 'WECHAT', 'G_LOCK3', 300003)
	`)
}

// newTestLockService 创建测试用 LockService。
func newTestLockService(t *testing.T) *LockService {
	t.Helper()
	if testDB == nil || testRDB == nil {
		t.Skip("mysql or redis not available")
	}
	return NewLockService(
		newTestTrialService(t),
		repository.NewOrderRepo(testDB),
		repository.NewActivityRepo(testDB),
		repository.NewRedisCacheRepo(testRDB),
		3*time.Second,
		10*time.Minute,
	)
}

// clearLockTestData 清理锁单测试产生的数据。
func clearLockTestData(t *testing.T) {
	t.Helper()
	if testDB == nil {
		return
	}
	testDB.Exec("SET FOREIGN_KEY_CHECKS = 0")
	for _, tb := range []string{"orders", "teams"} {
		testDB.Exec("DELETE FROM " + tb)
	}
	testDB.Exec("SET FOREIGN_KEY_CHECKS = 1")
	if testRDB != nil {
		ctx := context.Background()
		keys, _ := testRDB.Keys(ctx, "bgm:*").Result()
		for _, k := range keys {
			testRDB.Del(ctx, k)
		}
	}
}

// ==================== 新建团 ====================

func TestLock_NewTeam(t *testing.T) {
	svc := newTestLockService(t)
	defer clearLockTestData(t)
	ctx := context.Background()

	result, err := svc.Lock(ctx, LockRequest{
		UserID: "U1", ActivityID: 300001, GoodsID: "G_LOCK1",
		Source: "APP", Channel: "WECHAT", OutTradeNo: "EXT_NEW_001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OrderID == "" {
		t.Error("order_id should not be empty")
	}
	if result.TeamID == "" {
		t.Error("team_id should not be empty")
	}
	if result.OutTradeNo != "EXT_NEW_001" {
		t.Errorf("out_trade_no = %s, want EXT_NEW_001", result.OutTradeNo)
	}
	if result.OriginalPrice != "100.00" {
		t.Errorf("original_price = %s, want 100.00", result.OriginalPrice)
	}
	if result.PayPrice != "80.00" {
		t.Errorf("pay_price = %s, want 80.00 (100-20)", result.PayPrice)
	}
	if result.Status != model.OrderStatusLocked {
		t.Errorf("status = %d, want %d", result.Status, model.OrderStatusLocked)
	}

	// 验证 DB 中订单存在
	order, err := repository.NewOrderRepo(testDB).FindOrderByOrderID(ctx, result.OrderID)
	if err != nil {
		t.Fatalf("find order by id: %v", err)
	}
	if order.Status != model.OrderStatusLocked {
		t.Errorf("db status = %d, want %d", order.Status, model.OrderStatusLocked)
	}

	// 验证 team 存在且 lock_count=1
	team, err := repository.NewOrderRepo(testDB).FindTeamByID(ctx, result.TeamID)
	if err != nil {
		t.Fatalf("find team: %v", err)
	}
	if team.LockCount != 1 {
		t.Errorf("lock_count = %d, want 1", team.LockCount)
	}
	if team.Status != model.TeamStatusForming {
		t.Errorf("team status = %d, want %d", team.Status, model.TeamStatusForming)
	}
}

// ==================== 加入团 ====================

func TestLock_JoinTeam(t *testing.T) {
	svc := newTestLockService(t)
	defer clearLockTestData(t)
	ctx := context.Background()

	// 先创建团
	first, err := svc.Lock(ctx, LockRequest{
		UserID: "U1", ActivityID: 300001, GoodsID: "G_LOCK1",
		Source: "APP", Channel: "WECHAT", OutTradeNo: "EXT_JOIN_001",
	})
	if err != nil {
		t.Fatalf("first lock failed: %v", err)
	}
	teamID := first.TeamID

	// 第二人加入
	second, err := svc.Lock(ctx, LockRequest{
		UserID: "U2", ActivityID: 300001, GoodsID: "G_LOCK1",
		Source: "APP", Channel: "WECHAT", OutTradeNo: "EXT_JOIN_002",
		TeamID: teamID,
	})
	if err != nil {
		t.Fatalf("join team failed: %v", err)
	}
	if second.TeamID != teamID {
		t.Errorf("team_id = %s, want %s", second.TeamID, teamID)
	}
	if second.OrderID == first.OrderID {
		t.Error("order_id should be different")
	}

	// 验证 team.lock_count = 2
	team, err := repository.NewOrderRepo(testDB).FindTeamByID(ctx, teamID)
	if err != nil {
		t.Fatalf("find team: %v", err)
	}
	if team.LockCount != 2 {
		t.Errorf("lock_count = %d, want 2", team.LockCount)
	}
}

// ==================== 幂等 ====================

func TestLock_Idempotent_Cache(t *testing.T) {
	svc := newTestLockService(t)
	defer clearLockTestData(t)
	ctx := context.Background()

	req := LockRequest{
		UserID: "U1", ActivityID: 300001, GoodsID: "G_LOCK1",
		Source: "APP", Channel: "WECHAT", OutTradeNo: "EXT_IDEM_001",
	}

	// 第一次请求
	first, err := svc.Lock(ctx, req)
	if err != nil {
		t.Fatalf("first lock failed: %v", err)
	}

	// 第二次相同 outTradeNo（应从缓存返回）
	second, err := svc.Lock(ctx, req)
	if err != nil {
		t.Fatalf("second lock failed: %v", err)
	}

	if second.OrderID != first.OrderID {
		t.Errorf("idempotent: order_id mismatch: %s vs %s", second.OrderID, first.OrderID)
	}
	if second.TeamID != first.TeamID {
		t.Errorf("idempotent: team_id mismatch: %s vs %s", second.TeamID, first.TeamID)
	}

	// 验证 DB 中只有一条订单
	orders, _ := repository.NewOrderRepo(testDB).FindOrdersByUserAndActivity(ctx, "U1", 300001)
	if len(orders) != 1 {
		t.Errorf("expected 1 order in DB, got %d", len(orders))
	}
}

func TestLock_Idempotent_DB(t *testing.T) {
	svc := newTestLockService(t)
	defer clearLockTestData(t)
	ctx := context.Background()

	req := LockRequest{
		UserID: "U1", ActivityID: 300001, GoodsID: "G_LOCK1",
		Source: "APP", Channel: "WECHAT", OutTradeNo: "EXT_IDEM_DB",
	}

	// 第一次请求
	first, err := svc.Lock(ctx, req)
	if err != nil {
		t.Fatalf("first lock failed: %v", err)
	}

	// 清除缓存（模拟缓存过期）
	testRDB.Del(ctx, redisx.LockResultKey("U1", "EXT_IDEM_DB"))

	// 第二次相同 outTradeNo（应从 DB 返回）
	second, err := svc.Lock(ctx, req)
	if err != nil {
		t.Fatalf("second lock failed: %v", err)
	}

	if second.OrderID != first.OrderID {
		t.Errorf("idempotent: order_id mismatch: %s vs %s", second.OrderID, first.OrderID)
	}
}

// ==================== 活动校验（通过 TrialService） ====================

func TestLock_ActivityInvalid(t *testing.T) {
	svc := newTestLockService(t)
	defer clearLockTestData(t)
	ctx := context.Background()

	// 使用过期活动 200005，但 G_EXP 映射到 200005
	// 我们需要一个映射到过期活动的商品... 已存在: G_EXP -> 200005
	_, err := svc.Lock(ctx, LockRequest{
		UserID: "U1", ActivityID: 200005, GoodsID: "G_EXP",
		Source: "APP", Channel: "WECHAT", OutTradeNo: "EXT_EXP_001",
	})
	if err == nil {
		t.Fatal("expected error for expired activity")
	}
}

// ==================== 限购检查 ====================

func TestLock_TakeLimitReached(t *testing.T) {
	svc := newTestLockService(t)
	defer clearLockTestData(t)
	ctx := context.Background()

	// 活动 300002 的 take_limit=1
	// 插入一条已支付订单
	testDB.Create(&model.Order{
		UserID:         "U1",
		TeamID:         "T_DONE",
		OrderID:        "999999999999",
		ActivityID:     300002,
		GoodsID:        "G_LOCK2",
		Source:         "APP",
		Channel:        "WECHAT",
		OriginalPrice:  "100.00",
		DeductionPrice: "20.00",
		PayPrice:       "80.00",
		Status:         model.OrderStatusPaid,
		OutTradeNo:     "EXT_LIMIT_DONE",
	})

	_, err := svc.Lock(ctx, LockRequest{
		UserID: "U1", ActivityID: 300002, GoodsID: "G_LOCK2",
		Source: "APP", Channel: "WECHAT", OutTradeNo: "EXT_LIMIT_001",
	})
	if err == nil {
		t.Fatal("expected error for take limit reached")
	}

	var lockErr *LockError
	if !errors.As(err, &lockErr) || lockErr.Code != "E0103" {
		t.Errorf("expected E0103 take limit error, got %v", err)
	}
}

// ==================== 团满 ====================

func TestLock_TeamFull_RedisStock(t *testing.T) {
	svc := newTestLockService(t)
	defer clearLockTestData(t)
	ctx := context.Background()

	// 活动 300003 的 target_count=2
	// 第一次：用户1新建团
	first, err := svc.Lock(ctx, LockRequest{
		UserID: "U1", ActivityID: 300003, GoodsID: "G_LOCK3",
		Source: "APP", Channel: "WECHAT", OutTradeNo: "EXT_FULL_001",
	})
	if err != nil {
		t.Fatalf("first lock failed: %v", err)
	}

	// 第二次：用户2加入（第2个人=满标）
	_, err = svc.Lock(ctx, LockRequest{
		UserID: "U2", ActivityID: 300003, GoodsID: "G_LOCK3",
		Source: "APP", Channel: "WECHAT", OutTradeNo: "EXT_FULL_002",
		TeamID: first.TeamID,
	})
	if err != nil {
		t.Fatalf("second lock failed: %v", err)
	}

	// 第三次：用户3尝试加入（应满标）
	_, err = svc.Lock(ctx, LockRequest{
		UserID: "U3", ActivityID: 300003, GoodsID: "G_LOCK3",
		Source: "APP", Channel: "WECHAT", OutTradeNo: "EXT_FULL_003",
		TeamID: first.TeamID,
	})
	if err == nil {
		t.Fatal("expected error for team full")
	}
}

// ==================== 团不存在 ====================

func TestLock_TeamNotFound(t *testing.T) {
	svc := newTestLockService(t)
	defer clearLockTestData(t)
	ctx := context.Background()

	_, err := svc.Lock(ctx, LockRequest{
		UserID: "U1", ActivityID: 300001, GoodsID: "G_LOCK1",
		Source: "APP", Channel: "WECHAT", OutTradeNo: "EXT_NO_TEAM",
		TeamID: "99999999", // 不存在的团
	})
	if err == nil {
		t.Fatal("expected error for non-existent team")
	}
}

// ==================== 并发 ====================

func TestLock_Concurrent_SameOutTradeNo(t *testing.T) {
	svc := newTestLockService(t)
	defer clearLockTestData(t)
	ctx := context.Background()

	req := LockRequest{
		UserID: "U1", ActivityID: 300001, GoodsID: "G_LOCK1",
		Source: "APP", Channel: "WECHAT", OutTradeNo: "EXT_CONCURRENT",
	}

	var wg sync.WaitGroup
	results := make(chan *LockResult, 10)
	errs := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := svc.Lock(ctx, req)
			if err != nil {
				errs <- err
			} else {
				results <- result
			}
		}()
	}
	wg.Wait()
	close(results)
	close(errs)

	var successResults []*LockResult
	for r := range results {
		successResults = append(successResults, r)
	}

	t.Logf("success: %d, failures: %d", len(successResults), len(errs))

	if len(successResults) == 0 {
		t.Fatal("all requests failed")
	}

	// 所有成功结果应该返回相同的 orderId
	firstOrderID := successResults[0].OrderID
	for i, r := range successResults {
		if r.OrderID != firstOrderID {
			t.Errorf("result %d: order_id mismatch: %s vs %s", i, r.OrderID, firstOrderID)
		}
	}

	// 验证 DB 中只有一条订单
	orders, _ := repository.NewOrderRepo(testDB).FindOrdersByUserAndActivity(ctx, "U1", 300001)
	orderCount := 0
	for _, o := range orders {
		if o.OutTradeNo == "EXT_CONCURRENT" {
			orderCount++
		}
	}
	if orderCount != 1 {
		t.Errorf("expected 1 order in DB for EXT_CONCURRENT, got %d", orderCount)
	}
}

// ==================== NotifyURL ====================

func TestLock_WithNotifyURL(t *testing.T) {
	svc := newTestLockService(t)
	defer clearLockTestData(t)
	ctx := context.Background()

	result, err := svc.Lock(ctx, LockRequest{
		UserID: "U1", ActivityID: 300001, GoodsID: "G_LOCK1",
		Source: "APP", Channel: "WECHAT", OutTradeNo: "EXT_NOTIFY",
		NotifyURL: "https://example.com/callback",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	team, err := repository.NewOrderRepo(testDB).FindTeamByID(ctx, result.TeamID)
	if err != nil {
		t.Fatalf("find team: %v", err)
	}
	if team.NotifyURL == nil || *team.NotifyURL != "https://example.com/callback" {
		t.Errorf("notify_url = %v, want https://example.com/callback", team.NotifyURL)
	}
}

// ==================== 活动不匹配 ====================

func TestLock_ActivityMismatch(t *testing.T) {
	svc := newTestLockService(t)
	defer clearLockTestData(t)
	ctx := context.Background()

	// G_LOCK1 映射到 activity 300001，请求传 activity 300002 → mismatch
	_, err := svc.Lock(ctx, LockRequest{
		UserID: "U1", ActivityID: 300002, GoodsID: "G_LOCK1",
		Source: "APP", Channel: "WECHAT", OutTradeNo: "EXT_MISMATCH",
	})
	if err == nil {
		t.Fatal("expected error for activity mismatch")
	}
}

// ==================== 加团时团已过期 ====================

func TestLock_JoinExpiredTeam(t *testing.T) {
	svc := newTestLockService(t)
	defer clearLockTestData(t)
	ctx := context.Background()

	// 手动插入一个已过期的团
	expiredTeam := &model.Team{
		TeamID:         "88888888",
		ActivityID:     300001,
		Source:         "APP",
		Channel:        "WECHAT",
		OriginalPrice:  "100.00",
		DeductionPrice: "20.00",
		PayPrice:       "80.00",
		TargetCount:    3,
		LockCount:      1,
		CompleteCount:  0,
		Status:         model.TeamStatusForming,
		ValidStart:     time.Now().Add(-2 * time.Hour),
		ValidEnd:       time.Now().Add(-1 * time.Hour),
		NotifyType:     model.NotifyTypeHTTP,
	}
	if err := testDB.Create(expiredTeam).Error; err != nil {
		t.Fatalf("create expired team: %v", err)
	}

	_, err := svc.Lock(ctx, LockRequest{
		UserID: "U1", ActivityID: 300001, GoodsID: "G_LOCK1",
		Source: "APP", Channel: "WECHAT", OutTradeNo: "EXT_EXP_TEAM",
		TeamID: "88888888",
	})
	if err == nil {
		t.Fatal("expected error for expired team")
	}
}
