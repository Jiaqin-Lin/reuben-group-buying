package repository

import (
	"context"
	"testing"
	"time"

	"github.com/reuben/group-buying/internal/model"
)

// --- 辅助函数 ---

func createTestTeam(t *testing.T, teamID string) {
	t.Helper()
	now := time.Now()
	testDB.Exec(`INSERT INTO teams (team_id, activity_id, source, channel, original_price, deduction_price, pay_price, target_count, complete_count, lock_count, status, valid_start, valid_end, notify_type) VALUES
		(?, 100123, 'APP', 'WECHAT', '100.00', '20.00', '80.00', 3, 0, 0, 0, ?, ?, 'HTTP')`,
		teamID, now, now.Add(30*time.Minute),
	)
}

func createTestOrder(t *testing.T, orderID, outTradeNo, teamID, userID string, status int8) {
	t.Helper()
	testDB.Exec(`INSERT INTO orders (user_id, team_id, order_id, activity_id, goods_id, source, channel, original_price, deduction_price, pay_price, status, out_trade_no) VALUES
		(?, ?, ?, 100123, 'GOODS001', 'APP', 'WECHAT', '100.00', '20.00', '80.00', ?, ?)`,
		userID, teamID, orderID, status, outTradeNo,
	)
}

// --- 测试 ---

func TestOrderRepo_FindByOutTradeNo(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewOrderRepo(testDB)
	ctx := context.Background()

	createTestTeam(t, "T001")
	createTestOrder(t, "ORD001", "EXT_FIND_BY_OUT", "T001", "USER001", model.OrderStatusLocked)

	o, err := repo.FindOrderByOutTradeNo(ctx, "EXT_FIND_BY_OUT")
	if err != nil {
		t.Fatalf("FindOrderByOutTradeNo: %v", err)
	}
	if o.OrderID != "ORD001" {
		t.Errorf("expected order_id=ORD001, got %s", o.OrderID)
	}
}

func TestOrderRepo_FindByOutTradeNo_NotFound(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewOrderRepo(testDB)
	ctx := context.Background()

	_, err := repo.FindOrderByOutTradeNo(ctx, "NOT_EXIST")
	if err == nil {
		t.Fatal("expected error for non-existent out_trade_no")
	}
}

func TestOrderRepo_CountPaidOrdersByUserActivity(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewOrderRepo(testDB)
	ctx := context.Background()

	createTestTeam(t, "T002")
	createTestOrder(t, "ORD002", "EXT_COUNT_1", "T002", "USER002", model.OrderStatusPaid)
	createTestOrder(t, "ORD003", "EXT_COUNT_2", "T002", "USER002", model.OrderStatusLocked) // 未支付，不算

	count, err := repo.CountPaidOrdersByUserActivity(ctx, "USER002", 100123)
	if err != nil {
		t.Fatalf("CountPaidOrders: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count=1 (only paid), got %d", count)
	}
}

func TestOrderRepo_CreateTeamWithOrder(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewOrderRepo(testDB)
	ctx := context.Background()

	now := time.Now()
	team := &model.Team{
		TeamID:         "T_NEW_001",
		ActivityID:     100123,
		Source:         "APP",
		Channel:        "WECHAT",
		OriginalPrice:  "100.00",
		DeductionPrice: "20.00",
		PayPrice:       "80.00",
		TargetCount:    3,
		LockCount:      1,
		Status:         model.TeamStatusForming,
		ValidStart:     now,
		ValidEnd:       now.Add(30 * time.Minute),
		NotifyType:     model.NotifyTypeHTTP,
	}
	order := &model.Order{
		UserID:         "USER003",
		TeamID:         "T_NEW_001",
		OrderID:        "ORD_NEW_001",
		ActivityID:     100123,
		GoodsID:        "GOODS001",
		Source:         "APP",
		Channel:        "WECHAT",
		OriginalPrice:  "100.00",
		DeductionPrice: "20.00",
		PayPrice:       "80.00",
		Status:         model.OrderStatusLocked,
		OutTradeNo:     "EXT_NEW_001",
	}

	err := repo.CreateTeamWithOrder(ctx, team, order)
	if err != nil {
		t.Fatalf("CreateTeamWithOrder: %v", err)
	}

	// 验证团和订单都已写入
	gotTeam, err := repo.FindTeamByID(ctx, "T_NEW_001")
	if err != nil {
		t.Fatalf("verify team: %v", err)
	}
	if gotTeam.TeamID != "T_NEW_001" {
		t.Errorf("team not created")
	}

	gotOrder, err := repo.FindOrderByOrderID(ctx, "ORD_NEW_001")
	if err != nil {
		t.Fatalf("verify order: %v", err)
	}
	if gotOrder.OutTradeNo != "EXT_NEW_001" {
		t.Errorf("order not created")
	}
}

