// Package repository 数据访问层。
// 一个文件负责一组表/一组业务查询，文件顶部定义 interface，底部是 GORM 实现。
// 所有方法接收 context.Context，支持超时和链路追踪。
package repository

import (
	"context"
	"fmt"

	"github.com/reuben/group-buying/internal/model"
	"gorm.io/gorm"
)

// ActivityRepository 活动、折扣、活动-商品映射的数据访问接口。
type ActivityRepository interface {
	// FindActivityByID 按业务 activity_id 查活动。
	// 用于试算、锁单时校验活动状态和参与限制。
	FindActivityByID(ctx context.Context, activityID int64) (*model.Activity, error)

	// FindDiscountByID 按业务 discount_id 查折扣规则。
	// 折扣表达式由 service 层解析计算，repository 只做数据获取。
	FindDiscountByID(ctx context.Context, discountID string) (*model.Discount, error)

	// FindActivityProduct 按 (source, channel, goods_id) 查活动映射。
	// 这是试算入口：外部请求带 source+channel+goodsId，通过此方法找到对应活动。
	// 如果找不到，说明该商品在当前渠道没有拼团活动。
	FindActivityProduct(ctx context.Context, source, channel, goodsID string) (*model.ActivityProduct, error)

	// FindActivityWithDiscount 联表查活动+折扣，一次查询拿到试算所需全部数据。
	// 替代 N+1 查询：SELECT a.*, d.* FROM activities a JOIN discounts d ON a.discount_id = d.discount_id WHERE a.activity_id = ?
	FindActivityWithDiscount(ctx context.Context, activityID int64) (*ActivityWithDiscount, error)

	// FindActiveActivities 查询所有生效中的活动。
	// 用于缓存预热：启动时批量加载到本地/Redis 缓存。
	FindActiveActivities(ctx context.Context) ([]model.Activity, error)

	// FindDiscountByIDAndTag 查人群标签折扣（discount_type=1 + tag_id）。
	// 试算时如果用户命中人群，叠加应用到基础折扣之上。
	FindDiscountByIDAndTag(ctx context.Context, discountID, tagID string) (*model.Discount, error)
}

// ActivityWithDiscount 活动+折扣联表查询结果。
// 试算核心数据结构，一次查询拿到活动配置和折扣规则。
type ActivityWithDiscount struct {
	model.Activity
	Discount model.Discount `gorm:"embedded"`
}

// activityRepo GORM 实现。
type activityRepo struct {
	db *gorm.DB
}

// NewActivityRepo 构造函数。
func NewActivityRepo(db *gorm.DB) ActivityRepository {
	return &activityRepo{db: db}
}

// FindActivityByID 按 activity_id 查活动。
func (r *activityRepo) FindActivityByID(ctx context.Context, activityID int64) (*model.Activity, error) {
	var a model.Activity
	err := r.db.WithContext(ctx).
		Where("activity_id = ?", activityID).
		First(&a).Error
	if err != nil {
		return nil, fmt.Errorf("find activity %d: %w", activityID, err)
	}
	return &a, nil
}

// FindDiscountByID 按 discount_id 查折扣。
func (r *activityRepo) FindDiscountByID(ctx context.Context, discountID string) (*model.Discount, error) {
	var d model.Discount
	err := r.db.WithContext(ctx).
		Where("discount_id = ?", discountID).
		First(&d).Error
	if err != nil {
		return nil, fmt.Errorf("find discount %s: %w", discountID, err)
	}
	return &d, nil
}

// FindActivityProduct 按 (source, channel, goods_id) 查活动映射。
// 此 UK 保证一个商品在一个渠道下只能参与一个拼团活动。
func (r *activityRepo) FindActivityProduct(ctx context.Context, source, channel, goodsID string) (*model.ActivityProduct, error) {
	var ap model.ActivityProduct
	err := r.db.WithContext(ctx).
		Where("source = ? AND channel = ? AND goods_id = ?", source, channel, goodsID).
		First(&ap).Error
	if err != nil {
		return nil, fmt.Errorf("find activity_product (%s,%s,%s): %w", source, channel, goodsID, err)
	}
	return &ap, nil
}

// FindActivityWithDiscount 联表查活动+折扣。
// Go 版改进：一次 JOIN 代替 Java 版两次独立查询，减少 DB 往返。
func (r *activityRepo) FindActivityWithDiscount(ctx context.Context, activityID int64) (*ActivityWithDiscount, error) {
	var result ActivityWithDiscount
	err := r.db.WithContext(ctx).
		Table("activities").
		Select("activities.*, discounts.*").
		Joins("JOIN discounts ON discounts.discount_id = activities.discount_id").
		Where("activities.activity_id = ?", activityID).
		First(&result).Error
	if err != nil {
		return nil, fmt.Errorf("find activity with discount %d: %w", activityID, err)
	}
	return &result, nil
}

// FindActiveActivities 查询所有生效中的活动。
// WHERE status = 1 AND start_time <= NOW() AND end_time >= NOW()
func (r *activityRepo) FindActiveActivities(ctx context.Context) ([]model.Activity, error) {
	var activities []model.Activity
	err := r.db.WithContext(ctx).
		Where("status = ?", model.ActivityStatusActive).
		Where("start_time <= NOW()").
		Where("end_time >= NOW()").
		Find(&activities).Error
	if err != nil {
		return nil, fmt.Errorf("find active activities: %w", err)
	}
	return activities, nil
}

// FindDiscountByIDAndTag 查人群定向折扣。
// discount_type=1 表示这是人群标签折扣，需要 tag_id 匹配。
func (r *activityRepo) FindDiscountByIDAndTag(ctx context.Context, discountID, tagID string) (*model.Discount, error) {
	var d model.Discount
	err := r.db.WithContext(ctx).
		Where("discount_id = ? AND discount_type = ? AND tag_id = ?",
			discountID, model.DiscountTypeTag, tagID).
		First(&d).Error
	if err != nil {
		return nil, fmt.Errorf("find tag discount (%s,%s): %w", discountID, tagID, err)
	}
	return &d, nil
}
