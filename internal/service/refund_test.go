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
	"github.com/reuben/group-buying/internal/pay"
	"github.com/reuben/group-buying/internal/repository"
)

// seedRefundTestData 插入退单测试所需的种子数据（在 TestMain 中调用）。
func seedRefundTestData(db *gorm.DB) {
	// 清空
	db.Exec("SET FOREIGN_KEY_CHECKS = 0")
	for _, t := range []string{"notify_tasks", "orders", "teams"} {
		db.Exec("DELETE FROM " + t)
	}
	db.Exec("SET FOREIGN_KEY_CHECKS = 1")

	// 折扣
	db.Exec(`INSERT INTO discounts (discount_id, name, description, plan_type, expression) VALUES
		('D_REFUND', '退单测试直减20', '直减20', 'ZJ', '20.00')
		ON DUPLICATE KEY UPDATE name=name`)

	// 活动 — 600001: 正常3人团, 600002: 单人团（测试最后一人退单）
	db.Exec(`INSERT INTO activities (activity_id, name, discount_id, group_type, target_count, take_limit, valid_minutes, status, start_time, end_time) VALUES
		(600001, '退单-3人团', 'D_REFUND', 0, 3, 5, 30, 1, '2025-01-01', '2029-12-31'),
		(600002, '退单-1人团', 'D_REFUND', 0, 1, 5, 30, 1, '2025-01-01', '2029-12-31')
		ON DUPLICATE KEY UPDATE name=name`)

	// 商品
	db.Exec(`INSERT INTO products (goods_id, goods_name, original_price) VALUES
		('G_REFUND1', '退单测试商品1', 100.00),
		('G_REFUND2', '退单测试商品2', 100.00)
		ON DUPLICATE KEY UPDATE goods_name=goods_name`)

	// 活动-商品映射
	db.Exec(`INSERT INTO activity_products (source, channel, goods_id, activity_id) VALUES
		('APP', 'WECHAT', 'G_REFUND1', 600001),
		('APP', 'WECHAT', 'G_REFUND2', 600002)
		ON DUPLICATE KEY UPDATE activity_id=activity_id`)
}

// newTestRefundService 创建测试用 RefundService。
func newTestRefundService(t *testing.T) *RefundService {
	t.Helper()
	if testDB == nil || testRDB == nil {
		t.Skip("mysql or redis not available")
	}
	return NewRefundService(
		repository.NewOrderRepo(testDB),
		repository.NewPaymentRepo(testDB),
		repository.NewRedisCacheRepo(testRDB),
		repository.NewNotifyTaskRepo(testDB),
		pay.NewMock(),
	)
}

// clearRefundTestData 清理退单测试产生的数据。
func clearRefundTestData(t *testing.T) {
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

// lockForRefund 快捷锁单，返回 LockResult。
func lockForRefund(t *testing.T, userID, outTradeNo, goodsID string, activityID int64, teamID string) *LockResult {
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
		t.Fatalf("lockForRefund: %v", err)
	}
	return result
}

// lockAndSettle 锁单并结算，返回 LockResult（用于创建已支付订单）。
func lockAndSettle(t *testing.T, userID, outTradeNo, goodsID string, activityID int64, teamID string) *LockResult {
	t.Helper()
	lockResult := lockForRefund(t, userID, outTradeNo, goodsID, activityID, teamID)

	settleSvc := newTestSettlementService(t)
	_, err := settleSvc.Settle(context.Background(), SettlementRequest{
		UserID:       userID,
		OutTradeNo:   outTradeNo,
		OutTradeTime: time.Now(),
		Source:       "APP",
		Channel:      "WECHAT",
	})
	if err != nil {
		t.Fatalf("lockAndSettle: settle failed for %s: %v", outTradeNo, err)
	}
	return lockResult
}

// ==================== 未支付退单 ====================

