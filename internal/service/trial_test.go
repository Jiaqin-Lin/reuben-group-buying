package service

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/shopspring/decimal"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/reuben/group-buying/internal/errcode"
	"github.com/reuben/group-buying/internal/model"
	"github.com/reuben/group-buying/internal/repository"
)

// testDB 在 TestMain 中初始化，所有测试共享一个 MySQL 连接。
var testDB *gorm.DB

// testRDB 在 TestMain 中初始化，所有测试共享一个 Redis 连接。
var testRDB *goredis.Client

func TestMain(m *testing.M) {
	// MySQL 连接
	dsn := "dev:dev123@tcp(127.0.0.1:3306)/group_buy_market?charset=utf8mb4&parseTime=True&loc=UTC"
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Silent),
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		println("skip service tests: mysql not available")
		return
	}
	sqlDB, _ := db.DB()
	if sqlDB == nil || sqlDB.Ping() != nil {
		println("skip service tests: mysql ping failed")
		return
	}
	defer sqlDB.Close()
	testDB = db

	// Redis 连接
	rdb := goredis.NewClient(&goredis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "",
		DB:       1, // 用 DB 1 避免与其它测试冲突
	})
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		println("skip service tests: redis not available")
		testDB = nil
		return
	}
	testRDB = rdb

	// 准备种子数据
	seedTrialTestData(testDB)
	seedLockTestData(testDB)
	seedSettlementTestData(testDB)
	seedRefundTestData(testDB)
	rdb.FlushDB(ctx).Err()

	code := m.Run()

	rdb.Close()
	os.Exit(code)
}

// seedTrialTestData 插入试算测试所需的所有数据。
func seedTrialTestData(db *gorm.DB) {
	// 清空
	truncateTrialTables(db)

	// 折扣（4种类型）
	db.Exec(`INSERT INTO discounts (discount_id, name, description, plan_type, expression, discount_type, tag_id) VALUES
		('D_ZJ',   '直减20元',    '直减',   'ZJ', '20.00',   0, NULL),
		('D_MJ',   '满100减20',   '满减',   'MJ', '100.00-20.00', 0, NULL),
		('D_ZK',   '8折优惠',     '折扣',   'ZK', '0.80',    0, NULL),
		('D_N',    '9.9元购',     'N元购',  'N',  '9.90',    0, NULL),
		('D_TAG',  'VIP专享减5元','人群折扣','ZJ', '5.00',    1, 'TAG_VIP')
	`)

	// 活动（6个：1个活跃+1个过期+1个时间外+1个带人群标签+1个满减+1个N元购... actually let me just keep it simple）
	// 使用不同的 activity_id 区分场景
	db.Exec(`INSERT INTO activities (activity_id, name, discount_id, group_type, target_count, take_limit, valid_minutes, status, start_time, end_time, tag_id, tag_scope) VALUES
		(200001, '直减活动',       'D_ZJ',  0, 3, 5, 30, 1, '2025-01-01', '2029-12-31', NULL, NULL),
		(200002, '满减活动',       'D_MJ',  0, 3, 5, 30, 1, '2025-01-01', '2029-12-31', NULL, NULL),
		(200003, '折扣活动',       'D_ZK',  0, 3, 5, 30, 1, '2025-01-01', '2029-12-31', NULL, NULL),
		(200004, 'N元购活动',      'D_N',   0, 3, 5, 30, 1, '2025-01-01', '2029-12-31', NULL, NULL),
		(200005, '已过期活动',     'D_ZJ',  0, 3, 5, 30, 3, '2025-01-01', '2029-12-31', NULL, NULL),
		(200006, '未开始活动',     'D_ZJ',  0, 3, 5, 30, 1, '2029-01-01', '2029-12-31', NULL, NULL),
		(200007, '人群标签活动',   'D_ZJ',  0, 3, 5, 30, 1, '2025-01-01', '2029-12-31', 'TAG_VIP', '10'),
		(200008, '人群折扣活动',   'D_TAG', 0, 3, 5, 30, 1, '2025-01-01', '2029-12-31', NULL, NULL)
	`)

	// 商品
	db.Exec(`INSERT INTO products (goods_id, goods_name, original_price) VALUES
		('G_ZJ',  '直减商品', 100.00),
		('G_MJ',  '满减商品', 150.00),
		('G_MJ2', '满减商品2', 80.00),
		('G_ZK',  '折扣商品', 100.00),
		('G_N',   'N元购商品', 100.00),
		('G_EXP', '过期活动商品', 100.00),
		('G_FUT', '未开始活动商品', 100.00),
		('G_TAG', '人群标签商品', 100.00),
		('G_TAG2','人群折扣商品', 100.00),
		('G_NF',  '无活动商品', 100.00)
	`)

	// 活动-商品映射
	db.Exec(`INSERT INTO activity_products (source, channel, goods_id, activity_id) VALUES
		('APP', 'WECHAT', 'G_ZJ',  200001),
		('APP', 'WECHAT', 'G_MJ',  200002),
		('APP', 'WECHAT', 'G_MJ2', 200002),
		('APP', 'WECHAT', 'G_ZK',  200003),
		('APP', 'WECHAT', 'G_N',   200004),
		('APP', 'WECHAT', 'G_EXP', 200005),
		('APP', 'WECHAT', 'G_FUT', 200006),
		('APP', 'WECHAT', 'G_TAG', 200007),
		('APP', 'WECHAT', 'G_TAG2',200008)
	`)

	// 人群标签
	db.Exec(`INSERT INTO crowd_tags (tag_id, tag_name, tag_desc, statistics) VALUES
		('TAG_VIP', 'VIP用户', 'VIP等级>=3', 100)
	`)
	db.Exec(`INSERT INTO crowd_tag_details (tag_id, user_id) VALUES
		('TAG_VIP', 'VIP_USER')
	`)
}

