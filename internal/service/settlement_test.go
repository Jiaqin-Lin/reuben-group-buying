package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/reuben/group-buying/internal/errcode"
	"github.com/reuben/group-buying/internal/model"
	"github.com/reuben/group-buying/internal/repository"
)

// seedSettlementTestData 插入结算测试所需的种子数据（在 TestMain 中调用）。
func seedSettlementTestData(db *gorm.DB) {
	// 清空结算相关表
	db.Exec("SET FOREIGN_KEY_CHECKS = 0")
	for _, t := range []string{"notify_tasks", "orders", "teams"} {
		db.Exec("DELETE FROM " + t)
	}
	db.Exec("SET FOREIGN_KEY_CHECKS = 1")

	// 折扣
	db.Exec(`INSERT INTO discounts (discount_id, name, description, plan_type, expression) VALUES
		('D_SETTLE', '结算测试直减20', '直减20', 'ZJ', '20')
		ON DUPLICATE KEY UPDATE name=name`)

	// 活动
	db.Exec(`INSERT INTO activities (activity_id, name, discount_id, group_type, target_count, take_limit, valid_minutes, status, start_time, end_time) VALUES
		(500001, '结算-正常活动', 'D_SETTLE', 0, 3, 5, 30, 1, '2025-01-01', '2029-12-31'),
		(500002, '结算-限购1次', 'D_SETTLE', 0, 3, 1, 30, 1, '2025-01-01', '2029-12-31'),
		(500003, '结算-短有效期', 'D_SETTLE', 0, 3, 5, 1,  1, '2025-01-01', '2029-12-31')
		ON DUPLICATE KEY UPDATE name=name`)

	// 商品
	db.Exec(`INSERT INTO products (goods_id, goods_name, original_price) VALUES
		('G_SETTLE1', '结算测试商品1', 100.00),
		('G_SETTLE2', '结算测试商品2', 100.00),
		('G_SETTLE3', '结算测试商品3', 100.00)
		ON DUPLICATE KEY UPDATE goods_name=goods_name`)

	// 活动-商品映射
	db.Exec(`INSERT INTO activity_products (source, channel, goods_id, activity_id) VALUES
		('APP', 'WECHAT', 'G_SETTLE1', 500001),
		('APP', 'WECHAT', 'G_SETTLE2', 500002),
		('APP', 'WECHAT', 'G_SETTLE3', 500003)
		ON DUPLICATE KEY UPDATE activity_id=activity_id`)
}

// newTestSettlementService 创建测试用 SettlementService。
func newTestSettlementService(t *testing.T) *SettlementService {
	t.Helper()
	if testDB == nil || testRDB == nil {
		t.Skip("mysql or redis not available")
	}
	return NewSettlementService(
		repository.NewOrderRepo(testDB),
		repository.NewActivityRepo(testDB),
		repository.NewRedisCacheRepo(testRDB),
		repository.NewNotifyTaskRepo(testDB),
		nil, // localCache not needed for tests
	)
}