func TestRefund_Unpaid_Basic(t *testing.T) {
	svc := newTestRefundService(t)
	defer clearRefundTestData(t)
	ctx := context.Background()

	// 锁单（未支付）
	lockResult := lockForRefund(t, "U_UNPAID", "EXT_UNPAID_001", "G_REFUND1", 600001, "")

	// 退单
	result, err := svc.Refund(ctx, RefundRequest{
		UserID: "U_UNPAID", OutTradeNo: "EXT_UNPAID_001",
	})
	if err != nil {
		t.Fatalf("refund failed: %v", err)
	}
	if result.OrderID != lockResult.OrderID {
		t.Errorf("order_id = %s, want %s", result.OrderID, lockResult.OrderID)
	}
	if result.RefundType != "unpaid" {
		t.Errorf("refund_type = %s, want unpaid", result.RefundType)
	}

	// 验证 DB：order status = Refunded
	order, _ := repository.NewOrderRepo(testDB).FindOrderByOrderID(ctx, lockResult.OrderID)
	if order.Status != model.OrderStatusRefunded {
		t.Errorf("order status = %d, want %d (Refunded)", order.Status, model.OrderStatusRefunded)
	}

	// 验证 DB：team lock_count 减了 1
	team, _ := repository.NewOrderRepo(testDB).FindTeamByID(ctx, lockResult.TeamID)
	if team.LockCount != 0 {
		t.Errorf("lock_count = %d, want 0", team.LockCount)
	}

	// 验证 notify_task 已创建
	tasks, _ := repository.NewNotifyTaskRepo(testDB).FindPendingTasks(ctx, 10, 0)
	found := false
	for _, task := range tasks {
		if task.TeamID == lockResult.TeamID && task.Category != nil && *task.Category == model.NotifyCategoryUnpaidRefund {
			found = true
			break
		}
	}
	if !found {
		t.Error("notify_task (unpaid_refund) not found")
	}

	// 验证 Redis 名额已释放（holders 里不再有这个 outTradeNo）
	isMember, _ := repository.NewRedisCacheRepo(testRDB).CheckStock(ctx, 600001, lockResult.TeamID, "EXT_UNPAID_001")
	if isMember {
		t.Error("stock should be released, but outTradeNo still in holders set")
	}
}

// ==================== 已支付未成团退单 ====================

func TestRefund_Paid_Basic(t *testing.T) {
	svc := newTestRefundService(t)
	defer clearRefundTestData(t)
	ctx := context.Background()

	// 锁单 + 结算（变为 Paid）
	lockResult := lockAndSettle(t, "U_PAID", "EXT_PAID_001", "G_REFUND1", 600001, "")

	// 退单
	result, err := svc.Refund(ctx, RefundRequest{
		UserID: "U_PAID", OutTradeNo: "EXT_PAID_001",
	})
	if err != nil {
		t.Fatalf("refund failed: %v", err)
	}
	if result.RefundType != "paid" {
		t.Errorf("refund_type = %s, want paid", result.RefundType)
	}

	// 验证 DB：order status = Refunded
	order, _ := repository.NewOrderRepo(testDB).FindOrderByOrderID(ctx, lockResult.OrderID)
	if order.Status != model.OrderStatusRefunded {
		t.Errorf("order status = %d, want %d (Refunded)", order.Status, model.OrderStatusRefunded)
	}

	// 验证 DB：team lock_count 和 complete_count 都减了 1
	team, _ := repository.NewOrderRepo(testDB).FindTeamByID(ctx, lockResult.TeamID)
	if team.LockCount != 0 {
		t.Errorf("lock_count = %d, want 0", team.LockCount)
	}
	if team.CompleteCount != 0 {
		t.Errorf("complete_count = %d, want 0", team.CompleteCount)
	}

	// 验证 notify_task (paid_refund)
	tasks, _ := repository.NewNotifyTaskRepo(testDB).FindPendingTasks(ctx, 10, 0)
	found := false
	for _, task := range tasks {
		if task.TeamID == lockResult.TeamID && task.Category != nil && *task.Category == model.NotifyCategoryPaidRefund {
			found = true
			break
		}
	}
	if !found {
		t.Error("notify_task (paid_refund) not found")
	}

	// 验证 Redis 名额已释放
	isMember, _ := repository.NewRedisCacheRepo(testRDB).CheckStock(ctx, 600001, lockResult.TeamID, "EXT_PAID_001")
	if isMember {
		t.Error("stock should be released")
	}
}

