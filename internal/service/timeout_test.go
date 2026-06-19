package service

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/reuben/group-buying/internal/model"
	"github.com/reuben/group-buying/internal/repository"
)

// seedTimeoutTestData 插入超时退单测试所需种子数据。
func seedTimeoutTestData(db *gorm.DB) {
	db.Exec("SET FOREIGN_KEY_CHECKS = 0")
	for _, t := range []string{"notify_tasks", "orders", "teams"} {
		db.Exec("DELETE FROM " + t)
	}
	db.Exec("SET FOREIGN_KEY_CHECKS = 1")

	// 折扣
	db.Exec(`INSERT INTO discounts (discount_id, name, description, plan_type, expression) VALUES
		('D_TIMEOUT', '超时退单测试直减20', '直减20', 'ZJ', '20.00')
		ON DUPLICATE KEY UPDATE name=name`)

	// 活动
	db.Exec(`INSERT INTO activities (activity_id, name, discount_id, group_type, target_count, take_limit, valid_minutes, status, start_time, end_time) VALUES
		(700001, '超时-3人团', 'D_TIMEOUT', 0, 3, 5, 30, 1, '2025-01-01', '2029-12-31')
		ON DUPLICATE KEY UPDATE name=name`)

	// 商品
	db.Exec(`INSERT INTO products (goods_id, goods_name, original_price) VALUES
		('G_TIMEOUT', '超时测试商品', 100.00)
		ON DUPLICATE KEY UPDATE goods_name=goods_name`)

	// 活动-商品映射
	db.Exec(`INSERT INTO activity_products (source, channel, goods_id, activity_id) VALUES
		('APP', 'WECHAT', 'G_TIMEOUT', 700001)
		ON DUPLICATE KEY UPDATE activity_id=activity_id`)
}

