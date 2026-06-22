// Package cache 本地内存缓存层。
//
// 职责：将活动、折扣、商品、活动-商品映射等低频变更的参考数据全量加载到内存，
// 消除试算/锁单热路径上的 3-4 次 DB 查询。
//
// 刷新策略：
//   - 启动时全量加载（同步，阻塞直至完成）
//   - 定时刷新（每 5 分钟，可配置）
//   - Redis Pub/Sub 通知刷新（管理员修改配置后秒级生效）
//
// 并发安全：所有读操作用 RLock，写操作（Load）用 Lock。
// Load 期间不阻塞读（短暂不一致可接受，下一轮读自动修正）。
package cache

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"github.com/reuben/group-buying/internal/model"
)

// ActivityWithDiscount 活动+折扣聚合，试算核心数据结构。
type ActivityWithDiscount struct {
	Activity model.Activity
	Discount model.Discount
}

// Stats 缓存统计信息。
type Stats struct {
	Activities int       `json:"activities"`
	Discounts  int       `json:"discounts"`
	Products   int       `json:"products"`
	Mappings   int       `json:"mappings"`
	LastLoad   time.Time `json:"last_load"`
	LoadCount  int64     `json:"load_count"`
}

// LocalCache 本地内存缓存。
// 用 sync.RWMutex + 全量替换 map 的方式，比 sync.Map 更适合批量刷新场景。
type LocalCache struct {
	mu sync.RWMutex
	db *gorm.DB

	// "source:channel:goodsId" → activityId
	ap map[string]int64

	// activityId → Activity
	activities map[int64]*model.Activity

	// discountId → Discount
	discounts map[string]*model.Discount

	// goodsId → Product
	products map[string]*model.Product

	lastLoad  time.Time
	loadCount atomic.Int64

	// Cache for ActivityWithDiscount (computed from activities + discounts)
	// activityId → ActivityWithDiscount
	awd map[int64]*ActivityWithDiscount

	// goodsId → first active activityId（用于产品列表页展示活动摘要）
	goodsToActivity map[string]int64
}

// New 创建本地缓存实例。
func New(db *gorm.DB) *LocalCache {
	return &LocalCache{
		db:              db,
		ap:              make(map[string]int64),
		activities:      make(map[int64]*model.Activity),
		discounts:       make(map[string]*model.Discount),
		products:        make(map[string]*model.Product),
		awd:             make(map[int64]*ActivityWithDiscount),
		goodsToActivity: make(map[string]int64),
	}
}

