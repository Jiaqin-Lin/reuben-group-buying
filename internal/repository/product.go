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

	// FindAllProducts 查所有商品。
	// 主要用于产品列表页。注意：LocalCache 也有 GetAllProducts，
	// 此方法作为 fallback（缓存 miss 时使用）。
	FindAllProducts(ctx context.Context) ([]model.Product, error)
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

// FindAllProducts 查所有商品，按 goods_id 排序。
func (r *productRepo) FindAllProducts(ctx context.Context) ([]model.Product, error) {
	var products []model.Product
	err := r.db.WithContext(ctx).
		Order("goods_id ASC").
		Find(&products).Error
	if err != nil {
		return nil, fmt.Errorf("find all products: %w", err)
	}
	return products, nil
}