// ==================== 已成团退单（多人团退一人） ====================

func TestRefund_PaidTeam_Basic(t *testing.T) {
	svc := newTestRefundService(t)
	defer clearRefundTestData(t)
	ctx := context.Background()

	// 3 人锁同一个团
	first := lockAndSettle(t, "U_PT1", "EXT_PT_001", "G_REFUND1", 600001, "")
	teamID := first.TeamID
	lockAndSettle(t, "U_PT2", "EXT_PT_002", "G_REFUND1", 600001, teamID)
	lockAndSettle(t, "U_PT3", "EXT_PT_003", "G_REFUND1", 600001, teamID)

	// 退第一人（团还有 2 人 → CompleteRefunded）
	result, err := svc.Refund(ctx, RefundRequest{
		UserID: "U_PT1", OutTradeNo: "EXT_PT_001",
	})
	if err != nil {
		t.Fatalf("refund failed: %v", err)
	}
	if result.RefundType != "paid_team" {
		t.Errorf("refund_type = %s, want paid_team", result.RefundType)
	}
	if result.TeamStatus != model.TeamStatusCompleteRefunded {
		t.Errorf("team_status = %d, want %d (CompleteRefunded)", result.TeamStatus, model.TeamStatusCompleteRefunded)
	}

	// 验证 DB：order status = Refunded
	order, _ := repository.NewOrderRepo(testDB).FindOrderByOrderID(ctx, first.OrderID)
	if order.Status != model.OrderStatusRefunded {
		t.Errorf("order status = %d, want %d (Refunded)", order.Status, model.OrderStatusRefunded)
	}

	// 验证 DB：team status = CompleteRefunded
	team, _ := repository.NewOrderRepo(testDB).FindTeamByID(ctx, teamID)
	if team.Status != model.TeamStatusCompleteRefunded {
		t.Errorf("team status = %d, want %d (CompleteRefunded)", team.Status, model.TeamStatusCompleteRefunded)
	}
	if team.LockCount != 2 {
		t.Errorf("lock_count = %d, want 2", team.LockCount)
	}
	if team.CompleteCount != 2 {
		t.Errorf("complete_count = %d, want 2", team.CompleteCount)
	}

	// 验证 notify_task (paid_team_refund)
	tasks, _ := repository.NewNotifyTaskRepo(testDB).FindPendingTasks(ctx, 10, 0)
	found := false
	for _, task := range tasks {
		if task.TeamID == teamID && task.Category != nil && *task.Category == model.NotifyCategoryPaidTeamRefund {
			found = true
			break
		}
	}
	if !found {
		t.Error("notify_task (paid_team_refund) not found")
	}

	// 验证 Redis 名额未释放（已成团不释放）
	isMember, _ := repository.NewRedisCacheRepo(testRDB).CheckStock(ctx, 600001, teamID, "EXT_PT_001")
	if !isMember {
		t.Error("stock should NOT be released for completed team refund")
	}
}

// ==================== 已成团退单（最后一人） ====================

func TestRefund_PaidTeam_LastMember(t *testing.T) {
	svc := newTestRefundService(t)
	defer clearRefundTestData(t)
	ctx := context.Background()

	// 1 人团（activity 600002, target_count=1, goods G_REFUND2）
	lockResult := lockAndSettle(t, "U_LAST", "EXT_LAST_001", "G_REFUND2", 600002, "")

	// 退单（唯一成员 → 团 Failed）
	result, err := svc.Refund(ctx, RefundRequest{
		UserID: "U_LAST", OutTradeNo: "EXT_LAST_001",
	})
	if err != nil {
		t.Fatalf("refund failed: %v", err)
	}
	if result.TeamStatus != model.TeamStatusFailed {
		t.Errorf("team_status = %d, want %d (Failed)", result.TeamStatus, model.TeamStatusFailed)
	}

	// 验证 DB：team status = Failed
	team, _ := repository.NewOrderRepo(testDB).FindTeamByID(ctx, lockResult.TeamID)
	if team.Status != model.TeamStatusFailed {
		t.Errorf("team status = %d, want %d (Failed)", team.Status, model.TeamStatusFailed)
	}
	if team.LockCount != 0 {
		t.Errorf("lock_count = %d, want 0", team.LockCount)
	}
	if team.CompleteCount != 0 {
		t.Errorf("complete_count = %d, want 0", team.CompleteCount)
	}
}

