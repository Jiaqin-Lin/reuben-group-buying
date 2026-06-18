package repository

import (
	"context"
	"testing"
	"time"

	"github.com/reuben/group-buying/internal/model"
)

func TestPaymentRepo_CreateAndFind(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewPaymentRepo(testDB)
	ctx := context.Background()

	payment := &model.Payment{
		OrderID:    "ORD_PAY_001",
		OutTradeNo: "EXT_PAY_FIND",
		UserID:     "USER100",
		TeamID:     "T_PAY_001",
		Amount:     "80.00",
		Subject:    "测试商品",
		Status:     model.PaymentStatusPending,
		ExpireAt:   time.Now().Add(30 * time.Minute),
	}

	err := repo.CreatePayment(ctx, payment)
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	got, err := repo.FindPaymentByOrderID(ctx, "ORD_PAY_001")
	if err != nil {
		t.Fatalf("FindPaymentByOrderID: %v", err)
	}
	if got.Amount != "80.00" {
		t.Errorf("expected amount=80.00, got %s", got.Amount)
	}
	if got.Status != model.PaymentStatusPending {
		t.Errorf("expected status=0 (pending), got %d", got.Status)
	}
}

func TestPaymentRepo_FindByOutTradeNo(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewPaymentRepo(testDB)
	ctx := context.Background()

	// 先创建
	testDB.Exec(`INSERT INTO payments (order_id, out_trade_no, user_id, team_id, amount, subject, status, expire_at) VALUES
		('ORD_PAY_002', 'EXT_PAY_002', 'USER101', 'T_PAY_002', '80.00', 'test', 0, NOW() + INTERVAL 30 MINUTE)`)

	got, err := repo.FindPaymentByOutTradeNo(ctx, "EXT_PAY_002")
	if err != nil {
		t.Fatalf("FindPaymentByOutTradeNo: %v", err)
	}
	if got.OrderID != "ORD_PAY_002" {
		t.Errorf("expected order_id=ORD_PAY_002, got %s", got.OrderID)
	}
}

func TestPaymentRepo_UpdatePaymentPaid(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewPaymentRepo(testDB)
	ctx := context.Background()

	testDB.Exec(`INSERT INTO payments (order_id, out_trade_no, user_id, team_id, amount, subject, status, expire_at) VALUES
		('ORD_PAY_003', 'EXT_PAY_003', 'USER102', 'T_PAY_003', '80.00', 'test', 0, NOW() + INTERVAL 30 MINUTE)`)

	err := repo.UpdatePaymentPaid(ctx, "ORD_PAY_003", "ALIPAY_TRADE_001")
	if err != nil {
		t.Fatalf("UpdatePaymentPaid: %v", err)
	}

	got, err := repo.FindPaymentByOrderID(ctx, "ORD_PAY_003")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.Status != model.PaymentStatusPaid {
		t.Errorf("expected status=1 (paid), got %d", got.Status)
	}
	if got.TradeNo == nil || *got.TradeNo != "ALIPAY_TRADE_001" {
		t.Errorf("expected trade_no=ALIPAY_TRADE_001, got %v", got.TradeNo)
	}
	if got.PaidAt == nil {
		t.Error("expected paid_at to be set")
	}
}

func TestPaymentRepo_UpdatePaymentClosed(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewPaymentRepo(testDB)
	ctx := context.Background()

	testDB.Exec(`INSERT INTO payments (order_id, out_trade_no, user_id, team_id, amount, subject, status, expire_at) VALUES
		('ORD_PAY_004', 'EXT_PAY_004', 'USER103', 'T_PAY_004', '80.00', 'test', 0, NOW() + INTERVAL 30 MINUTE)`)

	err := repo.UpdatePaymentClosed(ctx, "ORD_PAY_004")
	if err != nil {
		t.Fatalf("UpdatePaymentClosed: %v", err)
	}

	got, err := repo.FindPaymentByOrderID(ctx, "ORD_PAY_004")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.Status != model.PaymentStatusClosed {
		t.Errorf("expected status=2 (closed), got %d", got.Status)
	}
}

func TestPaymentLog_CreateAndFind(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewPaymentRepo(testDB)
	ctx := context.Background()

	pl := &model.PaymentLog{
		OrderID:   "ORD_LOG_001",
		NotifyID:  "ALIPAY_NOTIFY_001",
		NotifyRaw: `{"trade_no":"2025010100001","total_amount":"80.00"}`,
		Status:    model.PayLogStatusUnverified,
	}

	err := repo.CreatePaymentLog(ctx, pl)
	if err != nil {
		t.Fatalf("CreatePaymentLog: %v", err)
	}

	got, err := repo.FindPaymentLogByNotifyID(ctx, "ALIPAY_NOTIFY_001")
	if err != nil {
		t.Fatalf("FindPaymentLogByNotifyID: %v", err)
	}
	if got.OrderID != "ORD_LOG_001" {
		t.Errorf("expected order_id=ORD_LOG_001, got %s", got.OrderID)
	}
}

func TestPaymentLog_Dedup(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewPaymentRepo(testDB)
	ctx := context.Background()

	// 第一次插入成功
	pl := &model.PaymentLog{
		OrderID:   "ORD_DEDUP_001",
		NotifyID:  "ALIPAY_DEDUP_001",
		NotifyRaw: "{}",
		Status:    model.PayLogStatusUnverified,
	}
	if err := repo.CreatePaymentLog(ctx, pl); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// 第二次插入相同 notify_id 应失败（UK 冲突）
	pl2 := &model.PaymentLog{
		OrderID:   "ORD_DEDUP_002",
		NotifyID:  "ALIPAY_DEDUP_001", // 相同 notify_id
		NotifyRaw: "{}",
		Status:    model.PayLogStatusUnverified,
	}
	err := repo.CreatePaymentLog(ctx, pl2)
	if err == nil {
		t.Fatal("expected duplicate key error for same notify_id")
	}
}