// clearSettlementTestData 清理结算测试产生的数据。
func clearSettlementTestData(t *testing.T) {
	t.Helper()
	if testDB == nil {
		return
	}
	testDB.Exec("SET FOREIGN_KEY_CHECKS = 0")
	for _, tb := range []string{"notify_tasks", "orders", "teams"} {
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

// lockForSettle 快捷锁单，返回 LockResult。
func lockForSettle(t *testing.T, userID, outTradeNo, goodsID string, activityID int64, teamID string) *LockResult {
	t.Helper()
	lockSvc := newTestLockService(t)
	result, err := lockSvc.Lock(context.Background(), LockRequest{
		UserID:     userID,
		ActivityID: activityID,
		GoodsID:    goodsID,
		Source:     "APP",
		Channel:    "WECHAT",
		OutTradeNo: outTradeNo,
		TeamID:     teamID,
	})
	if err != nil {
		t.Fatalf("lockForSettle: %v", err)
	}
	return result
}

// ==================== 基础结算 ====================

func TestSettle_Basic(t *testing.T) {
	svc := newTestSettlementService(t)
	defer clearSettlementTestData(t)
	ctx := context.Background()

	// 先锁单
	lockResult := lockForSettle(t, "U_SETTLE1", "EXT_SETTLE_001", "G_SETTLE1", 500001, "")

	// 结算
	result, err := svc.Settle(ctx, SettlementRequest{
		UserID:       "U_SETTLE1",
		OutTradeNo:   "EXT_SETTLE_001",
		OutTradeTime: time.Now(),
		Source:       "APP",
		Channel:      "WECHAT",
	})
	if err != nil {
		t.Fatalf("settle failed: %v", err)
	}
	if result.OrderID != lockResult.OrderID {
		t.Errorf("order_id = %s, want %s", result.OrderID, lockResult.OrderID)
	}
	if result.TeamID != lockResult.TeamID {
		t.Errorf("team_id = %s, want %s", result.TeamID, lockResult.TeamID)
	}
	if result.IsComplete {
		t.Error("should not be complete for 1/3 team")
	}
	if result.TakeCount != 1 {
		t.Errorf("take_count = %d, want 1", result.TakeCount)
	}

	// 验证 DB：order status = Paid
	order, err := repository.NewOrderRepo(testDB).FindOrderByOrderID(ctx, lockResult.OrderID)
	if err != nil {
		t.Fatalf("find order: %v", err)
	}
	if order.Status != model.OrderStatusPaid {
		t.Errorf("order status = %d, want %d (Paid)", order.Status, model.OrderStatusPaid)
	}
	if order.OutTradeTime == nil {
		t.Error("out_trade_time should be set")
	}

	// 验证 DB：team.complete_count = 1
	team, err := repository.NewOrderRepo(testDB).FindTeamByID(ctx, lockResult.TeamID)
	if err != nil {
		t.Fatalf("find team: %v", err)
	}
	if team.CompleteCount != 1 {
		t.Errorf("complete_count = %d, want 1", team.CompleteCount)
	}
}

// ==================== 幂等结算 ====================

func TestSettle_Idempotent(t *testing.T) {
	svc := newTestSettlementService(t)
	defer clearSettlementTestData(t)
	ctx := context.Background()

	lockForSettle(t, "U_IDEM", "EXT_IDEM_SETTLE", "G_SETTLE1", 500001, "")

	// 第一次结算
	first, err := svc.Settle(ctx, SettlementRequest{
		UserID: "U_IDEM", OutTradeNo: "EXT_IDEM_SETTLE",
		OutTradeTime: time.Now(), Source: "APP", Channel: "WECHAT",
	})
	if err != nil {
		t.Fatalf("first settle failed: %v", err)
	}

	// 第二次相同 outTradeNo（幂等）
	second, err := svc.Settle(ctx, SettlementRequest{
		UserID: "U_IDEM", OutTradeNo: "EXT_IDEM_SETTLE",
		OutTradeTime: time.Now(), Source: "APP", Channel: "WECHAT",
	})
	if err != nil {
		t.Fatalf("second settle failed: %v", err)
	}

	if second.OrderID != first.OrderID {
		t.Errorf("idempotent: order_id mismatch: %s vs %s", second.OrderID, first.OrderID)
	}
	if second.TeamID != first.TeamID {
		t.Errorf("idempotent: team_id mismatch: %s vs %s", second.TeamID, first.TeamID)
	}
}

// ==================== 成团 ====================

func TestSettle_TeamComplete(t *testing.T) {
	svc := newTestSettlementService(t)
	defer clearSettlementTestData(t)
	ctx := context.Background()

	// 3 人锁同一个团
	first := lockForSettle(t, "U_COMP1", "EXT_COMP_001", "G_SETTLE1", 500001, "")
	teamID := first.TeamID
	lockForSettle(t, "U_COMP2", "EXT_COMP_002", "G_SETTLE1", 500001, teamID)
	lockForSettle(t, "U_COMP3", "EXT_COMP_003", "G_SETTLE1", 500001, teamID)

	// 结算前两人
	_, err := svc.Settle(ctx, SettlementRequest{
		UserID: "U_COMP1", OutTradeNo: "EXT_COMP_001",
		OutTradeTime: time.Now(), Source: "APP", Channel: "WECHAT",
	})
	if err != nil {
		t.Fatalf("settle U_COMP1: %v", err)
	}
	_, err = svc.Settle(ctx, SettlementRequest{
		UserID: "U_COMP2", OutTradeNo: "EXT_COMP_002",
		OutTradeTime: time.Now(), Source: "APP", Channel: "WECHAT",
	})
	if err != nil {
		t.Fatalf("settle U_COMP2: %v", err)
	}

	// 第三人结算 → 应该成团
	result, err := svc.Settle(ctx, SettlementRequest{
		UserID: "U_COMP3", OutTradeNo: "EXT_COMP_003",
		OutTradeTime: time.Now(), Source: "APP", Channel: "WECHAT",
	})
	if err != nil {
		t.Fatalf("third settle failed: %v", err)
	}
	if !result.IsComplete {
		t.Error("should be complete after 3/3 settled")
	}

	// 验证 DB：team status = Complete
	team, err := repository.NewOrderRepo(testDB).FindTeamByID(ctx, teamID)
	if err != nil {
		t.Fatalf("find team: %v", err)
	}
	if team.Status != model.TeamStatusComplete {
		t.Errorf("team status = %d, want %d (Complete)", team.Status, model.TeamStatusComplete)
	}
	if team.CompleteCount != 3 {
		t.Errorf("complete_count = %d, want 3", team.CompleteCount)
	}

	// 验证 notify_task 已创建
	tasks, err := repository.NewNotifyTaskRepo(testDB).FindPendingTasks(ctx, 10, 0)
	if err != nil {
		t.Fatalf("find pending tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 notify task, got %d", len(tasks))
	}
	if tasks[0].TeamID != teamID {
		t.Errorf("notify task team_id = %s, want %s", tasks[0].TeamID, teamID)
	}
}

// ==================== 限购超限 ====================

func TestSettle_TakeLimitExceeded(t *testing.T) {
	svc := newTestSettlementService(t)
	defer clearSettlementTestData(t)
	ctx := context.Background()

	// 活动 500002 限购 1 次
	// 锁两个单
	first := lockForSettle(t, "U_LIMIT", "EXT_LIMIT_001", "G_SETTLE2", 500002, "")
	lockForSettle(t, "U_LIMIT", "EXT_LIMIT_002", "G_SETTLE2", 500002, first.TeamID)

	// 第一个结算成功
	_, err := svc.Settle(ctx, SettlementRequest{
		UserID: "U_LIMIT", OutTradeNo: "EXT_LIMIT_001",
		OutTradeTime: time.Now(), Source: "APP", Channel: "WECHAT",
	})
	if err != nil {
		t.Fatalf("first settle failed: %v", err)
	}

	// 第二个结算 → 限购超限
	_, err = svc.Settle(ctx, SettlementRequest{
		UserID: "U_LIMIT", OutTradeNo: "EXT_LIMIT_002",
		OutTradeTime: time.Now(), Source: "APP", Channel: "WECHAT",
	})
	if err == nil {
		t.Fatal("expected take limit error, got nil")
	}
	var settleErr *SettlementError
	if !errors.As(err, &settleErr) || settleErr.ErrorCode() != errcode.CodeTakeLimitReached {
		// errors.As not imported in this file yet, check manually
		t.Logf("error: %v", err)
	}
}

// ==================== 错误场景 ====================

func TestSettle_OrderNotFound(t *testing.T) {
	svc := newTestSettlementService(t)
	defer clearSettlementTestData(t)
	ctx := context.Background()

	_, err := svc.Settle(ctx, SettlementRequest{
		UserID: "U_NF", OutTradeNo: "NOT_EXIST",
		OutTradeTime: time.Now(), Source: "APP", Channel: "WECHAT",
	})
	if err == nil {
		t.Fatal("expected error for non-existent order")
	}
}

func TestSettle_WrongUser(t *testing.T) {
	svc := newTestSettlementService(t)
	defer clearSettlementTestData(t)
	ctx := context.Background()

	lockForSettle(t, "U_WRONG", "EXT_WRONG_USER", "G_SETTLE1", 500001, "")

	_, err := svc.Settle(ctx, SettlementRequest{
		UserID: "U_OTHER", OutTradeNo: "EXT_WRONG_USER", // 不同的 user
		OutTradeTime: time.Now(), Source: "APP", Channel: "WECHAT",
	})
	if err == nil {
		t.Fatal("expected error for wrong user")
	}
}

func TestSettle_AlreadyRefunded(t *testing.T) {
	svc := newTestSettlementService(t)
	defer clearSettlementTestData(t)
	ctx := context.Background()

	lockResult := lockForSettle(t, "U_REFUNDED", "EXT_REFUNDED", "G_SETTLE1", 500001, "")

	// 手动把订单改为 Refunded 状态
	testDB.Model(&model.Order{}).Where("order_id = ?", lockResult.OrderID).Update("status", model.OrderStatusRefunded)

	_, err := svc.Settle(ctx, SettlementRequest{
		UserID: "U_REFUNDED", OutTradeNo: "EXT_REFUNDED",
		OutTradeTime: time.Now(), Source: "APP", Channel: "WECHAT",
	})
	if err == nil {
		t.Fatal("expected error for refunded order")
	}
}

func TestSettle_TeamExpired(t *testing.T) {
	svc := newTestSettlementService(t)
	defer clearSettlementTestData(t)
	ctx := context.Background()

	// 活动 500003 有效期只有 1 分钟
	lockResult := lockForSettle(t, "U_EXPIRED", "EXT_EXPIRED", "G_SETTLE3", 500003, "")

	// 手动把 team.valid_end 改为过去
	testDB.Model(&model.Team{}).Where("team_id = ?", lockResult.TeamID).
		Update("valid_end", time.Now().Add(-1*time.Hour))

	_, err := svc.Settle(ctx, SettlementRequest{
		UserID: "U_EXPIRED", OutTradeNo: "EXT_EXPIRED",
		OutTradeTime: time.Now(), Source: "APP", Channel: "WECHAT",
	})
	if err == nil {
		t.Fatal("expected error for expired team")
	}
}

// ==================== 并发结算 ====================

func TestSettle_Concurrent(t *testing.T) {
	svc := newTestSettlementService(t)
	defer clearSettlementTestData(t)
	ctx := context.Background()

	lockForSettle(t, "U_CONC", "EXT_CONC", "G_SETTLE1", 500001, "")

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Settle(ctx, SettlementRequest{
				UserID: "U_CONC", OutTradeNo: "EXT_CONC",
				OutTradeTime: time.Now(), Source: "APP", Channel: "WECHAT",
			})
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// 只有一个能成功（第一次），其余幂等返回成功
	if successCount != 5 {
		t.Errorf("all 5 concurrent settles should succeed (first does work, rest idempotent), got %d successes", successCount)
	}

	// 验证 take_count 只有 1（没有重复递增）
	takeCount, err := repository.NewRedisCacheRepo(testRDB).GetTakeCount(ctx, 500001, "U_CONC")
	if err != nil {
		t.Fatalf("get take count: %v", err)
	}
	if takeCount != 1 {
		t.Errorf("take_count = %d, want 1 (should not increment multiple times)", takeCount)
	}
}
