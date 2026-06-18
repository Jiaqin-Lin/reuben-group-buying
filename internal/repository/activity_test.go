package repository

import (
	"context"
	"testing"

	"github.com/reuben/group-buying/internal/model"
)

func TestActivityRepo_FindByID(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewActivityRepo(testDB)
	ctx := context.Background()

	act, err := repo.FindActivityByID(ctx, 100123)
	if err != nil {
		t.Fatalf("FindActivityByID: %v", err)
	}
	if act.ActivityID != 100123 {
		t.Errorf("expected activity_id=100123, got %d", act.ActivityID)
	}
	if act.Status != model.ActivityStatusActive {
		t.Errorf("expected status=1 (active), got %d", act.Status)
	}
	if act.TargetCount != 3 {
		t.Errorf("expected target_count=3, got %d", act.TargetCount)
	}
}

func TestActivityRepo_FindByID_NotFound(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewActivityRepo(testDB)
	ctx := context.Background()

	_, err := repo.FindActivityByID(ctx, 999999)
	if err == nil {
		t.Fatal("expected error for non-existent activity")
	}
}

func TestActivityRepo_FindDiscountByID(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewActivityRepo(testDB)
	ctx := context.Background()

	d, err := repo.FindDiscountByID(ctx, "D001")
	if err != nil {
		t.Fatalf("FindDiscountByID: %v", err)
	}
	if d.PlanType != model.PlanZJ {
		t.Errorf("expected plan_type=ZJ, got %s", d.PlanType)
	}
	if d.Expression != "20" {
		t.Errorf("expected expression=20, got %s", d.Expression)
	}
}

func TestActivityRepo_FindActivityProduct(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewActivityRepo(testDB)
	ctx := context.Background()

	ap, err := repo.FindActivityProduct(ctx, "APP", "WECHAT", "GOODS001")
	if err != nil {
		t.Fatalf("FindActivityProduct: %v", err)
	}
	if ap.ActivityID != 100123 {
		t.Errorf("expected activity_id=100123, got %d", ap.ActivityID)
	}
}

func TestActivityRepo_FindActivityProduct_NotFound(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewActivityRepo(testDB)
	ctx := context.Background()

	// 不存在的渠道-商品组合
	_, err := repo.FindActivityProduct(ctx, "APP", "ALIPAY", "GOODS001")
	if err == nil {
		t.Fatal("expected error for non-existent mapping")
	}
}

func TestActivityRepo_FindActivityWithDiscount(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewActivityRepo(testDB)
	ctx := context.Background()

	result, err := repo.FindActivityWithDiscount(ctx, 100123)
	if err != nil {
		t.Fatalf("FindActivityWithDiscount: %v", err)
	}
	if result.ActivityID != 100123 {
		t.Errorf("expected activity_id=100123, got %d", result.ActivityID)
	}
	if result.Discount.DiscountID != "D001" {
		t.Errorf("expected discount_id=D001, got %s", result.Discount.DiscountID)
	}
}

func TestActivityRepo_FindActiveActivities(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewActivityRepo(testDB)
	ctx := context.Background()

	activities, err := repo.FindActiveActivities(ctx)
	if err != nil {
		t.Fatalf("FindActiveActivities: %v", err)
	}
	if len(activities) < 1 {
		t.Fatal("expected at least 1 active activity (100123)")
	}
	// 确认不包含 status=2 的已过期活动
	for _, a := range activities {
		if a.ActivityID == 100456 {
			t.Errorf("expired activity 100456 should not appear in active list")
		}
	}
}