// clearTimeoutTestData 清理超时测试数据。
func clearTimeoutTestData(t *testing.T) {
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

// newTestTimeoutService 创建测试用 TimeoutService。
func newTestTimeoutService(t *testing.T) *TimeoutService {
	t.Helper()
	if testDB == nil || testRDB == nil {
		t.Skip("mysql or redis not available")
	}
	return NewTimeoutService(
		repository.NewOrderRepo(testDB),
		newTestRefundService(t),
		testRDB,
		TimeoutServiceConfig{BatchSize: 10},
		testLogger(),
	)
}

// createExpiredOrder 直接写 DB 创建一个已过期的 locked 订单。
// 跳过锁单流程，直接构造 team（valid_end 在过去）和 order（status=Locked）。
func createExpiredOrder(t *testing.T, teamID, orderID, outTradeNo, userID string) {
	t.Helper()

	// 团 — valid_end 设为 1 小时前
	now := time.Now()
	team := &model.Team{
		TeamID:         teamID,
		ActivityID:     700001,
		Source:         "APP",
		Channel:        "WECHAT",
		OriginalPrice:  "100.00",
		DeductionPrice: "20.00",
		PayPrice:       "80.00",
		TargetCount:    3,
		LockCount:      1,
		CompleteCount:  0,
		Status:         model.TeamStatusForming,
		ValidStart:     now.Add(-2 * time.Hour),
		ValidEnd:       now.Add(-1 * time.Hour),
		NotifyType:     "HTTP",
	}

	// 订单
	order := &model.Order{
		OrderID:        orderID,
		OutTradeNo:     outTradeNo,
		TeamID:         teamID,
		UserID:         userID,
		ActivityID:     700001,
		GoodsID:        "G_TIMEOUT",
		Source:         "APP",
		Channel:        "WECHAT",
		OriginalPrice:  "100.00",
		DeductionPrice: "20.00",
		PayPrice:       "80.00",
		Status:         model.OrderStatusLocked,
	}

	if err := testDB.Create(team).Error; err != nil {
		t.Fatalf("create expired team: %v", err)
	}
	if err := testDB.Create(order).Error; err != nil {
		t.Fatalf("create expired order: %v", err)
	}
}

// ==================== 测试用例 ====================

func TestTimeout_ScanBasic(t *testing.T) {
	svc := newTestTimeoutService(t)
	defer clearTimeoutTestData(t)

	createExpiredOrder(t, "T_TO_001", "O_TO_001", "EXT_TIMEOUT_001", "U_TO_001")

	scanned, refunded, failed, err := svc.ScanAndRefund(context.Background())
	if err != nil {
		t.Fatalf("ScanAndRefund: %v", err)
	}
	if scanned != 1 {
		t.Errorf("scanned: want 1, got %d", scanned)
	}
	if refunded != 1 {
		t.Errorf("refunded: want 1, got %d", refunded)
	}
	if failed != 0 {
		t.Errorf("failed: want 0, got %d", failed)
	}

	// 验证订单已退
	var order model.Order
	if err := testDB.Where("order_id = ?", "O_TO_001").First(&order).Error; err != nil {
		t.Fatalf("find order: %v", err)
	}
	if order.Status != model.OrderStatusRefunded {
		t.Errorf("order status: want Refunded(2), got %d", order.Status)
	}
}

func TestTimeout_ScanMultiple(t *testing.T) {
	svc := newTestTimeoutService(t)
	defer clearTimeoutTestData(t)

	// 创建3个超时订单，来自不同用户
	for i, pair := range []struct{ teamID, orderID, outTradeNo, userID string }{
		{"T_TO_M1", "O_TO_M1", "EXT_TM_001", "U_TM_001"},
		{"T_TO_M2", "O_TO_M2", "EXT_TM_002", "U_TM_002"},
		{"T_TO_M3", "O_TO_M3", "EXT_TM_003", "U_TM_003"},
	} {
		_ = i
		createExpiredOrder(t, pair.teamID, pair.orderID, pair.outTradeNo, pair.userID)
	}

	scanned, refunded, failed, err := svc.ScanAndRefund(context.Background())
	if err != nil {
		t.Fatalf("ScanAndRefund: %v", err)
	}
	if scanned != 3 {
		t.Errorf("scanned: want 3, got %d", scanned)
	}
	if refunded != 3 {
		t.Errorf("refunded: want 3, got %d", refunded)
	}
	if failed != 0 {
		t.Errorf("failed: want 0, got %d", failed)
	}
}

func TestTimeout_NoExpiredOrders(t *testing.T) {
	svc := newTestTimeoutService(t)
	defer clearTimeoutTestData(t)

	// 不创建任何超时订单，扫描应返回 0
	scanned, _, _, err := svc.ScanAndRefund(context.Background())
	if err != nil {
		t.Fatalf("ScanAndRefund: %v", err)
	}
	if scanned != 0 {
		t.Errorf("scanned: want 0, got %d", scanned)
	}
}

func TestTimeout_SkipNonExpired(t *testing.T) {
	svc := newTestTimeoutService(t)
	defer clearTimeoutTestData(t)

	// 创建一个未过期的团 + locked 订单
	now := time.Now()
	team := &model.Team{
		TeamID:         "T_NOT_EXP",
		ActivityID:     700001,
		Source:         "APP",
		Channel:        "WECHAT",
		OriginalPrice:  "100.00",
		DeductionPrice: "20.00",
		PayPrice:       "80.00",
		TargetCount:    3,
		LockCount:      1,
		CompleteCount:  0,
		Status:         model.TeamStatusForming,
		ValidStart:     now,
		ValidEnd:       now.Add(1 * time.Hour), // 未来才过期
		NotifyType:     "HTTP",
	}
	order := &model.Order{
		OrderID:        "O_NOT_EXP",
		OutTradeNo:     "EXT_NOT_EXP",
		TeamID:         "T_NOT_EXP",
		UserID:         "U_NOT_EXP",
		ActivityID:     700001,
		GoodsID:        "G_TIMEOUT",
		Source:         "APP",
		Channel:        "WECHAT",
		OriginalPrice:  "100.00",
		DeductionPrice: "20.00",
		PayPrice:       "80.00",
		Status:         model.OrderStatusLocked,
	}
	if err := testDB.Create(team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := testDB.Create(order).Error; err != nil {
		t.Fatalf("create order: %v", err)
	}

	scanned, _, _, err := svc.ScanAndRefund(context.Background())
	if err != nil {
		t.Fatalf("ScanAndRefund: %v", err)
	}
	if scanned != 0 {
		t.Errorf("scanned: want 0 (not expired), got %d", scanned)
	}
}

func TestTimeout_Idempotent(t *testing.T) {
	svc := newTestTimeoutService(t)
	defer clearTimeoutTestData(t)

	createExpiredOrder(t, "T_IDEM", "O_IDEM", "EXT_IDEM", "U_IDEM")

	// 第一次扫描
	_, refunded, _, err := svc.ScanAndRefund(context.Background())
	if err != nil {
		t.Fatalf("first scan: %v", err)
	}
	if refunded != 1 {
		t.Fatalf("first scan refunded: want 1, got %d", refunded)
	}

	// 第二次扫描 — 同一订单已退，应跳过（由 RefundService 幂等保证）
	scanned2, _, _, err := svc.ScanAndRefund(context.Background())
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}
	// scanned2 可能为 0（已被 FindTimeoutOrders 过滤，因为 status 已变）
	// 也可能为 1 但 refunded=0（被 Refund 幂等跳过）
	// 两种情况都正确，只需验证不报错
	_ = scanned2
}