// truncateTrialTables 清空试算相关表。
func truncateTrialTables(db *gorm.DB) {
	db.Exec("SET FOREIGN_KEY_CHECKS = 0")
	tables := []string{
		"activity_products", "products", "activities", "discounts",
		"crowd_tag_details", "crowd_tags",
	}
	for _, t := range tables {
		db.Exec("DELETE FROM " + t)
	}
	db.Exec("SET FOREIGN_KEY_CHECKS = 1")
}

// newTestTrialService 创建用于测试的 TrialService。
func newTestTrialService(t *testing.T) *TrialService {
	t.Helper()
	if testDB == nil || testRDB == nil {
		t.Skip("mysql or redis not available")
	}
	return NewTrialService(
		repository.NewActivityRepo(testDB),
		repository.NewProductRepo(testDB),
		repository.NewRedisCacheRepo(testRDB),
		repository.NewCrowdRepo(testDB),
	)
}

// ==================== 折扣类型测试 ====================

func TestTrial_ZJ_DirectReduction(t *testing.T) {
	svc := newTestTrialService(t)
	ctx := context.Background()

	result, err := svc.Trial(ctx, TrialRequest{
		UserID: "U1", GoodsID: "G_ZJ", Source: "APP", Channel: "WECHAT",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OriginalPrice != "100.00" {
		t.Errorf("original = %s, want 100.00", result.OriginalPrice)
	}
	if result.PayPrice != "80.00" {
		t.Errorf("pay = %s, want 80.00 (100-20)", result.PayPrice)
	}
	if result.DeductionPrice != "20.00" {
		t.Errorf("deduction = %s, want 20.00", result.DeductionPrice)
	}
	if result.ActivityID != 200001 {
		t.Errorf("activity_id = %d, want 200001", result.ActivityID)
	}
}

func TestTrial_MJ_FullReduction_Satisfied(t *testing.T) {
	svc := newTestTrialService(t)
	ctx := context.Background()

	// G_MJ = 150.00, 满100减20 → 130.00
	result, err := svc.Trial(ctx, TrialRequest{
		UserID: "U1", GoodsID: "G_MJ", Source: "APP", Channel: "WECHAT",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PayPrice != "130.00" {
		t.Errorf("pay = %s, want 130.00 (150-20)", result.PayPrice)
	}
}

func TestTrial_MJ_FullReduction_NotSatisfied(t *testing.T) {
	svc := newTestTrialService(t)
	ctx := context.Background()

	// G_MJ2 = 80.00, 不满100 → 原价
	result, err := svc.Trial(ctx, TrialRequest{
		UserID: "U1", GoodsID: "G_MJ2", Source: "APP", Channel: "WECHAT",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PayPrice != "80.00" {
		t.Errorf("pay = %s, want 80.00 (not satisfied, original)", result.PayPrice)
	}
	if result.DeductionPrice != "0.00" {
		t.Errorf("deduction = %s, want 0.00", result.DeductionPrice)
	}
}

func TestTrial_ZK_Percentage(t *testing.T) {
	svc := newTestTrialService(t)
	ctx := context.Background()

	// G_ZK = 100.00, 8折 → 80.00
	result, err := svc.Trial(ctx, TrialRequest{
		UserID: "U1", GoodsID: "G_ZK", Source: "APP", Channel: "WECHAT",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PayPrice != "80.00" {
		t.Errorf("pay = %s, want 80.00 (100*0.8)", result.PayPrice)
	}
}

func TestTrial_N_FixedPrice(t *testing.T) {
	svc := newTestTrialService(t)
	ctx := context.Background()

	// G_N = 100.00, N元购 9.90
	result, err := svc.Trial(ctx, TrialRequest{
		UserID: "U1", GoodsID: "G_N", Source: "APP", Channel: "WECHAT",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PayPrice != "9.90" {
		t.Errorf("pay = %s, want 9.90", result.PayPrice)
	}
	if result.DeductionPrice != "90.10" {
		t.Errorf("deduction = %s, want 90.10", result.DeductionPrice)
	}
}

// ==================== 活动状态校验测试 ====================

func TestTrial_ActivityExpired(t *testing.T) {
	svc := newTestTrialService(t)
	ctx := context.Background()

	_, err := svc.Trial(ctx, TrialRequest{
		UserID: "U1", GoodsID: "G_EXP", Source: "APP", Channel: "WECHAT",
	})
	if err == nil {
		t.Fatal("expected error for expired activity")
	}
	var trialErr *TrialError
	if !errors.As(err, &trialErr) {
		t.Fatalf("expected TrialError, got %T: %v", err, err)
	}
	if trialErr.ErrorCode() != errcode.CodeActivityInactive {
		t.Errorf("error code = %s, want %s", trialErr.ErrorCode(), errcode.CodeActivityInactive)
	}
}

func TestTrial_ActivityNotStarted(t *testing.T) {
	svc := newTestTrialService(t)
	ctx := context.Background()

	_, err := svc.Trial(ctx, TrialRequest{
		UserID: "U1", GoodsID: "G_FUT", Source: "APP", Channel: "WECHAT",
	})
	if err == nil {
		t.Fatal("expected error for future activity")
	}
	var trialErr *TrialError
	if !errors.As(err, &trialErr) {
		t.Fatalf("expected TrialError, got %T: %v", err, err)
	}
	if trialErr.ErrorCode() != errcode.CodeActivityTimeInvalid {
		t.Errorf("error code = %s, want %s", trialErr.ErrorCode(), errcode.CodeActivityTimeInvalid)
	}
}

// ==================== 活动映射不存在测试 ====================

func TestTrial_NoActivityMapping(t *testing.T) {
	svc := newTestTrialService(t)
	ctx := context.Background()

	_, err := svc.Trial(ctx, TrialRequest{
		UserID: "U1", GoodsID: "G_NF", Source: "APP", Channel: "WECHAT",
	})
	if err == nil {
		t.Fatal("expected error for missing activity mapping")
	}
	var trialErr *TrialError
	if !errors.As(err, &trialErr) {
		t.Fatalf("expected TrialError, got %T: %v", err, err)
	}
	if trialErr.ErrorCode() != errcode.CodeTrialFailed {
		t.Errorf("error code = %s, want %s", trialErr.ErrorCode(), errcode.CodeTrialFailed)
	}
}

// ==================== 人群标签测试 ====================

func TestTrial_CrowdTag_NotInCrowd_Blocked(t *testing.T) {
	svc := newTestTrialService(t)
	ctx := context.Background()

	// G_TAG → activity 200007, tag_id=TAG_VIP, tag_scope=10 → 不在人群中不可见不可参与
	result, err := svc.Trial(ctx, TrialRequest{
		UserID: "NORMAL_USER", GoodsID: "G_TAG", Source: "APP", Channel: "WECHAT",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsVisible {
		t.Error("expected IsVisible=false for user not in crowd with scope=10")
	}
	if result.IsEnable {
		t.Error("expected IsEnable=false for user not in crowd with scope=10")
	}
	// 价格仍然正常计算
	if result.PayPrice != "80.00" {
		t.Errorf("pay = %s, want 80.00", result.PayPrice)
	}
}

func TestTrial_CrowdTag_InCrowd_Allowed(t *testing.T) {
	svc := newTestTrialService(t)
	ctx := context.Background()

	// VIP_USER 在 TAG_VIP 人群中 → 可见且可参与
	result, err := svc.Trial(ctx, TrialRequest{
		UserID: "VIP_USER", GoodsID: "G_TAG", Source: "APP", Channel: "WECHAT",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsVisible {
		t.Error("expected IsVisible=true for user in crowd")
	}
	if !result.IsEnable {
		t.Error("expected IsEnable=true for user in crowd")
	}
}

func TestTrial_DiscountTag_NotInCrowd_OriginalPrice(t *testing.T) {
	svc := newTestTrialService(t)
	ctx := context.Background()

	// G_TAG2 → activity 200008, discount D_TAG (discount_type=1, tag_id=TAG_VIP, ZJ 5.00)
	// NORMAL_USER 不在 TAG_VIP 中 → 原价
	result, err := svc.Trial(ctx, TrialRequest{
		UserID: "NORMAL_USER", GoodsID: "G_TAG2", Source: "APP", Channel: "WECHAT",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PayPrice != "100.00" {
		t.Errorf("pay = %s, want 100.00 (not in discount crowd, original)", result.PayPrice)
	}
	if result.DeductionPrice != "0.00" {
		t.Errorf("deduction = %s, want 0.00", result.DeductionPrice)
	}
}

func TestTrial_DiscountTag_InCrowd_Discounted(t *testing.T) {
	svc := newTestTrialService(t)
	ctx := context.Background()

	// VIP_USER 在 TAG_VIP 中 → 享受 ZJ 5.00 折扣
	result, err := svc.Trial(ctx, TrialRequest{
		UserID: "VIP_USER", GoodsID: "G_TAG2", Source: "APP", Channel: "WECHAT",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PayPrice != "95.00" {
		t.Errorf("pay = %s, want 95.00 (100-5)", result.PayPrice)
	}
	if result.DeductionPrice != "5.00" {
		t.Errorf("deduction = %s, want 5.00", result.DeductionPrice)
	}
}

// ==================== 参数校验测试 ====================

func TestTrial_InvalidParams(t *testing.T) {
	svc := newTestTrialService(t)
	ctx := context.Background()

	_, err := svc.Trial(ctx, TrialRequest{
		UserID: "", GoodsID: "G_ZJ", Source: "APP", Channel: "WECHAT",
	})
	if err == nil {
		t.Fatal("expected error for empty user_id")
	}
}

// ==================== 最低价格 0.01 测试 ====================

func TestTrial_MinimumPrice_OneCent(t *testing.T) {
	ctx := context.Background()

	// 把 N 元购的价格设为 0，应该返回 0.01
	// 直接测试 calculatePayPrice 函数
	originalPrice, _ := decimal.NewFromString("100.00")
	d := model.Discount{
		PlanType:     model.PlanZJ,
		Expression:   "200.00", // 直减200，100-200=-100
		DiscountType: model.DiscountTypeBase,
	}
	pay, err := calculatePayPrice(ctx, originalPrice, d, "U1", nil, nil) // nil repos won't be called for BASE type
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pay.StringFixed(2) != "0.01" {
		t.Errorf("pay = %s, want 0.01 (minimum)", pay.StringFixed(2))
	}
}