// Load 从 DB 全量加载参考数据。
//
// 查询策略：
//  1. 查所有生效中的活动（status=Active 且在时间范围内）
//  2. 查这些活动关联的折扣
//  3. 查所有商品
//  4. 查所有活动-商品映射
//
// 加载期间持有写锁，但数据量小（通常 < 10000 行），锁持有时间 < 100ms。
func (c *LocalCache) Load(ctx context.Context) error {
	start := time.Now()

	// 1. 加载活动
	var activities []model.Activity
	if err := c.db.WithContext(ctx).
		Where("status = ?", model.ActivityStatusActive).
		Where("start_time <= NOW()").
		Where("end_time >= NOW()").
		Find(&activities).Error; err != nil {
		return fmt.Errorf("load activities: %w", err)
	}

	// 2. 加载这些活动关联的折扣（按 discount_id IN）
	discountIDs := make([]string, 0, len(activities))
	for i := range activities {
		discountIDs = append(discountIDs, activities[i].DiscountID)
	}
	var discounts []model.Discount
	if len(discountIDs) > 0 {
		if err := c.db.WithContext(ctx).
			Where("discount_id IN ?", discountIDs).
			Find(&discounts).Error; err != nil {
			return fmt.Errorf("load discounts: %w", err)
		}
	}

	// 3. 加载所有商品
	var products []model.Product
	if err := c.db.WithContext(ctx).Find(&products).Error; err != nil {
		return fmt.Errorf("load products: %w", err)
	}

	// 4. 加载活动-商品映射
	var aps []model.ActivityProduct
	if err := c.db.WithContext(ctx).Find(&aps).Error; err != nil {
		return fmt.Errorf("load activity_products: %w", err)
	}

	// 构建新 map
	newAP := make(map[string]int64, len(aps))
	for i := range aps {
		key := apKey(aps[i].Source, aps[i].Channel, aps[i].GoodsID)
		newAP[key] = aps[i].ActivityID
	}

	newActivities := make(map[int64]*model.Activity, len(activities))
	for i := range activities {
		a := activities[i] // copy
		newActivities[a.ActivityID] = &a
	}

	newDiscounts := make(map[string]*model.Discount, len(discounts))
	for i := range discounts {
		d := discounts[i] // copy
		newDiscounts[d.DiscountID] = &d
	}

	newProducts := make(map[string]*model.Product, len(products))
	for i := range products {
		p := products[i] // copy
		newProducts[p.GoodsID] = &p
	}

	// 预计算 ActivityWithDiscount
	newAWD := make(map[int64]*ActivityWithDiscount, len(activities))
	for i := range activities {
		a := activities[i]
		if d, ok := newDiscounts[a.DiscountID]; ok {
			newAWD[a.ActivityID] = &ActivityWithDiscount{
				Activity: a,
				Discount: *d,
			}
		}
	}

	// 预计算 goodsId → 第一个活跃 activityId（产品列表用）
	newGoodsToActivity := make(map[string]int64, len(aps))
	for i := range aps {
		if _, exists := newGoodsToActivity[aps[i].GoodsID]; !exists {
			if _, ok := newActivities[aps[i].ActivityID]; ok {
				newGoodsToActivity[aps[i].GoodsID] = aps[i].ActivityID
			}
		}
	}

	// 原子替换
	c.mu.Lock()
	c.ap = newAP
	c.activities = newActivities
	c.discounts = newDiscounts
	c.products = newProducts
	c.awd = newAWD
	c.goodsToActivity = newGoodsToActivity
	c.lastLoad = time.Now()
	c.mu.Unlock()

	c.loadCount.Add(1)
	slog.InfoContext(ctx, "local cache loaded",
		"activities", len(activities),
		"discounts", len(discounts),
		"products", len(products),
		"mappings", len(aps),
		"elapsed", time.Since(start).String(),
	)
	return nil
}

// GetActivityProduct 查活动-商品映射。
// 返回 (activityId, true) 表示命中，(0, false) 表示未找到。
func (c *LocalCache) GetActivityProduct(source, channel, goodsID string) (int64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	id, ok := c.ap[apKey(source, channel, goodsID)]
	return id, ok
}

// GetActivityWithDiscount 查活动+折扣。
// 返回 (ActivityWithDiscount, true) 表示命中，(nil, false) 表示未找到。
func (c *LocalCache) GetActivityWithDiscount(activityID int64) (*ActivityWithDiscount, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	awd, ok := c.awd[activityID]
	return awd, ok
}

// GetProduct 查商品。
// 返回 (Product, true) 表示命中，(nil, false) 表示未找到。
func (c *LocalCache) GetProduct(goodsID string) (*model.Product, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.products[goodsID]
	return p, ok
}

// GetAllProducts 返回所有商品，按 goods_id 排序（只读，调用方不可修改）。
func (c *LocalCache) GetAllProducts() []*model.Product {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]*model.Product, 0, len(c.products))
	for _, p := range c.products {
		result = append(result, p)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].GoodsID < result[j].GoodsID
	})
	return result
}

// GetActivityForProduct 查商品关联的第一个活跃活动+折扣。
// 返回 (ActivityWithDiscount, true) 表示命中，(nil, false) 表示该商品无活跃活动。
func (c *LocalCache) GetActivityForProduct(goodsID string) (*ActivityWithDiscount, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	activityID, ok := c.goodsToActivity[goodsID]
	if !ok {
		return nil, false
	}
	awd, ok := c.awd[activityID]
	return awd, ok
}

// GetActivity 查活动（不含折扣）。
func (c *LocalCache) GetActivity(activityID int64) (*model.Activity, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	a, ok := c.activities[activityID]
	return a, ok
}

// Stats 返回缓存统计信息。
func (c *LocalCache) Stats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return Stats{
		Activities: len(c.activities),
		Discounts:  len(c.discounts),
		Products:   len(c.products),
		Mappings:   len(c.ap),
		LastLoad:   c.lastLoad,
		LoadCount:  c.loadCount.Load(),
	}
}

// apKey 构建活动-商品映射的 map key。
func apKey(source, channel, goodsID string) string {
	return source + ":" + channel + ":" + goodsID
}