func TestOrderRepo_JoinTeamWithOrder(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewOrderRepo(testDB)
	ctx := context.Background()

	// 先建团（lock_count=1，target_count=3，还有2个位置）
	createTestTeam(t, "T_JOIN_001")
	createTestOrder(t, "ORD_JOIN_1", "EXT_JOIN_1", "T_JOIN_001", "USER010", model.OrderStatusLocked)

	// 手动更新 lock_count 为 1
	testDB.Exec("UPDATE teams SET lock_count = 1 WHERE team_id = ?", "T_JOIN_001")

	order := &model.Order{
		UserID:         "USER011",
		TeamID:         "T_JOIN_001",
		OrderID:        "ORD_JOIN_2",
		ActivityID:     100123,
		GoodsID:        "GOODS001",
		Source:         "APP",
		Channel:        "WECHAT",
		OriginalPrice:  "100.00",
		DeductionPrice: "20.00",
		PayPrice:       "80.00",
		Status:         model.OrderStatusLocked,
		OutTradeNo:     "EXT_JOIN_2",
	}

	err := repo.JoinTeamWithOrder(ctx, "T_JOIN_001", order)
	if err != nil {
		t.Fatalf("JoinTeamWithOrder: %v", err)
	}

	// 验证 lock_count 增加了
	team, err := repo.FindTeamByID(ctx, "T_JOIN_001")
	if err != nil {
		t.Fatalf("verify team: %v", err)
	}
	if team.LockCount != 2 {
		t.Errorf("expected lock_count=2, got %d", team.LockCount)
	}

	// 验证订单已创建
	gotOrder, err := repo.FindOrderByOrderID(ctx, "ORD_JOIN_2")
	if err != nil {
		t.Fatalf("verify order: %v", err)
	}
	if gotOrder.TeamID != "T_JOIN_001" {
		t.Errorf("order team mismatch")
	}
}

func TestOrderRepo_JoinTeamWithOrder_Full(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewOrderRepo(testDB)
	ctx := context.Background()

	// 建团并设 lock_count=target_count=3（团满）
	createTestTeam(t, "T_FULL_001")
	testDB.Exec("UPDATE teams SET lock_count = 3 WHERE team_id = ?", "T_FULL_001")

	order := &model.Order{
		UserID:  "USER020",
		TeamID:  "T_FULL_001",
		OrderID: "ORD_FULL_1",
		Status:  model.OrderStatusLocked,
		// ... (省略部分字段)
		ActivityID:     100123,
		GoodsID:        "GOODS001",
		Source:         "APP",
		Channel:        "WECHAT",
		OriginalPrice:  "100.00",
		DeductionPrice: "20.00",
		PayPrice:       "80.00",
		OutTradeNo:     "EXT_FULL_1",
	}

	err := repo.JoinTeamWithOrder(ctx, "T_FULL_001", order)
	if err != ErrTeamFull {
		t.Fatalf("expected ErrTeamFull, got %v", err)
	}
}

func TestOrderRepo_FindTimeoutOrders(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewOrderRepo(testDB)
	ctx := context.Background()

	// 使用 UTC 时间，与 Docker MySQL 容器的时区一致
	// MySQL 的 NOW() 返回 UTC，Go 本地时间是 UTC+8，不一致会导致查询结果偏差
	now := time.Now().UTC()
	pastEnd := now.Add(-10 * time.Minute)
	futureEnd := now.Add(30 * time.Minute)

	// 过期团+未支付订单 → 应被扫到
	testDB.Exec(`INSERT INTO teams (team_id, activity_id, source, channel, original_price, deduction_price, pay_price, target_count, complete_count, lock_count, status, valid_start, valid_end, notify_type) VALUES
		('T_TIMEOUT', 100123, 'APP', 'WECHAT', '100.00', '20.00', '80.00', 3, 0, 1, 0, ?, ?, 'HTTP')`,
		pastEnd.Add(-30*time.Minute), pastEnd,
	)
	testDB.Exec(`INSERT INTO orders (user_id, team_id, order_id, activity_id, goods_id, source, channel, original_price, deduction_price, pay_price, status, out_trade_no) VALUES
		('USER040', 'T_TIMEOUT', 'ORD_TIMEOUT', 100123, 'GOODS001', 'APP', 'WECHAT', '100.00', '20.00', '80.00', 0, 'EXT_TIMEOUT')`)

	// 正常未过期团+未支付订单 → 不应被扫到
	testDB.Exec(`INSERT INTO teams (team_id, activity_id, source, channel, original_price, deduction_price, pay_price, target_count, complete_count, lock_count, status, valid_start, valid_end, notify_type) VALUES
		('T_NORMAL', 100123, 'APP', 'WECHAT', '100.00', '20.00', '80.00', 3, 0, 1, 0, ?, ?, 'HTTP')`,
		now, futureEnd,
	)
	testDB.Exec(`INSERT INTO orders (user_id, team_id, order_id, activity_id, goods_id, source, channel, original_price, deduction_price, pay_price, status, out_trade_no) VALUES
		('USER041', 'T_NORMAL', 'ORD_NORMAL', 100123, 'GOODS001', 'APP', 'WECHAT', '100.00', '20.00', '80.00', 0, 'EXT_NORMAL')`)

	orders, err := repo.FindTimeoutOrders(ctx, 100, 0)
	if err != nil {
		t.Fatalf("FindTimeoutOrders: %v", err)
	}

	found := false
	for _, o := range orders {
		if o.OrderID == "ORD_TIMEOUT" {
			found = true
		}
		if o.OrderID == "ORD_NORMAL" {
			t.Errorf("normal order should not appear in timeout results")
		}
	}
	if !found {
		t.Errorf("timeout order ORD_TIMEOUT not found")
	}
}

func TestOrderRepo_FindOrdersByTeamID(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewOrderRepo(testDB)
	ctx := context.Background()

	createTestTeam(t, "T_ORDERS_001")
	createTestOrder(t, "ORD_T_1", "EXT_T_1", "T_ORDERS_001", "USER050", model.OrderStatusPaid)
	createTestOrder(t, "ORD_T_2", "EXT_T_2", "T_ORDERS_001", "USER051", model.OrderStatusPaid)

	orders, err := repo.FindOrdersByTeamID(ctx, "T_ORDERS_001")
	if err != nil {
		t.Fatalf("FindOrdersByTeamID: %v", err)
	}
	if len(orders) != 2 {
		t.Errorf("expected 2 orders, got %d", len(orders))
	}
}