// ==================== 幂等退单 ====================

func TestRefund_Idempotent(t *testing.T) {
	svc := newTestRefundService(t)
	defer clearRefundTestData(t)
	ctx := context.Background()

	lockResult := lockForRefund(t, "U_IDEM_REF", "EXT_IDEM_REF", "G_REFUND1", 600001, "")

	// 第一次退单
	first, err := svc.Refund(ctx, RefundRequest{UserID: "U_IDEM_REF", OutTradeNo: "EXT_IDEM_REF"})
	if err != nil {
		t.Fatalf("first refund failed: %v", err)
	}

	// 第二次退单（幂等）
	second, err := svc.Refund(ctx, RefundRequest{UserID: "U_IDEM_REF", OutTradeNo: "EXT_IDEM_REF"})
	if err != nil {
		t.Fatalf("second refund failed: %v", err)
	}
	if second.OrderID != first.OrderID {
		t.Errorf("idempotent: order_id mismatch: %s vs %s", second.OrderID, first.OrderID)
	}

	// 验证 team lock_count 只减了 1（不是 2）
	team, _ := repository.NewOrderRepo(testDB).FindTeamByID(ctx, lockResult.TeamID)
	if team.LockCount != 0 {
		t.Errorf("lock_count = %d, want 0 (should only decrement once)", team.LockCount)
	}
}

// ==================== 并发退单 ====================

func TestRefund_Concurrent(t *testing.T) {
	svc := newTestRefundService(t)
	defer clearRefundTestData(t)
	ctx := context.Background()

	lockResult := lockForRefund(t, "U_CONC_REF", "EXT_CONC_REF", "G_REFUND1", 600001, "")

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Refund(ctx, RefundRequest{UserID: "U_CONC_REF", OutTradeNo: "EXT_CONC_REF"})
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// 所有并发请求都应该成功（第一次做实际工作，其余幂等返回）
	if successCount != 5 {
		t.Errorf("all 5 concurrent refunds should succeed, got %d", successCount)
	}

	// 验证 team lock_count 只减了 1
	team, _ := repository.NewOrderRepo(testDB).FindTeamByID(ctx, lockResult.TeamID)
	if team.LockCount != 0 {
		t.Errorf("lock_count = %d, want 0 (concurrent refunds should only decrement once)", team.LockCount)
	}
}

// ==================== 已成团并发退单 ====================

func TestRefund_PaidTeam_Concurrent(t *testing.T) {
	svc := newTestRefundService(t)
	defer clearRefundTestData(t)
	ctx := context.Background()

	// 2 人团，全部成团（activity 600001, target_count=3，但只锁+结算 2 人）
	// 手动建团并直接写 2 人成团状态来模拟 target_count=2 的团。
	// 实际上用 activity 600001 锁 3 人结算 3 人成团后，再退 2 人会产生两次 status=3 而非 3+2。
	// 这里直接构造 2 人成团的 DB 状态。
	lockResult1 := lockAndSettle(t, "U_PTC1", "EXT_PTC_01", "G_REFUND1", 600001, "")
	teamID := lockResult1.TeamID
	lockAndSettle(t, "U_PTC2", "EXT_PTC_02", "G_REFUND1", 600001, teamID)

	// 手动将团设为 Complete（跳过第三人，模拟 2 人成团）
	testDB.Model(&model.Team{}).Where("team_id = ?", teamID).Updates(map[string]any{
		"status":         model.TeamStatusComplete,
		"complete_count": 2,
		"lock_count":     2,
	})

	// 并发退两人（complete_count: 2→1→0, 预期: 3 和 2）
	var wg sync.WaitGroup
	results := make([]*RefundResult, 2)
	for i, userOut := range []struct {
		user, outTradeNo string
	}{
		{"U_PTC1", "EXT_PTC_01"},
		{"U_PTC2", "EXT_PTC_02"},
	} {
		wg.Add(1)
		go func(idx int, user, outTradeNo string) {
			defer wg.Done()
			r, err := svc.Refund(ctx, RefundRequest{UserID: user, OutTradeNo: outTradeNo})
			if err == nil {
				results[idx] = r
			}
		}(i, userOut.user, userOut.outTradeNo)
	}
	wg.Wait()

	if results[0] == nil || results[1] == nil {
		t.Fatal("both concurrent refunds should succeed")
	}

	// 一个产生 CompleteRefunded (status=3)，另一个产生 Failed (status=2)
	hasCompleteRefunded := false
	hasFailed := false
	for _, r := range results {
		switch r.TeamStatus {
		case model.TeamStatusCompleteRefunded:
			hasCompleteRefunded = true
		case model.TeamStatusFailed:
			hasFailed = true
		}
	}
	if !hasCompleteRefunded || !hasFailed {
		t.Errorf("expected one CompleteRefunded(3) and one Failed(2), got statuses: %d, %d",
			results[0].TeamStatus, results[1].TeamStatus)
	}

	// 验证 DB：team status 是 Failed（最后一步）
	team, _ := repository.NewOrderRepo(testDB).FindTeamByID(ctx, teamID)
	if team.Status != model.TeamStatusFailed {
		t.Errorf("final team status = %d, want %d (Failed)", team.Status, model.TeamStatusFailed)
	}
	if team.LockCount != 0 {
		t.Errorf("lock_count = %d, want 0", team.LockCount)
	}
	if team.CompleteCount != 0 {
		t.Errorf("complete_count = %d, want 0", team.CompleteCount)
	}
}

