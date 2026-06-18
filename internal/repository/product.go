package repository

import (
	"context"
	"fmt"

	"github.com/reuben/group-buying/internal/model"
	"gorm.io/gorm"
)

// ProductRepository 商品数据访问接口。
type ProductRepository interface {
	// FindProductByGoodsID 按 goods_id 查商品。
	// 用于试算时获取商品原价和名称。
	FindProductByGoodsID(ctx context.Context, goodsID string) (*model.Product, error)
}

// productRepo GORM 实现。
type productRepo struct {
	db *gorm.DB
}

// NewProductRepo 构造函数。
func NewProductRepo(db *gorm.DB) ProductRepository {
	return &productRepo{db: db}
}

// FindProductByGoodsID 按 goods_id 查商品。
func (r *productRepo) FindProductByGoodsID(ctx context.Context, goodsID string) (*model.Product, error) {
	var p model.Product
	err := r.db.WithContext(ctx).
		Where("goods_id = ?", goodsID).
		First(&p).Error
	if err != nil {
		return nil, fmt.Errorf("find product %s: %w", goodsID, err)
	}
	return &p, nil
}
