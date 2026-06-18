package repository

import (
	"context"
	"testing"
)

func TestProductRepo_FindByGoodsID(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewProductRepo(testDB)
	ctx := context.Background()

	p, err := repo.FindProductByGoodsID(ctx, "GOODS001")
	if err != nil {
		t.Fatalf("FindProductByGoodsID: %v", err)
	}
	if p.GoodsName != "测试商品" {
		t.Errorf("expected goods_name=测试商品, got %s", p.GoodsName)
	}
	if p.OriginalPrice != "100.00" {
		t.Errorf("expected original_price=100.00, got %s", p.OriginalPrice)
	}
}

func TestProductRepo_FindByGoodsID_NotFound(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewProductRepo(testDB)
	ctx := context.Background()

	_, err := repo.FindProductByGoodsID(ctx, "NONEXIST")
	if err == nil {
		t.Fatal("expected error for non-existent product")
	}
}