// ==================== 错误场景 ====================

func TestRefund_OrderNotFound(t *testing.T) {
	svc := newTestRefundService(t)
	defer clearRefundTestData(t)
	ctx := context.Background()

	_, err := svc.Refund(ctx, RefundRequest{
		UserID: "U_NF", OutTradeNo: "NOT_EXIST_REFUND",
	})
	if err == nil {
		t.Fatal("expected error for non-existent order")
	}
	var refundErr *RefundError
	if !errors.As(err, &refundErr) || refundErr.ErrorCode() != errcode.CodeOrderNotFound {
		t.Errorf("expected CodeOrderNotFound, got %v", err)
	}
}

func TestRefund_WrongUser(t *testing.T) {
	svc := newTestRefundService(t)
	defer clearRefundTestData(t)
	ctx := context.Background()

	lockForRefund(t, "U_WRONG_REF", "EXT_WRONG_REF", "G_REFUND1", 600001, "")

	_, err := svc.Refund(ctx, RefundRequest{
		UserID: "U_OTHER_REF", OutTradeNo: "EXT_WRONG_REF",
	})
	if err == nil {
		t.Fatal("expected error for wrong user")
	}
	var refundErr *RefundError
	if !errors.As(err, &refundErr) || refundErr.ErrorCode() != errcode.CodeOrderNotFound {
		t.Errorf("expected CodeOrderNotFound, got %v", err)
	}
}

func TestRefund_InvalidState(t *testing.T) {
	svc := newTestRefundService(t)
	defer clearRefundTestData(t)
	ctx := context.Background()

	// 锁一个单
	lockResult := lockForRefund(t, "U_INVALID", "EXT_INVALID", "G_REFUND1", 600001, "")

	// 手动把订单改为 Refunded 并把 team 改为 Complete（模拟不可能的状态组合）
	testDB.Model(&model.Order{}).Where("order_id = ?", lockResult.OrderID).Update("status", model.OrderStatusLocked)
	testDB.Model(&model.Team{}).Where("team_id = ?", lockResult.TeamID).Update("status", model.TeamStatusComplete)

	// 此时 order=Locked 但 team=Complete → 不允许退单
	_, err := svc.Refund(ctx, RefundRequest{
		UserID: "U_INVALID", OutTradeNo: "EXT_INVALID",
	})
	if err == nil {
		t.Fatal("expected error for invalid state")
	}
	var refundErr *RefundError
	if !errors.As(err, &refundErr) || refundErr.ErrorCode() != errcode.CodeRefundStateInvalid {
		t.Errorf("expected CodeRefundStateInvalid, got %v", err)
	}
}
